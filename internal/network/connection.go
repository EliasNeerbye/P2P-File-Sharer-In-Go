package network

import (
	"bufio"
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
	// Get absolute paths
	absBase, err := filepath.Abs(baseFolder)
	if err != nil {
		return false
	}

	targetPath := filepath.Join(baseFolder, requestedPath)
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}

	// Check if target path is within the base folder
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
	}
	return c
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

	// Handle disconnection based on application mode
	if c.isClient && !c.App.Config.DualMode {
		c.Log.Error("Lost connection to server, exiting...")
		fmt.Println("\nDisconnected from server. Press Enter to exit.")

		// Give a moment for log messages to be displayed
		time.Sleep(500 * time.Millisecond)

		// Exit the application
		os.Exit(1)
	}
}

func (c *Connection) SendMessage(msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = c.Writer.WriteString(string(data) + "\n")
	if err != nil {
		c.Log.Error("Failed to send message: %v", err)

		// If this is a client and not in dual mode, handle disconnection
		if c.isClient && !c.App.Config.DualMode {
			c.Close() // This will trigger the exit logic
		}

		return err
	}

	return c.Writer.Flush()
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

	c.Log.Debug("Received command: %s", cmd.Name)

	var response Message

	switch cmd.Name {
	case "LS", "LIST":
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

	// Security check
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

	// Security check
	if !isPathSafe(path, c.App.Config.Folder) {
		return Message{
			Type: MsgTypeError,
			Data: "Access denied: path is outside the shared folder",
		}
	}

	recursive := cmd.Name == "LSR"

	filepath.Abs(filepath.Join(c.App.Config.Folder, path))

	files, err := util.ListFiles(path, c.App.Config.Folder, recursive)
	if err != nil {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Failed to list files: %v", err),
		}
	}

	return Message{
		Type: MsgTypeCommandResult,
		Data: strings.Join(files, "\n"),
	}
}

func (c *Connection) handleGetCommand(cmd *Command) Message {
	if len(cmd.Args) < 1 {
		return Message{
			Type: MsgTypeError,
			Data: "GET requires a file path",
		}
	}

	filePath := cmd.Args[0]

	// Security check
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

	fullPath := filepath.Join(c.App.Config.Folder, filePath)

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
	defer file.Close()

	transfer := NewFileTransfer(filePath, info.Size(), TransferTypeSend, c)
	c.App.AddTransfer(transfer)

	startMsg := Message{
		Type: MsgTypeFileStart,
		Data: fmt.Sprintf("%s|%d", filePath, info.Size()),
	}
	if err := c.SendMessage(startMsg); err != nil {
		c.App.RemoveTransfer(transfer)
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Failed to send file start: %v", err),
		}
	}

	go func() {
		defer c.App.RemoveTransfer(transfer)

		buffer := make([]byte, 8192)
		totalSent := int64(0)
		lastProgress := int64(0)
		startTime := time.Now()

		for {
			n, err := file.Read(buffer)
			if err != nil && err != io.EOF {
				transfer.Status = TransferStatusFailed
				c.SendError(fmt.Sprintf("Failed to read file: %v", err))
				return
			}

			if n == 0 {
				break
			}

			dataMsg := Message{
				Type:   MsgTypeFileData,
				Data:   string(buffer[:n]),
				Binary: true,
			}
			if err := c.SendMessage(dataMsg); err != nil {
				transfer.Status = TransferStatusFailed
				c.Log.Error("Failed to send file data: %v", err)
				return
			}

			totalSent += int64(n)
			transfer.BytesTransferred = totalSent

			progress := (totalSent * 100) / info.Size()
			if progress > lastProgress+4 {
				lastProgress = progress
				elapsedTime := time.Since(startTime).Seconds()
				speed := float64(totalSent) / elapsedTime / 1024

				progMsg := Message{
					Type: MsgTypeProgress,
					Data: fmt.Sprintf("%s|%d|%d|%.2f", filePath, totalSent, info.Size(), speed),
				}
				c.SendMessage(progMsg)

				transfer.UpdateProgress(totalSent, speed)
			}
		}

		endMsg := Message{
			Type: MsgTypeFileEnd,
			Data: filePath,
		}
		if err := c.SendMessage(endMsg); err != nil {
			transfer.Status = TransferStatusFailed
			c.Log.Error("Failed to send file end: %v", err)
			return
		}

		transfer.Status = TransferStatusWaitingAck

		go func() {
			time.Sleep(10 * time.Second)
			if transfer.Status == TransferStatusWaitingAck {
				transfer.Status = TransferStatusFailed
				c.Log.Error("Transfer timed out waiting for ACK")
			}
		}()
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

	return Message{
		Type: MsgTypeCommandResult,
		Data: "Ready to receive file",
	}
}

