package network

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"local-file-sharer/internal/util"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func isPathSafe(requestedPath, baseFolder string) bool {

	if requestedPath == "" || requestedPath == "." {
		return true
	}

	normalizedPath := util.NormalizePath(requestedPath)

	absBase, err := filepath.Abs(baseFolder)
	if err != nil {
		return false
	}

	targetPath := filepath.Join(baseFolder, normalizedPath)
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}

	return strings.HasPrefix(absTarget, absBase)
}

type Connection struct {
	ID                string
	Conn              net.Conn
	Name              string
	RemoteName        string
	App               *App
	Log               *util.Logger
	Reader            *bufio.Reader
	Writer            *bufio.Writer
	isClient          bool
	responseHandlers  map[string]func(Message)
	responseHandlerMu sync.Mutex
	sendMutex         sync.Mutex
	ignoreList        *util.IgnoreList
}

func NewConnection(conn net.Conn, app *App, isClient bool) *Connection {
	id := conn.RemoteAddr().String()

	c := &Connection{
		ID:               id,
		Conn:             conn,
		App:              app,
		Log:              util.NewLogger(app.Config.Verbose, fmt.Sprintf("Conn-%s", id)),
		Reader:           bufio.NewReader(conn),
		Writer:           bufio.NewWriter(conn),
		Name:             app.Config.Name,
		isClient:         isClient,
		responseHandlers: make(map[string]func(Message)),
		ignoreList:       &util.IgnoreList{Patterns: []util.IgnorePattern{}},
	}
	return c
}

func (c *Connection) loadIgnoreList() *util.IgnoreList {
	ignoreList, err := util.LoadIgnoreFile(c.App.Config.Folder)
	if err != nil {
		c.Log.Warn("Failed to load .p2pignore: %v", err)
		return &util.IgnoreList{Patterns: []util.IgnorePattern{}}
	}

	c.ignoreList = ignoreList
	return ignoreList
}

func (c *Connection) RegisterResponseHandler(id string, handler func(Message)) {
	c.responseHandlerMu.Lock()
	defer c.responseHandlerMu.Unlock()
	c.responseHandlers[id] = handler
}

func (c *Connection) UnregisterResponseHandler(id string) {
	c.responseHandlerMu.Lock()
	defer c.responseHandlerMu.Unlock()
	delete(c.responseHandlers, id)
}

func (c *Connection) Start() {
	defer c.Close()

	if err := c.Handshake(); err != nil {
		c.Log.Error("Handshake failed: %v", err)
		return
	}

	if c.isClient {
		c.Log.Info("Connected to server %s (%s)", c.RemoteName, c.ID)
	} else {
		c.Log.Info("Client connected: %s (%s)", c.RemoteName, c.ID)
	}

	for {
		line, err := c.Reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				c.Log.Error("Read error: %v", err)
			}
			break
		}

		c.handleMessage(strings.TrimSpace(line))
	}
}

func (c *Connection) Handshake() error {
	handshake := Message{
		Type: MsgTypeHandshake,
		Data: c.Name,
	}

	if err := c.SendMessage(handshake); err != nil {
		return fmt.Errorf("failed to send handshake: %v", err)
	}

	line, err := c.Reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read handshake: %v", err)
	}

	var response Message
	if err := json.Unmarshal([]byte(line), &response); err != nil {
		return fmt.Errorf("invalid handshake format: %v", err)
	}

	if response.Type != MsgTypeHandshake {
		return fmt.Errorf("expected handshake, got %s", response.Type)
	}

	c.RemoteName = response.Data
	return nil
}

func (c *Connection) Close() {
	c.Conn.Close()
	c.App.RemoveConnection(c)
	c.Log.Info("Connection closed")

	if c.isClient {
		c.Log.Error("Lost connection to server, exiting...")
		fmt.Println("\nDisconnected from server. Press Enter to exit.")

		time.Sleep(500 * time.Millisecond)

		os.Exit(1)
	}
}

func (c *Connection) SendMessage(msg Message) error {
	c.sendMutex.Lock()
	defer c.sendMutex.Unlock()

	data, err := msg.Marshal()
	if err != nil {
		return err
	}

	c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	defer c.Conn.SetWriteDeadline(time.Time{})

	_, err = c.Writer.WriteString(string(data) + "\n")
	if err != nil {
		c.Log.Error("Failed to send message: %v", err)

		if c.isClient {
			c.Close()
		}

		return err
	}

	return c.Writer.Flush()
}

func (c *Connection) SendReliableMessage(msg Message) error {
	maxRetries := 3
	retryDelay := 100 * time.Millisecond

	var err error
	for i := 0; i < maxRetries; i++ {
		err = c.SendMessage(msg)
		if err == nil {
			return nil
		}

		c.Log.Warn("Message send failed (attempt %d/%d): %v", i+1, maxRetries, err)
		time.Sleep(retryDelay)
		retryDelay *= 2
	}

	return fmt.Errorf("failed to send message after %d attempts: %v", maxRetries, err)
}

