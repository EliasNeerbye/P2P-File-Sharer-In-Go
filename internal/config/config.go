package config

import (
	"flag"
	"fmt"
	"os"
)

type Config struct {
	TargetAddr string
	ListenAddr string
	Folder     string
	Name       string
	ReadOnly   bool
	WriteOnly  bool
	MaxSize    int
	Verify     bool
	Verbose    bool
	DualMode   bool
}

func Load() *Config {
	cfg := &Config{}

	var targetIP string
	var targetPort int

	flag.StringVar(&targetIP, "ip", "", "IP address of the peer to connect to (e.g., 192.168.1.10)")
	flag.IntVar(&targetPort, "port", 8080, "Target port of the peer")
	flag.StringVar(&cfg.ListenAddr, "listen", ":8080", "Local IP address and port to listen on (e.g., :8080)")
	flag.StringVar(&cfg.Folder, "folder", ".", "Directory used for sharing files and saving downloads")
	flag.StringVar(&cfg.Name, "name", getDefaultHostname(), "A friendly identifier for your node")
	flag.BoolVar(&cfg.ReadOnly, "readonly", false, "Restricts uploads—only downloads are allowed")
	flag.BoolVar(&cfg.WriteOnly, "writeonly", false, "Restricts downloads—only uploads are permitted")
	flag.IntVar(&cfg.MaxSize, "maxsize", 0, "Maximum file size in MB allowed for transfer (0 = unlimited)")
	flag.BoolVar(&cfg.Verify, "verify", true, "Enables checksum verification to ensure file integrity")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Enables detailed logging for debugging")
	flag.BoolVar(&cfg.DualMode, "dual", false, "Run as both client and server")

	flag.Parse()

	if targetIP != "" {
		cfg.TargetAddr = fmt.Sprintf("%s:%d", targetIP, targetPort)
	}

	return cfg
}

func getDefaultHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown-host"
	}
	return hostname
}
