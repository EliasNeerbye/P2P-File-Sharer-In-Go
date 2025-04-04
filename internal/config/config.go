package config

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	TargetAddr string // Peer address
	ListenAddr string // Local listening address
	Folder     string
	Name       string
	ReadOnly   bool
	WriteOnly  bool
	MaxSize    int
	Verify     bool
	Verbose    bool
}

func Load() *Config {
	cfg := &Config{}

	// Flag parsing
	flag.StringVar(&cfg.TargetAddr, "ip", "", "IP address and port of the peer to connect to (e.g., 192.168.1.10:8080)")
	flag.StringVar(&cfg.ListenAddr, "listen", ":8080", "Local address and port to listen on (e.g., :8080)")
	flag.StringVar(&cfg.Folder, "folder", ".", "Directory used for sharing files and saving downloads")
	flag.StringVar(&cfg.Name, "name", getDefaultHostname(), "A friendly identifier for your node")
	flag.BoolVar(&cfg.ReadOnly, "readonly", false, "Restricts uploads—only downloads are allowed")
	flag.BoolVar(&cfg.WriteOnly, "writeonly", false, "Restricts downloads—only uploads are permitted")
	flag.IntVar(&cfg.MaxSize, "maxsize", 0, "Maximum file size in MB allowed for transfer (0 = unlimited)")
	flag.BoolVar(&cfg.Verify, "verify", true, "Enables checksum verification to ensure file integrity")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Enables detailed logging for debugging")

	// Parse the flags
	flag.Parse()

	// Validate listen address (deny full IPs)
	if strings.Contains(cfg.ListenAddr, ".") || strings.Contains(cfg.ListenAddr, ":") && !strings.HasPrefix(cfg.ListenAddr, ":") {
		fmt.Fprintln(os.Stderr, "Error: --listen should not include an IP address. Only a port or :port format is allowed.")
		os.Exit(1)
	}

	return cfg
}

func getDefaultHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not find host name...\n\nErr: %v\n", err)
		return "unknown-host"
	}
	return hostname
}
