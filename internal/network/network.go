package network

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"local-file-sharer/cmd/sharego/app"
	"local-file-sharer/internal/transfer"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

type ClientState struct {
	RemoteWorkingDir string
	LocalWorkingDir  string
}

func StartDial(app *app.App) {
	log := app.Logger
	conn, err := net.Dial("tcp", app.Config.TargetAddr)
	if err != nil {
		log.Fatal("Failed to dial peer: %v", err)
		return
	}
	defer conn.Close()

	log.Success("Connected to peer: %s", conn.RemoteAddr())

	sendCommand(conn, &transfer.Command{
		Action: "HELLO",
		Data:   app.Config.Name,
	})

	clientState := &ClientState{
		RemoteWorkingDir: "",
		LocalWorkingDir:  "",
	}

	interactWithServer(app, conn, clientState)
}

func StartListening(app *app.App) {
	log := app.Logger
	ln, err := net.Listen("tcp", app.Config.ListenAddr)
	if err != nil {
		log.Fatal("Failed to start listener: %v", err)
		return
	}
	defer ln.Close()

	log.Success("Listening on %s", ln.Addr())

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signals
		log.Warn("Shutting down listener...")
		ln.Close()
		os.Exit(0)
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				log.Error("Temporary accept error: %v", err)
				continue
			}
			log.Error("Accept error: %v", err)
			return
		}
		go handleConnection(app, conn)
	}
}

func handleConnection(app *app.App, conn net.Conn) {
	log := app.Logger
	defer conn.Close()

	log.Info("New connection from: %s", conn.RemoteAddr())

	cmd, err := receiveCommand(conn)
	if err != nil {
		log.Error("Failed to receive hello command: %v", err)
		return
	}

	if cmd.Action != "HELLO" {
		log.Error("Expected HELLO command, got %s", cmd.Action)
		sendCommand(conn, &transfer.Command{
			Action:  "ERROR",
			Success: false,
			Message: "Expected HELLO command",
		})
		return
	}

	clientName := "Unknown"
	if name, ok := cmd.Data.(string); ok {
		clientName = name
	}

	log.Info("Client identified as: %s", clientName)

	sendCommand(conn, &transfer.Command{
		Action:  "HELLO",
		Data:    app.Config.Name,
		Success: true,
	})

	// Each client gets their own working directory starting at the root
	clientWorkingDir := ""

	for {
		cmd, err := receiveCommand(conn)
		if err != nil {
			if err == io.EOF {
				log.Info("Client %s disconnected", clientName)
			} else {
				log.Error("Error receiving command from %s: %v", clientName, err)
			}
			return
		}

		log.Debug("Received command: %s, path: %s", cmd.Action, cmd.Path)

		handleClientCommand(app, conn, cmd, clientName, &clientWorkingDir)
	}
}