func (c *Connection) handleMessage(messageStr string) {
	var msg Message
	if err := json.Unmarshal([]byte(messageStr), &msg); err != nil {
		c.Log.Error("Invalid message format: %v", err)
		return
	}

	if msg.ID != "" {
		c.responseHandlerMu.Lock()
		handler, exists := c.responseHandlers[msg.ID]
		c.responseHandlerMu.Unlock()

		if exists {
			handler(msg)
			return
		}
	}

	switch msg.Type {
	case MsgTypeCommand:
		c.handleCommand(msg)
	case MsgTypeFileStart:
		c.handleFileStart(msg)
	case MsgTypeFileData:
		c.handleFileData(msg)
	case MsgTypeFileEnd:
		c.handleFileEnd(msg)
	case MsgTypeProgress:
		c.handleProgress(msg)
	case MsgTypeACK:
		c.handleAck(msg)
	case MsgTypeError:
		c.Log.Error("Remote error: %s", msg.Data)
	case MsgTypeMessage:
		fmt.Printf("\n%s[MESSAGE FROM %s]%s %s\n", util.Bold+util.Purple, c.RemoteName, util.Reset, msg.Data)
	case MsgTypeCommandResult:
		fmt.Println(msg.Data)
	default:
		c.Log.Warn("Unknown message type: %s", msg.Type)
	}
}

func (c *Connection) handleCommand(msg Message) {
	cmd := ParseCommand(msg.Data)
	if cmd == nil {
		errorMsg := Message{
			Type: MsgTypeError,
			Data: "Invalid command format",
			ID:   msg.ID,
		}
		c.SendMessage(errorMsg)
		return
	}

	c.Log.Debug("Received command: %s %v", cmd.Name, cmd.Args)

	var response Message

	switch cmd.Name {
	case "LS", "LIST", "LSR":
		response = c.handleListCommand(cmd)
	case "CDR":
		response = c.handleCDCommand(cmd)
	case "GET":
		response = c.handleGetCommand(cmd)
	case "PUT":
		response = c.handlePutCommand(cmd)
	case "INFO":
		response = c.handleInfoCommand(cmd)
	case "GETDIR":
		response = c.handleGetDirCommand(cmd)
	case "PUTDIR":
		response = c.handlePutDirCommand(cmd)
	case "GETM":
		response = c.handleGetMultipleCommand(cmd)
	case "PUTM":
		response = c.handlePutMultipleCommand(cmd)
	case "STATUS":
		response = c.handleStatusCommand(cmd)
	default:
		response = Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Unknown command: %s", cmd.Name),
		}
	}

	response.ID = msg.ID
	c.SendMessage(response)
}

func (c *Connection) SendError(errorMsg string) error {
	msg := Message{
		Type: MsgTypeError,
		Data: errorMsg,
	}
	return c.SendMessage(msg)
}

func (c *Connection) handleCDCommand(cmd *Command) Message {
	if len(cmd.Args) < 1 {
		return Message{
			Type: MsgTypeError,
			Data: "CDR requires a directory path",
		}
	}

	path := cmd.Args[0]

	if !isPathSafe(path, c.App.Config.Folder) {
		return Message{
			Type: MsgTypeError,
			Data: "Access denied: path is outside the shared folder",
		}
	}

	fullPath := filepath.Join(c.App.Config.Folder, path)

	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Directory not found: %s", path),
		}
	}

	if !info.IsDir() {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Not a directory: %s", path),
		}
	}

	c.App.Config.Folder = fullPath

	ignoreList, _ := util.LoadIgnoreFile(c.App.Config.Folder)
	if ignoreList != nil {
		c.ignoreList = ignoreList
	}

	return Message{
		Type: MsgTypeCommandResult,
		Data: fmt.Sprintf("Changed to %s", path),
	}
}

