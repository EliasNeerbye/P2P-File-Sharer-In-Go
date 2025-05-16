package network

import (
	"local-file-sharer/internal/util"
	"net"
)

func StartDial(app *App) {
	log := util.NewLogger(app.Config.Verbose, "Client")

	log.Info("Connecting to %s", app.Config.TargetAddr)

	conn, err := net.Dial("tcp", app.Config.TargetAddr)
	if err != nil {
		log.Fatal("Failed to connect: %v", err)
		return
	}

	log.Info("Connected to %s", app.Config.TargetAddr)

	connection := NewConnection(conn, app, true)
	app.AddConnection(connection)

	go connection.Start()
	StartCommandInterface(app)
}