func interactWithServer(app *app.App, conn net.Conn, state *ClientState) {
	log := app.Logger

	cmd, err := receiveCommand(conn)
	if err != nil {
		log.Error("Failed to receive server hello: %v", err)
		return
	}

	if cmd.Action != "HELLO" {
		log.Error("Expected HELLO response, got %s", cmd.Action)
		return
	}

	serverName := "Unknown"
	if name, ok := cmd.Data.(string); ok {
		serverName = name
	}

	log.Success("Connected to server: %s", serverName)

	showClientHelp()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		// Show current working directories in prompt
		fmt.Printf("Local: %s, Remote: %s > ",
			formatCurrentDir(state.LocalWorkingDir),
			formatCurrentDir(state.RemoteWorkingDir))

		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		parts := strings.Fields(input)
		if len(parts) == 0 {
			continue
		}

		action := strings.ToUpper(parts[0])

		switch action {
		case "EXIT", "QUIT":
			log.Info("Disconnecting from server...")
			return

		case "HELP":
			showClientHelp()

		case "LIST", "LS":
			var target string
			var path string

			// Default to current directory
			if len(parts) < 2 {
				target = "remote"
				path = state.RemoteWorkingDir
			} else if len(parts) < 3 {
				// Check if the argument is "local" or "remote" or a path
				if strings.ToLower(parts[1]) == "local" {
					target = "local"
					path = state.LocalWorkingDir
				} else if strings.ToLower(parts[1]) == "remote" {
					target = "remote"
					path = state.RemoteWorkingDir
				} else {
					// Assume it's a path for remote listing
					target = "remote"
					path = combinePath(state.RemoteWorkingDir, parts[1])
				}
			} else {
				target = strings.ToLower(parts[1])
				workingDir := state.RemoteWorkingDir
				if target == "local" {
					workingDir = state.LocalWorkingDir
				}
				path = combinePath(workingDir, parts[2])
			}

			if target == "local" {
				// List local directory
				files, err := transfer.ListFiles(app.Config.Folder, path)
				if err != nil {
					log.Error("Failed to list local directory: %v", err)
					continue
				}

				displayFileList(files, "Local")
			} else {
				// List remote directory
				sendCommand(conn, &transfer.Command{
					Action: "LIST",
					Path:   path,
				})

				handleListResponse(app, conn)
			}

		case "CD":
			var target string
			var path string

			if len(parts) < 2 {
				log.Error("Usage: CD [local|remote] <directory>")
				continue
			} else if len(parts) < 3 {
				// Check if the argument is "local" or "remote" or a path
				if strings.ToLower(parts[1]) == "local" {
					log.Error("Usage: CD local <directory>")
					continue
				} else if strings.ToLower(parts[1]) == "remote" {
					log.Error("Usage: CD remote <directory>")
					continue
				} else {
					// Assume it's a remote path by default
					target = "remote"
					path = parts[1]
				}
			} else {
				target = strings.ToLower(parts[1])
				path = parts[2]
			}

			if target == "local" {
				// Change local directory
				newPath := combinePath(state.LocalWorkingDir, path)
				fullPath := filepath.Join(app.Config.Folder, newPath)

				if !transfer.IsPathSafe(app.Config.Folder, fullPath) {
					log.Error("Path traversal detected, access denied")
					continue
				}

				fileInfo, err := os.Stat(fullPath)
				if err != nil {
					log.Error("Failed to access directory: %v", err)
					continue
				}

				if !fileInfo.IsDir() {
					log.Error("Not a directory: %s", path)
					continue
				}

				state.LocalWorkingDir = newPath
				log.Info("Changed local directory to: %s", formatCurrentDir(state.LocalWorkingDir))
			} else {
				// Change remote directory
				newPath := combinePath(state.RemoteWorkingDir, path)

				sendCommand(conn, &transfer.Command{
					Action: "CD",
					Path:   newPath,
				})

				cmd, err := receiveCommand(conn)
				if err != nil {
					log.Error("Failed to receive CD response: %v", err)
					continue
				}

				if !cmd.Success {
					log.Error("Failed to change remote directory: %s", cmd.Message)
					continue
				}

				state.RemoteWorkingDir = newPath
				log.Info("Changed remote directory to: %s", formatCurrentDir(state.RemoteWorkingDir))
			}

		case "GET":
			if len(parts) < 2 {
				log.Error("Usage: GET <filename> [local_filename]")
				continue
			}

			remotePath := combinePath(state.RemoteWorkingDir, parts[1])

			localFilename := filepath.Base(parts[1])
			if len(parts) >= 3 {
				localFilename = parts[2]
			}

			localPath := combinePath(state.LocalWorkingDir, localFilename)

			handleGetCommand(app, conn, remotePath, localPath)

		case "PUT":
			if len(parts) < 2 {
				log.Error("Usage: PUT <filename> [remote_filename]")
				continue
			}

			localPath := combinePath(state.LocalWorkingDir, parts[1])

			remoteFilename := filepath.Base(parts[1])
			if len(parts) >= 3 {
				remoteFilename = parts[2]
			}

			remotePath := combinePath(state.RemoteWorkingDir, remoteFilename)

			handlePutCommand(app, conn, localPath, remotePath)

		case "INFO":
			if len(parts) < 2 {
				log.Error("Usage: INFO [local|remote] <filename>")
				continue
			}

			var target string
			var path string

			if len(parts) < 3 {
				// Check if the argument is "local" or "remote" or a path
				if strings.ToLower(parts[1]) == "local" {
					log.Error("Usage: INFO local <filename>")
					continue
				} else if strings.ToLower(parts[1]) == "remote" {
					log.Error("Usage: INFO remote <filename>")
					continue
				} else {
					// Assume it's a remote path by default
					target = "remote"
					path = combinePath(state.RemoteWorkingDir, parts[1])
				}
			} else {
				target = strings.ToLower(parts[1])
				if target == "local" {
					path = combinePath(state.LocalWorkingDir, parts[2])
				} else {
					path = combinePath(state.RemoteWorkingDir, parts[2])
				}
			}

			if target == "local" {
				// Get local file info
				fileInfo, err := transfer.GetFileInfo(app.Config.Folder, path)
				if err != nil {
					log.Error("Failed to get file info: %v", err)
					continue
				}

				displayFileInfo(fileInfo, "Local")
			} else {
				// Get remote file info
				sendCommand(conn, &transfer.Command{
					Action: "INFO",
					Path:   path,
				})

				handleInfoResponse(app, conn)
			}

		case "PWD":
			if len(parts) >= 2 && strings.ToLower(parts[1]) == "local" {
				log.Info("Local working directory: %s", formatCurrentDir(state.LocalWorkingDir))
			} else if len(parts) >= 2 && strings.ToLower(parts[1]) == "remote" {
				log.Info("Remote working directory: %s", formatCurrentDir(state.RemoteWorkingDir))
			} else {
				log.Info("Local working directory: %s", formatCurrentDir(state.LocalWorkingDir))
				log.Info("Remote working directory: %s", formatCurrentDir(state.RemoteWorkingDir))
			}

		default:
			log.Error("Unknown command: %s", action)
			showClientHelp()
		}
	}
}