func (c *Connection) handleListCommand(cmd *Command) Message {

	path := "."
	if len(cmd.Args) > 0 {
		path = cmd.Args[0]
	}

	c.Log.Debug("Listing files for path: '%s', base folder: '%s'", path, c.App.Config.Folder)

	if path == "." || path == "" {
		c.Log.Debug("Using base folder directly: %s", c.App.Config.Folder)

		files, err := os.ReadDir(c.App.Config.Folder)
		if err != nil {
			return Message{
				Type: MsgTypeError,
				Data: fmt.Sprintf("Failed to list base directory: %v", err),
			}
		}

		c.loadIgnoreList()

		var resultFiles []string
		recursive := cmd.Name == "LSR"

		for _, entry := range files {
			name := entry.Name()

			if name == ".p2pignore" {
				continue
			}

			if !c.ignoreList.ShouldIgnore(name, entry.IsDir()) {
				if entry.IsDir() {
					resultFiles = append(resultFiles, name+"/")
				} else {
					info, err := entry.Info()
					if err == nil {
						size := util.FormatFileSize(info.Size())
						resultFiles = append(resultFiles, fmt.Sprintf("%-40s %10s", name, size))
					} else {
						resultFiles = append(resultFiles, name)
					}
				}

				if recursive && entry.IsDir() {
					subpath := filepath.Join(c.App.Config.Folder, name)
					subfiles, err := util.ListFilesRecursive(subpath)
					if err != nil {
						continue
					}

					for _, subfile := range subfiles {
						relPath, err := filepath.Rel(c.App.Config.Folder, subfile)
						if err != nil {
							continue
						}

						if filepath.Base(subfile) == ".p2pignore" {
							continue
						}

						if !c.ignoreList.ShouldIgnore(relPath, false) {
							info, err := os.Stat(subfile)
							if err == nil && !info.IsDir() {
								size := util.FormatFileSize(info.Size())
								resultFiles = append(resultFiles, fmt.Sprintf("%-40s %10s", relPath, size))
							}
						}
					}
				}
			}
		}

		if len(resultFiles) == 0 {
			return Message{
				Type: MsgTypeCommandResult,
				Data: "Contents of .: (empty or all files are ignored)",
			}
		}

		result := fmt.Sprintf("Contents of .:\n%s", strings.Join(resultFiles, "\n"))
		return Message{
			Type: MsgTypeCommandResult,
			Data: result,
		}
	}

	normalizedPath := util.NormalizePath(path)
	c.Log.Debug("Normalized path: '%s'", normalizedPath)

	targetPath := filepath.Join(c.App.Config.Folder, normalizedPath)
	targetPath = filepath.Clean(targetPath)
	c.Log.Debug("Target path: '%s'", targetPath)

	_, err := os.Stat(targetPath)
	if err != nil {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Path not found: %v", err),
		}
	}

	absBase, _ := filepath.Abs(c.App.Config.Folder)
	absTarget, _ := filepath.Abs(targetPath)
	if !strings.HasPrefix(absTarget, absBase) {
		return Message{
			Type: MsgTypeError,
			Data: "Access denied: path is outside the shared folder",
		}
	}

	fileEntries, err := os.ReadDir(targetPath)
	if err != nil {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Failed to list files: %v", err),
		}
	}

	c.loadIgnoreList()

	var filteredFiles []string
	for _, entry := range fileEntries {
		name := entry.Name()

		if name == ".p2pignore" {
			continue
		}

		relativePath := filepath.Join(normalizedPath, name)
		isDir := entry.IsDir()

		if !c.ignoreList.ShouldIgnore(relativePath, isDir) {
			if isDir {
				filteredFiles = append(filteredFiles, name+"/")
			} else {
				info, err := entry.Info()
				if err == nil {
					size := util.FormatFileSize(info.Size())
					filteredFiles = append(filteredFiles, fmt.Sprintf("%-40s %10s", name, size))
				} else {
					filteredFiles = append(filteredFiles, name)
				}
			}
		}
	}

	recursive := cmd.Name == "LSR"
	if recursive {
		for _, entry := range fileEntries {
			if !entry.IsDir() {
				continue
			}

			subpath := filepath.Join(targetPath, entry.Name())
			subfiles, err := util.ListFilesRecursive(subpath)
			if err != nil {
				continue
			}

			for _, subfile := range subfiles {
				relPath, err := filepath.Rel(c.App.Config.Folder, subfile)
				if err != nil {
					continue
				}

				if filepath.Base(subfile) == ".p2pignore" {
					continue
				}

				if !c.ignoreList.ShouldIgnore(relPath, false) {
					info, err := os.Stat(subfile)
					if err == nil && !info.IsDir() {
						subRelPath, _ := filepath.Rel(targetPath, subfile)
						size := util.FormatFileSize(info.Size())
						filteredFiles = append(filteredFiles, fmt.Sprintf("%-40s %10s", subRelPath, size))
					}
				}
			}
		}
	}

	if len(filteredFiles) == 0 {
		result := fmt.Sprintf("Contents of %s: (empty or all files are ignored)", normalizedPath)
		return Message{
			Type: MsgTypeCommandResult,
			Data: result,
		}
	}

	result := fmt.Sprintf("Contents of %s:\n%s", normalizedPath, strings.Join(filteredFiles, "\n"))
	return Message{
		Type: MsgTypeCommandResult,
		Data: result,
	}
}

func (c *Connection) canInitiateTransfer() bool {
	c.App.mu.Lock()
	defer c.App.mu.Unlock()

	activeCount := 0
	for _, t := range c.App.Transfers {
		if t.Status == TransferStatusInProgress {
			activeCount++
		}
	}

	return activeCount < 3
}

