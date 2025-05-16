package network

import (
	"fmt"
	"io"
	"local-file-sharer/internal/util"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
)

type Command struct {
	Name string
	Args []string
}

func ParseCommand(cmdStr string) *Command {
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return nil
	}

	cmd := &Command{
		Name: strings.ToUpper(parts[0]),
		Args: parts[1:],
	}

	return cmd
}

type CommandParser struct {
	App *App
}

func NewCommandParser(app *App) *CommandParser {
	return &CommandParser{
		App: app,
	}
}

func StartCommandInterface(app *App) {
	log := app.Log
	parser := app.CommandParser

	fmt.Println("\n=== P2P File Sharer ===")
	fmt.Printf("Node: %s\n", app.Config.Name)
	fmt.Printf("Folder: %s\n\n", app.Config.Folder)

	setupGracefulShutdown(app)

	// Setup raw terminal mode for tab completion
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Error("Failed to set terminal to raw mode: %v", err)
	} else {
		defer term.Restore(int(os.Stdin.Fd()), oldState)
	}

	terminal := term.NewTerminal(os.Stdin, "> ")
	terminal.AutoCompleteCallback = func(line string, pos int, key rune) (string, int, bool) {
		if key != '\t' {
			return "", 0, false
		}

		parts := strings.Fields(line[:pos])
		if len(parts) == 0 {
			return "", 0, false
		}

		cmd := strings.ToUpper(parts[0])

		// Only provide completions for certain commands
		if (cmd == "GET" || cmd == "PUT" || cmd == "CD" || cmd == "LS" || cmd == "GETDIR" || cmd == "PUTDIR") && len(parts) == 2 && key == '\t' {
			partial := ""
			if len(parts) > 1 {
				partial = parts[1]
			}

			// Get completions
			var completions []string
			if cmd == "GET" || cmd == "GETDIR" {
				// Remote file completions would require server query, not implemented
				return "", 0, false
			} else {
				completions = util.GetFileCompletions(app.Config.Folder, partial)
			}

			if len(completions) == 1 {
				// Single completion - replace the current arg
				newLine := cmd + " " + completions[0]
				return newLine, len(newLine), true
			} else if len(completions) > 1 {
				// Multiple completions - show options
				fmt.Println()
				for _, c := range completions {
					fmt.Println(c)
				}
				fmt.Print("> " + line)
				return "", 0, false
			}
		}

		return "", 0, false
	}

	for app.Ready {
		line, err := terminal.ReadLine()
		if err != nil {
			if err != io.EOF {
				log.Error("Failed to read input: %v", err)
			}
			break
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		if err := parser.Execute(input); err != nil {
			log.Error("Command failed: %v", err)
			fmt.Print("> ")
		}
	}
}

func setupGracefulShutdown(app *App) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Println("\nShutting down gracefully...")

		app.mu.Lock()
		for _, conn := range app.Connections {
			conn.Conn.Close()
		}
		app.Connections = make(map[string]*Connection)
		app.mu.Unlock()

		app.Ready = false

		time.Sleep(500 * time.Millisecond)

		fmt.Println("Goodbye!")
		os.Exit(0)
	}()
}

func (p *CommandParser) Execute(input string) error {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil
	}

	cmdName := strings.ToUpper(parts[0])
	args := parts[1:]

	allowedDuringTransfer := map[string]bool{
		"STATUS": true,
		"HELP":   true,
		"INFO":   true,
		"QUIT":   true,
		"EXIT":   true,
		"PAUSE":  true,
		"RESUME": true,
		"CANCEL": true,
		"CLEAR":  true,
	}

	if !allowedDuringTransfer[cmdName] && p.App.IsActiveTransferInProgress() {
		return fmt.Errorf("cannot execute this command during active transfers, use STATUS to see progress")
	}

	var err error

	switch cmdName {
	case "LS", "LIST":
		err = p.handleLS(args)
	case "CD":
		err = p.handleCD(args)
	case "HELP":
		err = p.handleHelp()
	case "INFO":
		err = p.handleInfo()
	case "QUIT", "EXIT":
		err = p.handleQuit()
	case "PWD":
		err = p.handlePWD()
	case "LSR", "LISTREMOTE":
		err = p.handleRemoteLS(args)
	case "CDR":
		err = p.handleRemoteCD(args)
	case "GET":
		err = p.handleGet(args)
	case "PUT":
		err = p.handlePut(args)
	case "GETDIR":
		err = p.handleGetDir(args)
	case "PUTDIR":
		err = p.handlePutDir(args)
	case "GETM":
		err = p.handleGetMultiple(args)
	case "PUTM":
		err = p.handlePutMultiple(args)
	case "STATUS":
		err = p.handleStatus()
	case "MSG":
		err = p.handleMessage(args)
	case "PAUSE":
		err = p.handlePauseTransfer(args)
	case "RESUME":
		err = p.handleResumeTransfer(args)
	case "CANCEL":
		err = p.handleCancelTransfer(args)
	case "CLEAR":
		err = p.handleClear()
	default:
		return fmt.Errorf("unknown command: %s", cmdName)
	}

	if err == nil {
		fmt.Print("> ")
	}

	return err
}