func handleClientCommand(app *app.App, conn net.Conn, cmd *transfer.Command, clientName string, clientWorkingDir *string) {
	log := app.Logger
	cfg := app.Config

	switch cmd.Action {
	case "LIST":
		if cfg.WriteOnly {
			sendCommand(conn, &transfer.Command{
				Action:  "ERROR",
				Success: false,
				Message: "This server is in write-only mode, listing files is not allowed",
			})
			return
		}

		// Combine the client working directory with the requested path
		requestPath := *clientWorkingDir
		if cmd.Path != "" {
			requestPath = combinePath(*clientWorkingDir, cmd.Path)
		}

		files, err := transfer.ListFiles(cfg.Folder, requestPath)
		if err != nil {
			log.Error("Failed to list files: %v", err)
			sendCommand(conn, &transfer.Command{
				Action:  "ERROR",
				Success: false,
				Message: fmt.Sprintf("Failed to list files: %v", err),
			})
			return
		}

		sendCommand(conn, &transfer.Command{
			Action:  "LIST",
			Success: true,
			Data:    files,
		})

	case "CD":
		if cfg.WriteOnly {
			sendCommand(conn, &transfer.Command{
				Action:  "ERROR",
				Success: false,
				Message: "This server is in write-only mode, changing directory is not allowed",
			})
			return
		}

		requestPath := cmd.Path

		// Verify the path is valid and is a directory
		fullPath := filepath.Join(cfg.Folder, requestPath)

		if !transfer.IsPathSafe(cfg.Folder, fullPath) {
			sendCommand(conn, &transfer.Command{
				Action:  "ERROR",
				Success: false,
				Message: "Path traversal detected, access denied",
			})
			return
		}

		fileInfo, err := os.Stat(fullPath)
		if err != nil {
			log.Error("Failed to access directory: %v", err)
			sendCommand(conn, &transfer.Command{
				Action:  "ERROR",
				Success: false,
				Message: fmt.Sprintf("Failed to access directory: %v", err),
			})
			return
		}

		if !fileInfo.IsDir() {
			sendCommand(conn, &transfer.Command{
				Action:  "ERROR",
				Success: false,
				Message: "Not a directory",
			})
			return
		}

		// Update the client's working directory
		*clientWorkingDir = requestPath

		sendCommand(conn, &transfer.Command{
			Action:  "CD",
			Success: true,
			Message: fmt.Sprintf("Changed directory to: %s", formatCurrentDir(requestPath)),
		})

	case "INFO":
		if cfg.WriteOnly {
			sendCommand(conn, &transfer.Command{
				Action:  "ERROR",
				Success: false,
				Message: "This server is in write-only mode, getting file info is not allowed",
			})
			return
		}

		// Combine the client working directory with the requested path
		requestPath := combinePath(*clientWorkingDir, cmd.Path)

		fileInfo, err := transfer.GetFileInfo(cfg.Folder, requestPath)
		if err != nil {
			log.Error("Failed to get file info: %v", err)
			sendCommand(conn, &transfer.Command{
				Action:  "ERROR",
				Success: false,
				Message: fmt.Sprintf("Failed to get file info: %v", err),
			})
			return
		}

		sendCommand(conn, &transfer.Command{
			Action:  "INFO",
			Success: true,
			Data:    fileInfo,
		})

	case "GET":
		if cfg.WriteOnly {
			sendCommand(conn, &transfer.Command{
				Action:  "ERROR",
				Success: false,
				Message: "This server is in write-only mode, downloading files is not allowed",
			})
			return
		}

		// Combine the client working directory with the requested path
		requestPath := combinePath(*clientWorkingDir, cmd.Path)

		fileInfo, err := transfer.GetFileInfo(cfg.Folder, requestPath)
		if err != nil {
			log.Error("Failed to get file info: %v", err)
			sendCommand(conn, &transfer.Command{
				Action:  "ERROR",
				Success: false,
				Message: fmt.Sprintf("Failed to access file: %v", err),
			})
			return
		}

		if fileInfo.IsDir {
			sendCommand(conn, &transfer.Command{
				Action:  "ERROR",
				Success: false,
				Message: "Cannot download a directory",
			})
			return
		}

		if cfg.MaxSize > 0 && fileInfo.Size > int64(cfg.MaxSize*1024*1024) {
			sendCommand(conn, &transfer.Command{
				Action:  "ERROR",
				Success: false,
				Message: fmt.Sprintf("File size exceeds maximum allowed size of %d MB", cfg.MaxSize),
			})
			return
		}

		checksum := ""
		if cfg.Verify {
			checksum = fileInfo.Checksum
		}

		sendCommand(conn, &transfer.Command{
			Action:  "GET",
			Success: true,
			Data: transfer.FileInfo{
				Name:     fileInfo.Name,
				Size:     fileInfo.Size,
				Checksum: checksum,
			},
		})

		log.Info("Sending file %s to %s", requestPath, clientName)
		err = transfer.SendFile(cfg.Folder, requestPath, conn, cfg.MaxSize)
		if err != nil {
			log.Error("Failed to send file: %v", err)
			return
		}

		log.Success("File %s sent successfully to %s", requestPath, clientName)

	case "PUT":
		if cfg.ReadOnly {
			sendCommand(conn, &transfer.Command{
				Action:  "ERROR",
				Success: false,
				Message: "This server is in read-only mode, uploading files is not allowed",
			})
			return
		}

		var fileInfo transfer.FileInfo
		fileInfoData, err := json.Marshal(cmd.Data)
		if err != nil {
			log.Error("Failed to marshal file info: %v", err)
			sendCommand(conn, &transfer.Command{
				Action:  "ERROR",
				Success: false,
				Message: "Invalid file info",
			})
			return
		}

		if err := json.Unmarshal(fileInfoData, &fileInfo); err != nil {
			log.Error("Failed to unmarshal file info: %v", err)
			sendCommand(conn, &transfer.Command{
				Action:  "ERROR",
				Success: false,
				Message: "Invalid file info",
			})
			return
		}

		if cfg.MaxSize > 0 && fileInfo.Size > int64(cfg.MaxSize*1024*1024) {
			sendCommand(conn, &transfer.Command{
				Action:  "ERROR",
				Success: false,
				Message: fmt.Sprintf("File size exceeds maximum allowed size of %d MB", cfg.MaxSize),
			})
			return
		}

		// Combine the client working directory with the requested path
		requestPath := combinePath(*clientWorkingDir, cmd.Path)

		sendCommand(conn, &transfer.Command{
			Action:  "PUT",
			Success: true,
			Message: "Ready to receive file",
		})

		log.Info("Receiving file %s from %s", requestPath, clientName)
		err = transfer.ReceiveFile(cfg.Folder, requestPath, conn, fileInfo.Size, cfg.MaxSize)
		if err != nil {
			log.Error("Failed to receive file: %v", err)
			return
		}

		if cfg.Verify && fileInfo.Checksum != "" {
			match, err := transfer.VerifyChecksum(cfg.Folder, requestPath, fileInfo.Checksum)
			if err != nil {
				log.Error("Failed to verify checksum: %v", err)
			} else if !match {
				log.Error("Checksum verification failed for %s", requestPath)
			} else {
				log.Info("Checksum verification successful for %s", requestPath)
			}
		}

		log.Success("File %s received successfully from %s", requestPath, clientName)

	default:
		log.Error("Unknown command from %s: %s", clientName, cmd.Action)
		sendCommand(conn, &transfer.Command{
			Action:  "ERROR",
			Success: false,
			Message: fmt.Sprintf("Unknown command: %s", cmd.Action),
		})
	}
}

