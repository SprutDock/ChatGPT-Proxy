package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fail("expected ensure or status")
	}
	p, err := paths()
	if err != nil {
		fail("could not locate local agent storage")
	}
	switch os.Args[1] {
	case "ensure":
		interactive := len(os.Args) < 3 || os.Args[2] != "--background"
		if err := ensure(p, interactive); err != nil {
			fail(err.Error())
		}
	case "status":
		state, err := loadState(p)
		if err != nil {
			fail("could not read agent state")
		}
		if state.ActiveSerial == "" {
			fail("this device is not enrolled")
		}
		fmt.Printf("device=%s serial=%s\n", state.DeviceID, state.ActiveSerial)
	default:
		fail("unknown command")
	}
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