func (p *CommandParser) handleLS(args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	resolvedPath, err := util.ResolvePath(path, p.App.Config.Folder)
	if err != nil {
		return err
	}

	files, err := util.ListFiles(resolvedPath, p.App.Config.Folder, false)
	if err != nil {
		return err
	}

	relPath, err := filepath.Rel(p.App.Config.Folder, resolvedPath)
	if err != nil {
		relPath = path
	}
	if relPath == "" {
		relPath = "."
	}

	fmt.Printf("Contents of %s:\n", relPath)
	for _, file := range files {
		fmt.Println(file)
	}
	return nil
}

func (p *CommandParser) handlePWD() error {
	absPath, err := filepath.Abs(p.App.Config.Folder)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	fmt.Println(absPath)
	return nil
}

func (p *CommandParser) handleCD(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("CD requires a directory path")
	}

	path := args[0]

	if path == ".." {
		parentDir := filepath.Dir(p.App.Config.Folder)
		basePath, err := filepath.Abs(p.App.Config.Folder)
		if err != nil {
			return fmt.Errorf("failed to resolve current folder path: %v", err)
		}

		baseFolder := filepath.Dir(basePath)
		if !strings.HasPrefix(parentDir, baseFolder) {
			return fmt.Errorf("access denied: path is outside the shared folder")
		}

		p.App.Config.Folder = parentDir
		return nil
	}

	resolvedPath, err := util.ResolvePath(path, p.App.Config.Folder)
	if err != nil {
		return err
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to access directory: %v", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", path)
	}

	p.App.Config.Folder = resolvedPath
	return nil
}

func (p *CommandParser) handleRemoteCD(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("CDR requires a directory path")
	}

	result, err := p.executeRemoteCommand("CDR", args[0])
	if err != nil {
		return err
	}

	fmt.Println(result)
	return nil
}

func (p *CommandParser) handleQuit() error {
	fmt.Println("Shutting down gracefully...")

	p.App.mu.Lock()
	for _, conn := range p.App.Connections {
		conn.Conn.Close()
	}
	p.App.Connections = make(map[string]*Connection)
	p.App.mu.Unlock()

	time.Sleep(500 * time.Millisecond)

	fmt.Println("Goodbye!")
	os.Exit(0)
	return nil
}

func (p *CommandParser) handleClear() error {
	// ANSI escape sequence to clear screen and move cursor to top-left
	fmt.Print("\033[H\033[2J")
	return nil
}

func (p *CommandParser) handleHelp() error {
	help := `
Available Commands:
  Local Commands:
    LS, LIST [path]    - List files in local directory
    CD <path>          - Change local directory
    PWD                - Show current working directory
    INFO               - Show information about this node
    CLEAR              - Clear the terminal screen
    HELP               - Show this help message
    QUIT, EXIT         - Exit the application

  Remote Commands:
    LSR, LISTREMOTE [path] - List files in remote directory
    CDR <path>         - Change remote directory
    GET <file>         - Download a file from remote peer
    PUT <file>         - Upload a file to remote peer
    GETDIR [dir]       - Download a directory from remote peer (current dir if omitted)
    PUTDIR [dir]       - Upload a directory to remote peer (current dir if omitted)
    GETM <file1> <file2> ... - Download multiple specified files
    PUTM <pattern>     - Upload multiple files matching pattern
    STATUS             - Show active transfers
    MSG <message>      - Send a message to the remote peer
    
  Transfer Control:
    PAUSE <id>         - Pause a file transfer
    RESUME <id>        - Resume a paused transfer
    CANCEL <id>        - Cancel an active transfer

Note: Tab completion is available for local file paths.
`
	fmt.Println(help)
	return nil
}

func (p *CommandParser) handleInfo() error {
	fmt.Printf("Node: %s\n", p.App.Config.Name)
	fmt.Printf("Folder: %s\n", p.App.Config.Folder)
	fmt.Printf("Read-only: %t\n", p.App.Config.ReadOnly)
	fmt.Printf("Write-only: %t\n", p.App.Config.WriteOnly)
	if p.App.Config.MaxSize > 0 {
		fmt.Printf("Max file size: %d MB\n", p.App.Config.MaxSize)
	} else {
		fmt.Println("Max file size: Unlimited")
	}
	fmt.Printf("Verify transfers: %t\n", p.App.Config.Verify)
	return nil
}

