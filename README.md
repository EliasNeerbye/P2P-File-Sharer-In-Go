# 🌐 Local P2P File Sharing Application Project Plan 🌐

## 📝 Summary
This project aims to create a lightweight, efficient peer-to-peer file sharing application that operates exclusively on local networks. The application follows a mutual connection model where both users must explicitly specify each other's IP addresses to establish a connection. This design emphasizes user control and intentional sharing rather than automatic discovery. Once connected, users can share files according to configurable permission settings. The application focuses on simplicity, efficiency, and minimal dependencies while providing a secure and user-controlled file sharing experience.

## 🛠️ Tech Stack

| Component | Technology | Justification |
|-----------|------------|---------------|
| Programming Language | Go | Go's standard library has excellent networking support, built-in concurrency with goroutines, and produces standalone binaries that are easy to distribute. |
| Command-line Parsing | Go flag package | Built-in package for parsing command-line arguments without external dependencies. |
| Network Communication | Go net package | The standard library's net package provides all necessary TCP/UDP functionality without external dependencies. |
| Connection Establishment | Custom TCP handshake using net.Conn | Implements mutual verification where both parties must request the connection. Uses Go's net.Conn interface for reliable TCP communication. |
| File Transfer | Custom protocol with bufio package | Built on top of Go's bufio for efficient buffered I/O during file transfers. Utilizes io.Copy for optimized streaming. |
| Data Serialization | Go encoding/json package | Standard library JSON for message formats. Structures will define the protocol messages. |
| CLI Interface | Go fmt package & bufio.Scanner | Simple command-line interface using standard I/O capabilities and interactive prompts. |
| File System Access | Go os and io packages | Standard library packages for file operations with error handling. |
| Checksumming | crypto/md5 or crypto/sha256 | For file verification using standard cryptographic hash functions. |

## 🚩 Command Line Parameters

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--ip` | String | Yes | N/A | 🔌 IP address and port of the peer to connect to (format: `192.168.1.10:8080`). This is the address of the other user you want to connect with. |
| `--listen` | String | No | First available IP:8080 | 👂 IP address and port to listen on for incoming connections. If not specified, automatically binds to the machine's primary IP on port 8080. |
| `--folder` | String | No | Current directory | 📁 Directory to share files from and download files to. Defaults to the current working directory if not specified. |
| `--name` | String | No | System hostname | 🏷️ A friendly name to identify your node to other peers. Helps users confirm they're connecting to the right person. |
| `--readonly` | Boolean | No | false | 🔒 When set, prevents other peers from uploading files to your shared folder. Only allows them to download from you. |
| `--writeonly` | Boolean | No | false | 📤 When set, prevents you from downloading files from peers. Only allows you to upload files to them (if they permit it). |
| `--maxsize` | Integer | No | Unlimited | 📏 Maximum file size in MB that you're willing to transfer. Protects against extremely large transfers. |
| `--verify` | Boolean | No | true | ✅ When enabled, performs checksum verification on transferred files to ensure integrity. |
| `--verbose` | Boolean | No | false | 🔍 Enables detailed logging of network operations and file transfers for debugging. |

## 🗺️ Roadmap

1. **🎮 Command Line Interface Setup**
   - Implement parameter parsing with the flag package
   - Create handlers for all command flags
   - Add IP validation and listening address setup
   - Implement folder path validation and permissions checking
   - Set up logging framework with verbose mode support

2. **🤝 Connection Establishment System**
   - Implement TCP listener using net.Listen() on specified or default address
   - Create connection request mechanism using net.Dial() to specified peer IP
   - Develop mutual verification protocol where both sides must request connection
   - Use channels and goroutines to manage concurrent connections
   - Build user notification system for connection status

3. **📋 Basic Protocol Design**
   - Design JSON message structs for peer communication
   - Implement protocol handlers using type switches and interfaces
   - Create authentication/verification message exchange
   - Build file listing mechanism with metadata using os.ReadDir
   - Implement permission checking for readonly/writeonly modes

4. **📦 File Transfer Functionality**
   - Implement file chunking for efficient transfers using bufio.Reader/Writer
   - Create progress tracking with periodic updates using channels
   - Build integrity verification using crypto/md5 or crypto/sha256
   - Implement maximum file size enforcement with io.LimitReader
   - Develop error handling with defer statements and recover()

5. **👨‍💻 User Interface and Experience**
   - Develop interactive command prompts using bufio.Scanner
   - Implement clear connection status indicators with color coding
   - Create interface for browsing available files with pagination
   - Add download/upload interface with progress bars
   - Build graceful shutdown with signal.Notify for handling SIGINT

6. **🧪 Testing and Optimization**
   - Test transfers of different file sizes
   - Benchmark transfer speeds
   - Optimize buffer sizes and concurrent transfers with sync.WaitGroup
   - Test edge cases using controlled failure scenarios
   - Verify functionality of all command-line parameters

7. **📚 Documentation and Refinement**
   - Create user documentation including all command options
   - Add detailed comments to code following Go conventions
   - Create sample usage scenarios with example commands
   - Implement any remaining features
   - Conduct final testing across different devices and networks
  

### Folder Structure
```md
sharego/
├── cmd/
│   └── sharego/
│       └── main.go              # Entry point, command-line flags setup
│
├── internal/
│   ├── config/
│   │   └── config.go            # Configuration and flag handling
│   ├── network/
│   │   ├── connection.go        # Connection establishment
│   │   ├── handshake.go         # Mutual verification protocol
│   │   ├── listener.go          # Network listener
│   │   └── message.go           # Protocol message definitions
│   ├── transfer/
│   │   ├── file.go              # File operations
│   │   ├── download.go          # Download handling
│   │   ├── upload.go            # Upload handling
│   │   └── verify.go            # Checksum verification
│   ├── ui/
│   │   ├── prompt.go            # Interactive user prompts
│   │   └── progress.go          # Progress display
│   └── util/
│       ├── logger.go            # Logging utilities
│       └── fileutil.go          # File utility functions
│
├── pkg/
│   └── protocol/
│       ├── messages.go          # Protocol message definitions
│       └── constants.go         # Protocol constants
│
├── .gitignore
├── go.mod
├── go.sum
├── README.md
└── LICENSE
```
