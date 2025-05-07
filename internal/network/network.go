package network

import (
	"local-file-sharer/cmd/sharego/app"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func StartDial(app *app.App) {
	log := app.Logger
	conn, err := net.Dial("tcp", app.Config.TargetAddr)
	if err != nil {
		log.Fatal("Failed to dial peer: %v", err)
		return
	}
	defer conn.Close()

	log.Success("Connected to peer: %s", conn.RemoteAddr())

	// Send some data to the server
	message := "Hello from the client!"
	_, err = conn.Write([]byte(message))
	if err != nil {
		log.Error("Error sending data to server: %v", err)
		return
	}

	// Read the server's response
	buffer := make([]byte, 1024)
	_, err = conn.Read(buffer)
	if err != nil {
		log.Error("Error reading server's response: %v", err)
		return
	}

	log.Info("Received from server: %s", string(buffer))
}

func StartListening(app *app.App) {
	log := app.Logger
	ln, err := net.Listen("tcp", app.Config.ListenAddr)
	if err != nil {
		log.Fatal("Failed to start listener: %v", err)
		return
	}
	defer ln.Close()

	log.Success("Listening on %s", app.Config.ListenAddr)

	// Listen for interrupts to gracefully shut down
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
			log.Error("Accept error: %v", err)
			continue
		}
		go handleConnection(app, conn)
	}
}

func handleConnection(app *app.App, conn net.Conn) {
	log := app.Logger
	log.Info("New connection from: %s", conn.RemoteAddr())

	// Read data from the connection
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		log.Error("Error reading data from %v: %v", conn.RemoteAddr(), err)
		conn.Close()
		return
	}

	// Print received message
	log.Debug("Received message: %s", string(buffer[:n]))

	// Respond to the client
	response := "Hello from the server!"
	_, err = conn.Write([]byte(response))
	if err != nil {
		log.Error("Error sending data to %v: %v", conn.RemoteAddr(), err)
		conn.Close()
		return
	}

	conn.Close()
}
