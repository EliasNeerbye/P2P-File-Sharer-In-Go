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

func (a *App) IsServer() bool {
	return a.Config.TargetAddr == ""
}

func (a *App) IsReadOnly() bool {
	return a.Config.ReadOnly
}

func (a *App) IsWriteOnly() bool {
	return a.Config.WriteOnly
}
