package network

import (
	"encoding/base64"
	"encoding/json"
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
	MsgTypeMessage       = "MESSAGE"
)

type Message struct {
	Type   string `json:"type"`
	Data   string `json:"data"`
	Binary bool   `json:"binary,omitempty"`
	ID     string `json:"id,omitempty"`
}

func NewMessage(msgType, data string) Message {
	return Message{
		Type: msgType,
		Data: data,
	}
}

func NewBinaryMessage(msgType string, data []byte) Message {
	return Message{
		Type:   msgType,
		Data:   string(data),
		Binary: true,
	}
}

func (m *Message) Marshal() ([]byte, error) {
	if m.Binary {
		m.Data = base64.StdEncoding.EncodeToString([]byte(m.Data))
		m.Binary = false
	}

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