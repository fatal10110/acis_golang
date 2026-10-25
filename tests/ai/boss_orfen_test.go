package ai

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/orfen"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Orfen, Riba Iren, and a private Orfen leads here (Raikel's id, bound to
// no behavior in these tests).
const (
	orfenID        = int32(29014)
	orfenRibaID    = int32(29018)
	orfenPrivateID = int32(29015)
)

// Fixture stand-ins for the skills Orfen and Riba Iren want: paralysis,
// the raid curse, the strider slow and Orfen Heal.
const (
	orfenParalysis = modelskill.ID(4063)
	orfenCurse     = modelskill.ID(4515)
	orfenSlow      = modelskill.ID(4258)
	orfenHeal      = modelskill.ID(4516)
)

// orfenHate is the attack weight a hit of 10 adds on Orfen, of 1000 max
// MP: 10 / 1000 / 0.05 * 10000, truncated, as the hook computes it.
const orfenHate = 1999

// orfenSkills are the fixture skills, each a one-target magic attack of
// reach 600.
func orfenSkills() []modelskill.Definition {
	return []modelskill.Definition{
		ind1Skill(orfenParalysis, 600, false), ind1Skill(orfenCurse, 600, false),
		ind1Skill(orfenSlow, 600, false), ind1Skill(orfenHeal, 600, false),
	}
}

// orfenTemplate is Orfen's fixture: a level 1 grand boss of 1000 HP and
// 1000 MP that moves when walks is set, its points named later
// (setOrfenPoints).
func orfenTemplate(walks bool) *npc.Template {
	tmpl := warriorTemplate(orfenID, npc.AIParams{})
	tmpl.Type, tmpl.Name, tmpl.MPMax, tmpl.CanMove = "GrandBoss", "Orfen", 1000, walks
	return tmpl
}

// setOrfenPoints names tmpl's retreat point (b03_x1..z1) and its three
// lairs (2 to 4), each at the offset given from the character.
func (w *ind1World) setOrfenPoints(tmpl *npc.Template, points [4][2]int) {
	for i, p := range points {
		n := strconv.Itoa(i + 1)
		tmpl.AIParams["b03_x"+n] = strconv.Itoa(w.at.X + p[0])
		tmpl.AIParams["b03_y"+n] = strconv.Itoa(w.at.Y + p[1])
		tmpl.AIParams["b03_z"+n] = strconv.Itoa(w.at.Z)
	}
}

// bootOrfen boots Orfen's and Riba Iren's behaviors beside the character
// with templates, every NPC casting the fixture skills, every draw 0 until
// a test queues others. The scenarios of testdata/scenarios cover the
// lines, pulls, sounds and the hate of hits and clan calls; these tests
// cover what the scenario format cannot hold: the spawn parameters naming
// Orfen's points, its timer, its territory, a strider rider, a party and
// a heal on Orfen.
func bootOrfen(t *testing.T, templates ...*npc.Template) *ind1World {
	t.Helper()
	list := []script.Listing{
		{Path: "script.ai.individual.DefaultNpc"},
		{Path: "script.ai.boss.orfen.Orfen"},
		{Path: "script.ai.boss.orfen.OrfenPrivateRibaIren"},
	}
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":           defaultnpc.New,
		"script.ai.boss.orfen.Orfen":                orfen.Orfen,
		"script.ai.boss.orfen.OrfenPrivateRibaIren": orfen.RibaIren,
	}
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	w := bootRaid(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(orfenSkills())),
		gameservertest.WithScriptSkills(orfenSkills()...),
	}}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// wantOrfen fails unless h stands at dx, dy from the character, its flag
// and rest state read as given, and its attack desire on the character
// weighs attack.
func (w *ind1World) wantOrfen(t *testing.T, when string, h *npc.Hostile, dx, dy int, flag, retreated int32, attack float64) {
	t.Helper()
	at := positionOf(h)
	if at.X != w.at.X+dx || at.Y != w.at.Y+dy {
		t.Errorf("%s: Orfen at %d,%d from the character, want %d,%d", when, at.X-w.at.X, at.Y-w.at.Y, dx, dy)
	}
	if got := ind1Slot(h, script.SlotFlag); got != flag {
		t.Errorf("%s: flag = %d, want %d", when, got, flag)
	}
	if got := ind1Slot(h, script.SlotIntAI0); got != retreated {
		t.Errorf("%s: rest state = %d, want %d", when, got, retreated)
	}
	if d, _ := attackOn(h, w.player); d.Weight != attack {
		t.Errorf("%s: attack desire on the character = %v, want %v", when, d.Weight, attack)
	}
}

