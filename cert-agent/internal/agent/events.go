package agent

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

type EventStream struct {
	writer *bufio.Writer
	input  io.Reader
}

func NewEventStream(writer io.Writer, input io.Reader) *EventStream {
	return &EventStream{writer: bufio.NewWriter(writer), input: input}
}

func (events *EventStream) Emit(name string) error {
	if events == nil {
		return nil
	}
	encoded, err := json.Marshal(struct {
		Version int    `json:"v"`
		Event   string `json:"event"`
	}{Version: 1, Event: name})
	if err != nil {
		return err
	}
	if _, err := events.writer.Write(encoded); err != nil {
		return err
	}
	if err := events.writer.WriteByte('\n'); err != nil {
		return err
	}
	return events.writer.Flush()
}

func (events *EventStream) RequestInvitation() (string, error) {
	if err := events.Emit("invitation_required"); err != nil {
		return "", err
	}
	value, err := bufio.NewReader(io.LimitReader(events.input, 257)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(value), nil
}
