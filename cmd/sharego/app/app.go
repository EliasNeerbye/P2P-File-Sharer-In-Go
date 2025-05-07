package app

import (
	"local-file-sharer/internal/config"
	"local-file-sharer/internal/util"
	"net"
)

type App struct {
	Config *config.Config
	Conn   net.Conn
	Logger *util.Logger
}
