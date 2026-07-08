package network

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/link"
)

func TestStateStringNamesEachState(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateConnected, "connected"},
		{StateAuthed, "authed"},
		{StateEntering, "entering"},
		{StateInGame, "in-game"},
		{State(99), "state(99)"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", int(tt.state), got, tt.want)
		}
	}
}

func TestAllowedGatesOpcodesByState(t *testing.T) {
	tests := []struct {
		name   string
		state  State
		opcode byte
		want   bool
	}{
		{"connected accepts protocol version", StateConnected, 0x00, true},
		{"connected accepts login", StateConnected, 0x08, true},
		{"connected rejects create character before auth", StateConnected, 0x0b, false},
		{"connected rejects enter world", StateConnected, 0x03, false},

		{"authed accepts create character", StateAuthed, 0x0b, true},
		{"authed accepts select character", StateAuthed, 0x0d, true},
		{"authed accepts logout", StateAuthed, 0x09, true},
		{"authed rejects protocol version replay", StateAuthed, 0x00, false},
		{"authed rejects enter world before slot chosen", StateAuthed, 0x03, false},

		{"entering accepts enter world", StateEntering, 0x03, true},
		{"entering accepts quest list", StateEntering, 0x3f, true},
		{"entering rejects create character", StateEntering, 0x0b, false},

		{"in-game accepts logout", StateInGame, 0x09, true},
		{"in-game rejects create character", StateInGame, 0x0b, false},
		{"in-game rejects enter world replay", StateInGame, 0x03, false},

		{"unknown opcode rejected in every state", StateConnected, 0xFF, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Allowed(tt.state, tt.opcode); got != tt.want {
				t.Errorf("Allowed(%s, 0x%02x) = %v, want %v", tt.state, tt.opcode, got, tt.want)
			}
		})
	}
}

func TestClientStartsConnected(t *testing.T) {
	c := NewClient(nil)
	if got := c.State(); got != StateConnected {
		t.Fatalf("new client state = %s, want %s", got, StateConnected)
	}
	if name := c.AccountName(); name != "" {
		t.Fatalf("new client account name = %q, want empty", name)
	}
}

func TestClientAcceptRejectsCreateCharacterBeforeAuth(t *testing.T) {
	c := NewClient(nil)

	if c.Accept(0x0b) {
		t.Fatal("Accept(create character) = true before auth, want false")
	}

	c.SetAuthenticated("player1", link.SessionKey{})

	if !c.Accept(0x0b) {
		t.Fatal("Accept(create character) = false after auth, want true")
	}
}

func TestClientSetAuthenticatedAdvancesState(t *testing.T) {
	c := NewClient(nil)
	key := link.SessionKey{LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4}

	c.SetAuthenticated("player1", key)

	if got := c.State(); got != StateAuthed {
		t.Fatalf("state after SetAuthenticated = %s, want %s", got, StateAuthed)
	}
	if got := c.AccountName(); got != "player1" {
		t.Fatalf("account name after SetAuthenticated = %q, want %q", got, "player1")
	}
	if got := c.SessionKey(); got != key {
		t.Fatalf("session key after SetAuthenticated = %+v, want %+v", got, key)
	}
}

func TestClientStateTransitionsThroughLifecycle(t *testing.T) {
	c := NewClient(nil)

	steps := []State{StateConnected, StateAuthed, StateEntering, StateInGame}
	for i, want := range steps {
		if i > 0 {
			c.SetState(want)
		}
		if got := c.State(); got != want {
			t.Fatalf("step %d: state = %s, want %s", i, got, want)
		}
	}
}

func TestClientStateIsSafeForConcurrentAccess(t *testing.T) {
	c := NewClient(nil)

	var wg sync.WaitGroup
	states := []State{StateConnected, StateAuthed, StateEntering, StateInGame}
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			c.SetState(states[i%len(states)])
		}(i)
		go func() {
			defer wg.Done()
			c.State()
		}()
	}
	wg.Wait()
}
