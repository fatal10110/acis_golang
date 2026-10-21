package ai

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The NPC ids of the fifth PR of the sixth individual behavior batch: the
// datapack ids each class binds, and a fixture clan member with no
// behavior.
const (
	i6eHealer1ID     = int32(18132) // RoyalRushHealer1
	i6eHealer1ClanID = int32(18185) // RoyalRushHealer1
	i6eHealer2ID     = int32(18186) // RoyalRushHealer2
	i6eHealer2ClanID = int32(18221) // RoyalRushHealer2
	i6eHoldID        = int32(18137) // RoyalRushWizardDDMagic2Hold
	i6eHoldClanID    = int32(18170) // RoyalRushWizardDDMagic2Hold
	i6eShortID       = int32(18140) // RoyalRushWizardShortSelfRange
	i6eShortClanID   = int32(18172) // RoyalRushWizardShortSelfRange
	i6eRunnerID      = int32(21514) // WizardFiendArcherRunAway
	i6eRunnerClanID  = int32(21509) // WizardFiendArcherRunAway
	i6eMateID        = int32(29560) // no behavior
)

// The skills the PR's classes list by type, beside the batch's long and
// short range DD magics.
const (
	i6eRangeDebuffID = modelskill.ID(9410)
	i6eSelfDDID      = modelskill.ID(9411)
	i6eSelfDebuffID  = modelskill.ID(9412)
	i6eFiendID       = modelskill.ID(9413)
)

// individual6eScripts are the behavior chain down to Wizard, the classes
// of the PR in scripts.xml order, and the event listener.
func individual6eScripts() ([]script.Listing, script.Catalog) {
	entries := []struct {
		path string
		ctor func() script.Script
	}{
		{"script.ai.individual.DefaultNpc", defaultnpc.New},
		{"script.ai.individual.Monster.MonsterAI", monster.New},
		{"script.ai.individual.Monster.WizardBase.WizardBase", wizard.Base},
		{"script.ai.individual.Monster.WizardBase.Wizard.RoyalRushHealer1", wizard.RoyalRushHealer1},
		{"script.ai.individual.Monster.WizardBase.Wizard.RoyalRushHealer2", wizard.RoyalRushHealer2},
		{"script.ai.individual.Monster.WizardBase.Wizard.RoyalRushWizardDDMagic2Hold", wizard.RoyalRushDDMagic2Hold},
		{"script.ai.individual.Monster.WizardBase.Wizard.RoyalRushWizardShortSelfRange", wizard.RoyalRushShortSelfRange},
		{"script.ai.individual.Monster.WizardBase.Wizard.Wizard", wizard.Wizard},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardFiendArcherRunAway", wizard.FiendArcherRunAway},
		{"ai.EventListener", eventListener},
	}
	var list []script.Listing
	catalog := script.Catalog{}
	for _, e := range entries {
		list = append(list, script.Listing{Path: e.path})
		catalog[e.path] = e.ctor
	}
	return list, catalog
}

func bootIndividual6e(t *testing.T, extra raidBoot, templates ...*npc.Template) *individual6World {
	t.Helper()
	list, catalog := individual6eScripts()
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draw := new(atomic.Int32)
	extra.opts = append(extra.opts,
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithScriptRand(func(n int) int { return min(int(draw.Load()), n-1) }),
	)
	return &individual6World{warriorWorld: bootRaid(t, extra, templates...), draw: draw}
}

// i6eTemplate is a fixture wizard of id listing every skill type the PR's
// classes cast.
func i6eTemplate(id int32) *npc.Template {
	return i6bTemplate(id, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeWRangeDebuff: i6eRangeDebuffID, npc.SkillTypeWSelfRangeDDMagic: i6eSelfDDID,
		npc.SkillTypeWSelfRangeDebuff: i6eSelfDebuffID, npc.SkillTypeWLongRangeDDMagic: wLongDDID,
		npc.SkillTypeWShortRangeDDMagic: wShortDDID, npc.SkillTypeWFiendArcher: i6eFiendID,
	})
}