func handleListResponse(app *app.App, conn net.Conn) {
	log := app.Logger

	cmd, err := receiveCommand(conn)
	if err != nil {
		log.Error("Failed to receive LIST response: %v", err)
		return
	}

	if !cmd.Success {
		log.Error("LIST command failed: %s", cmd.Message)
		return
	}

	var files []transfer.FileInfo
	fileData, err := json.Marshal(cmd.Data)
	if err != nil {
		log.Error("Failed to marshal file list: %v", err)
		return
	}

	if err := json.Unmarshal(fileData, &files); err != nil {
		log.Error("Failed to unmarshal file list: %v", err)
		return
	}

	displayFileList(files, "Remote")
}

func displayFileList(files []transfer.FileInfo, label string) {
	fmt.Printf("\n%s files:\n", label)
	fmt.Println("------------------------------------------------------------")
	fmt.Printf("%-30s %12s  %s\n", "Name", "Size", "Type")
	fmt.Println("------------------------------------------------------------")
	for _, file := range files {
		fileType := "File"
		if file.IsDir {
			fileType = "Dir"
		}

		// Format size with units
		sizeStr := ""
		if file.IsDir {
			sizeStr = "-"
		} else if file.Size < 1024 {
			sizeStr = fmt.Sprintf("%d B", file.Size)
		} else if file.Size < 1024*1024 {
			sizeStr = fmt.Sprintf("%.2f KB", float64(file.Size)/1024)
		} else if file.Size < 1024*1024*1024 {
			sizeStr = fmt.Sprintf("%.2f MB", float64(file.Size)/(1024*1024))
		} else {
			sizeStr = fmt.Sprintf("%.2f GB", float64(file.Size)/(1024*1024*1024))
		}

		fmt.Printf("%-30s %12s  [%s]\n", file.Name, sizeStr, fileType)
	}
	fmt.Println("------------------------------------------------------------")
}

