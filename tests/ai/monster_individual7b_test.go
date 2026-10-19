package ai

import (
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids of #192 PR 2's classes, each a level 1 fixture monster.
const (
	ind7bPartyPrivateID  = int32(27261) // LV3PartyPrivateMonster
	ind7bWizardPrivateID = int32(27253) // LV3WizardPrivate, the private LV3Wizard raises
	ind7bRangerID        = int32(27289) // LV3Ranger
	ind7bSongDanceID     = int32(27269) // LV3SongDance
	ind7bSummonerID      = int32(27313) // LV3Summoner
	ind7bWizardID        = int32(27250) // LV3Wizard
)

// The skills the PR 2 fixture NPCs list by type.
const (
	ind7bArrowDefence = modelskill.ID(9620)
	ind7bArrowNormal  = modelskill.ID(9621)
	ind7bCheck1       = modelskill.ID(9622)
	ind7bCheck2       = modelskill.ID(9623)
	ind7bSelfRangeDD1 = modelskill.ID(9624)
	ind7bSelfRangeDD2 = modelskill.ID(9625)
	ind7bSelfRangeDD3 = modelskill.ID(9626)
	ind7bBuff1        = modelskill.ID(9627)
	ind7bSummonMode   = modelskill.ID(9628)
	ind7bSummonEffect = modelskill.ID(9629)
	ind7bSummonHeal1  = modelskill.ID(9630)
	ind7bSummonHeal2  = modelskill.ID(9631)
)

// ind7bBuffStack is the stack type of LV3Summoner's buff.
const ind7bBuffStack = "lv3_summoner_buff"

// ind7bSkills are the PR 2 fixture skills the NPCs cast: the modes, the
// self range blast, the buff and the summon skills on the NPC itself, the
// other two magics, the heals and LV3SongDance's debuffs at one target
// within 600.
func ind7bSkills() []modelskill.Definition {
	var defs []modelskill.Definition
	for _, id := range []modelskill.ID{ind7bArrowDefence, ind7bArrowNormal, ind7bSelfRangeDD1, ind7bBuff1, ind7bSummonMode, ind7bSummonEffect} {
		defs = append(defs, ind1Skill(id, 0, true))
	}
	for _, id := range []modelskill.ID{ind7bCheck1, ind7bCheck2, ind7bSelfRangeDD2, ind7bSelfRangeDD3, ind7bSummonHeal1, ind7bSummonHeal2} {
		defs = append(defs, ind1Skill(id, 600, false))
	}
	return append(defs, ind7bDebuffs()...)
}

// ind7bTemplate is ind7Template of id also listing every PR 2 skill by
// type.
func ind7bTemplate(id int32) *npc.Template {
	tmpl := ind7Template(id)
	for t, sk := range map[npc.SkillType]modelskill.ID{
		npc.SkillTypeArrowDefenceMode: ind7bArrowDefence, npc.SkillTypeArrowNormalMode: ind7bArrowNormal,
		npc.SkillTypeCheckSkill1: ind7bCheck1, npc.SkillTypeCheckSkill2: ind7bCheck2,
		npc.SkillTypeSelfRangeDDMagic1: ind7bSelfRangeDD1, npc.SkillTypeSelfRangeDDMagic2: ind7bSelfRangeDD2,
		npc.SkillTypeSelfRangeDDMagic3: ind7bSelfRangeDD3, npc.SkillTypeBuff1: ind7bBuff1,
		npc.SkillTypeSummonMode: ind7bSummonMode, npc.SkillTypeSummonEffect: ind7bSummonEffect,
		npc.SkillTypeSummonHeal1: ind7bSummonHeal1, npc.SkillTypeSummonHeal2: ind7bSummonHeal2,
	} {
		tmpl.SkillsByType[t] = modelskill.Ref{ID: sk, Level: 1}
	}
	return tmpl
}

// ind7bClanTemplate is tmpl in the clan "lv3", calling within 600.
func ind7bClanTemplate(tmpl *npc.Template) *npc.Template {
	tmpl.Clans, tmpl.ClanRange = []string{"lv3"}, 600
	return tmpl
}

// setParams stores the three spawn parameters in h's script memory.
func (w *ind7World) setParams(t *testing.T, h *npc.Hostile, p script.Params) {
	t.Helper()
	w.onQueue(t, h, func() {
		h.Scratch().SetInt(npc.IntParam1, p.P1)
		h.Scratch().SetInt(npc.IntParam2, p.P2)
		h.Scratch().SetInt(npc.IntParam3, p.P3)
	})
}

// params reads the three spawn parameters of h's script memory.
func params(h *npc.Hostile) script.Params {
	return script.Params{P1: ind1Slot(h, script.SlotParam1), P2: ind1Slot(h, script.SlotParam2), P3: ind1Slot(h, script.SlotParam3)}
}

// ind7bNear reports whether a and b differ by less than 1e-9: the hooks
// add their weights in an order the test does not repeat.
func ind7bNear(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// ind7bLive returns the live NPCs of template id in the world.
func (w *ind7World) ind7bLive(id int32) []*npc.Hostile {
	var out []*npc.Hostile
	for _, h := range w.liveNPCs(id) {
		if !h.Dead() {
			out = append(out, h)
		}
	}
	return out
}

// castOnce drops h's desires, has it want ref on itself alone and runs AI
// cycles until that cast has started and ended.
func (w *ind7World) castOnce(t *testing.T, h *npc.Hostile, ref modelskill.ID) {
	t.Helper()
	w.castOnceAt(t, h, script.NPCOf(h), ref)
}

// castOnceAt is castOnce for a cast at target.
func (w *ind7World) castOnceAt(t *testing.T, h *npc.Hostile, target script.Creature, ref modelskill.ID) {
	t.Helper()
	w.onQueue(t, h, func() {
		h.AI().Desires().Clear()
		script.NPCOf(h).AddCastDesire(target, modelskill.Ref{ID: ref, Level: 1}, 1000000)
	})
	w.castQueued(t, h)
}

// castQueued runs AI cycles until h has started a cast, then lets time
// pass until it has ended, starting nothing after it.
func (w *ind7World) castQueued(t *testing.T, h *npc.Hostile) {
	t.Helper()
	started := false
	w.srv.AdvanceUntil(t, "the cast", func() bool {
		if started && !h.CastingNow() {
			return true
		}
		w.ticks(t, 1)
		started = started || h.CastingNow()
		return false
	})
}

// TestLV3PartyPrivateTakesItsMastersPart ports LV3PartyPrivateMonster's
// created, attacked, party attacked, clan attacked and party died hooks,
// every draw 0. Raised with no spawn parameters by a master holding the
// character as its first and second and itself as its third, it takes
// those three, attacks the character (200, LV3Monster's created hook) and
// remembers its master. With its master alive, the character's hit of 10
// adds 10 / 8 * 10 = 12.5 from its own hook, 125 from LV3Monster's, the
// party share 10 / max HP / 0.05 * 50 of the hit on itself and the clan
// share 10 / 8 * 30 = 37.5 of its own clan call. A hit of 10 on its master
// adds the party share alone, a hit of 10 on a clan member the clan share
// alone. Its 5 s check keeps it while its master lives; its master's
// death takes it out of the world.
func TestLV3PartyPrivateTakesItsMastersPart(t *testing.T) {
	t.Parallel()
	w := bootIndividual7(t, raidBoot{},
		ind7bClanTemplate(ind7bTemplate(ind7bPartyPrivateID)), ind7bClanTemplate(ind7PrivateTemplate()), warriorTemplate(ind7MasterID, nil))
	me := w.player.ObjectID()
	master := w.spawn(t, ind7MasterID, 600, 0)
	w.setParams(t, master, script.Params{P1: me, P2: me, P3: master.ObjectID()})
	private := w.raise(t, master, ind7bPartyPrivateID, 300, 0, script.Params{})
	if got, want := params(private), (script.Params{P1: me, P2: me, P3: master.ObjectID()}); got != want {
		t.Fatalf("private spawn parameters = %+v, want its master's %+v", got, want)
	}
	if c1 := script.NPCOf(private).Slot(script.SlotCreature1); c1 == nil || c1.ObjectID() != master.ObjectID() {
		t.Fatalf("private creature slot 1 = %v, want its master", c1)
	}
	if got := w.attackWeight(private); got != 200 {
		t.Fatalf("private attack desire after it spawned = %v, want 200", got)
	}

	// The party share of a hit of 10: the fixture's maximum HP is its
	// computed one, not the template's.
	share := ((float64(10) / float64(script.NPCOf(private).MaxHP())) / 0.05) * 50
	w.hit(t, private, 10)
	if got, want := w.attackWeight(private), 200+12.5+125+share+37.5; !ind7bNear(got, want) {
		t.Fatalf("private attack desire after the hit = %v, want 200 + 12.5 + 125 + %v + 37.5", got, share)
	}
	w.clearDesires(t, private)
	w.hit(t, master, 10)
	if got := w.attackWeight(private); got != share {
		t.Fatalf("private attack desire after the hit on its master = %v, want %v", got, share)
	}
	w.clearDesires(t, private)
	member := w.spawn(t, ind7PrivateID, 300, 100)
	w.hit(t, member, 10)
	if got := w.attackWeight(private); got != 37.5 {
		t.Fatalf("private attack desire after the hit on its clan member = %v, want 37.5", got)
	}

	w.srv.Advance(t, 10*time.Second)
	if !w.inWorld(private) {
		t.Fatal("the private left the world while its master lives")
	}
	w.kill(t, master)
	if w.inWorld(private) {
		t.Fatal("the private stays in the world after its master's death")
	}
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("events heard = %q, want none", got)
	}
}

// TestLV3PartyPrivateIgnoresAStrangersSpell ports LV3PartyPrivateMonster's
// see-spell hook, which replaces LV3Monster's: a private whose master names
// another player keeps its place when the character casts a spell, while a
// dagger raised for the same player leaves (LV3Monster). With its master
// alive the spell adds no hate. The character's hit still makes the
// private leave through LV3Monster's attacked hook, telling its master
// (event 1000).
func TestLV3PartyPrivateIgnoresAStrangersSpell(t *testing.T) {
	t.Parallel()
	w := bootIndividual7(t, ind1AggroSpellBoot(t), ind7bTemplate(ind7bPartyPrivateID), ind7Template(ind7DaggerID), warriorTemplate(ind7MasterID, nil))
	me := w.player.ObjectID()
	master := w.spawn(t, ind7MasterID, 600, 0)
	stranger := script.Params{P1: me, P2: ind7Stranger, P3: master.ObjectID()}
	w.setParams(t, master, stranger)
	private := w.raise(t, master, ind7bPartyPrivateID, 150, 50, script.Params{})
	dagger := w.raise(t, master, ind7DaggerID, 150, -50, stranger)
	w.clearDesires(t, private)

	w.castAggroBuff()
	w.srv.AdvanceUntil(t, "the dagger leaving", func() bool { return !w.inWorld(dagger) })
	if !w.inWorld(private) {
		t.Fatal("the private left the world on a stranger's spell")
	}
	if got := w.attackWeight(private); got != 0 {
		t.Fatalf("private attack desire after the spell = %v, want none", got)
	}
	if got := w.heard.take(); !slices.Equal(got, []string{"event=1000 arg1=0 arg2=0"}) {
		t.Fatalf("events heard after the spell = %q, want the dagger's event 1000", got)
	}

	w.hit(t, private, 10)
	if w.inWorld(private) {
		t.Fatal("the private stays in the world after a stranger's hit")
	}
	if got := w.heard.take(); !slices.Equal(got, []string{"event=1000 arg1=0 arg2=0"}) {
		t.Fatalf("events heard after the hit = %q, want event 1000", got)
	}
}

// TestLV3PartyPrivateWithNoMaster spawns an LV3PartyPrivateMonster on its
// own: its created hook stops where it reads its missing master's
// parameters, so it keeps none, arms no timer and remembers no master. The
// character's hit, from a player its zero second parameter does not name,
// takes it out of the world, telling nobody.
func TestLV3PartyPrivateWithNoMaster(t *testing.T) {
	t.Parallel()
	w := bootIndividual7(t, raidBoot{}, ind7bTemplate(ind7bPartyPrivateID))
	private := w.spawn(t, ind7bPartyPrivateID, 300, 0)
	if got := params(private); got != (script.Params{}) {
		t.Fatalf("lone private spawn parameters = %+v, want none", got)
	}
	if got := w.attackWeight(private); got != 0 {
		t.Fatalf("lone private attack desire = %v, want none", got)
	}
	w.hit(t, private, 10)
	if w.inWorld(private) {
		t.Fatal("the lone private stays in the world after the character's hit")
	}
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("events heard = %q, want none", got)
	}
}