func (c *Connection) handleGetCommand(cmd *Command) Message {
	if len(cmd.Args) < 1 {
		return Message{
			Type: MsgTypeError,
			Data: "GET requires a file path",
		}
	}

	if !c.canInitiateTransfer() {
		return Message{
			Type: MsgTypeError,
			Data: "Too many active transfers, please wait for current transfers to complete",
		}
	}

	filePath := util.NormalizePath(cmd.Args[0])

	if !util.IsValidRelativePath(filePath) {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Invalid path: %s (contains invalid characters or points to a parent directory)", filePath),
		}
	}

	if !isPathSafe(filePath, c.App.Config.Folder) {
		return Message{
			Type: MsgTypeError,
			Data: "Access denied: path is outside the shared folder",
		}
	}

	if c.App.Config.ReadOnly {
		return Message{
			Type: MsgTypeError,
			Data: "This node is in read-only mode and cannot send files",
		}
	}

	if filepath.Base(filePath) == ".p2pignore" {
		return Message{
			Type: MsgTypeError,
			Data: "The .p2pignore file cannot be transferred",
		}
	}

	fullPath := filepath.Join(c.App.Config.Folder, filePath)

	c.loadIgnoreList()

	fileInfo, err := os.Stat(fullPath)
	if err == nil && c.ignoreList.ShouldIgnore(filePath, fileInfo.IsDir()) {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("File %s is in .p2pignore list and cannot be transferred", filePath),
		}
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("File not found: %v", err),
		}
	}

	if info.IsDir() {
		return Message{
			Type: MsgTypeError,
			Data: "GET cannot transfer directories, use GETDIR instead",
		}
	}

	if c.App.Config.MaxSize > 0 && info.Size() > int64(c.App.Config.MaxSize*1024*1024) {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("File size exceeds maximum allowed size of %d MB", c.App.Config.MaxSize),
		}
	}

	file, err := os.Open(fullPath)
	if err != nil {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Failed to open file: %v", err),
		}
	}

	transfer := NewFileTransfer(filePath, info.Size(), TransferTypeSend, c)
	transfer.File = file
	c.App.AddTransfer(transfer)

	startMsg := Message{
		Type: MsgTypeFileStart,
		Data: fmt.Sprintf("%s|%d", filePath, info.Size()),
	}
	if err := c.SendReliableMessage(startMsg); err != nil {
		file.Close()
		c.App.RemoveTransfer(transfer)
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Failed to send file start: %v", err),
		}
	}

	go func() {
		defer c.App.RemoveTransfer(transfer)
		defer file.Close()

		buffer := make([]byte, 262144)
		totalSent := int64(0)
		lastProgress := int64(0)
		lastProgressBytes := int64(0)
		startTime := time.Now()
		lastSpeedUpdate := startTime
		chunksSent := 0

		ackChan := make(chan bool, 1)
		ackID := fmt.Sprintf("ack-%s-%d", filePath, time.Now().UnixNano())

		c.RegisterResponseHandler(ackID, func(msg Message) {
			if msg.Type == MsgTypeACK && msg.Data == filePath {
				select {
				case ackChan <- true:
				default:
				}
			}
		})
		defer c.UnregisterResponseHandler(ackID)

		for {
			if transfer.Status == TransferStatusPaused {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			if transfer.Status == TransferStatusFailed {
				return
			}

			n, err := file.Read(buffer)
			if err != nil && err != io.EOF {
				transfer.Status = TransferStatusFailed
				c.SendError(fmt.Sprintf("Failed to read file: %v", err))
				return
			}

			if n == 0 {
				break
			}

			dataMsg := NewBinaryMessage(MsgTypeFileData, buffer[:n])
			dataMsg.ID = ackID
			if err := c.SendMessage(dataMsg); err != nil {
				transfer.Status = TransferStatusFailed
				c.Log.Error("Failed to send file data: %v", err)
				return
			}

			totalSent += int64(n)
			transfer.BytesTransferred = totalSent
			chunksSent++

			progress := (totalSent * 100) / info.Size()
			currentTime := time.Now()
			timeForSpeed := currentTime.Sub(lastSpeedUpdate).Seconds()

			if totalSent-lastProgressBytes > 1048576 || progress > lastProgress+2 || timeForSpeed > 1.0 {
				lastProgress = progress
				lastProgressBytes = totalSent
				elapsedTime := currentTime.Sub(startTime).Seconds()

				var speed float64 = 0
				if elapsedTime > 0 {

					speed = float64(totalSent) / elapsedTime / 1024

					if totalSent > 0 && (speed < 0.01 || timeForSpeed < 0.1) {
						speed = 0.01
					}

					progMsg := Message{
						Type: MsgTypeProgress,
						Data: fmt.Sprintf("%s|%d|%d|%.2f", filePath, totalSent, info.Size(), speed),
						ID:   ackID,
					}
					c.SendMessage(progMsg)

					transfer.UpdateProgress(totalSent, speed)
					lastSpeedUpdate = currentTime
				}
			}

			if chunksSent%20 == 0 {
				time.Sleep(5 * time.Millisecond)
			}
		}

		endMsg := Message{
			Type: MsgTypeFileEnd,
			Data: filePath,
			ID:   ackID,
		}
		if err := c.SendReliableMessage(endMsg); err != nil {
			transfer.Status = TransferStatusFailed
			c.Log.Error("Failed to send file end: %v", err)
			return
		}

		transfer.Status = TransferStatusWaitingAck

		select {
		case <-ackChan:
			transfer.Status = TransferStatusComplete
			c.Log.Success("Transfer completed and acknowledged: %s", filePath)
		case <-time.After(30 * time.Second):
			transfer.Status = TransferStatusFailed
			c.Log.Error("Transfer timed out waiting for ACK: %s", filePath)
		}
	}()

	return Message{
		Type: MsgTypeCommandResult,
		Data: fmt.Sprintf("Starting file transfer: %s", filePath),
	}
}

