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

// Register a handler function for a specific response
func (c *Connection) RegisterResponseHandler(id string, handler func(Message)) {
	c.responseHandlerMu.Lock()
	defer c.responseHandlerMu.Unlock()
	c.responseHandlers[id] = handler
}

// Unregister a response handler
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
	// Send our name
	handshake := Message{
		Type: MsgTypeHandshake,
		Data: c.Name,
	}

	if err := c.SendMessage(handshake); err != nil {
		return fmt.Errorf("failed to send handshake: %v", err)
	}

	// Read their name
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
}

func (c *Connection) SendMessage(msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = c.Writer.WriteString(string(data) + "\n")
	if err != nil {
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

	// Check if there's a registered handler for this message ID
	if msg.ID != "" {
		c.responseHandlerMu.Lock()
		handler, exists := c.responseHandlers[msg.ID]
		c.responseHandlerMu.Unlock()

		if exists {
			handler(msg)
			return
		}
	}

	// Otherwise process the message by type
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
	default:
		c.Log.Warn("Unknown message type: %s", msg.Type)
	}
}

func (c *Connection) handleCommand(msg Message) {
	cmd := ParseCommand(msg.Data)
	if cmd == nil {
		c.SendError("Invalid command format")
		return
	}

	c.Log.Debug("Received command: %s", cmd.Name)

	switch cmd.Name {
	case "LS", "LIST":
		c.handleListCommand(cmd)
	case "CDR":
		c.handleCDCommand(cmd)
	case "GET":
		c.handleGetCommand(cmd)
	case "PUT":
		c.handlePutCommand(cmd)
	case "INFO":
		c.handleInfoCommand(cmd)
	case "GETDIR":
		c.handleGetDirCommand(cmd)
	case "PUTDIR":
		c.handlePutDirCommand(cmd)
	case "GETM":
		c.handleGetMultipleCommand(cmd)
	case "PUTM":
		c.handlePutMultipleCommand(cmd)
	case "STATUS":
		c.handleStatusCommand(cmd)
	default:
		c.SendError(fmt.Sprintf("Unknown command: %s", cmd.Name))
	}
}

func (c *Connection) SendError(errorMsg string) {
	msg := Message{
		Type: MsgTypeError,
		Data: errorMsg,
	}
	c.SendMessage(msg)
}

func (c *Connection) handleCDCommand(cmd *Command) {
	if len(cmd.Args) < 1 {
		c.SendError("CDR requires a directory path")
		return
	}

	path := cmd.Args[0]
	fullPath := filepath.Join(c.App.Config.Folder, path)

	// Check if directory exists
	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		c.SendError(fmt.Sprintf("Directory not found: %s", path))
		return
	}

	if !info.IsDir() {
		c.SendError(fmt.Sprintf("Not a directory: %s", path))
		return
	}

	// Change to the directory (this only affects future file operations)
	c.App.Config.Folder = fullPath

	response := Message{
		Type: MsgTypeCommandResult,
		Data: fmt.Sprintf("Changed to %s", path),
	}
	c.SendMessage(response)
}

func (c *Connection) handleListCommand(cmd *Command) {
	path := "."
	if len(cmd.Args) > 0 {
		path = cmd.Args[0]
	}

	recursive := cmd.Name == "LSR"

	filepath.Abs(filepath.Join(c.App.Config.Folder, path))

	files, err := util.ListFiles(path, c.App.Config.Folder, recursive)
	if err != nil {
		c.SendError(fmt.Sprintf("Failed to list files: %v", err))
		return
	}

	response := Message{
		Type: MsgTypeCommandResult,
		Data: strings.Join(files, "\n"),
	}
	c.SendMessage(response)
}

