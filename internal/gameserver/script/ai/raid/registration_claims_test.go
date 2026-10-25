package raid

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/rs/zerolog"
)

// TestUnportedChildKeepsItsIDOnTheStandIns registers the chain with
// DrChaosGiganticGolem listed but not ported, and its reference claim on
// 25512: 25512 keeps its stand-ins, and the other ids RaidBossType1 binds
// run RaidBossType1.
func TestUnportedChildKeepsItsIDOnTheStandIns(t *testing.T) {
	const golem = "script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1Aggressive.DrChaosGiganticGolem"
	list := make([]script.Listing, len(chain))
	catalog := script.Catalog{}
	for i, c := range chain {
		list[i] = script.Listing{Path: c.path}
		if c.path != golem {
			catalog[c.path] = c.ctor
		}
	}
	// The bindings RaidBossType1 has on 25512, which the golem takes.
	claim := script.Bindings{}
	for _, ev := range []script.NPCEvent{script.EventAttacked, script.EventAttackFinished, script.EventCreated, script.EventSeeSpell} {
		claim[ev] = []int32{25512}
	}
	hostile := func(int32) (script.NPCKind, bool) { return script.KindHostile, true }
	r := script.Build(list, catalog, script.Config{KindOf: hostile, Log: zerolog.Nop(), Claims: map[string]script.Bindings{golem: claim}})
	if r.Behaves(25512) {
		t.Error("Behaves(25512) = true with DrChaosGiganticGolem unported, want the stand-ins")
	}
	ids := BossType1().NPCs
	if len(ids) != 39 {
		t.Fatalf("RaidBossType1 binds %d ids, want 39", len(ids))
	}
	for _, id := range ids {
		if id != 25512 && !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want RaidBossType1", id)
		}
	}
}

// TestUnportedAndreasKeepsItsIDOnTheStandIns registers the chain with
// RaidBossType4AndreasAggressive listed but not ported, and its reference
// claim on 29062: 29062 keeps its stand-ins, and the other ids
// RaidBossType4 binds run RaidBossType4.
func TestUnportedAndreasKeepsItsIDOnTheStandIns(t *testing.T) {
	const andreas = "script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType4AndreasAggressive"
	list := make([]script.Listing, len(chain))
	catalog := script.Catalog{}
	for i, c := range chain {
		list[i] = script.Listing{Path: c.path}
		if c.path != andreas {
			catalog[c.path] = c.ctor
		}
	}
	// The bindings RaidBossType4 has on 29062, which Andreas takes.
	claim := script.Bindings{}
	for _, ev := range []script.NPCEvent{script.EventAttacked, script.EventAttackFinished, script.EventCreated, script.EventPartyAttacked, script.EventPartyDied, script.EventSeeSpell} {
		claim[ev] = []int32{29062}
	}
	hostile := func(int32) (script.NPCKind, bool) { return script.KindHostile, true }
	r := script.Build(list, catalog, script.Config{KindOf: hostile, Log: zerolog.Nop(), Claims: map[string]script.Bindings{andreas: claim}})
	if r.Behaves(29062) {
		t.Error("Behaves(29062) = true with RaidBossType4AndreasAggressive unported, want the stand-ins")
	}
	ids := BossType4().NPCs
	if len(ids) != 45 {
		t.Fatalf("RaidBossType4 binds %d ids, want 45", len(ids))
	}
	for _, id := range ids {
		if id != 29062 && !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want RaidBossType4", id)
		}
	}
}