func (c *Connection) handlePutCommand(cmd *Command) Message {
	if len(cmd.Args) < 1 {
		return Message{
			Type: MsgTypeError,
			Data: "PUT requires a file path",
		}
	}

	if c.App.Config.WriteOnly {
		return Message{
			Type: MsgTypeError,
			Data: "This node is in write-only mode and cannot receive files",
		}
	}

	if !c.canInitiateTransfer() {
		return Message{
			Type: MsgTypeError,
			Data: "Too many active transfers, please wait for current transfers to complete",
		}
	}

	filePath := cmd.Args[0]
	if !util.IsValidRelativePath(filePath) {
		return Message{
			Type: MsgTypeError,
			Data: "Invalid path: path contains invalid characters or points to a parent directory",
		}
	}

	return Message{
		Type: MsgTypeCommandResult,
		Data: "Ready to receive file",
	}
}

func (c *Connection) handleInfoCommand(_ *Command) Message {
	info := fmt.Sprintf("Node: %s\n", c.Name)
	info += fmt.Sprintf("Folder: %s\n", c.App.Config.Folder)
	info += fmt.Sprintf("Read-only: %t\n", c.App.Config.ReadOnly)
	info += fmt.Sprintf("Write-only: %t\n", c.App.Config.WriteOnly)
	if c.App.Config.MaxSize > 0 {
		info += fmt.Sprintf("Max file size: %d MB\n", c.App.Config.MaxSize)
	} else {
		info += "Max file size: Unlimited\n"
	}

	if len(c.ignoreList.Patterns) > 0 {
		info += fmt.Sprintf("\nIgnore patterns (%d):\n", len(c.ignoreList.Patterns))
		for _, pattern := range c.ignoreList.Patterns {
			if pattern.IsDir {
				info += fmt.Sprintf("  %s/ (directory)\n", pattern.Pattern)
			} else {
				info += fmt.Sprintf("  %s\n", pattern.Pattern)
			}
		}
	}

	return Message{
		Type: MsgTypeCommandResult,
		Data: info,
	}
}

func (c *Connection) handleGetDirCommand(cmd *Command) Message {
	if len(cmd.Args) < 1 {
		return Message{
			Type: MsgTypeError,
			Data: "GETDIR requires a directory path",
		}
	}

	dirPath := util.NormalizePath(cmd.Args[0])

	if !util.IsValidRelativePath(dirPath) {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Invalid path: %s (contains invalid characters or points to a parent directory)", dirPath),
		}
	}

	if !isPathSafe(dirPath, c.App.Config.Folder) {
		return Message{
			Type: MsgTypeError,
			Data: "Access denied: path is outside the shared folder",
		}
	}

	if c.App.Config.ReadOnly {
		return Message{
			Type: MsgTypeError,
			Data: "This node is in read-only mode and cannot send files",
		}
	}

	fullPath := filepath.Join(c.App.Config.Folder, dirPath)

	info, err := os.Stat(fullPath)
	if err != nil {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Directory not found: %v", err),
		}
	}

	if !info.IsDir() {
		return Message{
			Type: MsgTypeError,
			Data: "GETDIR can only transfer directories, use GET for files",
		}
	}

	c.loadIgnoreList()

	allFiles, err := util.ListFilesRecursive(fullPath)
	if err != nil {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Failed to list directory: %v", err),
		}
	}

	var files []string
	for _, file := range allFiles {
		relPath := strings.TrimPrefix(file, c.App.Config.Folder)
		relPath = strings.TrimPrefix(relPath, "/")
		relPath = strings.TrimPrefix(relPath, "\\")

		relPath = util.NormalizePath(relPath)

		fileInfo, err := os.Stat(file)
		if err != nil || fileInfo.IsDir() {
			continue
		}

		if filepath.Base(file) == ".p2pignore" {
			continue
		}

		if !c.ignoreList.ShouldIgnore(relPath, false) {
			files = append(files, file)
		}
	}

	if len(files) == 0 {
		return Message{
			Type: MsgTypeCommandResult,
			Data: fmt.Sprintf("No transferable files found in directory %s (empty or all files ignored)", dirPath),
		}
	}

	var includedFilesList []string
	for _, file := range files {
		relPath := strings.TrimPrefix(file, c.App.Config.Folder)
		relPath = strings.TrimPrefix(relPath, "/")
		relPath = strings.TrimPrefix(relPath, "\\")
		relPath = util.NormalizePath(relPath)
		includedFilesList = append(includedFilesList, relPath)
	}

	dirMsg := Message{
		Type: MsgTypeCommandResult,
		Data: strings.Join(includedFilesList, "\n"),
	}
	c.SendMessage(dirMsg)

	for _, file := range files {
		relPath := strings.TrimPrefix(file, c.App.Config.Folder)
		relPath = strings.TrimPrefix(relPath, "/")
		relPath = strings.TrimPrefix(relPath, "\\")
		relPath = util.NormalizePath(relPath)

		info, err := os.Stat(file)
		if err != nil || info.IsDir() {
			continue
		}

		getCmd := &Command{
			Name: "GET",
			Args: []string{relPath},
		}
		c.handleGetCommand(getCmd)

		time.Sleep(500 * time.Millisecond)
	}

	return Message{
		Type: MsgTypeCommandResult,
		Data: fmt.Sprintf("Sending directory: %s (%d files)", dirPath, len(files)),
	}
}

