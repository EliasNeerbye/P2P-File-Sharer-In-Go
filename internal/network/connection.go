// internal/network/connection.go
package network

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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

const (
	maxRetries  = 5
	initialWait = 100 * time.Millisecond
)

func isPathSafe(requestedPath, baseFolder string) bool {
	absBase, err := filepath.Abs(baseFolder)
	if err != nil {
		return false
	}

	targetPath := filepath.Join(baseFolder, requestedPath)
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}

	return strings.HasPrefix(absTarget, absBase)
}

type PendingMessage struct {
	Message      Message
	RetryCount   int
	LastAttempt  time.Time
	ResponseChan chan Message
}

type Connection struct {
	ID                   string
	Conn                 net.Conn
	Name                 string
	RemoteName           string
	App                  *App
	Log                  *util.Logger
	Reader               *bufio.Reader
	Writer               *bufio.Writer
	isClient             bool
	responseHandlers     map[string]func(Message)
	responseHandlerMu    sync.Mutex
	sendMutex            sync.Mutex
	pendingMessages      map[string]*PendingMessage
	pendingMessagesMutex sync.Mutex
	healthCheck          *time.Ticker
	lastMessageReceived  time.Time
	lastPingSent         time.Time
	activePings          map[string]time.Time
	activePingsMutex     sync.Mutex
}

func NewConnection(conn net.Conn, app *App, isClient bool) *Connection {
	id := conn.RemoteAddr().String()
	c := &Connection{
		ID:                  id,
		Conn:                conn,
		App:                 app,
		Log:                 util.NewLogger(app.Config.Verbose, fmt.Sprintf("Conn-%s", id)),
		Reader:              bufio.NewReader(conn),
		Writer:              bufio.NewWriter(conn),
		Name:                app.Config.Name,
		isClient:            isClient,
		responseHandlers:    make(map[string]func(Message)),
		pendingMessages:     make(map[string]*PendingMessage),
		lastMessageReceived: time.Now(),
		activePings:         make(map[string]time.Time),
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

	c.startHealthCheck()
	go c.monitorPendingMessages()

	for {
		c.Conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		line, err := c.Reader.ReadString('\n')
		c.Conn.SetReadDeadline(time.Time{})

		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if time.Since(c.lastMessageReceived) > 60*time.Second {
					c.Log.Error("Connection timed out, no messages received for over 60 seconds")
					break
				}
				continue
			}

			if err != io.EOF {
				c.Log.Error("Read error: %v", err)
			}
			break
		}

		c.lastMessageReceived = time.Now()
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

	c.Conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	line, err := c.Reader.ReadString('\n')
	c.Conn.SetReadDeadline(time.Time{})

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
	if c.healthCheck != nil {
		c.healthCheck.Stop()
	}

	c.Conn.Close()
	c.App.RemoveConnection(c)
	c.Log.Info("Connection closed")

	if c.isClient {
		c.Log.Error("Lost connection to server, exiting...")
		fmt.Println("\nDisconnected from server. Press Enter to exit.")

		time.Sleep(500 * time.Millisecond)

		os.Exit(1)
	} else {
		fmt.Println("\nClient disconnected. Waiting for new connections...")
		fmt.Print("> ")
	}
}

func (c *Connection) startHealthCheck() {
	c.healthCheck = time.NewTicker(10 * time.Second)

	go func() {
		for range c.healthCheck.C {
			if time.Since(c.lastMessageReceived) > 25*time.Second &&
				time.Since(c.lastPingSent) > 10*time.Second {
				c.sendPing()
			}

			c.checkActivePings()
		}
	}()
}

func (c *Connection) sendPing() {
	pingID := fmt.Sprintf("ping-%d", time.Now().UnixNano())
	ping := Message{
		Type: MsgTypePing,
		ID:   pingID,
	}

	c.RegisterResponseHandler(pingID, func(msg Message) {
		if msg.Type == MsgTypePong {
			c.activePingsMutex.Lock()
			delete(c.activePings, pingID)
			c.activePingsMutex.Unlock()
		}
	})

	c.activePingsMutex.Lock()
	c.activePings[pingID] = time.Now()
	c.activePingsMutex.Unlock()

	c.lastPingSent = time.Now()
	c.SendMessage(ping)
}

func (c *Connection) checkActivePings() {
	c.activePingsMutex.Lock()
	defer c.activePingsMutex.Unlock()

	for id, sentTime := range c.activePings {
		if time.Since(sentTime) > 30*time.Second {
			c.Log.Warn("Ping %s timed out after 30 seconds", id)
			delete(c.activePings, id)
		}
	}
}