func handleInfoResponse(app *app.App, conn net.Conn) {
	log := app.Logger

	cmd, err := receiveCommand(conn)
	if err != nil {
		log.Error("Failed to receive INFO response: %v", err)
		return
	}

	if !cmd.Success {
		log.Error("INFO command failed: %s", cmd.Message)
		return
	}

	var fileInfo transfer.FileInfo
	infoData, err := json.Marshal(cmd.Data)
	if err != nil {
		log.Error("Failed to marshal file info: %v", err)
		return
	}

	if err := json.Unmarshal(infoData, &fileInfo); err != nil {
		log.Error("Failed to unmarshal file info: %v", err)
		return
	}

	displayFileInfo(&fileInfo, "Remote")
}

func displayFileInfo(fileInfo *transfer.FileInfo, label string) {
	fmt.Printf("\n%s File Information:\n", label)
	fmt.Println("------------------------------")
	fmt.Printf("Name:     %s\n", fileInfo.Name)

	// Format size with units
	if fileInfo.IsDir {
		fmt.Printf("Size:     -\n")
	} else if fileInfo.Size < 1024 {
		fmt.Printf("Size:     %d B\n", fileInfo.Size)
	} else if fileInfo.Size < 1024*1024 {
		fmt.Printf("Size:     %.2f KB\n", float64(fileInfo.Size)/1024)
	} else if fileInfo.Size < 1024*1024*1024 {
		fmt.Printf("Size:     %.2f MB\n", float64(fileInfo.Size)/(1024*1024))
	} else {
		fmt.Printf("Size:     %.2f GB\n", float64(fileInfo.Size)/(1024*1024*1024))
	}

	fmt.Printf("Type:     %s\n", map[bool]string{true: "Directory", false: "File"}[fileInfo.IsDir])
	fmt.Printf("Mode:     %s\n", fileInfo.Mode)
	if fileInfo.Checksum != "" {
		fmt.Printf("Checksum: %s\n", fileInfo.Checksum)
	}
	fmt.Println("------------------------------")
}

