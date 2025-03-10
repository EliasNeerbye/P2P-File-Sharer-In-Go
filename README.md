# 🌐 Local P2P File Sharing Application

## 📝 Overview

This project is a lightweight, peer-to-peer file sharing tool that works on local networks.

## 🛠️ Tech Stack

| Component                | Technology                           | Justification                                                                                   |
|--------------------------|--------------------------------------|-------------------------------------------------------------------------------------------------|
| Programming Language     | Go                                   | Built-in support for networking and concurrency; produces standalone binaries.                |
| Command-line Parsing     | Go flag package                      | Minimal external dependencies for parsing command-line arguments.                             |
| Network Communication    | Go net package                       | Robust TCP/UDP support using the standard library.                                            |
| File Transfer            | Go bufio & io packages               | Efficient file I/O with buffered operations and stream copying using `io.Copy`.                 |
| Data Serialization       | Go encoding/json package             | Simple, clear JSON-based messaging for the custom protocol.                                   |
| Logging                  | Standard log package (or a structured logger) | Explicit logging for tracing operations and debugging when `--verbose` is enabled.              |
| Testing                  | Go testing package                   | Unit testing and integration testing to ensure code reliability.                              |

---

## 🚩 Command-Line Parameters

| Flag           | Type    | Required | Default                 | Description                                                                                           |
|----------------|---------|----------|-------------------------|-------------------------------------------------------------------------------------------------------|
| `--ip`         | String  | Yes      | N/A                     | 🔌 IP address of the peer to connect to (e.g., `192.168.1.10`).                                       |
| `--port`       | Integer | No       | `8080`                  | 🎯 Target port of the peer. Separating the port from the IP improves clarity and flexibility.         |
| `--listen`     | String  | No       | Primary IP on 8080      | 👂 Local IP address and port to listen on for incoming connections.                                  |
| `--folder`     | String  | No       | Current directory       | 📁 Directory used for sharing files and saving downloads.                                           |
| `--name`       | String  | No       | System hostname         | 🏷️ A friendly identifier for your node. Helps confirm you're connecting to the correct peer.         |
| `--readonly`   | Boolean | No       | false                   | 🔒 When enabled, restricts uploads—only downloads are allowed.                                      |
| `--writeonly`  | Boolean | No       | false                   | 📤 When enabled, restricts downloads—only uploads are permitted.                                    |
| `--maxsize`    | Integer | No       | Unlimited (or set limit)| 📏 Maximum file size in MB allowed for transfer.                                                    |
| `--verify`     | Boolean | No       | true                    | ✅ Enables checksum verification (using crypto/md5/sha256) to ensure file integrity.                  |
| `--verbose`    | Boolean | No       | false                   | 🔍 Enables detailed logging for network operations and file transfers, aiding debugging.             |

---

## 🗺️ Roadmap

1. **Command-Line Interface & Configuration**
   - **Parameter Parsing:** Use Go’s `flag` package to read command-line parameters and store them in a dedicated configuration struct.
   - **Validation:** Validate the IP address, port, and folder permissions at startup. Report errors clearly.
   - **Best Practices Applied:**  
     - Use a dedicated configuration package to separate concerns.  
     - Apply clear error messages and exit gracefully if validation fails.

2. **Network Connection & Dependency Injection**
   - **Listener & Dialer:** Create a TCP listener using `net.Listen()` and connect using `net.Dial()`.  
   - **Mutual Verification:** Implement a simple handshake protocol where both peers confirm readiness.
   - **Concurrency:** Use goroutines and channels to manage connections concurrently.
   - **Best Practices Applied:**  
     - Avoid global state by passing configuration and dependencies explicitly through constructors.  
     - Encapsulate network logic in its own package with well-defined interfaces.

3. **File Transfer Functionality**
   - **Buffered I/O:** Use `bufio` and `io.Copy` to efficiently transfer file chunks.
   - **Integrity Checks:** Calculate checksums (MD5 or SHA256) to verify file integrity.
   - **File Size Enforcement:** Use `io.LimitReader` to enforce maximum file size limits.
   - **Best Practices Applied:**  
     - Wrap file I/O operations with proper error handling and cleanup (using `defer` where appropriate).  
     - Write unit tests for file chunking and checksum calculations.

4. **User Interface & Logging**
   - **CLI Prompts:** Use `bufio.Scanner` for interactive command prompts.
   - **Status Updates:** Show connection and transfer progress clearly using simple terminal outputs.
   - **Logging:** Centralize logging in a dedicated logger package that respects the `--verbose` flag.
   - **Best Practices Applied:**  
     - Use structured logging and document log messages for easier debugging.  
     - Keep the main function minimal by delegating to packages.

5. **Testing, Documentation & CI**
   - **Unit & Integration Tests:** Write tests using the standard `testing` package to cover all functionalities.
   - **Documentation:** Include comprehensive comments and a README explaining the project setup, usage, and design choices.
   - **CI/CD:** Set up automated tests with GitHub Actions or another CI tool to maintain code quality.
   - **Best Practices Applied:**  
     - Follow Go’s conventions for test naming and placement.  
     - Ensure every public function is documented, and code is formatted with `go fmt` and vetted with `go vet`.

---

## 🗂️ Folder Structure

This structure is intentionally minimal to help you learn core Go practices while keeping the code organized and maintainable:

```plaintext
sharego/
├── cmd/
│   └── sharego/
│       └── main.go           # Entry point: minimal setup, calls into internal packages
├── internal/
│   ├── config/
│   │   └── config.go         # Handles flag parsing, configuration struct, and validation
│   ├── network/
│   │   └── connection.go     # Implements connection logic, handshake, and mutual verification
│   ├── transfer/
│   │   └── file_transfer.go  # File transfer logic, buffering, integrity checks, and size limits
│   └── util/
│       └── logger.go         # Centralized logging and error helper functions
├── go.mod                    # Module definition and dependency management
└── README.md                 # Project documentation and usage instructions
```
