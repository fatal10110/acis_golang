package festival

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

var party = []Member{{ObjectID: 1, Name: "Ann"}, {ObjectID: 2, Name: "Bob"}, {ObjectID: 3, Name: "Cid"}}

func (h *scheduleHarness) signUp(cabal sevensigns.Cabal, id int, members []Member) {
	h.m.mu.Lock()
	h.m.setParticipantsLocked(cabal, id, members)
	h.m.mu.Unlock()
}

// The competition ending while a festival is under way lets it run to its
// end, its party then waiting to register its score; no cycle follows.
func TestCompetitionEndLetsTheFestivalUnderWayFinish(t *testing.T) {
	h := newScheduleHarness(t)
	h.m.Start()
	h.signUp(sevensigns.Dawn, 0, party)
	h.fire(t) // the first cycle
	h.fire(t) // the signup ends: the festival begins
	if !h.m.Initialized() || h.lastDelay() != 2*time.Minute {
		t.Fatalf("after the signup: initialized %v, next step in %v", h.m.Initialized(), h.lastDelay())
	}
	h.m.CompetitionEnded()
	for _, want := range []time.Duration{3 * time.Minute, 4 * time.Minute, 3 * time.Minute, 3 * time.Minute, 3 * time.Minute} {
		h.fire(t)
		if h.lastDelay() != want {
			t.Fatalf("next step in %v, want %v", h.lastDelay(), want)
		}
	}
	armed := len(h.pending)
	h.fire(t) // the end
	if h.m.Initialized() || len(h.pending) != armed {
		t.Fatalf("after the end: initialized %v, %d timers armed (was %d)", h.m.Initialized(), len(h.pending), armed)
	}
	if got := h.m.previous[side(sevensigns.Dawn)][0]; !slices.Equal(got, party) {
		t.Fatalf("party waiting to register = %v, want %v", got, party)
	}
}

// A participant's party edit reaches its festival only where the search
// for its festival looks: Dawn's festivals 0, 2 and 4 and Dusk's 1 and 3.
// Elsewhere the participant stays signed up as it was. A party left with
// fewer than MinPlayers members drops its run once the participant is in
// it, and asks for the participant's expulsion either way.
func TestUpdateParticipants(t *testing.T) {
	for _, tt := range []struct {
		cabal sevensigns.Cabal
		id    int
		found bool
	}{
		{sevensigns.Dawn, 0, true},
		{sevensigns.Dawn, 1, false},
		{sevensigns.Dawn, 2, true},
		{sevensigns.Dawn, 3, false},
		{sevensigns.Dawn, 4, true},
		{sevensigns.Dusk, 0, false},
		{sevensigns.Dusk, 1, true},
		{sevensigns.Dusk, 2, false},
		{sevensigns.Dusk, 3, true},
		{sevensigns.Dusk, 4, false},
	} {
		h := newScheduleHarness(t)
		h.m.cfg.MinPlayers = 2
		h.m.Start()
		h.signUp(tt.cabal, tt.id, party)
		stay := party[:2]
		if expel := h.m.UpdateParticipants(3, stay); expel {
			t.Errorf("%v %d: a party of %d asked for an expulsion", tt.cabal, tt.id, len(stay))
		}
		want := party
		if tt.found {
			want = stay
		}
		if got := h.m.participants[side(tt.cabal)][tt.id]; !slices.Equal(got, want) {
			t.Errorf("%v %d: signed up %v, want %v", tt.cabal, tt.id, got, want)
		}
	}

	h := newScheduleHarness(t)
	h.m.Start()
	h.signUp(sevensigns.Dawn, 0, party)
	h.m.cfg.MinPlayers = 3
	if !h.m.UpdateParticipants(3, party[:2]) {
		t.Fatal("a party left below the minimum asked for no expulsion")
	}
	if got := h.m.participants[side(sevensigns.Dawn)][0]; !slices.Equal(got, party[:2]) {
		t.Fatalf("the leaver's party = %v, want the two who stay", got)
	}
	if !h.m.UpdateParticipants(1, party[:2]) {
		t.Fatal("a dispersal below the minimum asked for no expulsion")
	}
	got, taken := h.m.participants[side(sevensigns.Dawn)][0]
	if !taken || got != nil || h.m.IsParticipant(1) {
		t.Fatalf("after the dispersal: %v (taken %v), want the festival taken by no one", got, taken)
	}
}

// fakeNPC is an NPC a fakeWorld placed.
type fakeNPC int32

func (n fakeNPC) ObjectID() int32 { return int32(n) }

// fakeWorld places NPCs and records which it deleted; onSpawn, when set,
// runs as the next monster is placed.
type fakeWorld struct {
	next     int32
	spawned  []NPC
	unspawns map[NPC]int
	onSpawn  func()
}

func (w *fakeWorld) place() NPC {
	w.next++
	return fakeNPC(w.next)
}

func (w *fakeWorld) RewardClanOf(string, int)         {}
func (w *fakeWorld) RemoveBloodOfferings()            {}
func (w *fakeWorld) Shout(string, string)             {}
func (w *fakeWorld) Locate(int32) (Spot, bool)        { return Spot{}, false }
func (w *fakeWorld) Admit(int32, int, int, int)       {}
func (w *fakeWorld) Relocate(int32, *Spot)            {}
func (w *fakeWorld) Tell(int32, string)               {}
func (w *fakeWorld) Settle([]int32)                   {}
func (w *fakeWorld) SpawnWitch(int, Spot) (NPC, bool) { return w.place(), true }
func (w *fakeWorld) Say(NPC, string)                  {}
func (w *fakeWorld) Swarm(NPC, int, int, int)         {}
func (w *fakeWorld) Unspawn(n NPC)                    { w.unspawns[n]++ }
func (w *fakeWorld) SpawnMonster(int, int, int, int, int) (NPC, bool) {
	if fn := w.onSpawn; fn != nil {
		w.onSpawn = nil
		fn()
	}
	n := w.place()
	w.spawned = append(w.spawned, n)
	return n, true
}

// A challenge whose wave is still being placed when the festival ends
// leaves none of it behind: each monster placed after the end is deleted
// as soon as it is placed.
func TestChallengeWaveAfterTheEndIsDeleted(t *testing.T) {
	h := newScheduleHarness(t)
	w := &fakeWorld{unspawns: map[NPC]int{}}
	h.m.SetWorld(w)
	h.m.Start()
	h.signUp(sevensigns.Dawn, 0, party)
	for range 7 { // the cycle, the signup, the waves and the chests
		h.fire(t)
	}
	before := len(w.spawned)
	w.onSpawn = func() { h.fire(t) } // the end comes as the wave starts
	if increased, ok := h.m.IncreaseChallenge(sevensigns.Dawn, 0); !increased || !ok {
		t.Fatalf("IncreaseChallenge = %v, %v; want the arena's challenge raised", increased, ok)
	}
	wave := w.spawned[before:]
	if len(wave) == 0 {
		t.Fatal("the challenge placed nothing")
	}
	for _, n := range wave {
		if w.unspawns[n] != 1 {
			t.Fatalf("challenge monster %d deleted %d times, want once", n.ObjectID(), w.unspawns[n])
		}
	}
	for _, n := range w.spawned[:before] {
		if w.unspawns[n] != 1 {
			t.Fatalf("festival monster %d deleted %d times at the end, want once", n.ObjectID(), w.unspawns[n])
		}
	}
}
