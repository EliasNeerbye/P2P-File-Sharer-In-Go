package main

import (
	"fmt"
	"local-file-sharer/internal/config"
)

func main() {
	println("fmt")
	config.Load()
	fancyPrint()
}

func fancyPrint() {
	fmt.Println("Current Configuration:")
	fmt.Printf("IP:        %v\n", config.IP)
	fmt.Printf("Port:      %d\n", config.Port)
	fmt.Printf("Listen:    %s\n", config.Listen)
	fmt.Printf("Folder:    %s\n", config.Folder)
	fmt.Printf("Name:      %s\n", config.Name)
	fmt.Printf("ReadOnly:  %t\n", config.ReadOnly)
	fmt.Printf("WriteOnly: %t\n", config.WriteOnly)
	fmt.Printf("MaxSize:   %d\n", config.MaxSize)
	fmt.Printf("Verify:    %t\n", config.Verify)
	fmt.Printf("Verbose:   %t\n", config.Verbose)
}
