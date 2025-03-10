# Local P2P File Sharing Application Project Plan

## Summary
This project aims to create a lightweight, efficient peer-to-peer file sharing application that operates exclusively on local networks. The application follows a mutual connection model where both users must explicitly specify each other's IP addresses to establish a connection. This design emphasizes user control and intentional sharing rather than automatic discovery. Once connected, users can share files according to configurable permission settings. The application focuses on simplicity, efficiency, and minimal dependencies while providing a secure and user-controlled file sharing experience.

## Tech Stack

| Component | Technology | Justification |
|-----------|------------|---------------|
| Programming Language | Go | Go's standard library has excellent networking support, built-in concurrency with goroutines, and produces standalone binaries that are easy to distribute. |
| Command-line Parsing | Go flag package | Built-in package for parsing command-line arguments without external dependencies. |
| Network Communication | Go net package | The standard library's net package provides all necessary TCP/UDP functionality without external dependencies. |
| Connection Establishment | Custom TCP handshake | Implements mutual verification where both parties must request the connection for it to be established. |
| File Transfer | Custom TCP protocol | Implementing a basic protocol over TCP for reliable file transfers using Go's built-in functionality. |
| Data Serialization | Go encoding/json | Standard library JSON for message formats, avoiding external dependencies. |
| CLI Interface | Go fmt package | Simple command-line interface using standard I/O capabilities. |
| File System Access | Go os and io packages | Standard library packages for file operations. |

## Command Line Parameters

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--ip` | String | Yes | N/A | IP address and port of the peer to connect to (format: `192.168.1.10:8080`). This is the address of the other user you want to connect with. |
| `--listen` | String | No | First available IP:8080 | IP address and port to listen on for incoming connections (format: `192.168.1.10:8080`). If not specified, automatically binds to the machine's primary IP on port 8080. |
| `--folder` | String | No | Current directory | Directory to share files from and download files to. Defaults to the current working directory if not specified. |
| `--name` | String | No | System hostname | A friendly name to identify your node to other peers. Helps users confirm they're connecting to the right person. |
| `--readonly` | Boolean | No | false | When set, prevents other peers from uploading files to your shared folder. Only allows them to download from you. |
| `--writeonly` | Boolean | No | false | When set, prevents you from downloading files from peers. Only allows you to upload files to them (if they permit it). |
| `--maxsize` | Integer | No | Unlimited | Maximum file size in MB that you're willing to transfer. Protects against extremely large transfers. |
| `--verify` | Boolean | No | true | When enabled, performs checksum verification on transferred files to ensure integrity. |
| `--verbose` | Boolean | No | false | Enables detailed logging of network operations and file transfers for debugging. |

## Roadmap

1. **Command Line Interface Setup** (Week 1)
   - Implement parameter parsing with the flag package
   - Create handlers for all command flags
   - Add IP validation and listening address setup
   - Implement folder path validation and permissions checking
   - Set up logging framework with verbose mode support

2. **Connection Establishment System** (Week 1-2)
   - Implement TCP listener on specified or default address
   - Create connection request mechanism to specified peer IP
   - Develop mutual verification protocol where both sides must request connection
   - Implement connection state management
   - Build user notification system for connection status

3. **Basic Protocol Design** (Week 2-3)
   - Design message formats for peer communication
   - Implement protocol handlers for different message types
   - Create authentication/verification message exchange
   - Build file listing mechanism with metadata
   - Implement permission checking for readonly/writeonly modes

4. **File Transfer Functionality** (Week 3-4)
   - Implement file chunking for efficient transfers
   - Create progress tracking and display system
   - Build integrity verification using checksums
   - Implement maximum file size enforcement
   - Develop error handling and recovery for failed transfers

5. **User Interface and Experience** (Week 4-5)
   - Develop interactive command prompts for user interaction
   - Implement clear connection status indicators
   - Create interface for browsing available files
   - Add download/upload interface with progress display
   - Build graceful connection termination with active transfer warnings

6. **Testing and Optimization** (Week 5)
   - Test transfers of different file sizes
   - Benchmark transfer speeds
   - Optimize chunk sizes and concurrent transfers
   - Test edge cases (sudden disconnection, permission conflicts)
   - Verify functionality of all command-line parameters

7. **Documentation and Refinement** (Week 6)
   - Create user documentation including all command options
   - Add detailed comments to code
   - Create sample usage scenarios with example commands
   - Implement any remaining features
   - Final testing across different devices and networks