// TestLV3WizardRaisesAndHealsItsPrivates ports LV3Wizard's created, party
// died and script event hooks with LV3WizardPrivate's created and
// LV3PartyPrivateMonster's party died hooks, every draw 0. Raised for the
// character, the wizard wants its summon mode and raises a private with its
// own spawn parameters where it stands. The private attacks the character
// (300, then LV3Monster's 200), takes its master's parameters and tells it
// its object id (event 1000), on which the wizard wants its first heal on
// it. Each second the private attacks the character again (50).
//
// The wizard is itself a private of the NPC that raised it, so its
// privates join that party, and a dead private tells the wizard twice: as
// its master and as another private of the party. Each death therefore
// raises two privates, with the summon effect, while the wizard's count of
// five lasts; once it is spent, the call wants the summon mode instead.
// The wizard's death takes every private it raised out of the world.
func TestLV3WizardRaisesAndHealsItsPrivates(t *testing.T) {
	t.Parallel()
	w := bootIndividual7(t, raidBoot{}, ind7bTemplate(ind7bWizardID), ind7bTemplate(ind7bWizardPrivateID), warriorTemplate(ind7MasterID, nil))
	me := w.player.ObjectID()
	master := w.spawn(t, ind7MasterID, 600, 0)
	raised := script.Params{P1: me, P2: me, P3: master.ObjectID()}
	wizard := w.raise(t, master, ind7bWizardID, 300, 0, raised)
	live := w.ind7bLive(ind7bWizardPrivateID)
	if len(live) != 1 {
		t.Fatalf("privates after the wizard spawned = %d, want 1", len(live))
	}
	private := live[0]
	if m := script.NPCOf(private).Master(); m == nil || m.ObjectID() != wizard.ObjectID() {
		t.Fatalf("private master = %v, want the wizard", m)
	}
	wx, wy, wz := wizard.Position()
	if x, y, z := private.Position(); x != wx || y != wy || z != wz {
		t.Fatalf("private stands at %d %d %d, want the wizard's %d %d %d", x, y, z, wx, wy, wz)
	}
	if got := params(private); got != raised {
		t.Fatalf("private spawn parameters = %+v, want %+v", got, raised)
	}
	w.wantCasts(t, "wizard after it spawned", wizard, map[modelskill.ID]ind1Cast{
		ind7bSummonMode: {wizard.ObjectID(), 1000000}, ind7bSummonHeal1: {private.ObjectID(), 1000000},
	})
	if got := ind1Slot(wizard, script.SlotIntAI0); got != 5 {
		t.Fatalf("wizard int slot 0 = %d, want 5", got)
	}
	if got := w.attackWeight(private); got != 500 {
		t.Fatalf("private attack desire after it spawned = %v, want 300 + 200", got)
	}
	if c1 := script.NPCOf(private).Slot(script.SlotCreature1); c1 == nil || c1.ObjectID() != wizard.ObjectID() {
		t.Fatalf("private creature slot 1 = %v, want the wizard", c1)
	}
	w.srv.Advance(t, time.Second)
	if got := w.attackWeight(private); got != 550 {
		t.Fatalf("private attack desire after 1 s = %v, want 550", got)
	}

	for _, step := range []struct{ live, left int32 }{{2, 3}, {3, 1}} {
		w.kill(t, w.ind7bLive(ind7bWizardPrivateID)[0])
		if got := int32(len(w.ind7bLive(ind7bWizardPrivateID))); got != step.live {
			t.Fatalf("live privates = %d, want %d", got, step.live)
		}
		if got := ind1Slot(wizard, script.SlotIntAI0); got != step.left {
			t.Fatalf("wizard int slot 0 = %d, want %d", got, step.left)
		}
	}
	w.clearDesires(t, wizard)
	before := w.ind7bLive(ind7bWizardPrivateID)
	w.kill(t, before[0])
	live = w.ind7bLive(ind7bWizardPrivateID)
	if len(live) != 3 {
		t.Fatalf("live privates after the count ran out = %d, want 3", len(live))
	}
	if got := ind1Slot(wizard, script.SlotIntAI0); got != 0 {
		t.Fatalf("wizard int slot 0 = %d, want 0", got)
	}
	var newest *npc.Hostile
	for _, h := range live {
		if !slices.Contains(before, h) {
			newest = h
		}
	}
	w.wantCasts(t, "wizard as its count ran out", wizard, map[modelskill.ID]ind1Cast{
		ind7bSummonEffect: {wizard.ObjectID(), 1000000}, ind7bSummonMode: {wizard.ObjectID(), 1000000}, ind7bSummonHeal1: {newest.ObjectID(), 1000000},
	})

	w.kill(t, wizard)
	if got := len(w.ind7bLive(ind7bWizardPrivateID)); got != 0 {
		t.Fatalf("live privates after the wizard's death = %d, want 0", got)
	}
}

