package main

import (
	"local-file-sharer/internal/config"
	"local-file-sharer/internal/network"
	"local-file-sharer/internal/util"
)

func main() {
	cfg := config.Load()
	log := util.NewLogger(cfg.Verbose, "Main")

	log.Info("Starting P2P File Sharer")
	printConfig(cfg, log)

	app := network.NewApp(cfg, log)

	if cfg.TargetAddr != "" {
		log.Info("Starting in client mode, connecting to %s", cfg.TargetAddr)
		network.StartDial(app)
	} else {
		log.Info("Starting in server mode, listening on %s", cfg.ListenAddr)
		network.StartListening(app)
	}
}

func printConfig(cfg *config.Config, log *util.Logger) {
	log.Info("Current Configuration:")
	log.Debug("IP:        %v", cfg.TargetAddr)
	log.Debug("Listen:    %s", cfg.ListenAddr)
	log.Debug("Folder:    %s", cfg.Folder)
	log.Debug("Name:      %s", cfg.Name)
	log.Debug("ReadOnly:  %t", cfg.ReadOnly)
	log.Debug("WriteOnly: %t", cfg.WriteOnly)
	log.Debug("MaxSize:   %d", cfg.MaxSize)
	log.Debug("Verify:    %t", cfg.Verify)
	log.Debug("Verbose:   %t", cfg.Verbose)
}