// i6eSkills are a fixture wizard's skills, each costing mp MP: one-target
// magics that harm nothing, and self range magics cast on the caster.
func i6eSkills(mp int) *modelskill.Table {
	var defs []modelskill.Definition
	for _, id := range []modelskill.ID{i6eRangeDebuffID, wLongDDID, wShortDDID, i6eFiendID} {
		d := harmlessSkill(id)
		d.MPConsume = mp
		defs = append(defs, d)
	}
	for _, id := range []modelskill.ID{i6eSelfDDID, i6eSelfDebuffID} {
		defs = append(defs, modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, MPConsume: mp,
			Magic: true, SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true,
		})
	}
	return modelskill.NewTable(defs)
}

// i6eArmed spawns an NPC of id dx, dy from the character, with the
// fixture skills costing mp MP.
func (w *individual6World) i6eArmed(t *testing.T, id int32, dx, dy, mp int) *npc.Hostile {
	t.Helper()
	h := w.spawn(t, id, dx, dy)
	w.srv.InstallNPCCastSeam(t, h, i6eSkills(mp))
	return h
}

// i6eCasts counts the MagicSkillUse frames of caster casting skill.
func i6eCasts(frames [][]byte, caster int32, skill modelskill.ID) int {
	var n int
	for _, f := range framesOf(frames, serverpackets.OpcodeMagicSkillUse, caster) {
		if len(f) >= 13 && modelskill.ID(binary.LittleEndian.Uint32(f[9:13])) == skill {
			n++
		}
	}
	return n
}

// i6eCastsWithin runs AI cycles, a second apart, until caster has cast
// skill want times, failing after 20 cycles.
func (w *individual6World) i6eCastsWithin(t *testing.T, h *npc.Hostile, skill modelskill.ID, want int) {
	t.Helper()
	var casts int
	for i := 0; casts < want; i++ {
		if i == 20 {
			t.Fatalf("npc %d casts of skill %d = %d, want %d", h.NpcID(), skill, casts, want)
		}
		frames := w.ticks(t, 1)
		w.srv.Advance(t, time.Second)
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
		casts += i6eCasts(frames, h.ObjectID(), skill)
	}
}

// i6eGaveUp fails unless h gave up its spells (int slot 0 set to 1) and
// wants to attack the character with weight.
func (w *individual6World) i6eGaveUp(t *testing.T, h *npc.Hostile, weight float64) {
	t.Helper()
	if got := h.Scratch().Int(npc.IntAI0); got != 1 {
		t.Fatalf("npc %d int slot 0 = %d, want 1 (spells given up)", h.NpcID(), got)
	}
	if got := i6cAttackWeight(h, w.player.ObjectID()); got != weight {
		t.Fatalf("npc %d attack desire on the character = %v, want %v", h.NpcID(), got, weight)
	}
}

// i6eSettled lets h live past its first 7 s and empties its desires and
// hate list.
func (w *individual6World) i6eSettled(t *testing.T, hs ...*npc.Hostile) {
	t.Helper()
	w.ticks(t, 8)
	for _, h := range hs {
		w.onQueue(t, h, func() { h.AI().Desires().Clear(); h.AI().Hates().Clear() })
	}
}

// i6eWalk waits for h's walk to start and runs its movement until it
// arrives.
func (w *individual6World) i6eWalk(t *testing.T, h *npc.Hostile) [][]byte {
	t.Helper()
	var frames [][]byte
	for i := 0; !h.IsMoving(); i++ {
		if i == 5 {
			t.Fatal("no walk after five cycles")
		}
		frames = append(frames, cycle(t, w.srv)...)
	}
	for i := 0; h.IsMoving(); i++ {
		if i >= int(20*time.Second/move.PositionUpdateInterval) {
			t.Fatal("the walk never arrived")
		}
		w.srv.TickPositions()
	}
	w.srv.Settle(t)
	return append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
}