// TestLV3WizardPrivateBlastsThenDies ports LV3WizardPrivate's timer and
// skill finished hooks, every draw 0. With the character within 100, its
// 1 s timer wants its self range blast (1000000) and attacks the character
// (50). Once a blast finishes, the draw of 0 and the character within 200
// (at 150, where the timer wants no blast) want its second magic on the
// character (3000), which it casts next. Once that magic finishes, the
// private dies.
func TestLV3WizardPrivateBlastsThenDies(t *testing.T) {
	t.Parallel()
	w := bootIndividual7(t, raidBoot{}, ind7bTemplate(ind7bWizardID), ind7bTemplate(ind7bWizardPrivateID), warriorTemplate(ind7MasterID, nil))
	me := w.player.ObjectID()
	master := w.spawn(t, ind7MasterID, 600, 0)
	w.raise(t, master, ind7bWizardID, 300, 0, script.Params{P1: me, P2: me, P3: master.ObjectID()})
	private := w.ind7bLive(ind7bWizardPrivateID)[0]
	w.moveCharacter(t, private, 50, 0)
	w.srv.Settle(t)
	w.clearDesires(t, private)
	w.srv.Advance(t, time.Second)
	w.wantCasts(t, "after 1 s", private, map[modelskill.ID]ind1Cast{ind7bSelfRangeDD1: {private.ObjectID(), 1000000}})
	if got := w.attackWeight(private); got != 50 {
		t.Fatalf("private attack desire after 1 s = %v, want 50", got)
	}

	w.moveCharacter(t, private, 150, 0)
	w.srv.Settle(t)
	w.onQueue(t, private, func() {
		private.AI().Desires().Clear()
		script.NPCOf(private).AddCastDesire(script.NPCOf(private), modelskill.Ref{ID: ind7bSelfRangeDD1, Level: 1}, 1000000)
	})
	var frames [][]byte
	w.srv.AdvanceUntil(t, "the private's death", func() bool {
		frames = append(frames, w.ticks(t, 1)...)
		return private.Dead()
	})
	var casts []string
	for _, f := range framesOf(frames, serverpackets.OpcodeMagicSkillUse, private.ObjectID()) {
		// opcode, caster, target, skill id, then the rest of the cast.
		casts = append(casts, fmt.Sprintf("%d>%d", binary.LittleEndian.Uint32(f[9:13]), int32(binary.LittleEndian.Uint32(f[5:9]))))
	}
	if want := []string{fmt.Sprintf("%d>%d", ind7bSelfRangeDD1, private.ObjectID()), fmt.Sprintf("%d>%d", ind7bSelfRangeDD2, me)}; !slices.Equal(casts, want) {
		t.Fatalf("private casts = %q, want the blast on itself, then the second magic on the character %q", casts, want)
	}
}

