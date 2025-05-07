package network

import (
	"bufio"
	"fmt"
	"local-file-sharer/internal/util"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
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

	// Print welcome message
	fmt.Println("\n=== P2P File Sharer ===")
	fmt.Printf("Node: %s\n", app.Config.Name)
	fmt.Printf("Folder: %s\n\n", app.Config.Folder)

	// Setup graceful shutdown
	setupGracefulShutdown(app)

	// Start input loop
	scanner := bufio.NewScanner(os.Stdin)

	for app.Ready {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		if err := parser.Execute(input); err != nil {
			log.Error("Command failed: %v", err)
		}
	}
}

func setupGracefulShutdown(app *App) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Println("\nShutting down gracefully...")

		// Close all connections
		app.mu.Lock()
		for _, conn := range app.Connections {
			conn.Conn.Close()
		}
		app.Connections = make(map[string]*Connection)
		app.mu.Unlock()

		// Mark app as not ready
		app.Ready = false

		// Wait a moment for connections to close
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

	var err error

	switch cmdName {
	// Local commands
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

	// Remote commands
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
	default:
		return fmt.Errorf("unknown command: %s", cmdName)
	}

	return err
}

func (p *CommandParser) handleLS(args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	files, err := p.executeLocalCommand("LS", path)
	if err != nil {
		return err
	}

	fmt.Println(files)
	return nil
}

func (p *CommandParser) handleCD(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("CD requires a directory path")
	}

	path := args[0]

	// Security check to prevent escaping shared folder
	baseFolder := p.App.Config.Folder
	absBase, err := filepath.Abs(baseFolder)
	if err != nil {
		return fmt.Errorf("failed to resolve base folder path: %v", err)
	}

	targetPath := filepath.Join(baseFolder, path)
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("failed to resolve target path: %v", err)
	}

	if !strings.HasPrefix(absTarget, absBase) {
		return fmt.Errorf("access denied: path is outside the shared folder")
	}

	// Change to the target directory
	if err := os.Chdir(absTarget); err != nil {
		return err
	}

	// Update the folder config to match
	p.App.Config.Folder = absTarget

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

	// Close all connections
	p.App.mu.Lock()
	for _, conn := range p.App.Connections {
		conn.Conn.Close()
	}
	p.App.Connections = make(map[string]*Connection)
	p.App.mu.Unlock()

	// Wait a moment for connections to close
	time.Sleep(500 * time.Millisecond)

	fmt.Println("Goodbye!")
	os.Exit(0)
	return nil
}

func (p *CommandParser) handleHelp() error {
	help := `
Available Commands:
  Local Commands:
    LS, LIST [path]    - List files in local directory
    CD <path>          - Change local directory
    INFO               - Show information about this node
    HELP               - Show this help message
    QUIT, EXIT         - Exit the application

  Remote Commands:
    LSR, LISTREMOTE [path] - List files in remote directory
    CDR <path>         - Change remote directory
    GET <file>         - Download a file from remote peer
    PUT <file>         - Upload a file to remote peer
    GETDIR [dir]       - Download a directory from remote peer (current dir if omitted)
    PUTDIR [dir]       - Upload a directory to remote peer (current dir if omitted)
    GETM <pattern>     - Download multiple files matching pattern
    PUTM <pattern>     - Upload multiple files matching pattern
    STATUS             - Show active transfers
    MSG <message>      - Send a message to the remote peer
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

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", filePath)
	}

	// Send PUT command
	result, err := p.executeRemoteCommand("PUT", filePath)
	if err != nil {
		return err
	}

	// If remote is ready, start transfer
	if strings.Contains(result, "Ready to receive") {
		conn := p.getFirstConnection()
		if conn == nil {
			return fmt.Errorf("no active connection")
		}

		// Create GET command for file
		getCmd := &Command{
			Name: "GET",
			Args: []string{filePath},
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

	_, err := p.executeRemoteCommand("GETDIR", path)
	return err
}

func (p *CommandParser) handlePutDir(args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Check if directory exists
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("directory not found: %s", path)
	}

	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", path)
	}

	// Send PUTDIR command
	result, err := p.executeRemoteCommand("PUTDIR", path)
	if err != nil {
		return err
	}

	// If remote is ready, start transfer
	if strings.Contains(result, "Ready to receive") {
		// Create a GET command for each file
		conn := p.getFirstConnection()
		if conn == nil {
			return fmt.Errorf("no active connection")
		}

		cmd := &Command{
			Name: "GETDIR",
			Args: []string{path},
		}

		conn.handleGetDirCommand(cmd)
	}

	return nil
}

func (p *CommandParser) handleGetMultiple(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("GETM requires a file pattern")
	}

	_, err := p.executeRemoteCommand("GETM", args[0])
	return err
}

func (p *CommandParser) handlePutMultiple(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("PUTM requires a file pattern")
	}

	pattern := args[0]

	// Find matching files
	matches, err := p.executeLocalCommand("FINDM", pattern)
	if err != nil {
		return err
	}

	if matches == "" {
		return fmt.Errorf("no files match the pattern")
	}

	// Send PUTM command
	result, err := p.executeRemoteCommand("PUTM", pattern)
	if err != nil {
		return err
	}

	// If remote is ready, start transfer
	if strings.Contains(result, "Ready to receive") {
		conn := p.getFirstConnection()
		if conn == nil {
			return fmt.Errorf("no active connection")
		}

		// For each matching file, send it
		for _, file := range strings.Split(matches, "\n") {
			if file == "" {
				continue
			}

			fileCmd := &Command{
				Name: "GET",
				Args: []string{file},
			}

			conn.handleGetCommand(fileCmd)
			time.Sleep(100 * time.Millisecond)
		}
	}

	return nil
}

func (p *CommandParser) handleStatus() error {
	_, err := p.executeRemoteCommand("STATUS", "")
	return err
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

func (p *CommandParser) executeLocalCommand(cmdName string, args ...string) (string, error) {
	switch cmdName {
	case "LS":
		path := "."
		if len(args) > 0 {
			path = args[0]
		}

		files, err := util.ListFiles(path, p.App.Config.Folder, false) // Non-recursive
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

// MessageHandler is a function type for handling messages
type MessageHandler func(messageStr string)

// Modified executeRemoteCommand to use a callback pattern
func (p *CommandParser) executeRemoteCommand(cmdName string, args ...string) (string, error) {
	conn := p.getFirstConnection()
	if conn == nil {
		return "", fmt.Errorf("no active connection")
	}

	// Create command string
	cmdStr := cmdName
	for _, arg := range args {
		if arg != "" {
			cmdStr += " " + arg
		}
	}

	// Create response channel
	respChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Register a message handler for this command response
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

	// Send command
	msg := Message{
		Type: MsgTypeCommand,
		Data: cmdStr,
		ID:   responseMsgID,
	}

	if err := conn.SendMessage(msg); err != nil {
		return "", fmt.Errorf("failed to send command: %v", err)
	}

	// Wait for response or timeout
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
