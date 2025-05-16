package network

import (
	"local-file-sharer/internal/util"
	"net"
	"sync"
)

func StartListening(app *App) {
	log := util.NewLogger(app.Config.Verbose, "Server")

	listener, err := net.Listen("tcp", app.Config.ListenAddr)
	if err != nil {
		log.Fatal("Failed to start listener: %v", err)
		return
	}

	log.Info("Listening on %s", app.Config.ListenAddr)
	log.Info("Waiting for connections...")

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Error("Failed to accept connection: %v", err)
				continue
			}

			log.Info("New connection from %s", conn.RemoteAddr())

			connection := NewConnection(conn, app, false)
			app.AddConnection(connection)

			go connection.Start()

			if len(app.Connections) == 1 {
				go StartCommandInterface(app)
			}
		}
	}()

	if app.Config.TargetAddr != "" {
		go StartDial(app)
	}

	wg.Wait()
}
