package skill

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// recallPlayer is a player cast participant carrying every state the
// recall handler reads, recording where it was sent.
type recallPlayer struct {
	skillTarget
	afraid, olympiad, bossZone, festival bool
	jailed, duel, riding, flying         bool

	teleports [][4]int
	recalls   []modelskill.RecallType
	// in7sDungeon is the dungeon membership; leftDungeonFirst records
	// whether it was cleared before the first move.
	in7sDungeon, leftDungeonFirst bool
}

func (p *recallPlayer) Afraid() bool                      { return p.afraid }
func (p *recallPlayer) OlympiadMode() bool                { return p.olympiad }
func (p *recallPlayer) InBossZone() bool                  { return p.bossZone }
func (p *recallPlayer) FestivalParticipant() bool         { return p.festival }
func (p *recallPlayer) Jailed() bool                      { return p.jailed }
func (p *recallPlayer) InDuel() bool                      { return p.duel }
func (p *recallPlayer) Riding() bool                      { return p.riding }
func (p *recallPlayer) Flying() bool                      { return p.flying }
func (p *recallPlayer) Recall(dest modelskill.RecallType) { p.recalls = append(p.recalls, dest) }
func (p *recallPlayer) TeleportTo(x, y, z, radius int) {
	p.teleports = append(p.teleports, [4]int{x, y, z, radius})
}

func (p *recallPlayer) SetIn7sDungeon(in bool) {
	if !in && p.in7sDungeon && !p.moved() {
		p.leftDungeonFirst = true
	}
	p.in7sDungeon = in
}

func (p *recallPlayer) moved() bool { return len(p.teleports)+len(p.recalls) > 0 }

func newRecallPlayer(id int32) *recallPlayer {
	p := &recallPlayer{}
	p.objectID = id
	p.isPlayer = true
	return p
}

func recall(caster Creature, def modelskill.Definition, targets ...Actor) {
	def.SkillType = "RECALL"
	NewDefaultRegistry().Use(Cast{Caster: caster, Skill: def, Targets: targets})
}

// TestRecallSendsTargetsByRecallType pins L2SkillTeleport's destination:
// without teleCoords each player target is recalled by the skill's
// recallType, town by default, for TELEPORT as for RECALL.
func TestRecallSendsTargetsByRecallType(t *testing.T) {
	for _, tc := range []struct {
		skillType string
		recall    modelskill.RecallType
	}{
		{"RECALL", modelskill.RecallTown},
		{"TELEPORT", modelskill.RecallTown},
		{"RECALL", modelskill.RecallCastle},
		{"RECALL", modelskill.RecallClanHall},
	} {
		caster := newRecallPlayer(1)
		member := newRecallPlayer(2)
		NewDefaultRegistry().Use(Cast{
			Caster:  caster,
			Skill:   modelskill.Definition{SkillType: tc.skillType, RecallType: tc.recall},
			Targets: []Actor{caster, member},
		})
		for _, p := range []*recallPlayer{caster, member} {
			if !slices.Equal(p.recalls, []modelskill.RecallType{tc.recall}) || len(p.teleports) != 0 {
				t.Fatalf("%s %v: player %d recalls %v teleports %v, want one recall", tc.skillType, tc.recall, p.objectID, p.recalls, p.teleports)
			}
		}
	}
}

// TestRecallTeleCoordsWinOverRecallType pins the teleCoords priority: a
// skill with coordinates teleports there with a 20 offset, whatever its
// recallType.
func TestRecallTeleCoordsWinOverRecallType(t *testing.T) {
	caster := newRecallPlayer(1)
	at := location.Location{X: -84200, Y: 244544, Z: -3728}
	recall(caster, modelskill.Definition{RecallType: modelskill.RecallCastle, TeleCoords: &at}, caster)
	if want := [][4]int{{-84200, 244544, -3728, 20}}; !slices.Equal(caster.teleports, want) || len(caster.recalls) != 0 {
		t.Fatalf("teleports %v recalls %v, want %v and no recall", caster.teleports, caster.recalls, want)
	}
}

