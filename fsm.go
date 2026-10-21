package fsm

import "fmt"

// Transition describes one allowed change.
type Transition[S, A comparable] struct {
	From   S
	Action A
	To     S
}

// Config describes a graph.
type Config[S, A comparable] struct {
	Initial     S
	Terminal    []S
	Transitions []Transition[S, A]
}

// Graph is an immutable transition graph.
type Graph[S, A comparable] struct {
	initial  S
	terminal map[S]struct{}
	edges    map[S]map[A]S
	actions  map[S][]A
}

// New constructs a [Graph].
// It rejects:
//
//	states unreachable from Initial,
//	duplicate (state, action) pairs,
//	duplicate terminal declarations,
//	outgoing transitions from terminal states.
func New[S, A comparable](cfg Config[S, A]) (*Graph[S, A], error) {
	g := &Graph[S, A]{
		initial:  cfg.Initial,
		terminal: make(map[S]struct{}, len(cfg.Terminal)),
		edges:    make(map[S]map[A]S),
		actions:  make(map[S][]A),
	}

	states := []S{cfg.Initial}
	seen := map[S]bool{cfg.Initial: true}
	addState := func(state S) {
		if !seen[state] {
			seen[state] = true
			states = append(states, state)
		}
	}

	for _, state := range cfg.Terminal {
		if _, ok := g.terminal[state]; ok {
			return nil, fmt.Errorf("fsm: duplicate terminal state %v", state)
		}
		g.terminal[state] = struct{}{}
		addState(state)
	}

	for _, tr := range cfg.Transitions {
		if _, ok := g.terminal[tr.From]; ok {
			return nil, fmt.Errorf("fsm: terminal state %v has an outgoing transition", tr.From)
		}

		if g.edges[tr.From] == nil {
			g.edges[tr.From] = make(map[A]S)
		}
		if _, ok := g.edges[tr.From][tr.Action]; ok {
			return nil, fmt.Errorf("fsm: duplicate transition from %v with action %v", tr.From, tr.Action)
		}

		g.edges[tr.From][tr.Action] = tr.To
		g.actions[tr.From] = append(g.actions[tr.From], tr.Action)

		addState(tr.From)
		addState(tr.To)
	}

	queue := []S{cfg.Initial}
	visited := map[S]bool{cfg.Initial: true}
	for i := 0; i < len(queue); i++ {
		for _, next := range g.edges[queue[i]] {
			if !visited[next] {
				visited[next] = true
				queue = append(queue, next)
			}
		}
	}

	for _, state := range states {
		if !visited[state] {
			return nil, fmt.Errorf("fsm: state %v is unreachable from initial state %v", state, cfg.Initial)
		}
	}
	return g, nil
}

// Initial state of the graph.
func (g *Graph[S, A]) Initial() S { return g.initial }

// Next resolves an action (without changing any state).
// Unknown states also fails.
func (g *Graph[S, A]) Next(state S, action A) (S, bool) {
	if next, ok := g.edges[state][action]; ok {
		return next, true
	}
	return state, false
}

// Actions returns a list of actions in configuration order, or nil.
func (g *Graph[S, A]) Actions(state S) []A {
	return append([]A(nil), g.actions[state]...)
}

// IsTerminal reports whether state was declared as a terminal.
// Unknown states return false.
func (g *Graph[S, A]) IsTerminal(state S) bool {
	_, ok := g.terminal[state]
	return ok
}
