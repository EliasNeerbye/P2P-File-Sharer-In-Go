package app

import (
	"local-file-sharer/internal/config"
	"net"
)

type App struct {
	Config *config.Config
	Conn   net.Conn
}