// TestRoyalRushHealer1AnswersHits runs RoyalRushHealer1's attacked and
// skill-finished hooks. A playable's hit on a draw under 33 wants its
// range debuff on the attacker, otherwise its self range DD magic on
// itself; one that cannot pay for the cast gives up its spells and wants
// to attack, weight 1000. The end of its range debuff, with the character
// on its hate list, brings its self range DD magic.
func TestRoyalRushHealer1AnswersHits(t *testing.T) {
	t.Parallel()
	w := bootIndividual6e(t, raidBoot{}, i6eTemplate(i6eHealer1ID))
	me := w.player.ObjectID()

	debuffer := w.i6eArmed(t, i6eHealer1ID, 100, 0, 0)
	w.hit(t, debuffer, 10)
	if !castOn(debuffer, i6eRangeDebuffID, me) {
		t.Fatal("healer wants no range debuff on the character after a hit on a draw of 0")
	}
	if _, ok := castDesireOf(debuffer, i6eSelfDDID); ok {
		t.Fatal("healer wants its self range DD magic after a hit on a draw of 0")
	}
	w.i6eCastsWithin(t, debuffer, i6eSelfDDID, 1)

	w.draw.Store(50)
	self := w.i6eArmed(t, i6eHealer1ID, 0, 100, 0)
	w.hit(t, self, 10)
	if !castOn(self, i6eSelfDDID, self.ObjectID()) {
		t.Fatal("healer wants no self range DD magic on itself after a hit on a draw of 50")
	}

	broke := w.i6eArmed(t, i6eHealer1ID, 100, 100, 100000)
	w.hit(t, broke, 10)
	w.i6eGaveUp(t, broke, 1000)
}

// TestRoyalRushHealer2AnswersHits runs RoyalRushHealer2's attacked and
// skill-finished hooks. Every draw 0, a playable's hit wants its self
// range debuff on itself and, from an attacker farther than 100, its long
// range DD magic on the attacker, else its self range DD magic. On a draw
// of 50 only the self range DD magic. The end of a cast on the character
// 300 away brings the long range DD magic again.
func TestRoyalRushHealer2AnswersHits(t *testing.T) {
	t.Parallel()
	w := bootIndividual6e(t, raidBoot{}, i6eTemplate(i6eHealer2ID))
	me := w.player.ObjectID()

	far := w.i6eArmed(t, i6eHealer2ID, 300, 0, 0)
	w.hit(t, far, 10)
	if !castOn(far, i6eSelfDebuffID, far.ObjectID()) || !castOn(far, wLongDDID, me) {
		t.Fatal("healer 300 away wants no self range debuff and long range DD magic after a hit on a draw of 0")
	}
	if _, ok := castDesireOf(far, i6eSelfDDID); ok {
		t.Fatal("healer 300 away wants its self range DD magic after a hit on a draw of 0")
	}

	near := w.i6eArmed(t, i6eHealer2ID, 50, 0, 0)
	w.hit(t, near, 10)
	if !castOn(near, i6eSelfDebuffID, near.ObjectID()) || !castOn(near, i6eSelfDDID, near.ObjectID()) {
		t.Fatal("healer 50 away wants no self range debuff and self range DD magic after a hit on a draw of 0")
	}
	if _, ok := castDesireOf(near, wLongDDID); ok {
		t.Fatal("healer 50 away wants its long range DD magic")
	}
	w.i6eCastsWithin(t, far, wLongDDID, 2)

	w.draw.Store(50)
	calm := w.i6eArmed(t, i6eHealer2ID, 300, 100, 0)
	w.hit(t, calm, 10)
	if !castOn(calm, i6eSelfDDID, calm.ObjectID()) {
		t.Fatal("healer wants no self range DD magic after a hit on a draw of 50")
	}
	for _, id := range []modelskill.ID{i6eSelfDebuffID, wLongDDID} {
		if _, ok := castDesireOf(calm, id); ok {
			t.Fatalf("healer wants skill %d after a hit on a draw of 50", id)
		}
	}
}

// TestRoyalRushHealersAnswerTheirClan runs both healers' clan-attacked
// hooks, every draw 0: a hit on their clan member wants their range
// debuff on the attacker; one that cannot pay for it gives up its spells
// and wants to attack.
func TestRoyalRushHealersAnswerTheirClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual6e(t, raidBoot{},
		inClan(i6eTemplate(i6eHealer1ClanID)), inClan(i6eTemplate(i6eHealer2ClanID)), inClan(warriorTemplate(i6eMateID, nil)))
	me := w.player.ObjectID()
	one := w.i6eArmed(t, i6eHealer1ClanID, 300, 0, 0)
	two := w.i6eArmed(t, i6eHealer2ClanID, 0, 300, 100000)
	mate := w.spawn(t, i6eMateID, 150, 150)

	w.hit(t, mate, 10)
	if !castOn(one, i6eRangeDebuffID, me) {
		t.Fatal("RoyalRushHealer1 wants no range debuff on its clan member's attacker")
	}
	w.i6eGaveUp(t, two, 1000)
}

