package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ---- from resurrect_test.go ----
type reviveFakeCaster struct {
	neutralCreature
	world.Presence
	fakeActor
	kind    actor.Kind
	wit     int
	refused []event.ReviveRefusal
}

func (c *reviveFakeCaster) WIT() int              { return c.wit }
func (c *reviveFakeCaster) Kind() actor.Kind      { return c.kind }
func (c *reviveFakeCaster) CharacterName() string { return "Healer" }
func (c *reviveFakeCaster) NotifyReviveRefused(r event.ReviveRefusal) {
	c.refused = append(c.refused, r)
}

type reviveFakeTarget struct {
	world.Presence
	neutralPlayer
	fakeActor
	restoredPercent float64
	offers          []reviveOffer
}

type reviveOffer struct {
	reviver string
	power   float64
	isPet   bool
}

func (t *reviveFakeTarget) ReviveRestoringExp(restorePercent float64) bool {
	t.restoredPercent = restorePercent
	return true
}

func (t *reviveFakeTarget) ReviveRequest(reviver player.Reviver, power float64, isPet bool) {
	t.offers = append(t.offers, reviveOffer{reviver.CharacterName(), power, isPet})
}
func (*reviveFakeTarget) Kind() actor.Kind { return actor.KindPlayer }

// TestResurrectByPlayerOffersEveryTarget: a player caster's resurrection
// asks each dead player first, carrying the WIT-scaled revive power, and
// revives nobody outright.
func TestResurrectByPlayerOffersEveryTarget(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &reviveFakeCaster{kind: actor.KindPlayer, wit: 30}
	a := &reviveFakeTarget{}
	b := &reviveFakeTarget{}

	if !registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "RESURRECT", Power: 40},
		Targets: []Actor{a, b, &placedFakeActor{}},
	}) {
		t.Fatal("Use() returned false for RESURRECT")
	}

	want := reviveOffer{"Healer", formulas.RevivePower(statbonus.WITBonus[30], 40), false}
	for i, target := range []*reviveFakeTarget{a, b} {
		if len(target.offers) != 1 || target.offers[0] != want {
			t.Fatalf("target %d offers = %+v, want [%+v]", i, target.offers, want)
		}
		if target.restoredPercent != 0 {
			t.Fatalf("target %d revived outright at %v, want only the offer", i, target.restoredPercent)
		}
	}
}

// TestResurrectByNonPlayerRevivesOutright: any other caster revives a dead
// player without asking, restoring the revive power's share of lost exp.
func TestResurrectByNonPlayerRevivesOutright(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &reviveFakeCaster{kind: actor.KindNPC, wit: 30}
	a := &reviveFakeTarget{}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "RESURRECT", Power: 40},
		Targets: []Actor{a},
	})
	if want := formulas.RevivePower(statbonus.WITBonus[30], 40); a.restoredPercent != want || len(a.offers) != 0 {
		t.Fatalf("restore percent = %v, offers = %+v; want %v and no offer", a.restoredPercent, a.offers, want)
	}
}

// reviveFakeSummon records how a resurrection reached a summon.
type reviveFakeSummon struct {
	world.Presence
	fakeActor
	outright []float64
	direct   []float64
}

func (*reviveFakeSummon) Kind() actor.Kind { return actor.KindSummon }
func (s *reviveFakeSummon) ResurrectOutright(power float64) {
	s.outright = append(s.outright, power)
}

func (s *reviveFakeSummon) ReviveRestoringExp(power float64) bool {
	s.direct = append(s.direct, power)
	return true
}

// TestResurrectByNonPlayerRevivesSummonOnItsQueue: any other caster hands a
// dead summon its revive, at the WIT-scaled power, through the summon's own
// queued resurrection (which drops the decay first), and asks nobody.
func TestResurrectByNonPlayerRevivesSummonOnItsQueue(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &reviveFakeCaster{kind: actor.KindNPC, wit: 30}
	s := &reviveFakeSummon{}
	p := &reviveFakeTarget{}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "RESURRECT", Power: 40},
		Targets: []Actor{s, p},
	})
	want := formulas.RevivePower(statbonus.WITBonus[30], 40)
	if len(s.outright) != 1 || s.outright[0] != want {
		t.Fatalf("summon outright resurrections = %v, want [%v]", s.outright, want)
	}
	if len(s.direct) != 0 {
		t.Fatalf("summon revived off its queue at %v, want only the queued resurrection", s.direct)
	}
	if len(p.offers) != 0 || p.restoredPercent != want {
		t.Fatalf("player offers = %+v, restore percent = %v; want no offer and %v", p.offers, p.restoredPercent, want)
	}
}

func TestResurrectWithoutCasterIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	a := &reviveFakeTarget{}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "RESURRECT"},
		Targets: []Actor{a},
	})
	if a.restoredPercent != 0 || len(a.offers) != 0 {
		t.Fatalf("restore percent = %v, offers = %+v; want untouched", a.restoredPercent, a.offers)
	}
}