// TestLV3RangerSwitchesItsArrowModes ports LV3Ranger's created, attacked,
// skill finished and timer hooks, every draw 0. It spawns wanting its
// arrow defence mode, int slots 0 and 1 at 1 and 0. The character, its top
// desire target, hitting it with its first check skill makes it want a
// physical special at 1000000 (1000 for the other LV3 monsters), clears
// slot 0, wants both arrow modes and sets slot 1
// to 5. Once its defence mode's cast finishes there, 5 s later it is back
// in defence (slot 0 at 1), wanting both modes. A hit by its second check
// skill sets slot 1 to 3; a check skill out of defence changes nothing.
func TestLV3RangerSwitchesItsArrowModes(t *testing.T) {
	t.Parallel()
	w := bootIndividual7(t, raidBoot{}, ind7bTemplate(ind7bRangerID), warriorTemplate(ind7MasterID, nil))
	me := w.player.ObjectID()
	master := w.spawn(t, ind7MasterID, 600, 0)
	ranger := w.raise(t, master, ind7bRangerID, 300, 0, script.Params{P1: me, P2: me, P3: master.ObjectID()})
	self := ranger.ObjectID()
	slots := func() [2]int32 {
		return [2]int32{ind1Slot(ranger, script.SlotIntAI0), ind1Slot(ranger, script.SlotIntAI1)}
	}
	w.wantCasts(t, "after it spawned", ranger, map[modelskill.ID]ind1Cast{ind7bArrowDefence: {self, 1000000}})
	if got := slots(); got != [2]int32{1, 0} {
		t.Fatalf("slots after it spawned = %v, want 1 0", got)
	}

	w.setTop(t, ranger, w.charOf(t))
	w.skillHit(t, ranger, ind7bCheck1, 1)
	if got := slots(); got != [2]int32{0, 5} {
		t.Fatalf("slots after the first check skill = %v, want 0 5", got)
	}
	w.wantCasts(t, "after the first check skill", ranger, map[modelskill.ID]ind1Cast{
		ind7bArrowDefence: {self, 1000000}, ind7bArrowNormal: {self, 1000000}, ind7Special1: {me, 1000000},
	})
	w.clearDesires(t, ranger)
	w.skillHit(t, ranger, ind7bCheck2, 1)
	if got := slots(); got != [2]int32{0, 5} {
		t.Fatalf("slots after a check skill out of defence = %v, want 0 5", got)
	}

	w.castOnce(t, ranger, ind7bArrowDefence)
	w.clearDesires(t, ranger)
	w.srv.Advance(t, 4900*time.Millisecond)
	if got := slots(); got != [2]int32{0, 5} {
		t.Fatalf("slots before 5 s = %v, want 0 5", got)
	}
	w.srv.AdvanceUntil(t, "the defence mode again", func() bool { return slots()[0] == 1 })
	w.wantCasts(t, "back in defence", ranger, map[modelskill.ID]ind1Cast{ind7bArrowDefence: {self, 1000000}, ind7bArrowNormal: {self, 1000000}})

	w.skillHit(t, ranger, ind7bCheck2, 1)
	if got := slots(); got != [2]int32{0, 3} {
		t.Fatalf("slots after the second check skill = %v, want 0 3", got)
	}
}