// TestRoyalRushWizardDDMagic2HoldHoldsAndStrikes runs
// RoyalRushWizardDDMagic2Hold's no-desire, attacked, clan-attacked and
// skill-finished hooks, every draw 0. With nothing to do it wants its
// spawn point, weight 30. A playable's hit wants its long range DD magic
// on an attacker 300 away and its short range one on one 50 away; given up
// spells, the hit adds 125 to its attack desire instead. A settled one
// whose clan member is hit wants the magic its member's distance to the
// attacker picks. The end of its cast brings the next one.
func TestRoyalRushWizardDDMagic2HoldHoldsAndStrikes(t *testing.T) {
	t.Parallel()
	w := bootIndividual6e(t, raidBoot{},
		i6eTemplate(i6eHoldID), inClan(i6eTemplate(i6eHoldClanID)), inClan(warriorTemplate(i6eMateID, nil)))
	me := w.player.ObjectID()

	idle := w.i6eArmed(t, i6eHoldID, 0, 500, 0)
	home := script.NPCOf(idle).SpawnLocation()
	w.onQueue(t, idle, func() { idle.AI().Desires().Clear() })
	for i := 0; i < 5; i++ {
		if _, ok := desire(idle, actorai.IntentionMoveTo); ok {
			break
		}
		cycle(t, w.srv)
	}
	if d, ok := desire(idle, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("idle hold wizard move desire = %+v (%v), want its spawn point %d,%d with weight 30", d, ok, home.X, home.Y)
	}
	if _, ok := desire(idle, actorai.IntentionWander); ok {
		t.Fatal("idle hold wizard wants to wander")
	}

	far := w.i6eArmed(t, i6eHoldID, 300, 0, 0)
	near := w.i6eArmed(t, i6eHoldID, 50, 0, 0)
	w.hit(t, far, 10)
	w.hit(t, near, 10)
	if !castOn(far, wLongDDID, me) || !castOn(near, wShortDDID, me) {
		t.Fatal("hold wizards want no long range DD magic 300 away and short range one 50 away")
	}

	quit := w.i6eArmed(t, i6eHoldID, 300, 100, 0)
	w.onQueue(t, quit, func() { quit.Scratch().SetInt(npc.IntAI0, 1) })
	w.hit(t, quit, 10)
	if got, want := i6cAttackWeight(quit, me), i6cAttackWeight(far, me)+125; got != want {
		t.Fatalf("given up hold wizard attack desire = %v, want %v", got, want)
	}
	if _, ok := castDesireOf(quit, wLongDDID); ok {
		t.Fatal("given up hold wizard wants its long range DD magic")
	}

	clan := w.i6eArmed(t, i6eHoldClanID, 0, -50, 0)
	mate := w.spawn(t, i6eMateID, 300, 300)
	w.i6eSettled(t, clan)
	w.hit(t, mate, 10)
	if !castOn(clan, wLongDDID, me) {
		t.Fatal("settled hold wizard wants no long range DD magic on an attacker 300 from its clan member")
	}

	w.i6eCastsWithin(t, far, wLongDDID, 2)
}