func (c *Connection) handleGetCommand(cmd *Command) {
	if len(cmd.Args) < 1 {
		c.SendError("GET requires a file path")
		return
	}

	filePath := cmd.Args[0]

	// Check read-only mode
	if c.App.Config.ReadOnly {
		c.SendError("This node is in read-only mode and cannot send files")
		return
	}

	fullPath := filepath.Join(c.App.Config.Folder, filePath)

	info, err := os.Stat(fullPath)
	if err != nil {
		c.SendError(fmt.Sprintf("File not found: %v", err))
		return
	}

	if info.IsDir() {
		c.SendError("GET cannot transfer directories, use GETDIR instead")
		return
	}

	// Check max size
	if c.App.Config.MaxSize > 0 && info.Size() > int64(c.App.Config.MaxSize*1024*1024) {
		c.SendError(fmt.Sprintf("File size exceeds maximum allowed size of %d MB", c.App.Config.MaxSize))
		return
	}

	file, err := os.Open(fullPath)
	if err != nil {
		c.SendError(fmt.Sprintf("Failed to open file: %v", err))
		return
	}
	defer file.Close()

	// Start a file transfer
	transfer := NewFileTransfer(filePath, info.Size(), TransferTypeSend, c)
	c.App.AddTransfer(transfer)

	// Send file start message
	startMsg := Message{
		Type: MsgTypeFileStart,
		Data: fmt.Sprintf("%s|%d", filePath, info.Size()),
	}
	if err := c.SendMessage(startMsg); err != nil {
		c.SendError(fmt.Sprintf("Failed to send file start: %v", err))
		return
	}

	// Start the transfer in a goroutine
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

			// Send progress updates (every 5%)
			progress := (totalSent * 100) / info.Size()
			if progress > lastProgress+4 {
				lastProgress = progress
				elapsedTime := time.Since(startTime).Seconds()
				speed := float64(totalSent) / elapsedTime / 1024 // KB/s

				progMsg := Message{
					Type: MsgTypeProgress,
					Data: fmt.Sprintf("%s|%d|%d|%.2f", filePath, totalSent, info.Size(), speed),
				}
				c.SendMessage(progMsg)

				transfer.UpdateProgress(totalSent, speed)
			}
		}

		// Send file end message
		endMsg := Message{
			Type: MsgTypeFileEnd,
			Data: filePath,
		}
		if err := c.SendMessage(endMsg); err != nil {
			transfer.Status = TransferStatusFailed
			c.Log.Error("Failed to send file end: %v", err)
			return
		}

		// Wait for ACK
		transfer.Status = TransferStatusWaitingAck

		// Timeout if no ACK received
		go func() {
			time.Sleep(10 * time.Second)
			if transfer.Status == TransferStatusWaitingAck {
				transfer.Status = TransferStatusFailed
				c.Log.Error("Transfer timed out waiting for ACK")
			}
		}()
	}()
}

func (c *Connection) handlePutCommand(cmd *Command) {
	// Implementation will be handled via file transfer messages
	if len(cmd.Args) < 1 {
		c.SendError("PUT requires a file path")
		return
	}

	// Check write-only mode
	if c.App.Config.WriteOnly {
		c.SendError("This node is in write-only mode and cannot receive files")
		return
	}

	// Acknowledge the command
	response := Message{
		Type: MsgTypeCommandResult,
		Data: "Ready to receive file",
	}
	c.SendMessage(response)
}

func (c *Connection) handleInfoCommand(cmd *Command) {
	info := fmt.Sprintf("Node: %s\n", c.Name)
	info += fmt.Sprintf("Folder: %s\n", c.App.Config.Folder)
	info += fmt.Sprintf("Read-only: %t\n", c.App.Config.ReadOnly)
	info += fmt.Sprintf("Write-only: %t\n", c.App.Config.WriteOnly)
	if c.App.Config.MaxSize > 0 {
		info += fmt.Sprintf("Max file size: %d MB\n", c.App.Config.MaxSize)
	} else {
		info += "Max file size: Unlimited\n"
	}

	response := Message{
		Type: MsgTypeCommandResult,
		Data: info,
	}
	c.SendMessage(response)
}

func (c *Connection) handleGetDirCommand(cmd *Command) {
	if len(cmd.Args) < 1 {
		c.SendError("GETDIR requires a directory path")
		return
	}

	dirPath := cmd.Args[0]

	// Check read-only mode
	if c.App.Config.ReadOnly {
		c.SendError("This node is in read-only mode and cannot send files")
		return
	}

	fullPath := filepath.Join(c.App.Config.Folder, dirPath)

	info, err := os.Stat(fullPath)
	if err != nil {
		c.SendError(fmt.Sprintf("Directory not found: %v", err))
		return
	}

	if !info.IsDir() {
		c.SendError("GETDIR can only transfer directories, use GET for files")
		return
	}

	// Get list of files in directory
	files, err := util.ListFilesRecursive(fullPath)
	if err != nil {
		c.SendError(fmt.Sprintf("Failed to list directory: %v", err))
		return
	}

	// Send directory structure first
	dirMsg := Message{
		Type: MsgTypeCommandResult,
		Data: strings.Join(files, "\n"),
	}
	c.SendMessage(dirMsg)

	// Then transfer each file
	for _, file := range files {
		relPath := strings.TrimPrefix(file, c.App.Config.Folder)
		relPath = strings.TrimPrefix(relPath, "/")

		info, err := os.Stat(file)
		if err != nil || info.IsDir() {
			continue // Skip directories
		}

		// Create GET command for each file
		getCmd := &Command{
			Name: "GET",
			Args: []string{relPath},
		}
		c.handleGetCommand(getCmd)

		// Small delay between files
		time.Sleep(100 * time.Millisecond)
	}
}

func (c *Connection) handlePutDirCommand(cmd *Command) {
	if len(cmd.Args) < 1 {
		c.SendError("PUTDIR requires a directory path")
		return
	}

	// Check write-only mode
	if c.App.Config.WriteOnly {
		c.SendError("This node is in write-only mode and cannot receive files")
		return
	}

	dirPath := cmd.Args[0]
	fullPath := filepath.Join(c.App.Config.Folder, dirPath)

	// Create directory if it doesn't exist
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		c.SendError(fmt.Sprintf("Failed to create directory: %v", err))
		return
	}

	// Acknowledge the command
	response := Message{
		Type: MsgTypeCommandResult,
		Data: "Ready to receive directory files",
	}
	c.SendMessage(response)

	// The rest will be handled by file transfer messages
}