func (p *CommandParser) handleRemoteLS(args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	result, err := p.executeRemoteCommand("LS", path)
	if err != nil {
		return err
	}

	fmt.Println(result)
	return nil
}

func (p *CommandParser) handleGet(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("GET requires a file path")
	}

	_, err := p.executeRemoteCommand("GET", args[0])
	return err
}

func (p *CommandParser) handlePut(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("PUT requires a file path")
	}

	filePath := args[0]

	resolvedPath, err := util.ResolvePath(filePath, p.App.Config.Folder)
	if err != nil {
		return err
	}

	fileInfo, err := os.Stat(resolvedPath)
	if err != nil {
		return fmt.Errorf("file not found: %s", filePath)
	}

	if fileInfo.IsDir() {
		return fmt.Errorf("this is a dir, not a file")
	}

	relPath, err := filepath.Rel(p.App.Config.Folder, resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to get relative path: %v", err)
	}

	result, err := p.executeRemoteCommand("PUT", relPath)
	if err != nil {
		return err
	}

	if strings.Contains(result, "Ready to receive") {
		conn := p.getFirstConnection()
		if conn == nil {
			return fmt.Errorf("no active connection")
		}

		getCmd := &Command{
			Name: "GET",
			Args: []string{relPath},
		}

		conn.handleGetCommand(getCmd)
	}

	return nil
}

func (p *CommandParser) handleGetDir(args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	result, err := p.executeRemoteCommand("GETDIR", path)
	if err != nil {
		return err
	}

	files := strings.Split(result, "\n")
	for _, file := range files {
		if file == "" {
			continue
		}

		fmt.Printf("Getting file: %s\n", file)
		err := p.handleGet([]string{file})
		if err != nil {
			fmt.Printf("Error getting file %s: %v\n", file, err)
		}

		time.Sleep(1000 * time.Millisecond)
	}

	return nil
}

func (p *CommandParser) handlePutDir(args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	resolvedPath, err := util.ResolvePath(path, p.App.Config.Folder)
	if err != nil {
		return err
	}

	info, err := os.Stat(resolvedPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("directory not found: %s", path)
	}

	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", path)
	}

	relPath, err := filepath.Rel(p.App.Config.Folder, resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to get relative path: %v", err)
	}

	result, err := p.executeRemoteCommand("PUTDIR", relPath)
	if err != nil {
		return err
	}

	if strings.Contains(result, "Ready to receive") {
		conn := p.getFirstConnection()
		if conn == nil {
			return fmt.Errorf("no active connection")
		}

		cmd := &Command{
			Name: "GETDIR",
			Args: []string{relPath},
		}

		conn.handleGetDirCommand(cmd)
	}

	return nil
}

func (p *CommandParser) handleGetMultiple(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("GETM requires at least one file")
	}

	for _, file := range args {
		fmt.Printf("Getting file: %s\n", file)
		err := p.handleGet([]string{file})
		if err != nil {
			fmt.Printf("Error getting file %s: %v\n", file, err)
		}

		time.Sleep(1000 * time.Millisecond)
	}

	return nil
}

func (p *CommandParser) handlePutMultiple(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("PUTM requires a file pattern")
	}

	pattern := args[0]

	matches, err := p.executeLocalCommand("FINDM", pattern)
	if err != nil {
		return err
	}

	if matches == "" {
		return fmt.Errorf("no files match the pattern")
	}

	result, err := p.executeRemoteCommand("PUTM", pattern)
	if err != nil {
		return err
	}

	if strings.Contains(result, "Ready to receive") {
		conn := p.getFirstConnection()
		if conn == nil {
			return fmt.Errorf("no active connection")
		}

		fileList := strings.Split(matches, "\n")
		for _, file := range fileList {
			if file == "" {
				continue
			}

			fmt.Printf("Uploading file: %s\n", file)
			fileCmd := &Command{
				Name: "GET",
				Args: []string{file},
			}

			conn.handleGetCommand(fileCmd)
			time.Sleep(1000 * time.Millisecond)
		}
	}

	return nil
}

func (p *CommandParser) handleStatus() error {
	transfers := p.App.GetCurrentTransfers()

	if len(transfers) == 0 {
		fmt.Println("No active transfers")
		return nil
	}

	fmt.Printf("Active transfers: %d\n", len(transfers))
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

		fmt.Printf("[%d] %s %s: %.1f%% complete (%.2f KB/s) - %s\n",
			t.ID, typeStr, t.Name, pct, t.Speed, statusStr)
	}

	return nil
}