// TestRoyalRushWizardShortSelfRangeStrikes runs
// RoyalRushWizardShortSelfRange's attacked, clan-attacked and
// skill-finished hooks. A playable's hit, with the attacker on its hate
// list, wants its self range DD magic on a draw of 0 and its short range
// one on the attacker on a draw of 50; one that cannot pay gives up its
// spells and wants to attack, 1000 more. A settled one whose clan member is
// hit wants its short range DD magic on the attacker. The end of its cast
// brings the next one.
func TestRoyalRushWizardShortSelfRangeStrikes(t *testing.T) {
	t.Parallel()
	w := bootIndividual6e(t, raidBoot{},
		i6eTemplate(i6eShortID), inClan(i6eTemplate(i6eShortClanID)), inClan(warriorTemplate(i6eMateID, nil)))
	me := w.player.ObjectID()

	self := w.i6eArmed(t, i6eShortID, 100, 0, 0)
	w.hit(t, self, 10)
	if !castOn(self, i6eSelfDDID, self.ObjectID()) {
		t.Fatal("short self range wizard wants no self range DD magic after a hit on a draw of 0")
	}

	w.draw.Store(50)
	short := w.i6eArmed(t, i6eShortID, 0, 100, 0)
	broke := w.i6eArmed(t, i6eShortID, 100, 100, 100000)
	w.hit(t, short, 10)
	w.hit(t, broke, 10)
	if !castOn(short, wShortDDID, me) {
		t.Fatal("short self range wizard wants no short range DD magic after a hit on a draw of 50")
	}
	w.i6eGaveUp(t, broke, i6cAttackWeight(short, me)+1000)

	clan := w.i6eArmed(t, i6eShortClanID, 300, 300, 0)
	mate := w.spawn(t, i6eMateID, 200, 300)
	w.i6eSettled(t, clan)
	w.hit(t, mate, 10)
	if !castOn(clan, wShortDDID, me) {
		t.Fatal("settled short self range wizard wants no short range DD magic on its clan member's attacker")
	}

	w.i6eCastsWithin(t, short, wShortDDID, 2)
}

// TestWizardFiendArcherRunAwayShoots runs WizardFiendArcherRunAway's
// created, attacked, clan-attacked, skill-finished and move-finished
// hooks, every draw 0. Created, it clears int slot 3. A playable's hit
// keeps the attacker in creature slot 0 and wants its fiend archer skill
// on it; one that cannot pay gives up its spells and attacks. The end of
// a cast on the character 50 away makes it flee from it, weight 1000. A
// settled one whose clan member is hit shoots the attacker. Arriving from
// a walk with the character on its hate list, it shoots it.
func TestWizardFiendArcherRunAwayShoots(t *testing.T) {
	t.Parallel()
	w := bootIndividual6e(t, raidBoot{},
		i6eTemplate(i6eRunnerID), inClan(i6eTemplate(i6eRunnerClanID)), inClan(warriorTemplate(i6eMateID, nil)))
	me := w.player.ObjectID()

	h := w.i6eArmed(t, i6eRunnerID, 50, 0, 0)
	if got := h.Scratch().Int(npc.IntAI3); got != 0 {
		t.Fatalf("runner int slot 3 after its creation = %d, want 0", got)
	}
	w.hit(t, h, 10)
	if !castOn(h, i6eFiendID, me) {
		t.Fatal("runner wants no fiend archer skill on the character after a hit")
	}
	if c, ok := h.Scratch().Creature(npc.CreatureAI0).(interface{ ObjectID() int32 }); !ok || c.ObjectID() != me {
		t.Fatalf("runner creature slot 0 = %v, want the character", h.Scratch().Creature(npc.CreatureAI0))
	}
	for i := 0; ; i++ {
		if i == 20 {
			t.Fatal("runner never fled the character 50 away after its cast")
		}
		w.ticks(t, 1)
		w.srv.Advance(t, time.Second)
		if d, ok := desire(h, actorai.IntentionFlee); ok && d.Weight == 1000 && d.FinalTarget != nil && d.FinalTarget.ObjectID() == me {
			break
		}
	}

	broke := w.i6eArmed(t, i6eRunnerID, 300, 0, 100000)
	w.hit(t, broke, 10)
	if got := broke.Scratch().Int(npc.IntAI0); got != 1 {
		t.Fatalf("runner short of MP int slot 0 = %d, want 1", got)
	}
	if got := i6cAttackWeight(broke, me); got < 1000 {
		t.Fatalf("runner short of MP attack desire = %v, want at least 1000", got)
	}

	clan := w.i6eArmed(t, i6eRunnerClanID, 0, 300, 0)
	mate := w.spawn(t, i6eMateID, 100, 300)
	w.i6eSettled(t, clan)
	w.hit(t, mate, 10)
	if !castOn(clan, i6eFiendID, me) {
		t.Fatal("settled runner wants no fiend archer skill on its clan member's attacker")
	}

	walker := w.i6eArmed(t, i6eRunnerID, 0, 600, 0)
	x, y, z := walker.Position()
	w.onQueue(t, walker, func() {
		walker.AI().Desires().Clear()
		walker.AI().Hates().Add(w.player, 100)
	})
	if !walker.AI().AddMoveToDesire(location.Location{X: x + 100, Y: y, Z: z}, 1e6) {
		t.Fatal("the walk was not queued")
	}
	w.i6eWalk(t, walker)
	if !castOn(walker, i6eFiendID, me) {
		t.Fatal("runner arriving from a walk wants no fiend archer skill on its most hated")
	}
}

