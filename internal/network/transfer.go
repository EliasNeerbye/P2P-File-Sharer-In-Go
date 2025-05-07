package network

import (
	"fmt"
	"os"
	"time"
)

const (
	TransferTypeSend    = "send"
	TransferTypeReceive = "receive"

	TransferStatusInProgress = "in_progress"
	TransferStatusComplete   = "complete"
	TransferStatusFailed     = "failed"
	TransferStatusWaitingAck = "waiting_ack"
)

type FileTransfer struct {
	ID               int
	Name             string
	Type             string
	Status           string
	TotalSize        int64
	BytesTransferred int64
	StartTime        time.Time
	LastUpdate       time.Time
	Speed            float64
	Conn             *Connection
	File             *os.File
	LastProgress     int64
}

func NewFileTransfer(name string, size int64, transferType string, conn *Connection) *FileTransfer {
	return &FileTransfer{
		Name:             name,
		Type:             transferType,
		Status:           TransferStatusInProgress,
		TotalSize:        size,
		BytesTransferred: 0,
		StartTime:        time.Now(),
		LastUpdate:       time.Now(),
		Speed:            0,
		Conn:             conn,
	}
}

func (t *FileTransfer) UpdateProgress(bytesTransferred int64, speed float64) {
	t.BytesTransferred = bytesTransferred
	t.Speed = speed
	t.LastUpdate = time.Now()

	// Calculate percentage
	var percentage float64
	if t.TotalSize > 0 {
		percentage = float64(t.BytesTransferred) * 100 / float64(t.TotalSize)
	}

	// Calculate ETA
	var eta string
	if speed > 0 {
		remainingBytes := t.TotalSize - t.BytesTransferred
		remainingSeconds := int(float64(remainingBytes) / (speed * 1024))
		if remainingSeconds < 60 {
			eta = fmt.Sprintf("%ds", remainingSeconds)
		} else if remainingSeconds < 3600 {
			eta = fmt.Sprintf("%dm %ds", remainingSeconds/60, remainingSeconds%60)
		} else {
			eta = fmt.Sprintf("%dh %dm", remainingSeconds/3600, (remainingSeconds%3600)/60)
		}
	} else {
		eta = "calculating..."
	}

	// Print progress
	progBar := generateProgressBar(percentage, 30)
	typeStr := "↓ Receiving"
	if t.Type == TransferTypeSend {
		typeStr = "↑ Sending"
	}

	fmt.Printf("\r%s %s: %s %.1f%% (%.2f KB/s) ETA: %s",
		typeStr, t.Name, progBar, percentage, speed, eta)
}

func generateProgressBar(percentage float64, width int) string {
	filled := int(percentage * float64(width) / 100)

	bar := "["
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "="
		} else if i == filled {
			bar += ">"
		} else {
			bar += " "
		}
	}
	bar += "]"

	return bar
}
