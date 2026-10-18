package script

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/rs/zerolog"
)

// playerCombatant is a player the world tracks under objectID.
type playerCombatant struct{ combatant }

func (*playerCombatant) Kind() actor.Kind { return actor.KindPlayer }

// A handle on a civilian NPC reports the creature it was spawned for, a
// player's handle, and reports the NPC gone once it decays.
func TestNPCHandleSummonerAndDecayed(t *testing.T) {
	summoner := &playerCombatant{combatant{objectID: 7}}
	inst, err := npc.NewInstance(100, &npc.Template{ID: 30001, TemplateID: 30001, Type: "Merchant", Name: "Grocer", Level: 1, HPMax: 10})
	if err != nil {
		t.Fatal(err)
	}
	inst.Summoner = summoner
	f, err := npc.NewFolk(inst)
	if err != nil {
		t.Fatal(err)
	}

	h := NPCOf(f)
	if h == nil || h.mind() != brain(f) {
		t.Fatalf("folk handle = %+v, want one whose brain is the NPC's own AI", h)
	}
	p, ok := h.Summoner().(*Player)
	if !ok || combatantOf(p) != summoner {
		t.Fatalf("summoner = %#v, want the player handle on the summoner", h.Summoner())
	}
	if h.Decayed() {
		t.Fatal("live NPC reports gone")
	}
	f.Decay(nil, nil)
	if !h.Decayed() {
		t.Fatal("decayed NPC reports alive")
	}

	inst.Summoner = nil
	if s := NPCOf(f).Summoner(); s != nil {
		t.Fatalf("summoner of an NPC spawned for no one = %#v, want nil", s)
	}
	var none *NPC
	if !none.Decayed() || none.Summoner() != nil || NPCOf(nil) != nil || PlayerOf(nil) != nil {
		t.Fatal("a handle on nothing is not empty")
	}
}

// Scheduling the despawn of a handle on nothing panics, as the place of
// one does: the invocation aborts there.
func TestScheduleDespawnOfNothingPanics(t *testing.T) {
	s := NewSpawner(nil, zerolog.Nop())
	for name, h := range map[string]*NPC{"nil handle": nil, "handle of nil": NPCOf(nil)} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s: ScheduleDespawn did not panic", name)
				}
			}()
			s.ScheduleDespawn(h, time.Second)
		}()
	}
}

// A spawner built over no population places nothing until it is bound,
// and is bound once only.
func TestSpawnerBindOnce(t *testing.T) {
	s := NewSpawner(nil, zerolog.Nop())
	mustPanic := func(what string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Fatalf("%s did not panic", what)
			}
		}()
		fn()
	}
	mustPanic("spawn through an unbound spawner", func() { s.AddSpawn(1, Loc{}, false, 0) })
	mustPanic("bind to no population", func() { s.Bind(nil) })
	s.Bind(nopPopulation{})
	if got := s.AddSpawn(1, Loc{}, false, 0); got != nil {
		t.Fatalf("spawn placed %v, want nothing", got)
	}
	mustPanic("second bind", func() { s.Bind(nopPopulation{}) })
}

// nopPopulation places nothing: every NPC id lacks a template.
type nopPopulation struct{}

func (nopPopulation) AddSpawn(int32, int, int, int, int, bool, time.Duration) (attackable.Combatant, error) {
	return nil, npc.ErrNoTemplate
}

func (nopPopulation) CreatePrivate(npc.PartyMember, int32, *location.Location, int, time.Duration, [3]int32) (npc.PartyMember, error) {
	return nil, npc.ErrNoTemplate
}

func (nopPopulation) CreatePrivates(npc.PartyMember) error { return nil }

func (nopPopulation) ScheduleDespawn(attackable.Combatant, time.Duration) {}

func (nopPopulation) RespawnDelay(*npc.Hostile) time.Duration { return 0 }

func (nopPopulation) ScheduleRespawn(*npc.Hostile, time.Duration) error { return nil }

func (nopPopulation) DecayMe(*npc.Hostile) {}

func (nopPopulation) MakerEvent(string, string, int, int) bool { return false }

func (nopPopulation) DoRespawn(npc.PartyMember) {}

func (nopPopulation) DrawSpawnPoint(npc.PartyMember) (location.Location, int, bool) {
	return location.Location{}, 0, false
}

func (nopPopulation) SetRespawnDelay(npc.PartyMember, time.Duration) error { return nil }