// i6eRunnerDen is an npcmaker spawning one runner whose spawn names a
// flee point 200 east of it.
const i6eRunnerDen = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="runner_den" minZ="-1000" maxZ="1000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="runner_den" territory="runner_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="21514" total="1" pos="40;20;30;0" respawn="60sec"><ai><set name="flee_x" val="240"/><set name="flee_y" val="20"/><set name="flee_z" val="30"/></ai></npc>
	</npcmaker>
</list>`

// i6eDenRunner boots the den with the listener 100 from the flee point and
// returns the runner, its skills costing nothing, and the listener.
func i6eDenRunner(t *testing.T) (*individual6World, *npc.Hostile, int32) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(i6eRunnerDen), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	w := bootIndividual6e(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNpcSpawns(table)}},
		i6eTemplate(i6eRunnerID), warriorTemplate(plwListenerID, nil))
	var h *npc.Hostile
	for _, obj := range w.srv.State.Objects() {
		if o, ok := obj.(*npc.Hostile); ok && o.NpcID() == int(i6eRunnerID) {
			h = o
		}
	}
	if h == nil {
		t.Fatal("the den spawned no runner")
	}
	w.srv.InstallNPCCastSeam(t, h, i6eSkills(0))
	listener, err := w.srv.NpcSpawns.AddSpawn(plwListenerID, 240, 120, 30, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn the listener: %v", err)
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	return w, h, listener.ObjectID()
}

// i6eFlee lands a hit of 60% of its HP, then one of 10 on a draw of 0
// below half HP, and checks the runner runs to its flee point, weight
// 2000.
func (w *individual6World) i6eFlee(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.draw.Store(99)
	w.hit(t, h, int(h.MaxHP())*6/10)
	w.draw.Store(0)
	w.hit(t, h, 10)
	if got := h.Scratch().Int(npc.IntAI3); got != 1 {
		t.Fatalf("runner int slot 3 after a hit below half HP = %d, want 1 (running)", got)
	}
	if d, ok := desire(h, actorai.IntentionMoveTo); !ok || d.Weight != 2000 || d.Location.X != 240 || d.Location.Y != 20 || d.Location.Z != 30 {
		t.Fatalf("running runner move desire = %+v (%v), want 240,20,30 with weight 2000", d, ok)
	}
	if _, ok := castDesireOf(h, i6eFiendID); ok {
		t.Fatal("running runner wants its fiend archer skill")
	}
}

// i6eArrive walks the running runner to its flee point with every draw
// arrivalDraw and checks it wanders there.
func (w *individual6World) i6eArrive(t *testing.T, h *npc.Hostile, arrivalDraw int32) {
	t.Helper()
	w.draw.Store(arrivalDraw)
	w.i6eWalk(t, h)
	if got := h.Scratch().Int(npc.IntAI3); got != 2 {
		t.Fatalf("runner int slot 3 at its flee point = %d, want 2", got)
	}
	if d, ok := desire(h, actorai.IntentionWander); !ok || d.Weight != 50 || d.Timer != 5 {
		t.Fatalf("runner at its flee point wander desire = %+v (%v), want timer 5 and weight 50", d, ok)
	}
}

// TestWizardFiendArcherRunAwayRunsAndCallsItsKin runs the runner's flee:
// below half HP a playable's hit, on a draw under 10, sends it to its flee
// point. There, on a draw under 50, its 2001 timer 1 s later wants to
// attack the creature it last fought, weight 1000, and broadcasts script
// event 10000 with that creature within 400. Walking home then ends the
// run.
func TestWizardFiendArcherRunAwayRunsAndCallsItsKin(t *testing.T) {
	t.Parallel()
	w, h, listener := i6eDenRunner(t)
	me := w.player.ObjectID()
	home := script.NPCOf(h).SpawnLocation()
	w.i6eFlee(t, h)
	w.i6eArrive(t, h, 0)

	w.srv.Advance(t, time.Second)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got, want := says(frames, listener), []string{fmt.Sprintf("event 10000 %d", me)}; !slices.Equal(got, want) {
		t.Fatalf("events heard 1 s after the run = %q, want %q", got, want)
	}
	if got := h.Scratch().Int(npc.IntAI3); got != 3 {
		t.Fatalf("runner int slot 3 after its call = %d, want 3", got)
	}
	if got := i6cAttackWeight(h, me); got < 1000 {
		t.Fatalf("runner attack desire on the character after its call = %v, want at least 1000", got)
	}

	// Stop its chase of the character first, so the walk home is its next.
	w.onQueue(t, h, func() { script.NPCOf(h).RemoveAllDesire() })
	if !h.AI().AddMoveToDesire(location.Location{X: home.X, Y: home.Y, Z: home.Z}, 1e6) {
		t.Fatal("the walk home was not queued")
	}
	w.i6eWalk(t, h)
	if got := h.Scratch().Int(npc.IntAI3); got != 0 {
		t.Fatalf("runner int slot 3 back home = %d, want 0", got)
	}
}

// TestWizardFiendArcherRunAwayHealsBeforeGoingHome runs the runner's 2002
// timer: arriving at its flee point on a draw of 50 or more starts it 1 s
// later; with int slot 0 at 2 it checks every second until the runner is
// at full HP, then wants its spawn point, weight 5000.
func TestWizardFiendArcherRunAwayHealsBeforeGoingHome(t *testing.T) {
	t.Parallel()
	w, h, listener := i6eDenRunner(t)
	home := script.NPCOf(h).SpawnLocation()
	w.i6eFlee(t, h)
	w.i6eArrive(t, h, 99)
	w.onQueue(t, h, func() { h.Scratch().SetInt(npc.IntAI0, 2) })

	w.srv.Advance(t, time.Second)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), listener); len(got) != 0 {
		t.Fatalf("events heard after a run on a draw of 99 = %q, want none", got)
	}
	if d, ok := desire(h, actorai.IntentionMoveTo); ok && d.Weight == 5000 {
		t.Fatal("wounded runner wants to walk home")
	}
	w.onQueue(t, h, func() { h.SetHP(float64(h.MaxHP())) })
	w.srv.Advance(t, time.Second)
	if d, ok := desire(h, actorai.IntentionMoveTo); !ok || d.Weight != 5000 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("healed runner move desire = %+v (%v), want its spawn point %d,%d with weight 5000", d, ok, home.X, home.Y)
	}
}

// TestWizardFiendArcherRunAwayRootedGivesUpTheRun runs the attacked hook
// of a running runner that a root holds farther than 40 from its top
// desire target: it ends the run (int slot 3 set to 3), drops its walk to
// the flee point and shoots its attacker.
func TestWizardFiendArcherRunAwayRootedGivesUpTheRun(t *testing.T) {
	t.Parallel()
	w, h, _ := i6eDenRunner(t)
	me := w.player.ObjectID()
	w.i6eFlee(t, h)
	w.teleport(t, h, 200, 0)
	root, err := effect.New(effect.Skill{ID: i6eFiendID, Level: 1}, modelskill.EffectTemplate{Name: "Root", Time: 600})
	if err != nil {
		t.Fatalf("effect.New(Root): %v", err)
	}
	root.Effector, root.Effected = h, h
	w.onQueue(t, h, func() {
		h.EffectList().Add(root)
		script.NPCOf(h).AddAttackDesire(script.PlayerOf(w.player), 1e7)
	})
	if !script.NPCOf(h).Rooted() {
		t.Fatal("the runner is not rooted")
	}

	w.hit(t, h, 10)
	if got := h.Scratch().Int(npc.IntAI3); got != 3 {
		t.Fatalf("rooted runner int slot 3 after a hit = %d, want 3 (run given up)", got)
	}
	if d, ok := desire(h, actorai.IntentionMoveTo); ok {
		t.Fatalf("rooted runner still wants to walk: %+v", d)
	}
	if !castOn(h, i6eFiendID, me) {
		t.Fatal("rooted runner wants no fiend archer skill on its attacker")
	}
}
