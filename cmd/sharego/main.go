package main

import (
	"local-file-sharer/cmd/sharego/app"
	"local-file-sharer/internal/config"
	"local-file-sharer/internal/network"
	"local-file-sharer/internal/util"
)

func main() {
	cfg := config.Load()
	log := util.NewLogger(cfg.Verbose, "Main")

	a := &app.App{
		Config: cfg,
		Logger: log,
	}

	log.Info("Starting P2P File Sharer")
	printConfig(a)

	if cfg.TargetAddr != "" {
		log.Info("Starting in client mode, connecting to %s", cfg.TargetAddr)
		network.StartDial(a)
	} else {
		log.Info("Starting in server mode, listening on %s", cfg.ListenAddr)
		network.StartListening(a)
	}
}

func printConfig(app *app.App) {
	app.Logger.Info("Current Configuration:")
	app.Logger.Debug("IP:        %v", app.Config.TargetAddr)
	app.Logger.Debug("Listen:    %s", app.Config.ListenAddr)
	app.Logger.Debug("Folder:    %s", app.Config.Folder)
	app.Logger.Debug("Name:      %s", app.Config.Name)
	app.Logger.Debug("ReadOnly:  %t", app.Config.ReadOnly)
	app.Logger.Debug("WriteOnly: %t", app.Config.WriteOnly)
	app.Logger.Debug("MaxSize:   %d", app.Config.MaxSize)
	app.Logger.Debug("Verify:    %t", app.Config.Verify)
	app.Logger.Debug("Verbose:   %t", app.Config.Verbose)
}
