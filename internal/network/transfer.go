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
	TransferStatusPaused     = "paused"
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
	LastProgressTime time.Time
	AvgSpeed         float64
	Retries          int
}

func NewFileTransfer(name string, size int64, transferType string, conn *Connection) *FileTransfer {
	now := time.Now()
	return &FileTransfer{
		Name:             name,
		Type:             transferType,
		Status:           TransferStatusInProgress,
		TotalSize:        size,
		BytesTransferred: 0,
		StartTime:        now,
		LastUpdate:       now,
		LastProgressTime: now,
		Speed:            0,
		AvgSpeed:         0,
		Conn:             conn,
	}
}

func (t *FileTransfer) UpdateProgress(bytesTransferred int64, speed float64) {
	now := time.Now()
	elapsed := now.Sub(t.LastProgressTime).Seconds()

	if elapsed > 0 && t.LastProgressTime != t.StartTime {
		currentSpeed := float64(bytesTransferred-t.BytesTransferred) / elapsed / 1024
		if t.AvgSpeed == 0 {
			t.AvgSpeed = currentSpeed
		} else {
			t.AvgSpeed = 0.7*t.AvgSpeed + 0.3*currentSpeed
		}
		speed = t.AvgSpeed
	}

	t.BytesTransferred = bytesTransferred
	t.Speed = speed
	t.LastUpdate = now
	t.LastProgressTime = now

	var percentage float64
	if t.TotalSize > 0 {
		percentage = float64(t.BytesTransferred) * 100 / float64(t.TotalSize)
	}

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

	progBar := generateProgressBar(percentage, 30)
	typeStr := "↓ Receiving"
	if t.Type == TransferTypeSend {
		typeStr = "↑ Sending"
	}

	fmt.Printf("\r%-60s", " ")
	fmt.Printf("\r%s %s: %s %.1f%% (%.2f KB/s) ETA: %s",
		typeStr, t.Name, progBar, percentage, speed, eta)

	if percentage >= 100 && t.Status == TransferStatusComplete {
		fmt.Printf("\nTransfer complete: %s\n> ", t.Name)
	}
}

func (t *FileTransfer) Pause() {
	if t.Status == TransferStatusInProgress {
		t.Status = TransferStatusPaused
		fmt.Printf("\nTransfer paused: %s\n> ", t.Name)
	}
}

func (t *FileTransfer) Resume() {
	if t.Status == TransferStatusPaused {
		t.Status = TransferStatusInProgress
		t.LastProgressTime = time.Now()
		fmt.Printf("\nTransfer resumed: %s\n> ", t.Name)
	}
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