// TestOrfenRestsThenGoesBackToALair covers Orfen's created, attacked and
// timer hooks. It appears playing BS01_A bound to itself. A hit while it
// stands whole moves nothing. A hit while it stands under half its HP
// (read before the hit's damage) sends it to its retreat point, counts
// one in its flag, drops its attack desires and then adds the hit's own.
// Its check every 10 s after it appeared leaves it there while it stands
// under 95% of its HP inside its territory; once over 95%, the next check
// sends it to a lair (a draw of 40 of 100: the second of three), counts
// one more and drops its attack desires.
func TestOrfenRestsThenGoesBackToALair(t *testing.T) {
	t.Parallel()
	tmpl := orfenTemplate(false)
	w := bootOrfen(t, tmpl)
	w.setOrfenPoints(tmpl, [4][2]int{{400, 150}, {550, 0}, {250, 0}, {400, -150}})
	w.draws.set(99)
	s := w.spawnFrames(t, orfenID, 400, 0)
	h := s.h
	want := ind8MusicPayload(h.ObjectID(), "BS01_A", positionOf(h))
	if got := ind8Pick(s.frames, serverpackets.OpcodePlaySound); len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("PlaySound frames as Orfen appears = %x, want one of BS01_A bound to it", got)
	}

	w.hit(t, h, 10)
	w.wantOrfen(t, "a hit while whole", h, 400, 0, 0, 0, orfenHate)

	w.setHPShare(t, h, 0.4)
	w.hit(t, h, 10)
	w.wantOrfen(t, "a hit under half its HP", h, 400, 150, 1, 1, orfenHate)

	w.srv.Advance(t, 10*time.Second)
	w.wantOrfen(t, "the 10 s check under 95% of its HP", h, 400, 150, 1, 1, orfenHate)

	w.setHPShare(t, h, 1)
	w.draws.set(40)
	w.srv.Advance(t, 10*time.Second)
	w.wantOrfen(t, "the 20 s check over 95% of its HP", h, 250, 0, 2, 0, 0)
}

// TestOrfenComesBackToItsRetreatPoint covers the timer hook's other rest
// branch: resting under 95% of its HP outside its territory (its drift
// range around its spawn point), Orfen moved off its retreat point is sent
// back there at its next check, its flag unchanged.
func TestOrfenComesBackToItsRetreatPoint(t *testing.T) {
	t.Parallel()
	tmpl := orfenTemplate(false)
	w := bootOrfen(t, tmpl)
	w.setOrfenPoints(tmpl, [4][2]int{{1000, 0}, {550, 0}, {250, 0}, {400, -150}})
	w.draws.set(99)
	h := w.spawn(t, orfenID, 400, 0)
	w.setHPShare(t, h, 0.4)
	w.hit(t, h, 10)
	w.wantOrfen(t, "a hit under half its HP", h, 1000, 0, 1, 1, orfenHate)

	w.teleport(t, h, 0, 100)
	if h.InTerritory() {
		t.Fatal("setup: Orfen moved inside its territory, want outside")
	}
	w.srv.Advance(t, 10*time.Second)
	w.wantOrfen(t, "the 10 s check outside its territory", h, 1000, 0, 1, 1, orfenHate)
}

// orfenChase runs position ticks until h's walk ends: no AI tick runs, so
// nothing but its hooks touches its desires.
func (w *ind1World) orfenChase(t *testing.T, h *npc.Hostile) {
	t.Helper()
	for i := 0; h.IsMoving(); i++ {
		if i == 300 {
			t.Fatal("the chase never arrived")
		}
		w.srv.TickPositions()
	}
	w.srv.Settle(t)
}

// TestOrfenLeavesItsTerritory covers the out-of-territory hook: chasing
// the character out of its territory, Orfen's arrival there drops its
// attack desires, sends it to a lair (a draw of 66 of 100: the third),
// and counts one in its flag. Another Orfen, resting after a hit under
// half its HP, is sent back to its retreat point instead as it arrives
// outside, with no draw.
func TestOrfenLeavesItsTerritory(t *testing.T) {
	t.Parallel()
	tmpl := orfenTemplate(true)
	tmpl.Level = 20 // as the character: no curse cast holds it in place
	w := bootOrfen(t, tmpl)
	w.setOrfenPoints(tmpl, [4][2]int{{600, 150}, {750, 0}, {450, 0}, {600, -150}})
	w.draws.set(99)
	h := w.spawn(t, orfenID, 600, 0)

	w.draws.set(99, 66)
	w.onQueue(t, h, func() { script.NPCOf(h).AddAttackDesire(script.PlayerOf(w.player), 100) })
	w.orfenChase(t, h)
	w.wantOrfen(t, "the first arrival outside", h, 600, -150, 1, 0, 0)

	w.deleteMe(t, h)
	w.draws.set(99)
	h = w.spawn(t, orfenID, 600, 0)
	w.setHPShare(t, h, 0.4)
	w.hit(t, h, 10)
	w.wantOrfen(t, "the hit under half its HP", h, 600, 150, 1, 1, orfenHate)

	w.orfenChase(t, h)
	w.wantOrfen(t, "the arrival outside while resting", h, 600, 150, 2, 1, 0)
}

