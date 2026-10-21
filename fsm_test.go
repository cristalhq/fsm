package fsm

import (
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestNext(t *testing.T) {
	g := paymentGraph(t)

	state := g.Initial()
	if state != "new" {
		t.Fatalf("Initial() = %q", state)
	}

	for _, step := range []struct{ action, want string }{
		{"start", "pending"},
		{"refresh", "pending"},
		{"failed", "failed"},
		{"retry", "pending"},
		{"authorized", "hold"},
		{"capture", "processing"},
		{"captured", "paid"},
	} {
		next, ok := g.Next(state, step.action)
		if !ok || next != step.want {
			t.Fatalf("Next(%q, %q) = %q, %v; want %q", state, step.action, next, ok, step.want)
		}
		state = next
	}

	for _, state := range []string{"new", "paid", "missing"} {
		next, ok := g.Next(state, "unknown")
		if next != state {
			t.Errorf("Next(%q, unknown) = %q, %v", state, next, ok)
		}
	}

	if g.Initial() != "new" {
		t.Fatal("Next changed the initial state")
	}
}

func TestActionsAndTerminal(t *testing.T) {
	g := paymentGraph(t)

	want := []string{"authorized", "failed", "refresh"}
	got := g.Actions("pending")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Actions = %v; want %v", got, want)
	}

	got[0] = "corrupted"
	if !reflect.DeepEqual(g.Actions("pending"), want) {
		t.Fatal("Actions leaked storage")
	}

	for _, state := range []string{"paid", "missing"} {
		if g.Actions(state) != nil {
			t.Errorf("Actions(%q) should be nil", state)
		}
	}

	for _, state := range []string{"new", "failed", "paid", "missing"} {
		if got := g.IsTerminal(state); got != (state == "paid") {
			t.Errorf("IsTerminal(%q) = %v", state, got)
		}
	}
}

func TestNewInvalid(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cfg     Config[string, string]
		message string
	}{
		{
			"duplicate",
			Config[string, string]{
				Initial: "a",
				Transitions: []Transition[string, string]{
					{"a", "go", "b"}, {"a", "go", "c"},
				},
			},
			"duplicate transition",
		},
		{
			"identical duplicate",
			Config[string, string]{
				Initial: "a",
				Transitions: []Transition[string, string]{
					{"a", "go", "b"}, {"a", "go", "b"},
				},
			},
			"duplicate transition",
		},
		{
			"terminal edge",
			Config[string, string]{
				Initial:  "a",
				Terminal: []string{"a"},
				Transitions: []Transition[string, string]{
					{"a", "go", "a"},
				},
			},
			"outgoing transition",
		},
		{
			"duplicate terminal",
			Config[string, string]{
				Initial:  "a",
				Terminal: []string{"a", "a"},
			},
			"duplicate terminal",
		},
		{
			"unreachable terminal",
			Config[string, string]{
				Initial:  "a",
				Terminal: []string{"b"},
			},
			"unreachable",
		},
		{
			"unreachable source",
			Config[string, string]{
				Initial: "a",
				Transitions: []Transition[string, string]{
					{"b", "go", "a"},
				},
			},
			"unreachable",
		},
		{
			"disconnected cycle",
			Config[string, string]{
				Initial: "a",
				Transitions: []Transition[string, string]{
					{"b", "go", "c"}, {"c", "go", "b"},
				},
			},
			"unreachable",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, err := New(tc.cfg)
			if g != nil || err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("New = %v, %v; want %q error", g, err, tc.message)
			}
		})
	}
}

func TestDegenerateGraphs(t *testing.T) {
	for _, terminal := range [][]int{nil, {0}} {
		g, err := New(Config[int, int]{Terminal: terminal})
		if err != nil {
			t.Fatal(err)
		}
		if g.Initial() != 0 || g.Actions(0) != nil || g.IsTerminal(0) != (terminal != nil) {
			t.Fatal("incorrect single-state graph")
		}
	}

	// A zero-valued destination must not be mistaken for a missing edge.
	g, err := New(Config[int, int]{
		Initial: 1, Transitions: []Transition[int, int]{{1, 0, 0}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if next, ok := g.Next(1, 0); next != 0 || !ok {
		t.Fatalf("Next = %d, %v", next, ok)
	}
}

func TestConfigIsolation(t *testing.T) {
	cfg := paymentConfig()

	g, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	cfg.Initial = "other"
	cfg.Terminal[0] = "pending"
	cfg.Transitions[0] = Transition[string, string]{"new", "start", "broken"}

	if g.Initial() != "new" || !g.IsTerminal("paid") || g.IsTerminal("pending") {
		t.Fatal("graph retained configuration storage")
	}
	if next, ok := g.Next("new", "start"); next != "pending" || !ok {
		t.Fatalf("Next = %q, %v", next, err)
	}
}

func TestBranchesAndMultipleTerminals(t *testing.T) {
	type state uint8
	type action uint8

	g, err := New(Config[state, action]{
		Initial:  0,
		Terminal: []state{2, 3},
		Transitions: []Transition[state, action]{
			{0, 0, 1},
			{0, 1, 3},
			{1, 0, 2}, // The same action may occur in different states.
			{1, 1, 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, tr := range []Transition[state, action]{
		{0, 0, 1},
		{0, 1, 3},
		{1, 0, 2},
		{1, 1, 3},
	} {
		if next, ok := g.Next(tr.From, tr.Action); !ok || next != tr.To {
			t.Fatalf("Next(%d, %d) = %d, %v", tr.From, tr.Action, next, !ok)
		}
	}
	for _, s := range []state{2, 3} {
		if !g.IsTerminal(s) {
			t.Errorf("state %d should be terminal", s)
		}
	}
}

func TestConcurrentReads(t *testing.T) {
	g := paymentGraph(t)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 100 {
				if next, ok := g.Next("hold", "capture"); next != "processing" || !ok {
					t.Errorf("Next = %q, %v", next, ok)
				}
				actions := g.Actions("pending")
				actions[0] = "local"
				_ = g.Initial()
				_ = g.IsTerminal("paid")
			}
		})
	}
	wg.Wait()
}

func FuzzNext(f *testing.F) {
	f.Add("new", "start")
	f.Add("hold", "capture")
	f.Add("missing", "unknown")
	f.Add("pending", "refresh")

	g, err := New(paymentConfig())
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, state, action string) {
		want := state
		found := false
		for _, tr := range paymentConfig().Transitions {
			if tr.From == state && tr.Action == action {
				want, found = tr.To, true
				break
			}
		}

		got, ok := g.Next(state, action)
		if got != want {
			t.Fatalf("Next(%q, %q) = %q, %v; want %q, allowed=%v", state, action, got, ok, want, found)
		}
	})
}

func paymentGraph(t testing.TB) *Graph[string, string] {
	t.Helper()

	g, err := New(paymentConfig())
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func paymentConfig() Config[string, string] {
	return Config[string, string]{
		Initial:  "new",
		Terminal: []string{"paid"},
		Transitions: []Transition[string, string]{
			{"new", "start", "pending"},
			{"pending", "authorized", "hold"},
			{"pending", "failed", "failed"},
			{"failed", "retry", "pending"},
			{"hold", "capture", "processing"},
			{"processing", "captured", "paid"},
			{"pending", "refresh", "pending"},
		},
	}
}