// LV3SongDance's debuffs, cast at one target (the batch's other debuffs
// are LV3Orc's, cast on itself).
const (
	ind7bSongDebuff1 = modelskill.ID(9632)
	ind7bSongDebuff2 = modelskill.ID(9633)
	ind7bSongDebuff3 = modelskill.ID(9634)
)

// ind7bDebuffs are LV3SongDance's debuffs: the first adds no effect, as
// the character's spell does, the other two their own.
func ind7bDebuffs() []modelskill.Definition {
	defs := []modelskill.Definition{ind1Skill(ind7bSongDebuff1, 600, false), ind1Skill(ind7bSongDebuff2, 600, false), ind1Skill(ind7bSongDebuff3, 600, false)}
	defs[1].EffectID, defs[2].EffectID = 4002, 4003
	return defs
}

// ind7bDancerTemplate is ind7bTemplate of id listing LV3SongDance's
// debuffs.
func ind7bDancerTemplate(id int32) *npc.Template {
	tmpl := ind7bTemplate(id)
	tmpl.SkillsByType[npc.SkillTypeDebuff1] = modelskill.Ref{ID: ind7bSongDebuff1, Level: 1}
	tmpl.SkillsByType[npc.SkillTypeDebuff2] = modelskill.Ref{ID: ind7bSongDebuff2, Level: 1}
	tmpl.SkillsByType[npc.SkillTypeDebuff3] = modelskill.Ref{ID: ind7bSongDebuff3, Level: 1}
	return tmpl
}

