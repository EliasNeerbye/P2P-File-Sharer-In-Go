package network

import (
	"local-file-sharer/internal/config"
	"local-file-sharer/internal/util"
	"sync"
)

type App struct {
	Config        *config.Config
	Log           *util.Logger
	Connections   map[string]*Connection
	Transfers     map[string]*FileTransfer
	CommandParser *CommandParser
	mu            sync.Mutex
	Ready         bool
	transferID    int
}

func NewApp(cfg *config.Config, log *util.Logger) *App {
	app := &App{
		Config:      cfg,
		Log:         log,
		Connections: make(map[string]*Connection),
		Transfers:   make(map[string]*FileTransfer),
		Ready:       true,
	}
	app.CommandParser = NewCommandParser(app)
	return app
}

func (a *App) Shutdown() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Close all connections
	for _, conn := range a.Connections {
		conn.Conn.Close()
	}

	a.Ready = false
}

func (a *App) AddConnection(conn *Connection) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Connections[conn.ID] = conn
}

func (a *App) RemoveConnection(conn *Connection) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.Connections, conn.ID)
}

func (a *App) AddTransfer(transfer *FileTransfer) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.transferID++
	transfer.ID = a.transferID
	a.Transfers[transfer.Name] = transfer
}

func (a *App) RemoveTransfer(transfer *FileTransfer) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.Transfers, transfer.Name)
}

func (a *App) GetCurrentTransfers() []*FileTransfer {
	a.mu.Lock()
	defer a.mu.Unlock()
	transfers := make([]*FileTransfer, 0, len(a.Transfers))
	for _, t := range a.Transfers {
		transfers = append(transfers, t)
	}
	return transfers
}
