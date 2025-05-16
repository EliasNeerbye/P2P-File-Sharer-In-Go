package network

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

const (
	MsgTypeHandshake     = "HANDSHAKE"
	MsgTypeCommand       = "COMMAND"
	MsgTypeCommandResult = "COMMANDRESULT"
	MsgTypeFileStart     = "FILESTART"
	MsgTypeFileData      = "FILEDATA"
	MsgTypeFileEnd       = "FILEEND"
	MsgTypeError         = "ERROR"
	MsgTypeProgress      = "PROGRESS"
	MsgTypeACK           = "ACK"
	MsgTypeNACK          = "NACK"
	MsgTypeMessage       = "MESSAGE"
	MsgTypePing          = "PING"
	MsgTypePong          = "PONG"
)

type Message struct {
	Type       string    `json:"type"`
	Data       string    `json:"data"`
	Binary     bool      `json:"binary,omitempty"`
	ID         string    `json:"id,omitempty"`
	Timestamp  time.Time `json:"timestamp,omitempty"`
	RetryCount int       `json:"retry_count,omitempty"`
}

func NewMessage(msgType, data string) Message {
	return Message{
		Type:      msgType,
		Data:      data,
		Timestamp: time.Now(),
	}
}

func NewBinaryMessage(msgType string, data []byte) Message {
	return Message{
		Type:      msgType,
		Data:      string(data),
		Binary:    true,
		Timestamp: time.Now(),
	}
}

func (m *Message) Marshal() ([]byte, error) {
	if m.Binary {
		m.Data = base64.StdEncoding.EncodeToString([]byte(m.Data))
		m.Binary = false
	}

	m.Timestamp = time.Now()
	return json.Marshal(m)
}

func Unmarshal(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (m *Message) GetBinaryData() ([]byte, error) {
	if m.Binary {
		return []byte(m.Data), nil
	}
	return base64.StdEncoding.DecodeString(m.Data)
}

func (m *Message) String() string {
	if m.ID != "" {
		return fmt.Sprintf("%s[%s] %s", m.Type, m.ID, m.Data)
	}
	return fmt.Sprintf("%s %s", m.Type, m.Data)
}

func NewAckMessage(msgID string) Message {
	return Message{
		Type:      MsgTypeACK,
		ID:        msgID,
		Timestamp: time.Now(),
	}
}

func (m *Message) RequiresAck() bool {
	return m.Type == MsgTypeFileStart ||
		m.Type == MsgTypeFileEnd ||
		m.Type == MsgTypeCommand ||
		m.Type == MsgTypeCommandResult
}

func (m *Message) Clone() Message {
	return Message{
		Type:       m.Type,
		Data:       m.Data,
		Binary:     m.Binary,
		ID:         m.ID,
		Timestamp:  m.Timestamp,
		RetryCount: m.RetryCount,
	}
}

func (m *Message) IncrementRetry() {
	m.RetryCount++
}