func (p *CommandParser) handleMessage(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("MSG requires a message")
	}

	message := strings.Join(args, " ")

	conn := p.getFirstConnection()
	if conn == nil {
		return fmt.Errorf("no active connection")
	}

	msgObj := Message{
		Type: MsgTypeMessage,
		Data: message,
	}

	if err := conn.SendMessage(msgObj); err != nil {
		return fmt.Errorf("failed to send message: %v", err)
	}

	return nil
}

func (p *CommandParser) handlePauseTransfer(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("PAUSE requires a transfer ID")
	}

	id, err := util.ParseInt64(args[0])
	if err != nil {
		return fmt.Errorf("invalid transfer ID: %v", err)
	}

	found := false
	for _, transfer := range p.App.GetTransfers() {
		if transfer.ID == int(id) {
			transfer.Pause()
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("no active transfer with ID %d", id)
	}

	return nil
}

func (p *CommandParser) handleResumeTransfer(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("RESUME requires a transfer ID")
	}

	id, err := util.ParseInt64(args[0])
	if err != nil {
		return fmt.Errorf("invalid transfer ID: %v", err)
	}

	found := false
	for _, transfer := range p.App.GetTransfers() {
		if transfer.ID == int(id) {
			transfer.Resume()
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("no paused transfer with ID %d", id)
	}

	return nil
}

func (p *CommandParser) handleCancelTransfer(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("CANCEL requires a transfer ID")
	}

	id, err := util.ParseInt64(args[0])
	if err != nil {
		return fmt.Errorf("invalid transfer ID: %v", err)
	}

	found := false
	for _, transfer := range p.App.GetTransfers() {
		if transfer.ID == int(id) {
			transfer.Status = TransferStatusFailed
			if transfer.File != nil {
				transfer.File.Close()
				transfer.File = nil
			}
			p.App.RemoveTransfer(transfer)
			found = true
			fmt.Printf("Transfer %d canceled\n", id)
			break
		}
	}

	if !found {
		return fmt.Errorf("no active transfer with ID %d", id)
	}

	return nil
}

func (p *CommandParser) executeLocalCommand(cmdName string, args ...string) (string, error) {
	switch cmdName {
	case "LS":
		path := "."
		if len(args) > 0 {
			path = args[0]
		}

		resolvedPath, err := util.ResolvePath(path, p.App.Config.Folder)
		if err != nil {
			return "", err
		}

		files, err := util.ListFiles(resolvedPath, p.App.Config.Folder, false)
		if err != nil {
			return "", err
		}

		return strings.Join(files, "\n"), nil

	case "FINDM":
		pattern := args[0]
		matches, err := util.FindMatchingFiles(p.App.Config.Folder, pattern)
		if err != nil {
			return "", err
		}

		return strings.Join(matches, "\n"), nil

	default:
		return "", fmt.Errorf("unknown local command: %s", cmdName)
	}
}

type MessageHandler func(messageStr string)

func (p *CommandParser) executeRemoteCommand(cmdName string, args ...string) (string, error) {
	conn := p.getFirstConnection()
	if conn == nil {
		return "", fmt.Errorf("no active connection")
	}

	cmdStr := cmdName
	for _, arg := range args {
		if arg != "" {
			cmdStr += " " + arg
		}
	}

	respChan := make(chan string, 1)
	errChan := make(chan error, 1)

	responseMsgID := fmt.Sprintf("cmd-%d", time.Now().UnixNano())
	conn.RegisterResponseHandler(responseMsgID, func(msg Message) {
		switch msg.Type {
		case MsgTypeCommandResult:
			respChan <- msg.Data
		case MsgTypeError:
			errChan <- fmt.Errorf("remote error: %s", msg.Data)
		}
	})
	defer conn.UnregisterResponseHandler(responseMsgID)

	msg := Message{
		Type: MsgTypeCommand,
		Data: cmdStr,
		ID:   responseMsgID,
	}

	if err := conn.SendReliableMessage(msg); err != nil {
		return "", fmt.Errorf("failed to send command: %v", err)
	}

	select {
	case resp := <-respChan:
		return resp, nil
	case err := <-errChan:
		return "", err
	case <-time.After(10 * time.Second):
		return "", fmt.Errorf("command timed out")
	}
}

func (p *CommandParser) getFirstConnection() *Connection {
	p.App.mu.Lock()
	defer p.App.mu.Unlock()

	for _, conn := range p.App.Connections {
		return conn
	}

	return nil
}