func (c *Connection) handleGetMultipleCommand(cmd *Command) {
	if len(cmd.Args) < 1 {
		c.SendError("GETM requires a file pattern")
		return
	}

	pattern := cmd.Args[0]

	// Check read-only mode
	if c.App.Config.ReadOnly {
		c.SendError("This node is in read-only mode and cannot send files")
		return
	}

	// Find matching files
	matches, err := util.FindMatchingFiles(c.App.Config.Folder, pattern)
	if err != nil {
		c.SendError(fmt.Sprintf("Failed to find matching files: %v", err))
		return
	}

	if len(matches) == 0 {
		c.SendError("No files match the pattern")
		return
	}

	// Send list of files first
	listMsg := Message{
		Type: MsgTypeCommandResult,
		Data: fmt.Sprintf("Found %d files matching pattern %s:\n%s",
			len(matches), pattern, strings.Join(matches, "\n")),
	}
	c.SendMessage(listMsg)

	// Then transfer each file
	for _, file := range matches {
		relPath := strings.TrimPrefix(file, c.App.Config.Folder)
		relPath = strings.TrimPrefix(relPath, "/")

		// Create GET command for each file
		getCmd := &Command{
			Name: "GET",
			Args: []string{relPath},
		}
		c.handleGetCommand(getCmd)

		// Small delay between files
		time.Sleep(100 * time.Millisecond)
	}
}

func (c *Connection) handlePutMultipleCommand(cmd *Command) {
	if len(cmd.Args) < 1 {
		c.SendError("PUTM requires a file pattern")
		return
	}

	// Check write-only mode
	if c.App.Config.WriteOnly {
		c.SendError("This node is in write-only mode and cannot receive files")
		return
	}

	// Acknowledge the command
	response := Message{
		Type: MsgTypeCommandResult,
		Data: "Ready to receive multiple files",
	}
	c.SendMessage(response)

	// The rest will be handled by file transfer messages
}

func (c *Connection) handleStatusCommand(cmd *Command) {
	transfers := c.App.GetCurrentTransfers()

	if len(transfers) == 0 {
		response := Message{
			Type: MsgTypeCommandResult,
			Data: "No active transfers",
		}
		c.SendMessage(response)
		return
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

	response := Message{
		Type: MsgTypeCommandResult,
		Data: statusText,
	}
	c.SendMessage(response)
}

func (c *Connection) handleFileStart(msg Message) {
	parts := strings.Split(msg.Data, "|")
	if len(parts) != 2 {
		c.SendError("Invalid file start format")
		return
	}

	filePath := parts[0]
	fileSize, err := util.ParseInt64(parts[1])
	if err != nil {
		c.SendError("Invalid file size")
		return
	}

	// Check max size
	if c.App.Config.MaxSize > 0 && fileSize > int64(c.App.Config.MaxSize*1024*1024) {
		c.SendError(fmt.Sprintf("File size exceeds maximum allowed size of %d MB", c.App.Config.MaxSize))
		return
	}

	// Check write-only mode
	if c.App.Config.WriteOnly {
		c.SendError("This node is in write-only mode and cannot receive files")
		return
	}

	fullPath := filepath.Join(c.App.Config.Folder, filePath)

	// Create directory if it doesn't exist
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.SendError(fmt.Sprintf("Failed to create directory: %v", err))
		return
	}

	// Create file
	file, err := os.Create(fullPath)
	if err != nil {
		c.SendError(fmt.Sprintf("Failed to create file: %v", err))
		return
	}

	// Start a file transfer
	transfer := NewFileTransfer(filePath, fileSize, TransferTypeReceive, c)
	transfer.File = file
	c.App.AddTransfer(transfer)

	c.Log.Info("Starting to receive file %s (%d bytes)", filePath, fileSize)
}

func (c *Connection) handleFileData(msg Message) {
	// Find the active transfer
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

	// Update progress
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

	// Find the active transfer
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

	// Close the file
	if transfer.File != nil {
		transfer.File.Close()
		transfer.File = nil
	}

	// Send ACK
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
	// Using totalSize instead of total to avoid unused variable
	totalSize, _ := util.ParseInt64(parts[2])
	speed, _ := util.ParseFloat64(parts[3])

	// Find the transfer
	var transfer *FileTransfer
	for _, t := range c.App.Transfers {
		if t.Name == filePath && t.Conn == c {
			transfer = t
			break
		}
	}

	if transfer != nil {
		transfer.BytesTransferred = received
		transfer.TotalSize = totalSize // Use totalSize
		transfer.Speed = speed
		transfer.UpdateProgress(received, speed)
	}
}

func (c *Connection) handleAck(msg Message) {
	filePath := msg.Data

	// Find the transfer
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
