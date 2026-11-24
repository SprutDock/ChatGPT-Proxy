package main

import (
	"fmt"
	"os"

	"sprutdock/cert-agent/internal/agent"
	"sprutdock/cert-agent/internal/codex"
	"sprutdock/cert-agent/internal/platform"
	"sprutdock/cert-agent/internal/state"
)

func main() {
	if len(os.Args) < 2 {
		fail("expected ensure, status, stop, or configure-codex")
	}
	if os.Args[1] == "configure-codex" {
		if len(os.Args) != 2 {
			fail("configure-codex does not accept options")
		}
		if err := codex.Configure(); err != nil {
			fail(err.Error())
		}
		return
	}
	p, err := platform.Paths()
	if err != nil {
		fail("could not locate local agent storage")
	}
	switch os.Args[1] {
	case "ensure":
		interactive, events := true, false
		for _, argument := range os.Args[2:] {
			switch argument {
			case "--background":
				interactive = false
			case "--events":
				events = true
			default:
				fail("unknown ensure option")
			}
		}
		var err error
		if events {
			err = agent.EnsureWithEvents(p, interactive, agent.NewEventStream(os.Stdout, os.Stdin))
		} else {
			err = agent.Ensure(p, interactive)
		}
		if err != nil {
			fail(err.Error())
		}
	case "status":
		currentState, err := state.LoadState(p)
		if err != nil {
			fail("could not read agent state")
		}
		if currentState.ActiveSerial == "" {
			fail("this device is not enrolled")
		}
		fmt.Printf("device=%s serial=%s\n", currentState.DeviceID, currentState.ActiveSerial)
	case "stop":
		if err := platform.StopStunnel(p); err != nil {
			fail(err.Error())
		}
	default:
		fail("unknown command")
	}
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
