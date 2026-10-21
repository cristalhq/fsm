package fsm_test

import (
	"fmt"

	"github.com/cristalhq/fsm"
)

func Example() {
	type State string
	type Action string

	const (
		New        State = "new"
		Hold       State = "hold"
		Processing State = "processing"
		Paid       State = "paid"

		Authorize Action = "authorized"
		Capture   Action = "capture"
		Captured  Action = "captured"
	)

	g, err := fsm.New(fsm.Config[State, Action]{
		Initial:  New,
		Terminal: []State{Paid}, // This example does not model refunds.
		Transitions: []fsm.Transition[State, Action]{
			{New, Authorize, Hold},
			{Hold, Capture, Processing},
			{Processing, Captured, Paid},
		},
	})
	if err != nil {
		panic(err)
	}

	state := g.Initial()
	for _, action := range []Action{Authorize, Capture, Captured} {
		var ok bool
		state, ok = g.Next(state, action)
		if !ok {
			panic("no such state")
		}
		fmt.Println(state)
	}

	fmt.Println(g.IsTerminal(state))

	// Output:
	// hold
	// processing
	// paid
	// true
}
