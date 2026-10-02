package party

import "testing"

// lootParty forms a party of ms led by ms[0] under rule.
func lootParty(t *testing.T, r *Registry[*member], rule LootRule, ms ...*member) {
	t.Helper()
	if status, _ := r.BeginInvite(ms[0].id, int32(rule)); status != InviteReady {
		t.Fatalf("BeginInvite = %v", status)
	}
	r.Answer(ms[0], ms[1], true)
	for _, m := range ms[2:] {
		if status, _ := r.BeginInvite(ms[0].id, 0); status != InviteReady {
			t.Fatalf("BeginInvite = %v", status)
		}
		r.Answer(ms[0], m, true)
	}
}

func everyone(*member) bool { return true }

func TestLooterOutsideParty(t *testing.T) {
	r := NewRegistry[*member](nil)
	ms := newMembers(1)
	if _, _, ok := r.Looter(ms[0].id, false, everyone); ok {
		t.Fatal("a player in no party has a looter")
	}
}

// TestLooterFindersKeepers keeps every item, spoil or not, with the
// picker.
func TestLooterFindersKeepers(t *testing.T) {
	r := NewRegistry[*member](nil)
	ms := newMembers(3)
	lootParty(t, r, LootFindersKeepers, ms...)
	for _, spoil := range []bool{false, true} {
		looter, members, ok := r.Looter(ms[1].id, spoil, everyone)
		if !ok || looter != ms[1] || !sameMembers(members, ms...) {
			t.Fatalf("spoil %v: looter = %v %v, members %v; want the picker", spoil, looter, ok, members)
		}
	}
}

// TestLooterByTurn walks the party's turn from the member after the last
// looter, skipping ineligible members, and leaves spoil with the picker
// unless the rule includes it. The turn is the party's, whoever picks.
func TestLooterByTurn(t *testing.T) {
	r := NewRegistry[*member](nil)
	ms := newMembers(3)
	lootParty(t, r, LootByTurn, ms...)

	want := []*member{ms[1], ms[2], ms[0], ms[1]}
	for i, w := range want {
		picker := ms[i%3]
		if looter, _, _ := r.Looter(picker.id, false, everyone); looter != w {
			t.Fatalf("turn %d: looter = %s, want %s", i, looter.name, w.name)
		}
	}
	if looter, _, _ := r.Looter(ms[0].id, true, everyone); looter != ms[0] {
		t.Fatalf("spoil looter = %s, want the picker under a rule without spoil", looter.name)
	}
	// The spoil did not move the turn: ms[2] is next, but it is not
	// eligible, so ms[0] takes it.
	notC := func(m *member) bool { return m != ms[2] }
	if looter, _, _ := r.Looter(ms[1].id, false, notC); looter != ms[0] {
		t.Fatalf("looter = %s, want A after skipping C", looter.name)
	}
	// Nobody eligible: the picker keeps it, and the turn comes back where
	// it was.
	if looter, _, _ := r.Looter(ms[2].id, false, func(*member) bool { return false }); looter != ms[2] {
		t.Fatalf("looter = %s, want the picker when nobody is eligible", looter.name)
	}
	if looter, _, _ := r.Looter(ms[2].id, false, everyone); looter != ms[1] {
		t.Fatalf("looter = %s, want B after a full turn found nobody", looter.name)
	}
}

func TestLooterByTurnIncludingSpoil(t *testing.T) {
	r := NewRegistry[*member](nil)
	ms := newMembers(2)
	lootParty(t, r, LootByTurnIncludingSpoil, ms...)
	if looter, _, _ := r.Looter(ms[0].id, true, everyone); looter != ms[1] {
		t.Fatalf("spoil looter = %s, want B in turn", looter.name)
	}
}

// TestLooterRandom draws among the eligible members only, in join order,
// and leaves spoil with the picker unless the rule includes it.
func TestLooterRandom(t *testing.T) {
	for _, tc := range []struct {
		rule      LootRule
		spoil     bool
		wantDrawn bool
	}{
		{LootRandom, false, true},
		{LootRandom, true, false},
		{LootRandomIncludingSpoil, true, true},
		{LootRandomIncludingSpoil, false, true},
	} {
		r := NewRegistry[*member](nil)
		var drawnFrom int
		r.intn = func(n int) int { drawnFrom = n; return n - 1 }
		ms := newMembers(4)
		lootParty(t, r, tc.rule, ms...)
		notD := func(m *member) bool { return m != ms[3] }
		looter, _, _ := r.Looter(ms[0].id, tc.spoil, notD)
		if !tc.wantDrawn {
			if looter != ms[0] || drawnFrom != 0 {
				t.Fatalf("rule %d spoil %v: looter = %s drawn from %d, want the picker undrawn", tc.rule, tc.spoil, looter.name, drawnFrom)
			}
			continue
		}
		if drawnFrom != 3 || looter != ms[2] {
			t.Fatalf("rule %d spoil %v: looter = %s drawn from %d, want C, the last of 3 eligible", tc.rule, tc.spoil, looter.name, drawnFrom)
		}
	}
}

func TestLooterRandomNobodyEligible(t *testing.T) {
	r := NewRegistry[*member](nil)
	r.intn = func(int) int { t.Fatal("drew among no candidates"); return 0 }
	ms := newMembers(2)
	lootParty(t, r, LootRandom, ms...)
	if looter, _, _ := r.Looter(ms[1].id, false, func(*member) bool { return false }); looter != ms[1] {
		t.Fatalf("looter = %s, want the picker", looter.name)
	}
}
