package network

import (
	"fmt"
	"local-file-sharer/cmd/sharego/app"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func StartDial(app *app.App) {
	conn, err := net.Dial("tcp", app.Config.TargetAddr)
	if err != nil {
		log.Fatal("Failed to dial peer: ", err)
		return
	}
	defer conn.Close()

	fmt.Println("Connected to peer: ", conn.RemoteAddr())

	// Send some data to the server
	message := "Hello from the client!"
	_, err = conn.Write([]byte(message))
	if err != nil {
		log.Printf("Error sending data to server: %v\n", err)
		conn.Close()
		return
	}

	// Read the server's response
	buffer := make([]byte, 1024)
	_, err = conn.Read(buffer)
	if err != nil {
		log.Printf("Error reading server's response: %v\n", err)
		conn.Close()
		return
	}

	fmt.Println("Received from server:", string(buffer))
}

func StartListening(app *app.App) {
	ln, err := net.Listen("tcp", app.Config.ListenAddr)
	if err != nil {
		log.Fatal("Failed to start listener: ", err)
		return
	}
	defer ln.Close()

	fmt.Println("Listening on", app.Config.ListenAddr)

	// Listen for interrupts to gracefully shut down
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signals
		fmt.Println("Shutting down listener...")
		ln.Close()
		os.Exit(0)
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("Accept error: ", err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	fmt.Println("Handling connection from: ", conn.RemoteAddr())

	// Read data from the connection
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		log.Printf("Error reading data from %v: %v\n", conn.RemoteAddr(), err)
		conn.Close()
		return
	}

	// Print received message
	fmt.Printf("Received message: %s\n", string(buffer[:n]))

	// Respond to the client
	response := "Hello from the server!"
	_, err = conn.Write([]byte(response))
	if err != nil {
		log.Printf("Error sending data to %v: %v\n", conn.RemoteAddr(), err)
		conn.Close()
		return
	}

	conn.Close()
}