// TestLV3SongDanceDebuffsUntilEachLands ports LV3SongDance's created,
// attacked, skill finished and see-spell hooks, every draw 0. The
// character's hit of 10 adds 125 from its own hook and 125 from
// LV3Monster's and wants each of its three debuffs on the character
// (1000000). A finished first debuff sets int slot 0, and the next hit
// wants only the other two. A spell of the player its second spawn
// parameter names clears the slot of each debuff that adds the spell's
// effect: the first only.
func TestLV3SongDanceDebuffsUntilEachLands(t *testing.T) {
	t.Parallel()
	boot := ind1AggroSpellBoot(t)
	boot.opts = append(boot.opts, gameservertest.WithScriptSkills(ind7bDebuffs()...))
	w := bootIndividual7(t, boot, ind7bDancerTemplate(ind7bSongDanceID), warriorTemplate(ind7MasterID, nil))
	me := w.player.ObjectID()
	master := w.spawn(t, ind7MasterID, 600, 0)
	dancer := w.raise(t, master, ind7bSongDanceID, 300, 0, script.Params{P1: me, P2: me, P3: master.ObjectID()})
	slots := func() [3]int32 {
		return [3]int32{ind1Slot(dancer, script.SlotIntAI0), ind1Slot(dancer, script.SlotIntAI1), ind1Slot(dancer, script.SlotIntAI2)}
	}
	w.clearDesires(t, dancer)
	w.hit(t, dancer, 10)
	if got := w.attackWeight(dancer); got != 250 {
		t.Fatalf("dancer attack desire after the hit = %v, want 125 + 125", got)
	}
	w.wantCasts(t, "after the hit", dancer, map[modelskill.ID]ind1Cast{ind7bSongDebuff1: {me, 1000000}, ind7bSongDebuff2: {me, 1000000}, ind7bSongDebuff3: {me, 1000000}})

	w.castOnceAt(t, dancer, w.charOf(t), ind7bSongDebuff1)
	if got := slots(); got != [3]int32{1, 0, 0} {
		t.Fatalf("slots after the first debuff = %v, want 1 0 0", got)
	}
	w.clearDesires(t, dancer)
	w.hit(t, dancer, 10)
	w.wantCasts(t, "after the next hit", dancer, map[modelskill.ID]ind1Cast{ind7bSongDebuff2: {me, 1000000}, ind7bSongDebuff3: {me, 1000000}})

	w.onQueue(t, dancer, func() {
		for _, slot := range []script.IntSlot{script.SlotIntAI0, script.SlotIntAI1, script.SlotIntAI2} {
			dancer.Scratch().SetInt(slot, 1)
		}
	})
	w.castAggroBuff()
	w.srv.AdvanceUntil(t, "the first debuff wanted again", func() bool { return slots()[0] == 0 })
	if got := slots(); got != [3]int32{0, 1, 1} {
		t.Fatalf("slots after the spell = %v, want 0 1 1", got)
	}
	if !w.inWorld(dancer) {
		t.Fatal("the dancer left the world on its own player's spell")
	}
}