func (c *Connection) SendMessage(msg Message) error {
	c.sendMutex.Lock()
	defer c.sendMutex.Unlock()

	if msg.ID == "" && msg.RequiresAck() {
		msg.ID = fmt.Sprintf("msg-%d", time.Now().UnixNano())
	}

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

	err = c.Writer.Flush()
	if err != nil {
		c.Log.Error("Failed to flush message: %v", err)
		return err
	}

	if msg.RequiresAck() && msg.Type != MsgTypeCommandResult {
		c.addPendingMessage(msg)
	}

	return nil
}

func (c *Connection) addPendingMessage(msg Message) {
	if msg.ID == "" {
		return
	}

	c.pendingMessagesMutex.Lock()
	defer c.pendingMessagesMutex.Unlock()

	c.pendingMessages[msg.ID] = &PendingMessage{
		Message:      msg.Clone(),
		RetryCount:   0,
		LastAttempt:  time.Now(),
		ResponseChan: make(chan Message, 1),
	}
}

func (c *Connection) monitorPendingMessages() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()

		c.pendingMessagesMutex.Lock()
		for id, pending := range c.pendingMessages {
			if pending.RetryCount >= maxRetries {
				c.Log.Error("Message %s failed after %d retries", id, maxRetries)
				delete(c.pendingMessages, id)
				continue
			}

			waitTime := initialWait * (1 << uint(pending.RetryCount))
			if now.Sub(pending.LastAttempt) > waitTime {
				msg := pending.Message.Clone()
				msg.IncrementRetry()
				pending.RetryCount++
				pending.LastAttempt = now

				go func(m Message) {
					c.sendMutex.Lock()
					data, _ := m.Marshal()
					c.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
					c.Writer.WriteString(string(data) + "\n")
					c.Writer.Flush()
					c.Conn.SetWriteDeadline(time.Time{})
					c.sendMutex.Unlock()
				}(msg)
			}
		}
		c.pendingMessagesMutex.Unlock()
	}
}

func (c *Connection) removePendingMessage(id string) {
	c.pendingMessagesMutex.Lock()
	defer c.pendingMessagesMutex.Unlock()
	delete(c.pendingMessages, id)
}

func (c *Connection) SendReliableMessage(msg Message) error {
	if msg.ID == "" {
		msg.ID = fmt.Sprintf("reliable-%d", time.Now().UnixNano())
	}

	responseChan := make(chan Message, 1)

	c.RegisterResponseHandler(msg.ID, func(resp Message) {
		responseChan <- resp
	})

	defer c.UnregisterResponseHandler(msg.ID)

	if err := c.SendMessage(msg); err != nil {
		return err
	}

	select {
	case <-responseChan:
		return nil
	case <-time.After(15 * time.Second):
		return fmt.Errorf("reliable message timed out waiting for response")
	}
}

