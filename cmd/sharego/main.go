package main

import (
	"fmt"
	"local-file-sharer/cmd/sharego/app"
	"local-file-sharer/internal/config"
	"local-file-sharer/internal/network"
)

func main() {
	cfg := config.Load()

	a := &app.App{
		Config: cfg,
	}

	fancyPrint(a)

	if cfg.TargetAddr != "" {
		network.StartDial(a)
	} else {
		network.StartListening(a)
	}
}

func fancyPrint(app *app.App) {
	fmt.Println("Current Configuration:")
	fmt.Printf("IP:        %v\n", app.Config.TargetAddr)
	fmt.Printf("Listen:    %s\n", app.Config.ListenAddr)
	fmt.Printf("Folder:    %s\n", app.Config.Folder)
	fmt.Printf("Name:      %s\n", app.Config.Name)
	fmt.Printf("ReadOnly:  %t\n", app.Config.ReadOnly)
	fmt.Printf("WriteOnly: %t\n", app.Config.WriteOnly)
	fmt.Printf("MaxSize:   %d\n", app.Config.MaxSize)
	fmt.Printf("Verify:    %t\n", app.Config.Verify)
	fmt.Printf("Verbose:   %t\n", app.Config.Verbose)
}
