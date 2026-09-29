package attackable_test

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
)

// fakePlayable is a player, or a summon when owner is set.
type fakePlayable struct {
	attackabletest.Combatant
	level, karma      int
	blessed, pvpZone  bool
	olympiad, started bool
	cursedWeapon      bool
	owner             *fakePlayable
}

func (p *fakePlayable) Position() (x, y, z int)  { return 0, 0, 0 }
func (p *fakePlayable) Heading() int             { return 0 }
func (p *fakePlayable) Level() int               { return p.level }
func (p *fakePlayable) ProtectionBlessing() bool { return p.blessed }
func (p *fakePlayable) InPvPZone() bool          { return p.pvpZone }
func (p *fakePlayable) OlympiadMode() bool       { return p.olympiad }
func (p *fakePlayable) OlympiadStarted() bool    { return p.started }
func (p *fakePlayable) CursedWeaponEquipped() bool {
	return p.cursedWeapon
}

func (p *fakePlayable) Kind() actor.Kind {
	if p.owner != nil {
		return actor.KindSummon
	}
	return actor.KindPlayer
}

func (p *fakePlayable) Karma() int {
	if p.owner != nil {
		return p.owner.Karma()
	}
	return p.karma
}

func (p *fakePlayable) Owner() (attackable.Combatant, bool) {
	if p.owner == nil {
		return nil, false
	}
	return p.owner, true
}

// fakeNPC is a monster attacker: it carries karma and a level, but is not a
// playable.
type fakeNPC struct{ fakePlayable }

func (*fakeNPC) Kind() actor.Kind { return actor.KindNPC }

func TestPlayableRefuses(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		target   func() *fakePlayable
		attacker func() attackable.Combatant
		want     bool
	}{
		{
			name:     "karma player 10 levels above a blessed player",
			target:   func() *fakePlayable { return &fakePlayable{level: 10, blessed: true} },
			attacker: func() attackable.Combatant { return &fakePlayable{level: 20, karma: 1} },
			want:     true,
		},
		{
			name:     "karma player 9 levels above a blessed player",
			target:   func() *fakePlayable { return &fakePlayable{level: 10, blessed: true} },
			attacker: func() attackable.Combatant { return &fakePlayable{level: 19, karma: 1} },
		},
		{
			name:     "karma-free player 10 levels above a blessed player",
			target:   func() *fakePlayable { return &fakePlayable{level: 10, blessed: true} },
			attacker: func() attackable.Combatant { return &fakePlayable{level: 20} },
		},
		{
			name:     "blessed player on a karma player 10 levels above it",
			target:   func() *fakePlayable { return &fakePlayable{level: 30, karma: 1} },
			attacker: func() attackable.Combatant { return &fakePlayable{level: 20, blessed: true} },
			want:     true,
		},
		{
			name:     "blessed target inside a PvP zone",
			target:   func() *fakePlayable { return &fakePlayable{level: 10, blessed: true, pvpZone: true} },
			attacker: func() attackable.Combatant { return &fakePlayable{level: 30, karma: 1} },
		},
		{
			name: "karma owner's summon 10 levels above a blessed player",
			target: func() *fakePlayable {
				return &fakePlayable{level: 10, blessed: true}
			},
			attacker: func() attackable.Combatant {
				return &fakePlayable{level: 20, owner: &fakePlayable{level: 1, karma: 1}}
			},
			want: true,
		},
		{
			name: "blessed summon uses its own level, not its owner's",
			target: func() *fakePlayable {
				return &fakePlayable{level: 10, blessed: true, owner: &fakePlayable{level: 40}}
			},
			attacker: func() attackable.Combatant { return &fakePlayable{level: 20, karma: 1} },
			want:     true,
		},
		{
			name:     "monster on a blessed player",
			target:   func() *fakePlayable { return &fakePlayable{level: 10, blessed: true} },
			attacker: func() attackable.Combatant { return &fakeNPC{fakePlayable{level: 30, karma: 1}} },
		},
		{
			name:     "target in an Olympiad match not started",
			target:   func() *fakePlayable { return &fakePlayable{level: 40, olympiad: true} },
			attacker: func() attackable.Combatant { return &fakePlayable{level: 40} },
			want:     true,
		},
		{
			name:     "Olympiad refusal comes before the PvP zone",
			target:   func() *fakePlayable { return &fakePlayable{level: 40, olympiad: true, pvpZone: true} },
			attacker: func() attackable.Combatant { return &fakePlayable{level: 40} },
			want:     true,
		},
		{
			name:     "target in a started Olympiad match",
			target:   func() *fakePlayable { return &fakePlayable{level: 40, olympiad: true, started: true} },
			attacker: func() attackable.Combatant { return &fakePlayable{level: 40} },
		},
		{
			name: "summon of an owner in an Olympiad match not started",
			target: func() *fakePlayable {
				return &fakePlayable{level: 40, owner: &fakePlayable{level: 40, olympiad: true}}
			},
			attacker: func() attackable.Combatant { return &fakePlayable{level: 40} },
			want:     true,
		},
		{
			name:     "cursed-weapon holder attacked by a level 20 player",
			target:   func() *fakePlayable { return &fakePlayable{level: 60, cursedWeapon: true} },
			attacker: func() attackable.Combatant { return &fakePlayable{level: 20} },
			want:     true,
		},
		{
			name:     "cursed-weapon holder attacked by a level 21 player",
			target:   func() *fakePlayable { return &fakePlayable{level: 60, cursedWeapon: true} },
			attacker: func() attackable.Combatant { return &fakePlayable{level: 21} },
		},
		{
			name:   "cursed-weapon holder's summon on a level 20 player",
			target: func() *fakePlayable { return &fakePlayable{level: 20} },
			attacker: func() attackable.Combatant {
				return &fakePlayable{level: 60, owner: &fakePlayable{level: 60, cursedWeapon: true}}
			},
			want: true,
		},
		{
			name:     "cursed-weapon holder inside a PvP zone",
			target:   func() *fakePlayable { return &fakePlayable{level: 20, pvpZone: true} },
			attacker: func() attackable.Combatant { return &fakePlayable{level: 60, cursedWeapon: true} },
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := attackable.PlayableRefuses(tt.target(), tt.attacker()); got != tt.want {
				t.Fatalf("PlayableRefuses = %v, want %v", got, tt.want)
			}
		})
	}
}
