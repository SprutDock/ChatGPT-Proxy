package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEventStreamWritesVersionedEventsWithoutInput(t *testing.T) {
	var output bytes.Buffer
	events := &EventStream{writer: bufio.NewWriter(&output)}
	for _, name := range []string{"checking", "setup_started", "ready"} {
		if err := events.Emit(name); err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d event lines, want 3", len(lines))
	}
	for index, line := range lines {
		var event struct {
			Version int    `json:"v"`
			Name    string `json:"event"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("event line %d is not JSON: %v", index, err)
		}
		if event.Version != 1 || event.Name != []string{"checking", "setup_started", "ready"}[index] {
			t.Fatalf("event line %d = %#v", index, event)
		}
	}
}

func TestRequestInvitationDoesNotEchoSecret(t *testing.T) {
	secret := strings.Repeat("a", 48)
	var output bytes.Buffer
	events := &EventStream{writer: bufio.NewWriter(&output), input: strings.NewReader(secret + "\n")}
	invitation, err := events.RequestInvitation()
	if err != nil {
		t.Fatal(err)
	}
	if invitation != secret {
		t.Fatal("invitation input was changed")
	}
	if strings.Contains(output.String(), secret) || !strings.Contains(output.String(), `"event":"invitation_required"`) {
		t.Fatal("event output contains a secret or omitted the invitation request")
	}
}

func TestValidInvitationLength(t *testing.T) {
	for _, test := range []struct {
		length int
		valid  bool
	}{{31, false}, {32, true}, {256, true}, {257, false}} {
		if got := validInvitation(strings.Repeat("x", test.length)); got != test.valid {
			t.Errorf("validInvitation length %d = %t, want %t", test.length, got, test.valid)
		}
	}
}