func (c *Connection) handlePutDirCommand(cmd *Command) Message {
	if len(cmd.Args) < 1 {
		return Message{
			Type: MsgTypeError,
			Data: "PUTDIR requires a directory path",
		}
	}

	dirPath := util.NormalizePath(cmd.Args[0])

	if !util.IsValidRelativePath(dirPath) {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Invalid path: %s (contains invalid characters or points to a parent directory)", dirPath),
		}
	}

	if !isPathSafe(dirPath, c.App.Config.Folder) {
		return Message{
			Type: MsgTypeError,
			Data: "Access denied: path is outside the shared folder",
		}
	}

	if c.App.Config.WriteOnly {
		return Message{
			Type: MsgTypeError,
			Data: "This node is in write-only mode and cannot receive files",
		}
	}

	fullPath := filepath.Join(c.App.Config.Folder, dirPath)

	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Failed to create directory: %v", err),
		}
	}

	ignoreFilePath := filepath.Join(fullPath, ".p2pignore")
	if _, err := os.Stat(ignoreFilePath); os.IsNotExist(err) {

		defaultIgnore := `# P2P File Sharing Ignore File
# Files and directories matching these patterns will not be shared
# Examples:
# *.tmp
# *.log
# temp/
`

		_ = os.WriteFile(ignoreFilePath, []byte(defaultIgnore), 0644)
	}

	return Message{
		Type: MsgTypeCommandResult,
		Data: fmt.Sprintf("Ready to receive directory files into %s", dirPath),
	}
}

func (c *Connection) handleGetMultipleCommand(cmd *Command) Message {
	if len(cmd.Args) < 1 {
		return Message{
			Type: MsgTypeError,
			Data: "GETM requires at least one file",
		}
	}

	if c.App.Config.ReadOnly {
		return Message{
			Type: MsgTypeError,
			Data: "This node is in read-only mode and cannot send files",
		}
	}

	c.loadIgnoreList()

	var filesToSend []string
	var ignoredFiles []string
	var notFoundFiles []string

	for _, filePath := range cmd.Args {
		fullPath := filepath.Join(c.App.Config.Folder, filePath)
		fileInfo, err := os.Stat(fullPath)

		if err != nil {
			notFoundFiles = append(notFoundFiles, filePath)
			continue
		}

		if filepath.Base(filePath) == ".p2pignore" {
			ignoredFiles = append(ignoredFiles, filePath+" (.p2pignore files cannot be transferred)")
			continue
		}

		if c.ignoreList.ShouldIgnore(filePath, fileInfo.IsDir()) {
			ignoredFiles = append(ignoredFiles, filePath+" (in .p2pignore list)")
			continue
		}

		if fileInfo.IsDir() {
			ignoredFiles = append(ignoredFiles, filePath+" (is a directory, use GETDIR)")
			continue
		}

		filesToSend = append(filesToSend, filePath)
	}

	if len(filesToSend) == 0 {
		result := "No files to transfer:\n"
		if len(notFoundFiles) > 0 {
			result += "- Files not found: " + strings.Join(notFoundFiles, ", ") + "\n"
		}
		if len(ignoredFiles) > 0 {
			result += "- Files ignored: " + strings.Join(ignoredFiles, ", ") + "\n"
		}
		return Message{
			Type: MsgTypeCommandResult,
			Data: result,
		}
	}

	var resultInfo string
	if len(notFoundFiles) > 0 || len(ignoredFiles) > 0 {
		resultInfo = fmt.Sprintf("Sending %d of %d requested files.\n",
			len(filesToSend), len(cmd.Args))

		if len(notFoundFiles) > 0 {
			resultInfo += "- Files not found: " + strings.Join(notFoundFiles, ", ") + "\n"
		}
		if len(ignoredFiles) > 0 {
			resultInfo += "- Files ignored: " + strings.Join(ignoredFiles, ", ") + "\n"
		}

		infoMsg := Message{
			Type: MsgTypeCommandResult,
			Data: resultInfo,
		}
		c.SendMessage(infoMsg)
	}

	for _, file := range filesToSend {
		getCmd := &Command{
			Name: "GET",
			Args: []string{file},
		}
		c.handleGetCommand(getCmd)
		time.Sleep(500 * time.Millisecond)
	}

	return Message{
		Type: MsgTypeCommandResult,
		Data: fmt.Sprintf("Sending %d files", len(filesToSend)),
	}
}