// TestLV3SummonerKeepsItsBuff ports LV3Summoner's created and timer hooks,
// every draw 0. Every 3 s, while it does not hold its buff, it drops its
// desires, wants the buff on itself (1000000) and attacks the creature its
// first spawn parameter named (200). Its 1 s check reads the buff's
// abnormal level, so once it holds the buff the 3 s timer leaves it be,
// and once the buff is gone it buffs again.
func TestLV3SummonerKeepsItsBuff(t *testing.T) {
	t.Parallel()
	buff := ind1Skill(ind7bBuff1, 0, true)
	buff.Effects = []modelskill.EffectTemplate{{Name: "Buff", Count: 1, Time: 300, StackType: ind7bBuffStack, StackOrder: 1}}
	w := bootIndividual7(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(buff)}},
		ind7bTemplate(ind7bSummonerID), warriorTemplate(ind7MasterID, nil))
	me := w.player.ObjectID()
	master := w.spawn(t, ind7MasterID, 600, 0)
	summoner := w.raise(t, master, ind7bSummonerID, 300, 0, script.Params{P1: me, P2: me, P3: master.ObjectID()})
	self := summoner.ObjectID()
	w.clearDesires(t, summoner)
	w.srv.Advance(t, 3*time.Second)
	w.wantCasts(t, "after 3 s", summoner, map[modelskill.ID]ind1Cast{ind7bBuff1: {self, 1000000}})
	if got := w.attackWeight(summoner); got != 200 {
		t.Fatalf("summoner attack desire after 3 s = %v, want 200", got)
	}

	held, err := effect.New(effect.Skill{ID: ind7bBuff1, Level: 1}, buff.Effects[0])
	if err != nil {
		t.Fatalf("effect.New(Buff): %v", err)
	}
	held.Effector, held.Effected = summoner, summoner
	w.onQueue(t, summoner, func() { summoner.EffectList().Add(held) })
	w.clearDesires(t, summoner)
	w.srv.Advance(t, 3*time.Second)
	w.wantCasts(t, "after 6 s, holding the buff", summoner, nil)
	if got := ind1Slot(summoner, script.SlotIntAI0); got != 1 {
		t.Fatalf("int slot 0 holding the buff = %d, want 1", got)
	}

	w.onQueue(t, summoner, func() { summoner.EffectList().Remove(held) })
	w.srv.Advance(t, 3*time.Second)
	w.wantCasts(t, "after 9 s, the buff gone", summoner, map[modelskill.ID]ind1Cast{ind7bBuff1: {self, 1000000}})
}
