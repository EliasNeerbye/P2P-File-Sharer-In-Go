# 🌐 Local P2P File Sharing Application

## 📝 Overview

A lightweight, peer-to-peer file sharing tool for local networks. This application allows for easy file transfers between computers on the same network without requiring internet access or external servers.

## ⭐ Key Features

- **Simple Connection Modes**: Run as client, server, or both simultaneously
- **Interactive Command Interface**: Easy-to-use command prompt for file operations
- **Bidirectional Transfers**: Send and receive files in both directions
- **Directory Transfers**: Transfer entire directories with a single command
- **Multiple File Selection**: Transfer multiple files at once
- **Transfer Controls**: Pause, resume, and cancel active transfers
- **Progress Tracking**: Real-time progress bars with speed and ETA
- **Security Controls**: Read-only and write-only modes, path validation
- **Size Limitations**: Configurable maximum file size
- **Concurrency**: Manage multiple simultaneous transfers
- **File Ignore Support**: Use `.p2pignore` files to exclude certain files from transfers

## 🛠️ Tech Stack

| Component             | Technology                     | Details                                                                             |
| --------------------- | ------------------------------ | ----------------------------------------------------------------------------------- |
| Programming Language  | Go                             | Using Go's built-in networking, concurrency, and I/O capabilities                   |
| Command-line Parsing  | Go flag package                | Parses command-line arguments with support for various connection modes             |
| Network Communication | Go net package                 | TCP-based connections with custom protocol for reliable transfers                   |
| File Transfer         | Go bufio & io packages         | Buffered I/O operations for efficient file transfers                                |
| Protocol              | Custom JSON-based messaging    | Message types for commands, file transfers, progress updates, and acknowledgments   |
| Directory Operations  | Go filepath & os packages      | Safe directory traversal with path validation to prevent escaping the shared folder |
| Logging               | Custom logging system          | Color-coded logging with different verbosity levels                                 |
| User Interface        | Terminal-based interactive CLI | Command parser with support for local and remote operations                         |

## 🚩 Command-Line Parameters

| Flag          | Type    | Required | Default           | Description                                                           |
| ------------- | ------- | -------- | ----------------- | --------------------------------------------------------------------- |
| `--ip`        | String  | No       | ""                | 🔌 IP address of the peer to connect to (e.g., `192.168.1.10`)        |
| `--port`      | Integer | No       | `8080`            | 🎯 Target port of the peer                                            |
| `--listen`    | String  | No       | `:8080`           | 👂 Local IP address and port to listen on for incoming connections    |
| `--folder`    | String  | No       | Current directory | 📁 Directory used for sharing files and saving downloads              |
| `--name`      | String  | No       | System hostname   | 🏷️ A friendly identifier for your node                                |
| `--readonly`  | Boolean | No       | false             | 🔒 When enabled, restricts uploads—only downloads are allowed         |
| `--writeonly` | Boolean | No       | false             | 📤 When enabled, restricts downloads—only uploads are permitted       |
| `--maxsize`   | Integer | No       | 0 (Unlimited)     | 📏 Maximum file size in MB allowed for transfer                       |
| `--verify`    | Boolean | No       | true              | ✅ Enables checksum verification to ensure file integrity             |
| `--verbose`   | Boolean | No       | false             | 🔍 Enables detailed logging for network operations and file transfers |

## 💻 Usage Examples

### Starting as a Server (Just Listening)

```
./file-sharer --listen=:8080 --folder=./shared
```

### Starting as a Client (Just Connecting)

```
./file-sharer --ip=192.168.1.10 --port=8080 --folder=./downloads
```

### Starting as Both (Listening and Connecting)

```
./file-sharer --ip=192.168.1.10 --listen=:8080 --folder=./shared
```

### Starting with Restrictions

```
./file-sharer --ip=192.168.1.10 --readonly --maxsize=100
```

## 📋 Available Commands

Once the application is running, you'll see an interactive command prompt. Here are the available commands:

### Local Commands

- `LS [path]` - List files in local directory
- `CD <path>` - Change local directory
- `PWD` - Show current working directory
- `INFO` - Show information about this node
- `HELP` - Show help message with all commands
- `QUIT` or `EXIT` - Exit the application

### Remote Commands

- `LSR [path]` - List files in remote directory
- `CDR <path>` - Change remote directory
- `GET <file>` - Download a file from remote peer
- `PUT <file>` - Upload a file to remote peer
- `GETDIR [dir]` - Download a directory from remote peer
- `PUTDIR [dir]` - Upload a directory to remote peer
- `GETM <file1> <file2> ...` - Download multiple files
- `PUTM <file1> <file2> ...` - Upload multiple files
- `STATUS` - Show active transfers
- `MSG <message>` - Send a message to the remote peer

### Transfer Control

- `PAUSE <id>` - Pause a file transfer
- `RESUME <id>` - Resume a paused transfer
- `CANCEL <id>` - Cancel an active transfer

## 🔧 Configuration

### .p2pignore Files

You can create a `.p2pignore` file in any shared directory to specify files and directories that should not be transferred. The format is similar to `.gitignore`:

```
# Comments start with a hash symbol
*.tmp
*.log
temp/
private_data.txt
```

The application will automatically respect these ignore patterns when listing and transferring files.

## 🗂️ Project Structure

```
local-file-sharer/
├── cmd/
│   └── main.go                # Application entry point
├── internal/
│   ├── config/
│   │   └── config.go          # Command-line flags and configuration
│   ├── network/
│   │   ├── app.go             # Application state management
│   │   ├── client.go          # Client connection initialization
│   │   ├── command.go         # Command parsing and execution
│   │   ├── connection.go      # Connection management and message handling
│   │   ├── protocol.go        # Message protocol definition
│   │   ├── server.go          # Server listener implementation
│   │   └── transfer.go        # File transfer operations
│   └── util/
│       ├── file.go            # File and directory utility functions
│       ├── ignore.go          # Ignore file handling
│       ├── logger.go          # Logging system with colored output
│       └── path.go            # Path manipulation and validation
├── .gitignore                 # Git ignore file
├── LICENSE                    # GNU GPL v3
├── README.md                  # This file
└── go.mod                     # Go module definition
```

## 🔒 Security Considerations

- The application validates all file paths to prevent directory traversal attacks
- Both readonly and writeonly modes allow you to restrict operations
- File size limits can be set to prevent large file transfers
- All connections are authenticated with a simple handshake
- `.p2pignore` files allow you to prevent sensitive files from being shared
- Note that this tool is designed for trusted local networks, not the public internet

## 📈 Transfer Performance

The application includes several features to optimize transfer performance:

- Buffered I/O for efficient reading and writing
- Progress tracking with estimated time of arrival (ETA)
- Speed calculations based on a weighted average for more stable readings
- Automatic file naming to handle duplicate files
- Pause and resume functionality for long transfers

## 📄 License

This project is licensed under the GNU General Public License v3 - see the LICENSE file for details.