func (c *Connection) handlePutMultipleCommand(cmd *Command) Message {
	if len(cmd.Args) < 1 {
		return Message{
			Type: MsgTypeError,
			Data: "PUTM requires at least one file",
		}
	}

	if c.App.Config.WriteOnly {
		return Message{
			Type: MsgTypeError,
			Data: "This node is in write-only mode and cannot receive files",
		}
	}

	var validFiles []string
	var invalidFiles []string

	for _, file := range cmd.Args {
		if !util.IsValidRelativePath(file) {
			invalidFiles = append(invalidFiles, file+" (invalid path)")
			continue
		}

		if filepath.Base(file) == ".p2pignore" {
			invalidFiles = append(invalidFiles, file+" (.p2pignore files cannot be transferred)")
			continue
		}

		validFiles = append(validFiles, file)
	}

	if len(validFiles) == 0 {
		result := "No valid files to receive:\n"
		if len(invalidFiles) > 0 {
			result += "- Invalid files: " + strings.Join(invalidFiles, ", ") + "\n"
		}

		return Message{
			Type: MsgTypeError,
			Data: result,
		}
	}

	if len(invalidFiles) > 0 {
		return Message{
			Type: MsgTypeCommandResult,
			Data: fmt.Sprintf("Ready to receive %d of %d files. \nSkipping invalid files: %s",
				len(validFiles), len(cmd.Args), strings.Join(invalidFiles, ", ")),
		}
	}

	return Message{
		Type: MsgTypeCommandResult,
		Data: fmt.Sprintf("Ready to receive %d files", len(validFiles)),
	}
}

func (c *Connection) handleStatusCommand(_ *Command) Message {
	transfers := c.App.GetCurrentTransfers()

	if len(transfers) == 0 {
		return Message{
			Type: MsgTypeCommandResult,
			Data: "No active transfers",
		}
	}

	var statusText string
	statusText = fmt.Sprintf("Active transfers: %d\n", len(transfers))

	for _, t := range transfers {
		pct := float64(t.BytesTransferred) / float64(t.TotalSize) * 100
		typeStr := "Receiving"
		if t.Type == TransferTypeSend {
			typeStr = "Sending"
		}

		statusStr := "In Progress"
		switch t.Status {
		case TransferStatusPaused:
			statusStr = "Paused"
		case TransferStatusWaitingAck:
			statusStr = "Waiting for acknowledgment"
		case TransferStatusComplete:
			statusStr = "Complete"
		case TransferStatusFailed:
			statusStr = "Failed"
		}

		statusText += fmt.Sprintf("[%d] %s %s: %.1f%% complete (%.2f KB/s) - %s\n",
			t.ID, typeStr, t.Name, pct, t.Speed, statusStr)
	}

	return Message{
		Type: MsgTypeCommandResult,
		Data: statusText,
	}
}

func createUniqueFilename(path string) string {

	return util.EnsureUniqueFilename(path)
}

func (c *Connection) handleFileStart(msg Message) {
	parts := strings.Split(msg.Data, "|")
	if len(parts) != 2 {
		c.SendError("Invalid file start format")
		return
	}

	filePath := util.NormalizePath(parts[0])

	if !util.IsValidRelativePath(filePath) {
		c.SendError(fmt.Sprintf("Invalid path: %s (contains invalid characters or points to a parent directory)", filePath))
		return
	}

	if !isPathSafe(filePath, c.App.Config.Folder) {
		c.SendError("Access denied: path is outside the shared folder")
		return
	}

	fileSize, err := util.ParseInt64(parts[1])
	if err != nil {
		c.SendError("Invalid file size")
		return
	}

	if c.App.Config.MaxSize > 0 && fileSize > int64(c.App.Config.MaxSize*1024*1024) {
		c.SendError(fmt.Sprintf("File size exceeds maximum allowed size of %d MB", c.App.Config.MaxSize))
		return
	}

	if c.App.Config.WriteOnly {
		c.SendError("This node is in write-only mode and cannot receive files")
		return
	}

	if filepath.Base(filePath) == ".p2pignore" {
		c.SendError("The .p2pignore file cannot be transferred")
		return
	}

	fullPath := filepath.Join(c.App.Config.Folder, filePath)

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.SendError(fmt.Sprintf("Failed to create directory: %v", err))
		return
	}

	if _, err := os.Stat(fullPath); err == nil {
		newPath := createUniqueFilename(fullPath)
		c.Log.Info("File already exists, using unique name: %s", filepath.Base(newPath))
		fullPath = newPath
	}

	file, err := os.Create(fullPath)
	if err != nil {
		c.SendError(fmt.Sprintf("Failed to create file: %v", err))
		return
	}

	transfer := NewFileTransfer(filePath, fileSize, TransferTypeReceive, c)
	transfer.File = file
	c.App.AddTransfer(transfer)

	c.Log.Info("Starting to receive file %s (%d bytes)", filePath, fileSize)
}