func handleGetCommand(app *app.App, conn net.Conn, remotePath, localPath string) {
	log := app.Logger

	if app.Config.WriteOnly {
		log.Error("This client is in write-only mode, downloading files is not allowed")
		return
	}

	sendCommand(conn, &transfer.Command{
		Action: "GET",
		Path:   remotePath,
	})

	cmd, err := receiveCommand(conn)
	if err != nil {
		log.Error("Failed to receive GET response: %v", err)
		return
	}

	if !cmd.Success {
		log.Error("GET command failed: %s", cmd.Message)
		return
	}

	var fileInfo transfer.FileInfo
	infoData, err := json.Marshal(cmd.Data)
	if err != nil {
		log.Error("Failed to marshal file info: %v", err)
		return
	}

	if err := json.Unmarshal(infoData, &fileInfo); err != nil {
		log.Error("Failed to unmarshal file info: %v", err)
		return
	}

	log.Info("Receiving file %s (%d bytes)", fileInfo.Name, fileInfo.Size)

	err = transfer.ReceiveFile(app.Config.Folder, localPath, conn, fileInfo.Size, app.Config.MaxSize)
	if err != nil {
		log.Error("Failed to receive file: %v", err)
		return
	}

	if app.Config.Verify && fileInfo.Checksum != "" {
		match, err := transfer.VerifyChecksum(app.Config.Folder, localPath, fileInfo.Checksum)
		if err != nil {
			log.Error("Failed to verify checksum: %v", err)
		} else if !match {
			log.Error("Checksum verification failed for %s", localPath)
		} else {
			log.Info("Checksum verification successful for %s", localPath)
		}
	}

	log.Success("File %s received successfully", localPath)
}

func handlePutCommand(app *app.App, conn net.Conn, localPath, remotePath string) {
	log := app.Logger

	if app.Config.ReadOnly {
		log.Error("This client is in read-only mode, uploading files is not allowed")
		return
	}

	fileInfo, err := transfer.GetFileInfo(app.Config.Folder, localPath)
	if err != nil {
		log.Error("Failed to access file: %v", err)
		return
	}

	if fileInfo.IsDir {
		log.Error("Cannot upload a directory")
		return
	}

	if app.Config.MaxSize > 0 && fileInfo.Size > int64(app.Config.MaxSize*1024*1024) {
		log.Error("File size exceeds maximum allowed size of %d MB", app.Config.MaxSize)
		return
	}

	sendCommand(conn, &transfer.Command{
		Action: "PUT",
		Path:   remotePath,
		Data:   fileInfo,
	})

	cmd, err := receiveCommand(conn)
	if err != nil {
		log.Error("Failed to receive PUT response: %v", err)
		return
	}

	if !cmd.Success {
		log.Error("PUT command failed: %s", cmd.Message)
		return
	}

	log.Info("Sending file %s (%d bytes)", fileInfo.Name, fileInfo.Size)
	err = transfer.SendFile(app.Config.Folder, localPath, conn, app.Config.MaxSize)
	if err != nil {
		log.Error("Failed to send file: %v", err)
		return
	}

	log.Success("File %s sent successfully", fileInfo.Name)
}

func showClientHelp() {
	fmt.Println("\nAvailable commands:")
	fmt.Println("------------------------------------------------------------")
	fmt.Println("LS, LIST [local|remote] [path]  - List files in directory")
	fmt.Println("CD [local|remote] <path>        - Change current directory")
	fmt.Println("PWD [local|remote]              - Show current directory")
	fmt.Println("GET <filename> [local_filename] - Download a file")
	fmt.Println("PUT <filename> [remote_filename]- Upload a file")
	fmt.Println("INFO [local|remote] <filename>  - Show information about a file")
	fmt.Println("HELP                           - Show this help")
	fmt.Println("EXIT, QUIT                     - Disconnect and exit")
	fmt.Println("------------------------------------------------------------")
}

func formatCurrentDir(dir string) string {
	if dir == "" {
		return "/"
	}
	return "/" + dir
}

func combinePath(basePath, relativePath string) string {
	// Handle special case for root directory
	if relativePath == "/" {
		return ""
	}

	// Handle absolute paths (starting with /)
	if strings.HasPrefix(relativePath, "/") {
		// Remove leading slash and return
		return strings.TrimPrefix(relativePath, "/")
	}

	// For empty base path, just return the relative path
	if basePath == "" {
		return relativePath
	}

	// Otherwise combine the paths
	return filepath.Join(basePath, relativePath)
}

func sendCommand(conn net.Conn, cmd *transfer.Command) error {
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	data = append(data, '\n')

	_, err = conn.Write(data)
	if err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}

	return nil
}

func receiveCommand(conn net.Conn) (*transfer.Command, error) {
	reader := bufio.NewReader(conn)
	data, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	var cmd transfer.Command
	if err := json.Unmarshal(data[:len(data)-1], &cmd); err != nil {
		return nil, fmt.Errorf("failed to unmarshal command: %w", err)
	}

	return &cmd, nil
}
