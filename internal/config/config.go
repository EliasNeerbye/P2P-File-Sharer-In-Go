package config

import (
	"flag"
	"fmt"
	"os"
)

var (
	IP        string
	Port      int
	Listen    string
	Folder    string
	Name      string
	ReadOnly  bool
	WriteOnly bool
	MaxSize   int
	Verify    bool
	Verbose   bool
)

func Load() {
	flag.StringVar(&IP, "ip", "", "IP address of the peer to connect to (required)")
	flag.IntVar(&Port, "port", 8080, "Target port of the peer")
	flag.StringVar(&Listen, "listen", "Primary IP on 8080", "Local IP and port to listen on")
	flag.StringVar(&Folder, "folder", ".", "Directory used for sharing files and saving downloads")
	flag.StringVar(&Name, "name", getDefaultHostname(), "A friendly identifier for your node")
	flag.BoolVar(&ReadOnly, "readonly", false, "Restricts uploads—only downloads are allowed")
	flag.BoolVar(&WriteOnly, "writeonly", false, "Restricts downloads—only uploads are permitted")
	flag.IntVar(&MaxSize, "maxsize", 0, "Maximum file size in MB allowed for transfer (0 = unlimited)")
	flag.BoolVar(&Verify, "verify", true, "Enables checksum verification to ensure file integrity")
	flag.BoolVar(&Verbose, "verbose", false, "Enables detailed logging for debugging")

	flag.Parse()

	if IP == "" {
		fmt.Fprintln(os.Stderr, "Error: --ip is required")
		flag.Usage()
		os.Exit(1)
	}
}

func getDefaultHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not find host name...\n\nErr: %v\n", err)
		return "unknown-host"
	}
	return hostname
}