// TestRecallCasterGate pins the player caster's refusals: afraid, at an
// Olympiad match or in a boss zone, nobody moves and the spiritshot stays
// charged.
func TestRecallCasterGate(t *testing.T) {
	for name, set := range map[string]func(*recallPlayer){
		"afraid":    func(p *recallPlayer) { p.afraid = true },
		"olympiad":  func(p *recallPlayer) { p.olympiad = true },
		"boss zone": func(p *recallPlayer) { p.bossZone = true },
	} {
		caster := newRecallPlayer(1)
		member := newRecallPlayer(2)
		set(caster)
		caster.charged = map[item.ShotKind]bool{item.ShotSpirit: true}
		recall(caster, modelskill.Definition{}, caster, member)
		if caster.moved() || member.moved() {
			t.Fatalf("%s caster: someone moved", name)
		}
		if len(caster.shots) != 0 {
			t.Fatalf("%s caster: spent shots %v, want none", name, caster.shots)
		}
	}
}

// TestRecallTargetGate pins the per-target refusals: festival, jail, duel,
// strider and wyvern keep any target in place, the caster included; the
// Olympiad and a boss zone keep only a target other than the caster.
func TestRecallTargetGate(t *testing.T) {
	for _, tc := range []struct {
		name       string
		set        func(*recallPlayer)
		casterStay bool
	}{
		{"festival", func(p *recallPlayer) { p.festival = true }, true},
		{"jailed", func(p *recallPlayer) { p.jailed = true }, true},
		{"duel", func(p *recallPlayer) { p.duel = true }, true},
		{"riding", func(p *recallPlayer) { p.riding = true }, true},
		{"flying", func(p *recallPlayer) { p.flying = true }, true},
		{"olympiad", func(p *recallPlayer) { p.olympiad = true }, false},
		{"boss zone", func(p *recallPlayer) { p.bossZone = true }, false},
	} {
		// Gated as another player's target.
		caster := newRecallPlayer(1)
		member := newRecallPlayer(2)
		other := newRecallPlayer(3)
		tc.set(member)
		recall(caster, modelskill.Definition{}, caster, member, other)
		if member.moved() || !caster.moved() || !other.moved() {
			t.Fatalf("%s member: caster %v member %v other %v, want only the member kept", tc.name, caster.moved(), member.moved(), other.moved())
		}

		// The same state on a caster recalling itself. The Olympiad and
		// boss zone refuse it as a caster; its own target check skips them.
		if !tc.casterStay {
			continue
		}
		self := newRecallPlayer(1)
		tc.set(self)
		recall(self, modelskill.Definition{}, self)
		if self.moved() {
			t.Fatalf("%s caster: recalled itself, want kept", tc.name)
		}
	}
}

// TestRecallSkipsNonPlayers pins that only player targets move: an NPC
// target is ignored, a player after it still recalled.
func TestRecallSkipsNonPlayers(t *testing.T) {
	caster := newRecallPlayer(1)
	npc := newRecallPlayer(2)
	npc.isPlayer = false
	member := newRecallPlayer(3)
	recall(caster, modelskill.Definition{}, npc, member)
	if npc.moved() || !member.moved() {
		t.Fatalf("npc moved %v, player after it moved %v; want only the player", npc.moved(), member.moved())
	}
}

// TestRecallSpendsSpiritshot pins the shot write after the target loop:
// the blessed spiritshot when one was charged at the start, otherwise the
// plain one, written with the skill's static-reuse flag, whether or not
// anyone moved.
func TestRecallSpendsSpiritshot(t *testing.T) {
	for _, tc := range []struct {
		name    string
		charged map[item.ShotKind]bool
		static  bool
		jailed  bool
		want    item.ShotKind
	}{
		{"plain", map[item.ShotKind]bool{item.ShotSpirit: true}, false, false, item.ShotSpirit},
		{"blessed", map[item.ShotKind]bool{item.ShotBlessedSpirit: true}, false, false, item.ShotBlessedSpirit},
		{"none charged", nil, false, false, item.ShotSpirit},
		{"static reuse", map[item.ShotKind]bool{item.ShotBlessedSpirit: true}, true, false, item.ShotBlessedSpirit},
		{"target kept", map[item.ShotKind]bool{item.ShotSpirit: true}, false, true, item.ShotSpirit},
	} {
		caster := newRecallPlayer(1)
		caster.charged = tc.charged
		caster.jailed = tc.jailed
		recall(caster, modelskill.Definition{StaticReuse: tc.static}, caster)
		if !slices.Equal(caster.shots, []item.ShotKind{tc.want}) || !slices.Equal(caster.shotFlags, []bool{tc.static}) {
			t.Fatalf("%s: shots %v flags %v, want %v written %v", tc.name, caster.shots, caster.shotFlags, tc.want, tc.static)
		}
	}
}