// TestOrfenSlowsAStriderRider covers the attacked hook's mount rule: hit by
// the level 20 character riding a strider, free of the slow, Orfen wants
// its strider slow (4258) on it at 1000000, beside its curse (4515: the
// character stands more than 8 levels above it) and the hit's attack
// desire.
func TestOrfenSlowsAStriderRider(t *testing.T) {
	t.Parallel()
	tmpl := orfenTemplate(false)
	w := bootOrfen(t, tmpl)
	w.setOrfenPoints(tmpl, [4][2]int{{400, 150}, {550, 0}, {250, 0}, {400, -150}})
	w.draws.set(99)
	h := w.spawn(t, orfenID, 400, 0)
	var mountType int32
	w.onCharacter(t, func(c *player.Character) {
		c.Mount(12526, 0)
		mountType = c.MountType()
	})
	if mountType != 1 {
		t.Fatalf("setup: mount type = %d, want 1 (a strider)", mountType)
	}

	w.hit(t, h, 10)
	w.wantCasts(t, "after the hit", h, map[modelskill.ID]ind1Cast{
		orfenSlow: {w.player.ObjectID(), 1000000}, orfenCurse: {w.player.ObjectID(), 1000000},
	})
	w.wantOrfen(t, "after the hit", h, 400, 0, 0, 0, orfenHate)
}

// TestOrfenHatesForItsParty covers the party-attacked hook: a hit of 10 on
// the private Orfen leads makes Orfen want to attack the character at
// 10 / 1000 / 0.05 * 10 * the private's weight point (1) * 1000,
// truncated (1999). A hit on Orfen itself adds only its attacked hook's
// weight: its own call adds nothing.
func TestOrfenHatesForItsParty(t *testing.T) {
	t.Parallel()
	tmpl := orfenTemplate(false)
	tmpl.Privates = []npc.PrivateEntry{{NpcID: int(orfenPrivateID), Weight: 1}}
	private := warriorTemplate(orfenPrivateID, nil)
	private.CanMove = false
	w := bootOrfen(t, tmpl, private)
	w.setOrfenPoints(tmpl, [4][2]int{{400, 150}, {550, 0}, {250, 0}, {400, -150}})
	w.draws.set(99)
	h := w.spawn(t, orfenID, 400, 0)
	privates := privatesOf(t, h, 1)

	w.hit(t, privates[0], 10)
	if got, _ := attackOn(h, w.player); got.Weight != 1999 {
		t.Fatalf("Orfen's attack desire after a hit on its private = %v, want 1999", got.Weight)
	}
	w.hit(t, h, 10)
	if got, _ := attackOn(h, w.player); got.Weight != 1999+orfenHate {
		t.Fatalf("Orfen's attack desire after a hit on itself = %v, want %v", got.Weight, 1999+orfenHate)
	}
}

// TestRibaIrenHealsOrfen covers Riba Iren's clan-attacked rule for Orfen:
// Orfen hit while under half its HP (read before the hit's damage) makes
// Riba Iren want Orfen Heal (4516) on it at 1000000 on a draw under 90 of
// 100 (89), and not on a draw of 90. Orfen's attacked hook draws first
// (99 for its pull chance, 99 for its area paralysis).
func TestRibaIrenHealsOrfen(t *testing.T) {
	t.Parallel()
	tmpl := orfenTemplate(false)
	tmpl.Clans, tmpl.ClanRange = []string{"orfen_clan"}, 3000
	riba := warriorTemplate(orfenRibaID, nil)
	riba.CanMove = false
	riba.Clans, riba.ClanRange = []string{"orfen_clan"}, 3000
	w := bootOrfen(t, tmpl, riba)
	w.setOrfenPoints(tmpl, [4][2]int{{400, 150}, {550, 0}, {250, 0}, {400, -150}})
	w.draws.set(99)
	h := w.spawn(t, orfenID, 400, 0)
	healer := w.spawn(t, orfenRibaID, 300, 100)

	w.setHPShare(t, h, 0.4)
	w.draws.set(99, 99, 99, 89)
	w.hit(t, h, 10)
	w.wantCasts(t, "Orfen hit under half its HP, draw 89", healer, map[modelskill.ID]ind1Cast{orfenHeal: {h.ObjectID(), 1000000}})

	w.draws.set(99, 99, 99, 90)
	w.hit(t, h, 10)
	w.wantCasts(t, "Orfen hit under half its HP, draw 90", healer, map[modelskill.ID]ind1Cast{orfenHeal: {h.ObjectID(), 1000000}})
}
