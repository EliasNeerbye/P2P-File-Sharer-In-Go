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
	LastSpeedUpdate  time.Time
	Retries          int
	AckIDs           map[string]bool
	LastBytes        int64
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
		LastSpeedUpdate:  now,
		Speed:            0,
		Conn:             conn,
		AckIDs:           make(map[string]bool),
		LastBytes:        0,
	}
}

func (t *FileTransfer) UpdateProgress(bytesTransferred int64, calculatedSpeed float64) {
	now := time.Now()

	t.BytesTransferred = bytesTransferred
	t.LastUpdate = now

	elapsed := now.Sub(t.LastSpeedUpdate).Seconds()

	if elapsed >= 1.0 {
		bytesThisInterval := bytesTransferred - t.LastBytes
		currentSpeed := float64(bytesThisInterval) / elapsed / 1024

		if currentSpeed < 0 {
			currentSpeed = 0
		}

		if t.Speed == 0 {
			t.Speed = currentSpeed
		} else {
			t.Speed = 0.8*t.Speed + 0.2*currentSpeed
		}

		t.LastSpeedUpdate = now
		t.LastBytes = bytesTransferred
	}

	if calculatedSpeed > 0 && t.Speed == 0 {
		t.Speed = calculatedSpeed
	}

	var percentage float64
	if t.TotalSize > 0 {
		percentage = float64(t.BytesTransferred) * 100 / float64(t.TotalSize)
	}

	var eta string
	if t.Speed > 0.01 && t.TotalSize > t.BytesTransferred {
		remainingBytes := t.TotalSize - t.BytesTransferred
		remainingSeconds := int(float64(remainingBytes) / (t.Speed * 1024))

		if remainingSeconds <= 0 {
			eta = "0s"
		} else if remainingSeconds < 60 {
			eta = fmt.Sprintf("%ds", remainingSeconds)
		} else if remainingSeconds < 3600 {
			eta = fmt.Sprintf("%dm %ds", remainingSeconds/60, remainingSeconds%60)
		} else {
			eta = fmt.Sprintf("%dh %dm", remainingSeconds/3600, (remainingSeconds%3600)/60)
		}
	} else {
		if percentage >= 99.0 {
			eta = "0s"
		} else {
			eta = "calculating..."
		}
	}

	progBar := generateProgressBar(percentage, 30)
	typeStr := "↓ Receiving"
	if t.Type == TransferTypeSend {
		typeStr = "↑ Sending"
	}

	displaySpeed := t.Speed
	if displaySpeed < 0.01 {
		displaySpeed = 0.01
	}

	fmt.Printf("\r%-90s", " ")
	fmt.Printf("\r%s %s: %s %.1f%% (%.2f KB/s) ETA: %s",
		typeStr, t.Name, progBar, percentage, displaySpeed, eta)
}

func (t *FileTransfer) Pause() {
	if t.Status == TransferStatusInProgress {
		t.Status = TransferStatusPaused
		fmt.Printf("\nTransfer paused: %s\n", t.Name)
	}
}

func (t *FileTransfer) Resume() {
	if t.Status == TransferStatusPaused {
		t.Status = TransferStatusInProgress
		t.LastProgressTime = time.Now()
		t.LastSpeedUpdate = time.Now()
		fmt.Printf("\nTransfer resumed: %s\n", t.Name)
	}
}

func (t *FileTransfer) WaitForAcknowledgment(timeout time.Duration) bool {
	if len(t.AckIDs) == 0 {
		return true
	}

	checkInterval := 100 * time.Millisecond
	endTime := time.Now().Add(timeout)

	for time.Now().Before(endTime) {
		if len(t.AckIDs) == 0 {
			return true
		}
		time.Sleep(checkInterval)
	}

	return false
}

func (t *FileTransfer) RegisterAckID(id string) {
	t.AckIDs[id] = true
}

func (t *FileTransfer) AcknowledgeID(id string) {
	delete(t.AckIDs, id)
}

func generateProgressBar(percentage float64, width int) string {
	filled := int(percentage * float64(width) / 100)

	bar := "["
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "="
		} else if i == filled && percentage < 100 {
			bar += ">"
		} else {
			bar += " "
		}
	}
	bar += "]"

	return bar
}