func (c *Connection) handleInfoCommand(cmd *Command) Message {
	info := fmt.Sprintf("Node: %s\n", c.Name)
	info += fmt.Sprintf("Folder: %s\n", c.App.Config.Folder)
	info += fmt.Sprintf("Read-only: %t\n", c.App.Config.ReadOnly)
	info += fmt.Sprintf("Write-only: %t\n", c.App.Config.WriteOnly)
	if c.App.Config.MaxSize > 0 {
		info += fmt.Sprintf("Max file size: %d MB\n", c.App.Config.MaxSize)
	} else {
		info += "Max file size: Unlimited\n"
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

	dirPath := cmd.Args[0]

	// Security check
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

	files, err := util.ListFilesRecursive(fullPath)
	if err != nil {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Failed to list directory: %v", err),
		}
	}

	dirMsg := Message{
		Type: MsgTypeCommandResult,
		Data: strings.Join(files, "\n"),
	}
	c.SendMessage(dirMsg)

	for _, file := range files {
		relPath := strings.TrimPrefix(file, c.App.Config.Folder)
		relPath = strings.TrimPrefix(relPath, "/")

		info, err := os.Stat(file)
		if err != nil || info.IsDir() {
			continue
		}

		getCmd := &Command{
			Name: "GET",
			Args: []string{relPath},
		}
		c.handleGetCommand(getCmd)

		time.Sleep(100 * time.Millisecond)
	}

	return Message{
		Type: MsgTypeCommandResult,
		Data: fmt.Sprintf("Sending directory: %s", dirPath),
	}
}

func (c *Connection) handlePutDirCommand(cmd *Command) Message {
	if len(cmd.Args) < 1 {
		return Message{
			Type: MsgTypeError,
			Data: "PUTDIR requires a directory path",
		}
	}

	// Security check
	if !isPathSafe(cmd.Args[0], c.App.Config.Folder) {
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

	dirPath := cmd.Args[0]
	fullPath := filepath.Join(c.App.Config.Folder, dirPath)

	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Failed to create directory: %v", err),
		}
	}

	return Message{
		Type: MsgTypeCommandResult,
		Data: "Ready to receive directory files",
	}
}

func (c *Connection) handleGetMultipleCommand(cmd *Command) Message {
	if len(cmd.Args) < 1 {
		return Message{
			Type: MsgTypeError,
			Data: "GETM requires a file pattern",
		}
	}

	pattern := cmd.Args[0]

	if c.App.Config.ReadOnly {
		return Message{
			Type: MsgTypeError,
			Data: "This node is in read-only mode and cannot send files",
		}
	}

	matches, err := util.FindMatchingFiles(c.App.Config.Folder, pattern)
	if err != nil {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Failed to find matching files: %v", err),
		}
	}

	if len(matches) == 0 {
		return Message{
			Type: MsgTypeError,
			Data: "No files match the pattern",
		}
	}

	listMsg := Message{
		Type: MsgTypeCommandResult,
		Data: fmt.Sprintf("Found %d files matching pattern %s:\n%s",
			len(matches), pattern, strings.Join(matches, "\n")),
	}
	c.SendMessage(listMsg)

	for _, file := range matches {
		relPath := strings.TrimPrefix(file, c.App.Config.Folder)
		relPath = strings.TrimPrefix(relPath, "/")

		getCmd := &Command{
			Name: "GET",
			Args: []string{relPath},
		}
		c.handleGetCommand(getCmd)

		time.Sleep(100 * time.Millisecond)
	}

	return Message{
		Type: MsgTypeCommandResult,
		Data: fmt.Sprintf("Sending %d files matching pattern: %s", len(matches), pattern),
	}
}

func (c *Connection) handlePutMultipleCommand(cmd *Command) Message {
	if len(cmd.Args) < 1 {
		return Message{
			Type: MsgTypeError,
			Data: "PUTM requires a file pattern",
		}
	}

	if c.App.Config.WriteOnly {
		return Message{
			Type: MsgTypeError,
			Data: "This node is in write-only mode and cannot receive files",
		}
	}

	return Message{
		Type: MsgTypeCommandResult,
		Data: "Ready to receive multiple files",
	}
}

func (c *Connection) handleStatusCommand(cmd *Command) Message {
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

		statusText += fmt.Sprintf("[%d] %s %s: %.1f%% complete (%.2f KB/s)\n",
			t.ID, typeStr, t.Name, pct, t.Speed)
	}

	return Message{
		Type: MsgTypeCommandResult,
		Data: statusText,
	}
}

func (c *Connection) handleFileStart(msg Message) {
	parts := strings.Split(msg.Data, "|")
	if len(parts) != 2 {
		c.SendError("Invalid file start format")
		return
	}

	filePath := parts[0]

	// Security check
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

	fullPath := filepath.Join(c.App.Config.Folder, filePath)

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.SendError(fmt.Sprintf("Failed to create directory: %v", err))
		return
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
	var transfer *FileTransfer
	for _, t := range c.App.Transfers {
		if t.Conn == c && t.Type == TransferTypeReceive {
			transfer = t
			break
		}
	}

	if transfer == nil {
		c.SendError("No active file transfer")
		return
	}

	data := []byte(msg.Data)
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
			transfer.LastProgress = progress
			transfer.UpdateProgress(transfer.BytesTransferred, transfer.Speed)
		}
	}
}

func (c *Connection) handleFileEnd(msg Message) {
	filePath := msg.Data

	var transfer *FileTransfer
	for _, t := range c.App.Transfers {
		if t.Conn == c && t.Type == TransferTypeReceive && t.Name == filePath {
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
	c.SendMessage(ackMsg)

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
		if t.Name == filePath && t.Conn == c && t.Status == TransferStatusWaitingAck {
			transfer = t
			break
		}
	}

	if transfer != nil {
		transfer.Status = TransferStatusComplete
		c.Log.Success("File transfer acknowledged: %s", filePath)
	}
}