func (c *Connection) handleFileData(msg Message) {
	transfers := c.App.GetTransfers()
	var transfer *FileTransfer

	for _, t := range transfers {
		if t.Conn == c && t.Type == TransferTypeReceive {
			transfer = t
			break
		}
	}

	if transfer == nil {
		c.SendError("No active file transfer")
		return
	}

	if transfer.Status == TransferStatusPaused {
		return
	}

	if transfer.File == nil {
		c.SendError("File not open for writing")
		return
	}

	var data []byte
	var err error
	if msg.Binary {
		data = []byte(msg.Data)
	} else {
		data, err = base64.StdEncoding.DecodeString(msg.Data)
		if err != nil {
			data = []byte(msg.Data)
		}
	}

	n, err := transfer.File.Write(data)
	if err != nil {
		c.SendError(fmt.Sprintf("Failed to write file: %v", err))
		transfer.Status = TransferStatusFailed
		return
	}

	transfer.BytesTransferred += int64(n)

	if transfer.TotalSize > 0 {
		progress := (transfer.BytesTransferred * 100) / transfer.TotalSize
		if progress > transfer.LastProgress+4 {
			elapsedTime := time.Since(transfer.StartTime).Seconds()
			if elapsedTime > 0 {
				speed := float64(transfer.BytesTransferred) / elapsedTime / 1024
				transfer.LastProgress = progress
				transfer.UpdateProgress(transfer.BytesTransferred, speed)
			}
		}
	}

	if msg.ID != "" {
		ackMsg := Message{
			Type: MsgTypeACK,
			Data: transfer.Name,
			ID:   msg.ID,
		}
		c.SendMessage(ackMsg)
	}
}

func (c *Connection) handleFileEnd(msg Message) {

	filePath := util.NormalizePath(msg.Data)

	transfers := c.App.GetTransfers()
	var transfer *FileTransfer

	for _, t := range transfers {

		if t.Conn == c && t.Type == TransferTypeReceive &&
			(t.Name == filePath || util.NormalizePath(t.Name) == filePath) {
			transfer = t
			break
		}
	}

	if transfer == nil {
		c.SendError("No active file transfer for " + filePath)
		return
	}

	if transfer.File != nil {
		transfer.File.Close()
		transfer.File = nil
	}

	ackMsg := Message{
		Type: MsgTypeACK,
		Data: filePath,
	}

	if msg.ID != "" {
		ackMsg.ID = msg.ID
	}

	for i := 0; i < 5; i++ {
		c.SendReliableMessage(ackMsg)
		time.Sleep(100 * time.Millisecond)
	}

	transfer.Status = TransferStatusComplete
	c.Log.Success("File transfer complete: %s", filePath)

	c.App.RemoveTransfer(transfer)
}

func (c *Connection) handleProgress(msg Message) {
	parts := strings.Split(msg.Data, "|")
	if len(parts) != 4 {
		return
	}

	filePath := parts[0]
	received, _ := util.ParseInt64(parts[1])
	totalSize, _ := util.ParseInt64(parts[2])
	speed, _ := util.ParseFloat64(parts[3])

	var transfer *FileTransfer
	for _, t := range c.App.Transfers {
		if t.Name == filePath && t.Conn == c {
			transfer = t
			break
		}
	}

	if transfer != nil {
		transfer.BytesTransferred = received
		transfer.TotalSize = totalSize
		transfer.Speed = speed
		transfer.UpdateProgress(received, speed)
	}
}

func (c *Connection) handleAck(msg Message) {
	filePath := msg.Data

	var transfer *FileTransfer
	for _, t := range c.App.Transfers {
		if t.Name == filePath && t.Conn == c && (t.Status == TransferStatusWaitingAck || t.Status == TransferStatusInProgress) {
			transfer = t
			break
		}
	}

	if transfer != nil {
		if transfer.Status == TransferStatusWaitingAck {
			transfer.Status = TransferStatusComplete
			c.Log.Success("File transfer acknowledged: %s", filePath)
		}
	}
}
