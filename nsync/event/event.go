package event

import (
	"encoding/json"

	"github.com/Sirupsen/logrus"
)

type Event struct {
	MsgCh chan []byte
}

type Message struct {
	Name string      `json:"name"`
	Data interface{} `json:"data"`
}

func NewEvent(capacity int) *Event {
	msgCh := make(chan []byte, capacity)
	return &Event{MsgCh: msgCh}
}

func (e *Event) Send(name string, data interface{}) error {
	message := Message{
		Name: name,
		Data: data,
	}

	b, err := json.Marshal(message)
	if err != nil {
		return err
	}

	logrus.Infof("b: %+v", string(b))

	e.MsgCh <- b
	return nil
}