func (c *Connection) handleMessage(messageStr string) {
	var msg Message
	if err := json.Unmarshal([]byte(messageStr), &msg); err != nil {
		c.Log.Error("Invalid message format: %v", err)
		return
	}

	if msg.RequiresAck() && msg.Type != MsgTypeACK {
		ackMsg := NewAckMessage(msg.ID)
		c.SendMessage(ackMsg)
	}

	if msg.Type == MsgTypeACK {
		c.removePendingMessage(msg.ID)
	}

	if msg.Type == MsgTypePing {
		pongMsg := Message{
			Type: MsgTypePong,
			ID:   msg.ID,
		}
		c.SendMessage(pongMsg)
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
		fmt.Printf("\n%s[ERROR]%s %s\n\n> ", util.Bold+util.Red, util.Reset, msg.Data)
	case MsgTypeMessage:
		fmt.Printf("\n%s[MESSAGE FROM %s]%s %s\n\n> ", util.Bold+util.Purple, c.RemoteName, util.Reset, msg.Data)
	case MsgTypeCommandResult:
		fmt.Printf("\n%s\n", msg.Data)
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

	if !isPathSafe(path, c.App.Config.Folder) {
		return Message{
			Type: MsgTypeError,
			Data: "Access denied: path is outside the shared folder",
		}
	}

	recursive := cmd.Name == "LSR"

	fullPath := filepath.Join(c.App.Config.Folder, path)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Failed to resolve path: %v", err),
		}
	}

	files, err := util.ListFiles(absPath, c.App.Config.Folder, recursive)
	if err != nil {
		return Message{
			Type: MsgTypeError,
			Data: fmt.Sprintf("Failed to list files: %v", err),
		}
	}

	relPath, _ := filepath.Rel(c.App.Config.Folder, absPath)
	if relPath == "" {
		relPath = "."
	}
	result := fmt.Sprintf("Contents of %s:\n%s", relPath, strings.Join(files, "\n"))

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

	filePath := cmd.Args[0]

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

	// Skip restricted files
	if filepath.Base(filePath) == ".fshignore" || filepath.Base(filePath) == ".gitignore" {
		return Message{
			Type: MsgTypeError,
			Data: "This file is restricted for transfer",
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

	transfer := NewFileTransfer(filePath, info.Size(), TransferTypeSend, c)
	transfer.File = file
	c.App.AddTransfer(transfer)

	if c.App.Config.Verify {
		checksum, err := transfer.CalculateChecksum()
		if err != nil {
			c.Log.Warn("Failed to calculate checksum: %v", err)
		} else {
			transfer.Checksum = checksum
		}
	}

	startMsg := Message{
		Type: MsgTypeFileStart,
		Data: fmt.Sprintf("%s|%d|%s", filePath, info.Size(), transfer.Checksum),
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
		chunksSent := 0

		for {
			if transfer.WaitForPauseIfNeeded() {
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

			// Check for pause or cancellation
			if transfer.Status == TransferStatusFailed {
				return
			}

			dataMsg := NewBinaryMessage(MsgTypeFileData, buffer[:n])
			if err := c.SendMessage(dataMsg); err != nil {
				transfer.Status = TransferStatusFailed
				c.Log.Error("Failed to send file data: %v", err)
				return
			}

			totalSent += int64(n)
			transfer.BytesTransferred = totalSent
			chunksSent++

			progress := (totalSent * 100) / info.Size()
			if totalSent-lastProgressBytes > 1048576 || progress > lastProgress+2 {
				lastProgress = progress
				lastProgressBytes = totalSent
				elapsedTime := time.Since(startTime).Seconds()
				speed := float64(totalSent) / elapsedTime / 1024

				progMsg := Message{
					Type: MsgTypeProgress,
					Data: fmt.Sprintf("%s|%d|%d|%.2f", filePath, totalSent, info.Size(), speed),
				}
				c.SendMessage(progMsg)

				transfer.UpdateProgress(totalSent, speed)
			}

			if chunksSent%20 == 0 {
				time.Sleep(5 * time.Millisecond)
			}
		}

		endMsg := Message{
			Type: MsgTypeFileEnd,
			Data: fmt.Sprintf("%s|%s", filePath, transfer.Checksum),
		}

		// Use a reliable send for the end message with multiple retries
		var endErr error
		for i := 0; i < 5; i++ {
			endErr = c.SendReliableMessage(endMsg)
			if endErr == nil {
				break
			}
			time.Sleep(500 * time.Millisecond * time.Duration(i+1))
		}

		if endErr != nil {
			transfer.Status = TransferStatusFailed
			c.Log.Error("Failed to send file end: %v", endErr)
			return
		}

		transfer.Status = TransferStatusComplete
		c.Log.Success("File transfer complete: %s", filePath)
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

	fileList := make([]string, 0, len(files))
	for _, file := range files {
		relPath, err := filepath.Rel(c.App.Config.Folder, file)
		if err == nil {
			// Skip system and ignore files
			baseName := filepath.Base(relPath)
			if baseName != ".fshignore" && baseName != ".gitignore" {
				fileList = append(fileList, relPath)
			}
		}
	}

	return Message{
		Type: MsgTypeCommandResult,
		Data: strings.Join(fileList, "\n"),
	}
}

func (c *Connection) handlePutDirCommand(cmd *Command) Message {
	if len(cmd.Args) < 1 {
		return Message{
			Type: MsgTypeError,
			Data: "PUTDIR requires a directory path",
		}
	}

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
	if len(cmd.Args) == 0 {
		return Message{
			Type: MsgTypeError,
			Data: "GETM requires at least one file",
		}
	}

	filePaths := cmd.Args
	var matchedFiles []string

	for _, path := range filePaths {
		if strings.Contains(path, "*") || strings.Contains(path, "?") {
			matches, err := util.FindMatchingFiles(c.App.Config.Folder, path)
			if err != nil {
				return Message{
					Type: MsgTypeError,
					Data: fmt.Sprintf("Failed to find matching files: %v", err),
				}
			}

			if len(matches) > 0 {
				matchedFiles = append(matchedFiles, matches...)
			}
		} else {
			fullPath := filepath.Join(c.App.Config.Folder, path)
			if _, err := os.Stat(fullPath); err == nil {
				matchedFiles = append(matchedFiles, path)
			}
		}
	}

	// Filter out ignore files
	filteredFiles := make([]string, 0, len(matchedFiles))
	for _, path := range matchedFiles {
		baseName := filepath.Base(path)
		if baseName != ".fshignore" && baseName != ".gitignore" {
			filteredFiles = append(filteredFiles, path)
		}
	}

	if len(filteredFiles) == 0 {
		return Message{
			Type: MsgTypeError,
			Data: "No files match the specified patterns or names",
		}
	}

	return Message{
		Type: MsgTypeCommandResult,
		Data: strings.Join(filteredFiles, "\n"),
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
	if len(parts) < 2 {
		c.SendError("Invalid file start format")
		return
	}

	filePath := parts[0]

	if !isPathSafe(filePath, c.App.Config.Folder) {
		c.SendError("Access denied: path is outside the shared folder")
		return
	}

	fileSize, err := util.ParseInt64(parts[1])
	if err != nil {
		c.SendError("Invalid file size")
		return
	}

	checksum := ""
	if len(parts) > 2 {
		checksum = parts[2]
	}

	if c.App.Config.MaxSize > 0 && fileSize > int64(c.App.Config.MaxSize*1024*1024) {
		c.SendError(fmt.Sprintf("File size exceeds maximum allowed size of %d MB", c.App.Config.MaxSize))
		return
	}

	if c.App.Config.WriteOnly {
		c.SendError("This node is in write-only mode and cannot receive files")
		return
	}

	// Skip restricted files
	if filepath.Base(filePath) == ".fshignore" || filepath.Base(filePath) == ".gitignore" {
		c.SendError("This file is restricted for transfer")
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
	transfer.Checksum = checksum
	c.App.AddTransfer(transfer)

	c.Log.Info("Starting to receive file %s (%d bytes)", filePath, fileSize)
}

func (c *Connection) handleFileData(msg Message) {
	transfers := c.App.GetTransfers()
	var transfer *FileTransfer

	for _, t := range transfers {
		if t.Conn == c && t.Type == TransferTypeReceive && t.Status == TransferStatusInProgress {
			transfer = t
			break
		}
	}

	if transfer == nil {
		c.SendError("No active file transfer")
		return
	}

	if transfer.File == nil {
		c.SendError("File not open for writing")
		return
	}

	if transfer.Status == TransferStatusPaused {
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
			transfer.LastProgress = progress
			elapsedTime := time.Since(transfer.StartTime).Seconds()
			speed := float64(transfer.BytesTransferred) / elapsedTime / 1024
			transfer.UpdateProgress(transfer.BytesTransferred, speed)
		}
	}
}

func (c *Connection) handleFileEnd(msg Message) {
	parts := strings.Split(msg.Data, "|")
	filePath := parts[0]
	remoteChecksum := ""
	if len(parts) > 1 {
		remoteChecksum = parts[1]
	}

	transfers := c.App.GetTransfers()
	var transfer *FileTransfer

	for _, t := range transfers {
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

	// Verify checksum if needed
	if c.App.Config.Verify && remoteChecksum != "" {
		file, err := os.Open(filepath.Join(c.App.Config.Folder, filePath))
		if err != nil {
			c.Log.Error("Failed to open file for checksum verification: %v", err)
		} else {
			defer file.Close()

			hash := sha256.New()
			if _, err := io.Copy(hash, file); err != nil {
				c.Log.Error("Failed to calculate checksum: %v", err)
			} else {
				localChecksum := hex.EncodeToString(hash.Sum(nil))
				if localChecksum != remoteChecksum {
					c.Log.Error("Checksum verification failed: expected %s, got %s", remoteChecksum, localChecksum)
					transfer.Status = TransferStatusFailed

					// Delete the corrupted file
					file.Close()
					os.Remove(filepath.Join(c.App.Config.Folder, filePath))

					c.SendError(fmt.Sprintf("Checksum verification failed for %s", filePath))
					return
				}
				c.Log.Success("Checksum verification successful for %s", filePath)
			}
		}
	}

	transfer.Status = TransferStatusComplete
	c.Log.Success("File transfer complete: %s", filePath)

	// Send ACK with retry to ensure it reaches the sender
	ackMsg := Message{
		Type: MsgTypeACK,
		Data: filePath,
	}

	for i := 0; i < 5; i++ {
		c.SendMessage(ackMsg)
		time.Sleep(300 * time.Millisecond)
	}

	fmt.Printf("\nFile transfer complete: %s\n> ", filePath)
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
		transfer.Status = TransferStatusComplete
		c.Log.Success("File transfer acknowledged: %s", filePath)
		fmt.Printf("\nFile transfer complete: %s\n> ", filePath)
		c.App.RemoveTransfer(transfer)
	}
}
