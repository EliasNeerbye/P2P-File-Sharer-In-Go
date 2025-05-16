package network

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"local-file-sharer/internal/util"
	"os"
	"path/filepath"
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
	Checksum         string
	PauseSignal      chan bool
	IsPaused         bool
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
		PauseSignal:      make(chan bool, 1),
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

	fmt.Printf("\r%-70s", " ")
	fmt.Printf("\r%s %s: %s %.1f%% (%.2f KB/s) ETA: %s",
		typeStr, t.Name, progBar, percentage, speed, eta)

	if percentage >= 100 && t.Status == TransferStatusComplete {
		fmt.Printf("\n\n%sTransfer complete: %s%s\n\n> ", util.Green+util.Bold, t.Name, util.Reset)
	}
}

func (t *FileTransfer) CalculateChecksum() (string, error) {
	if t.File == nil {
		return "", fmt.Errorf("file not open")
	}

	currentPos, err := t.File.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", err
	}

	_, err = t.File.Seek(0, io.SeekStart)
	if err != nil {
		return "", err
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, t.File); err != nil {
		return "", err
	}

	_, err = t.File.Seek(currentPos, io.SeekStart)
	if err != nil {
		return "", err
	}

	checksum := hex.EncodeToString(hash.Sum(nil))
	t.Checksum = checksum
	return checksum, nil
}

func (t *FileTransfer) VerifyChecksum(expectedChecksum string) (bool, error) {
	actualChecksum, err := t.CalculateChecksum()
	if err != nil {
		return false, err
	}

	return actualChecksum == expectedChecksum, nil
}

func (t *FileTransfer) Pause() {
	if t.Status == TransferStatusInProgress {
		t.Status = TransferStatusPaused
		t.IsPaused = true
		select {
		case t.PauseSignal <- true:
		default:
		}
		fmt.Printf("\n\n%sTransfer paused: %s%s\n\n> ", util.Yellow+util.Bold, t.Name, util.Reset)
	}
}

func (t *FileTransfer) Resume() {
	if t.Status == TransferStatusPaused {
		t.Status = TransferStatusInProgress
		t.IsPaused = false
		t.LastProgressTime = time.Now()
		fmt.Printf("\n\n%sTransfer resumed: %s%s\n\n> ", util.Green+util.Bold, t.Name, util.Reset)
	}
}

func (t *FileTransfer) ShouldIgnore() bool {
	baseName := filepath.Base(t.Name)
	return baseName == ".fshignore" || baseName == ".gitignore"
}

func (t *FileTransfer) WaitForPauseIfNeeded() bool {
	if t.IsPaused {
		fmt.Printf("\n%sWaiting for transfer to resume: %s%s\n", util.Yellow, t.Name, util.Reset)
		for t.IsPaused {
			time.Sleep(500 * time.Millisecond)
			if t.Status == TransferStatusFailed {
				return true
			}
		}
	}
	return false
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
