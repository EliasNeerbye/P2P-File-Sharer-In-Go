package network

import (
	"encoding/base64"
	"strings"
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

func (m *Message) Marshal() ([]byte, error) {
	if m.Binary {

		m.Data = base64.StdEncoding.EncodeToString([]byte(m.Data))
		m.Binary = false
	}

	var b strings.Builder
	b.WriteString(`{"type":"`)
	b.WriteString(m.Type)
	b.WriteString(`","data":"`)
	b.WriteString(escapeJSON(m.Data))
	b.WriteString(`"`)

	if m.ID != "" {
		b.WriteString(`,"id":"`)
		b.WriteString(m.ID)
		b.WriteString(`"`)
	}

	if m.Binary {
		b.WriteString(`,"binary":true`)
	}

	b.WriteString(`}`)

	return []byte(b.String()), nil
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}
