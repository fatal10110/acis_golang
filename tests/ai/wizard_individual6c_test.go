package ai

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
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
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids of the third PR of the sixth individual behavior batch. The
// saint, DD2 and DD2 curse are the datapack ids their classes bind;
// PartyLeaderWizard and PartyPrivateWizard bind none, so the tests bind
// each to a fixture id.
const (
	i6cSaintID   = int32(21541) // PartyLeaderWizardSaintAggressive
	i6cDD2ID     = int32(21542) // PartyPrivateWizardDD2
	i6cCurseID   = int32(20972) // PartyPrivateWizardDD2Curse
	i6cLeaderID  = int32(29530) // PartyLeaderWizard (fixture binding)
	i6cPrivateID = int32(29531) // PartyPrivateWizard (fixture binding)
	i6cMateID    = int32(29532) // no behavior
	i6cFoeID     = int32(29533) // no behavior
	i6cPlainID   = int32(29534) // no behavior
)

// The skills the saint and the curse list by type, and the character's
// buffs.
const (
	i6cSelfRangeBuffID = modelskill.ID(9380)
	i6cRangeDebuffID   = modelskill.ID(9381)
	i6cLong1ID         = modelskill.ID(9382)
	i6cLong2ID         = modelskill.ID(9383)
	i6cDebuffID        = modelskill.ID(9384)
	i6cAggroBuffID     = modelskill.ID(9385)
	i6cMuteID          = modelskill.ID(9386)
)

// individual6cScripts are the behavior chain down to WizardBase, the
// classes of the PR in scripts.xml order, and the event listener.
func individual6cScripts() ([]script.Listing, script.Catalog) {
	entries := []struct {
		path string
		ctor func() script.Script
	}{
		{"script.ai.individual.DefaultNpc", defaultnpc.New},
		{"script.ai.individual.Monster.MonsterAI", monster.New},
		{"script.ai.individual.Monster.WizardBase.WizardBase", wizard.Base},
		{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizard", boundTo(wizard.PartyLeader, i6cLeaderID)},
		{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardSaintAggressive", wizard.PartyLeaderSaintAggressive},
		{"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizard", boundTo(wizard.PartyPrivate, i6cPrivateID)},
		{"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardDD2", wizard.PartyPrivateDD2},
		{"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardDD2Curse", wizard.PartyPrivateDD2Curse},
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

func bootIndividual6c(t *testing.T, extra raidBoot, templates ...*npc.Template) *individual6World {
	t.Helper()
	list, catalog := individual6cScripts()
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

// i6cLeader is a fixture leader of id with ai as its AI parameters,
// declaring n privates of private, each respawning after 240 s.
func i6cLeader(id int32, ai npc.AIParams, private int32, n int) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	for range n {
		tmpl.Privates = append(tmpl.Privates, npc.PrivateEntry{NpcID: int(private), Weight: 1, RespawnDelay: 240 * time.Second})
	}
	return tmpl
}

// i6cDD2Template is a fixture DD2 wizard of id listing its two DD magics and
// its debuff.
func i6cDD2Template(id int32) *npc.Template {
	return i6bTemplate(id, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeWLongRangeDDMagic: wLongDDID, npc.SkillTypeWShortRangeDDMagic: wShortDDID, npc.SkillTypeDebuff: i6cDebuffID,
	})
}

// i6cDD2Skills are a fixture DD2 wizard's skills, its DD magics costing mp
// MP.
func i6cDD2Skills(mp int) *modelskill.Table {
	long, short := harmlessSkill(wLongDDID), harmlessSkill(wShortDDID)
	long.MPConsume, short.MPConsume = mp, mp
	return modelskill.NewTable([]modelskill.Definition{long, short, harmlessSkill(i6cDebuffID)})
}

// i6cSaintTemplate is a fixture saint listing its buff, debuff and two
// long range DD magics.
func i6cSaintTemplate(privates int) *npc.Template {
	tmpl := i6bTemplate(i6cSaintID, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeSelfRangeBuff: i6cSelfRangeBuffID, npc.SkillTypeRangeDebuff: i6cRangeDebuffID,
		npc.SkillTypeWLongRangeDDMagic1: i6cLong1ID, npc.SkillTypeWLongRangeDDMagic2: i6cLong2ID,
	})
	for range privates {
		tmpl.Privates = append(tmpl.Privates, npc.PrivateEntry{NpcID: int(i6cPlainID), Weight: 1, RespawnDelay: 240 * time.Second})
	}
	return tmpl
}

// i6cSaintSkills are a fixture saint's skills, its range debuff costing mp
// MP.
func i6cSaintSkills(mp int) *modelskill.Table {
	debuff := harmlessSkill(i6cRangeDebuffID)
	debuff.MPConsume = mp
	return modelskill.NewTable([]modelskill.Definition{
		npcSkill(i6cSelfRangeBuffID, 1, false), debuff, harmlessSkill(i6cLong1ID), harmlessSkill(i6cLong2ID),
	})
}

// i6cWants reports whether h wants to cast skill id at target with weight
// 1000000.
func i6cWants(h *npc.Hostile, id modelskill.ID, target int32) bool {
	return i6bCastWeighs(h, id, target, 1000000)
}

// TestPartyPrivateWizardFollowsItsLeaderAndOutlivesIt runs
// PartyPrivateWizard's created, no-desire and 1006 timer hooks. Created, it
// clears int slot 0; with nothing to do it follows its live leader with
// weight 50, and the 10 s check keeps it. The leader's death lets its
// privates go (no master), so the private wanders (5, weight 5) and the
// 10 s check, which only hides a private whose master is dead, keeps it in
// the world.
func TestPartyPrivateWizardFollowsItsLeaderAndOutlivesIt(t *testing.T) {
	t.Parallel()
	w := bootIndividual6c(t, raidBoot{}, i6cLeader(i6cLeaderID, nil, i6cPrivateID, 1), warriorTemplate(i6cPrivateID, nil))
	leader := w.spawn(t, i6cLeaderID, 400, 0)
	private := privatesOf(t, leader, 1)[0]
	if got := private.Scratch().Int(npc.IntAI0); got != 0 {
		t.Fatalf("private int slot 0 = %d, want 0", got)
	}

	w.ticks(t, 2)
	if d, ok := desire(private, actorai.IntentionFollow); !ok || d.Weight != 50 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != leader.ObjectID() {
		t.Fatalf("private follow desire = %+v (%v), want its leader with weight 50", d, ok)
	}
	w.srv.Advance(t, 10*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if !w.inWorld(private) {
		t.Fatal("the 10 s check hid the private of a live leader")
	}

	w.kill(t, leader)
	if private.Master() != nil {
		t.Fatal("the private still has its dead leader as master")
	}
	w.onQueue(t, private, func() { private.AI().Desires().Clear() })
	w.ticks(t, 2)
	if d, ok := desire(private, actorai.IntentionWander); !ok || d.Weight != 5 || d.Timer != 5 {
		t.Fatalf("masterless private wander desire = %+v (%v), want timer 5, weight 5", d, ok)
	}
	for range 2 {
		w.srv.Advance(t, 10*time.Second)
	}
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if !w.inWorld(private) {
		t.Fatal("the 10 s check hid a masterless private")
	}
	if got := framesOf(frames, serverpackets.OpcodeDeleteObject, private.ObjectID()); len(got) != 0 {
		t.Fatalf("DeleteObject frames for the masterless private = %d, want none", len(got))
	}
}

// TestPartyPrivateWizardHatesForItsParty pins PartyPrivateWizard's hate list
// formulas on a level 1 private, every draw 0. A hit of 10 on it enters the
// character at its attacked formula, 10 / 8 * 100 + 300, plus its own
// party call, 10 / 8 * 10: 437.5. A hit of 10 on its leader adds its party
// call again, 12.5. With its leader alive, a hit on its clan member adds
// nothing, however long it has lived. Its death shouts its ShoutMsg4 line.
func TestPartyPrivateWizardHatesForItsParty(t *testing.T) {
	t.Parallel()
	w := bootIndividual6c(t, raidBoot{},
		i6cLeader(i6cLeaderID, nil, i6cPrivateID, 1),
		inClan(warriorTemplate(i6cPrivateID, npc.AIParams{"ShoutMsg4": "1000403"})),
		inClan(warriorTemplate(i6cMateID, nil)))
	leader := w.spawn(t, i6cLeaderID, 400, 0)
	private := privatesOf(t, leader, 1)[0]
	mate := w.spawn(t, i6cMateID, 400, 150)

	w.hit(t, private, 10)
	if got := w.hateListOf(private); got != 437.5 {
		t.Fatalf("private hate list entry after a hit on it = %v, want 425 + 12.5", got)
	}
	w.hit(t, leader, 10)
	if got := w.hateListOf(private); got != 450 {
		t.Fatalf("private hate list entry after a hit on its leader = %v, want 437.5 + 12.5", got)
	}
	w.ticks(t, 8)
	if private.LifeTime() <= 7 {
		t.Fatalf("setup: private life time = %d, want past 7", private.LifeTime())
	}
	w.hit(t, mate, 10)
	if got := w.hateListOf(private); got != 450 {
		t.Fatalf("private hate list entry after a hit on its clan member = %v, want 450 unchanged", got)
	}

	w.kill(t, private)
	w.srv.Advance(t, 3*time.Second)
	shouts := framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeNpcSay, private.ObjectID())
	if got := says(shouts, private.ObjectID()); len(shouts) != 1 || shouts[0][5] != 1 || !slices.Equal(got, []string{script.NpcString(1000403)}) {
		t.Fatalf("private death lines = %q, want one shout of the 1000403 line", got)
	}
}

// TestPartyPrivateWizardTurnsOnItsLeadersTarget runs PartyPrivateWizard's
// script-event hook, every draw 0. A ShoutTarget leader hit by the
// character under 40% of its HP flags the character and broadcasts event
// 10002 with its own id: its private, whose top desire target is not the
// character, says the first of its lines (1000292) and enters the
// character in its hate list at 2000, before its party call adds 12.5. The private answers no other event: the
// leader's 10016 that follows, which would make a WizardBase drop its hate
// list, leaves it. Once the private's top desire target is the character,
// the next shout makes it say nothing and adds nothing.
func TestPartyPrivateWizardTurnsOnItsLeadersTarget(t *testing.T) {
	t.Parallel()
	w := bootIndividual6c(t, raidBoot{}, i6cLeader(i6cLeaderID, npc.AIParams{"ShoutTarget": "1"}, i6cPrivateID, 1), warriorTemplate(i6cPrivateID, nil))
	leader := w.spawn(t, i6cLeaderID, 400, 0)
	private := privatesOf(t, leader, 1)[0]
	w.setPlayerHP(t, 0.3)

	w.hit(t, leader, 10)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got := says(frames, leader.ObjectID()); !slices.Equal(got, []string{script.NpcString(1000291, "Hunter")}) {
		t.Fatalf("leader lines on the shouting hit = %q, want the 1000291 line naming Hunter", got)
	}
	if got := says(frames, private.ObjectID()); !slices.Equal(got, []string{script.NpcString(1000292)}) {
		t.Fatalf("private lines on its leader's shout = %q, want the 1000292 line", got)
	}
	// The leader's target, 2000, then its party call, 10 / 8 * 10.
	if got := w.hateListOf(private); got != 2000+12.5 {
		t.Fatalf("private hate list entry after the shout = %v, want 2012.5", got)
	}

	w.onQueue(t, private, func() { script.NPCOf(private).SetTopDesireTarget(script.PlayerOf(w.player)) })
	w.hit(t, leader, 10)
	frames = w.srv.ReadQueued(t, w.srv.Client)
	if got := says(frames, leader.ObjectID()); len(got) != 1 {
		t.Fatalf("leader lines on the second shouting hit = %q, want one", got)
	}
	if got := says(frames, private.ObjectID()); len(got) != 0 {
		t.Fatalf("private lines on a shout of its top desire target = %q, want none", got)
	}
	if got := w.hateListOf(private); got != 2012.5+12.5 {
		t.Fatalf("private hate list entry after the second shout = %v, want 2025: only its party call", got)
	}
}

// TestPartyPrivateWizardWaitsOutASilence runs PartyPrivateWizard's 1003 and
// 1002 timers, every draw 0. A hit on a silenced private sets its int slot
// 0 and arms 1003 for 10 s; while the silence lasts it re-arms. Once the
// silence is gone, 1003 clears the slot and runs the attacked hook again
// for its most hated with a damage of 100: 425 + 100 / 8 * 100 = 1675.
// Every 10 s, 1002 drops the creatures 1000 or more away from its hate list.
func TestPartyPrivateWizardWaitsOutASilence(t *testing.T) {
	t.Parallel()
	w := bootIndividual6c(t, raidBoot{}, warriorTemplate(i6cPrivateID, nil))
	h := w.spawn(t, i6cPrivateID, 400, 0)
	mute, err := effect.New(effect.Skill{ID: i6cMuteID, Level: 1}, modelskill.EffectTemplate{Name: "Mute", Time: 600})
	if err != nil {
		t.Fatalf("effect.New(Mute): %v", err)
	}
	mute.Effector, mute.Effected = h, h
	w.onQueue(t, h, func() { h.EffectList().Add(mute) })

	w.hit(t, h, 10)
	if got := h.Scratch().Int(npc.IntAI0); got != 1 {
		t.Fatalf("silenced private int slot 0 = %d, want 1", got)
	}
	w.srv.Advance(t, 10*time.Second)
	if got := w.hateListOf(h); got != 425 {
		t.Fatalf("hate list entry while silenced = %v, want 425", got)
	}
	w.onQueue(t, h, func() { h.EffectList().Remove(mute) })
	w.srv.Advance(t, 10*time.Second)
	if got := h.Scratch().Int(npc.IntAI0); got != 0 {
		t.Fatalf("freed private int slot 0 = %d, want 0", got)
	}
	if got := w.hateListOf(h); got != 1675 {
		t.Fatalf("hate list entry after the timer's attacked call = %v, want 1675", got)
	}

	x, y, z := h.Position()
	w.instantTeleport(t, location.Location{X: x - 1200, Y: y, Z: z})
	w.srv.Advance(t, 10*time.Second)
	if got := w.hateListOf(h); got != 0 {
		t.Fatalf("hate list entry of a character 1200 away after 1002 = %v, want none", got)
	}
}

// TestPartyPrivateWizardIgnoresBuffsWithoutADeadMaster runs
// PartyPrivateWizard's see-spell hook: a self buff of the character with
// 100 aggro points enters it in a PartyLeaderWizard's hate list at
// 100 / 8 * 75 = 937.5, but in no private's, neither the one with a live
// leader nor a masterless one.
func TestPartyPrivateWizardIgnoresBuffsWithoutADeadMaster(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	buff := modelskill.Definition{
		ID: i6cAggroBuffID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true, AggroPoints: 100,
	}
	table := modelskill.NewTable([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}, buff})
	w := bootIndividual6c(t, raidBoot{
		opts: []gameservertest.Option{
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(buff),
		},
		ready: func(srv *gameservertest.Server, objID int32) {
			if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(buff.ID), 1); err != nil {
				t.Fatalf("seed skill %d: %v", buff.ID, err)
			}
		},
	}, i6cLeader(i6cLeaderID, nil, i6cPrivateID, 1), warriorTemplate(i6cPrivateID, nil))
	leader := w.spawn(t, i6cLeaderID, 150, 0)
	private := privatesOf(t, leader, 1)[0]
	alone := w.spawn(t, i6cPrivateID, 0, 150)

	castOwnSkill(w.warriorWorld, i6cAggroBuffID)
	w.srv.AdvanceUntil(t, "the buff's hate", func() bool { return w.hateListOf(leader) != 0 })
	if got := w.hateListOf(leader); got != 937.5 {
		t.Fatalf("leader hate list entry after the buff = %v, want 937.5", got)
	}
	for _, h := range []*npc.Hostile{private, alone} {
		if got := w.hateListOf(h); got != 0 {
			t.Fatalf("private %d hate list entry after the buff = %v, want none", h.ObjectID(), got)
		}
	}
}

// TestPartyPrivateWizardDD2StrikesByRange runs PartyPrivateWizardDD2's
// attacked and skill-finished hooks, every draw 0. After the parent hooks
// have entered the character in its hate list, a hit from 50 away wants its
// short range DD magic on the character and one from 300 away its long
// range one. One that cannot pay for its magic sets int slot 0 and wants to
// attack the character with weight 1000 instead; its next hit adds the
// attack formula, 10 / 8 * 100. Each cast that ends wants the magic again
// on its most hated, so the near one casts twice.
func TestPartyPrivateWizardDD2StrikesByRange(t *testing.T) {
	t.Parallel()
	w := bootIndividual6c(t, raidBoot{}, i6cDD2Template(i6cDD2ID))
	near := w.spawn(t, i6cDD2ID, 50, 0)
	far := w.spawn(t, i6cDD2ID, 300, 0)
	broke := w.spawn(t, i6cDD2ID, 0, 50)
	w.srv.InstallNPCCastSeam(t, near, i6cDD2Skills(0))
	w.srv.InstallNPCCastSeam(t, far, i6cDD2Skills(0))
	w.srv.InstallNPCCastSeam(t, broke, i6cDD2Skills(100000))
	me := w.player.ObjectID()

	w.hit(t, near, 10)
	if !castOn(near, wShortDDID, me) {
		t.Fatal("DD2 private 50 away wants no short range DD magic on the character")
	}
	w.hit(t, far, 10)
	if !castOn(far, wLongDDID, me) {
		t.Fatal("DD2 private 300 away wants no long range DD magic on the character")
	}
	w.hit(t, broke, 10)
	if d, ok := desire(broke, actorai.IntentionAttack); broke.Scratch().Int(npc.IntAI0) != 1 || !ok || d.Weight != 1000 {
		t.Fatalf("DD2 private short of MP: int slot 0 %d, attack desire %+v (%v); want 1 and weight 1000", broke.Scratch().Int(npc.IntAI0), d, ok)
	}
	w.hit(t, broke, 10)
	if d, _ := desire(broke, actorai.IntentionAttack); d.Weight != 1125 {
		t.Fatalf("DD2 private short of MP attack desire after its next hit = %v, want 1125", d.Weight)
	}

	var casts int
	for i := 0; casts < 2; i++ {
		if i == 20 {
			t.Fatalf("near DD2 private short range casts = %d, want 2", casts)
		}
		frames := w.ticks(t, 1)
		w.srv.Advance(t, time.Second)
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
		casts += len(framesOf(frames, serverpackets.OpcodeMagicSkillUse, near.ObjectID()))
	}
}

// TestPartyPrivateWizardDD2AnswersItsPartyAndClan runs
// PartyPrivateWizardDD2's party-attacked and clan-attacked hooks, every
// draw 0. A hit on its leader enters the character in its hate list (its
// parent's party call), so it wants its long range DD magic on the
// character 300 away. A settled DD2 private with an empty hate list whose
// clan member is hit wants it too, and, with no dead master, gains no hate.
func TestPartyPrivateWizardDD2AnswersItsPartyAndClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual6c(t, raidBoot{},
		i6cLeader(i6cLeaderID, nil, i6cDD2ID, 1),
		inClan(i6cDD2Template(i6cDD2ID)),
		inClan(warriorTemplate(i6cMateID, nil)))
	me := w.player.ObjectID()
	leader := w.spawn(t, i6cLeaderID, 300, 0)
	private := privatesOf(t, leader, 1)[0]
	w.srv.InstallNPCCastSeam(t, private, i6cDD2Skills(0))
	w.teleport(t, private, 0, 100)
	w.onQueue(t, private, func() { private.AI().Desires().Clear() })

	w.hit(t, leader, 10)
	if !castOn(private, wLongDDID, me) {
		t.Fatal("DD2 private wants no long range DD magic on its leader's attacker")
	}
	if got := w.hateListOf(private); got != 312.5 {
		t.Fatalf("DD2 private hate list entry after a hit on its leader = %v, want 312.5", got)
	}

	clan := w.spawn(t, i6cDD2ID, 0, 300)
	mate := w.spawn(t, i6cMateID, 100, 300)
	w.srv.InstallNPCCastSeam(t, clan, i6cDD2Skills(0))
	w.ticks(t, 8)
	w.onQueue(t, clan, func() { clan.AI().Desires().Clear(); clan.AI().Hates().Clear() })
	w.hit(t, mate, 10)
	if !castOn(clan, wLongDDID, me) {
		t.Fatal("settled DD2 private wants no long range DD magic on its clan member's attacker")
	}
	if got := w.hateListOf(clan); got != 0 {
		t.Fatalf("DD2 private hate list entry after a clan hit = %v, want none", got)
	}
}

// TestPartyPrivateWizardDD2CurseCursesAttackers runs
// PartyPrivateWizardDD2Curse's attacked and clan-attacked hooks, every draw
// 0. A hit of 10 runs its parent's hooks twice (425, then 125 more) and,
// with the character on its hate list and holding no effect of its debuff,
// wants the character cursed, weight 1000000, and struck with its short
// range DD magic, weight 1000000 from each parent call. A settled one whose clan
// member is hit, with an empty hate list, wants the attacker cursed too.
func TestPartyPrivateWizardDD2CurseCursesAttackers(t *testing.T) {
	t.Parallel()
	w := bootIndividual6c(t, raidBoot{}, inClan(i6cDD2Template(i6cCurseID)), inClan(warriorTemplate(i6cMateID, nil)))
	me := w.player.ObjectID()
	h := w.spawn(t, i6cCurseID, 50, 0)
	other := w.spawn(t, i6cCurseID, 300, 0)
	mate := w.spawn(t, i6cMateID, 300, 100)
	w.srv.InstallNPCCastSeam(t, h, i6cDD2Skills(0))
	w.srv.InstallNPCCastSeam(t, other, i6cDD2Skills(0))

	w.hit(t, h, 10)
	if got := w.hateListOf(h); got != 550 {
		t.Fatalf("curse private hate list entry after a hit = %v, want 425 + 125 from the two parent calls", got)
	}
	if !i6cWants(h, i6cDebuffID, me) {
		t.Fatal("curse private wants no debuff on its attacker")
	}
	if !i6bCastWeighs(h, wShortDDID, me, 2000000) {
		t.Fatal("curse private wants no short range DD magic on its attacker, weight 1000000 from each parent call")
	}

	w.ticks(t, 8)
	w.onQueue(t, other, func() { other.AI().Desires().Clear(); other.AI().Hates().Clear() })
	w.hit(t, mate, 10)
	if !i6cWants(other, i6cDebuffID, me) {
		t.Fatal("settled curse private wants no debuff on its clan member's attacker")
	}
}

// TestPartyLeaderWizardSaintBuffsOnATimer runs
// PartyLeaderWizardSaintAggressive's 1005 timer, every draw 0: 5 s after it
// spawns it wants its self range buff on itself, weight 1000000, and again
// 120 s later.
func TestPartyLeaderWizardSaintBuffsOnATimer(t *testing.T) {
	t.Parallel()
	w := bootIndividual6c(t, raidBoot{}, i6cSaintTemplate(0))
	h := w.spawn(t, i6cSaintID, 1500, 0)
	w.srv.InstallNPCCastSeam(t, h, i6cSaintSkills(0))
	if _, ok := castDesireOf(h, i6cSelfRangeBuffID); ok {
		t.Fatal("saint wants its buff as it spawns")
	}
	w.srv.Advance(t, 5*time.Second)
	if !i6cWants(h, i6cSelfRangeBuffID, h.ObjectID()) {
		t.Fatal("saint wants no self range buff on itself 5 s after it spawned")
	}
	w.onQueue(t, h, func() { h.AI().Desires().Clear() })
	w.srv.Advance(t, 119*time.Second)
	if _, ok := castDesireOf(h, i6cSelfRangeBuffID); ok {
		t.Fatal("saint wants its buff again before 120 s")
	}
	w.srv.Advance(t, time.Second)
	if !i6cWants(h, i6cSelfRangeBuffID, h.ObjectID()) {
		t.Fatal("saint wants no self range buff on itself 120 s after the first")
	}
}

// TestPartyLeaderWizardSaintSeesAndStrikes runs the saint's see-creature
// hook. Young, it does nothing on sight. Settled in its territory with an
// empty hate list, the character it sees enters its hate list and, on a
// draw under 33, is wanted under its range debuff, weight 1000000; on a
// higher draw, under its first long range DD magic.
func TestPartyLeaderWizardSaintSeesAndStrikes(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		draw  int32
		young bool
		want  modelskill.ID
	}{{"debuff", 0, false, i6cRangeDebuffID}, {"strike", 50, false, i6cLong1ID}, {"young", 0, true, 0}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual6c(t, raidBoot{}, i6cSaintTemplate(0))
			h := w.spawn(t, i6cSaintID, 1500, 0)
			w.srv.InstallNPCCastSeam(t, h, i6cSaintSkills(0))
			if !row.young {
				w.ticks(t, 9)
			}
			w.draw.Store(row.draw)
			w.moveNextTo(t, h, 300, 0)
			w.ticks(t, 1)
			if row.young {
				for _, id := range []modelskill.ID{i6cRangeDebuffID, i6cLong1ID} {
					if d, ok := castDesireOf(h, id); ok {
						t.Fatalf("young saint desire of %d = %+v, want none", id, d)
					}
				}
				if got := w.hateListOf(h); got != 0 {
					t.Fatalf("young saint hate list entry = %v, want none", got)
				}
				return
			}
			if !i6cWants(h, row.want, w.player.ObjectID()) {
				t.Fatalf("settled saint wants no skill %d on the character it sees", row.want)
			}
			if w.hateListOf(h) == 0 {
				t.Fatal("settled saint did not enter the character it sees in its hate list")
			}
		})
	}
}

// TestPartyLeaderWizardSaintAnswersHits runs the saint's attacked hook. On a
// draw under 80, a hit wants its first long range DD magic on the attacker.
// On a higher draw, with the attacker in no party and the top desire
// target, it casts nothing; with another top desire target it wants its
// range debuff on the attacker (a draw over 33). A saint that cannot pay
// for the debuff sets int slot 0 and wants to attack the character with
// weight 1000; then its next hit adds the attack formula, 10 / 8 * 100.
// Once the character leads a party, it is answered with the range debuff
// even as the top desire target.
func TestPartyLeaderWizardSaintAnswersHits(t *testing.T) {
	t.Parallel()
	w := bootIndividual6c(t, raidBoot{}, i6cSaintTemplate(0), warriorTemplate(i6cFoeID, nil))
	me := w.player.ObjectID()
	h := w.spawn(t, i6cSaintID, 150, 0)
	foe := w.spawn(t, i6cFoeID, 200, 0)
	w.srv.InstallNPCCastSeam(t, h, i6cSaintSkills(0))

	w.hit(t, h, 10)
	if !i6cWants(h, i6cLong1ID, me) {
		t.Fatal("saint wants no first long range DD magic on its attacker")
	}

	w.draw.Store(99)
	w.onQueue(t, h, func() {
		h.AI().Desires().Clear()
		script.NPCOf(h).SetTopDesireTarget(script.PlayerOf(w.player))
	})
	w.hit(t, h, 10)
	for _, id := range []modelskill.ID{i6cLong1ID, i6cLong2ID, i6cRangeDebuffID} {
		if d, ok := castDesireOf(h, id); ok {
			t.Fatalf("saint answering its top desire target wants skill %d: %+v", id, d)
		}
	}

	w.onQueue(t, h, func() {
		h.AI().Desires().Clear()
		script.NPCOf(h).SetTopDesireTarget(script.NPCOf(foe))
	})
	w.hit(t, h, 10)
	if !i6cWants(h, i6cRangeDebuffID, me) {
		t.Fatal("saint with another top desire target wants no range debuff on its attacker")
	}

	broke := w.spawn(t, i6cSaintID, 0, 150)
	w.srv.InstallNPCCastSeam(t, broke, i6cSaintSkills(100000))
	w.onQueue(t, broke, func() { script.NPCOf(broke).SetTopDesireTarget(script.NPCOf(foe)) })
	w.hit(t, broke, 10)
	if got := broke.Scratch().Int(npc.IntAI0); got != 1 {
		t.Fatalf("saint short of MP int slot 0 = %d, want 1", got)
	}
	if got := i6cAttackWeight(broke, me); got != 1000 {
		t.Fatalf("saint short of MP attack desire on the character = %v, want 1000", got)
	}
	w.hit(t, broke, 10)
	if got := i6cAttackWeight(broke, me); got != 1125 {
		t.Fatalf("saint short of MP attack desire after its next hit = %v, want 1125", got)
	}

	// In a party, the character is answered even as the top desire target.
	w.joinMate(t)
	w.onQueue(t, h, func() {
		h.AI().Desires().Clear()
		script.NPCOf(h).SetTopDesireTarget(script.PlayerOf(w.player))
	})
	w.hit(t, h, 10)
	if !i6cWants(h, i6cRangeDebuffID, me) {
		t.Fatal("saint wants no range debuff on its attacker in a party, its top desire target")
	}
}

// i6cAttackWeight returns the weight of h's attack desire on target, 0 for
// none.
func i6cAttackWeight(h *npc.Hostile, target int32) float64 {
	for _, d := range h.AI().Desires().Snapshot() {
		if d.Kind == actorai.IntentionAttack && d.FinalTarget != nil && d.FinalTarget.ObjectID() == target {
			return d.Weight
		}
	}
	return 0
}

// TestPartyLeaderWizardSaintAnswersItsPartyAndClan runs the saint's
// party-attacked and clan-attacked hooks. A hit on its private wants, on a
// draw under 80, its second long range DD magic on the attacker, else its
// range debuff. A settled saint with an empty hate list whose clan member
// is hit wants its first long range DD magic on the attacker.
func TestPartyLeaderWizardSaintAnswersItsPartyAndClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual6c(t, raidBoot{}, inClan(i6cSaintTemplate(1)), warriorTemplate(i6cPlainID, nil), inClan(warriorTemplate(i6cMateID, nil)))
	me := w.player.ObjectID()
	h := w.spawn(t, i6cSaintID, 300, 0)
	private := privatesOf(t, h, 1)[0]
	w.srv.InstallNPCCastSeam(t, h, i6cSaintSkills(0))
	w.onQueue(t, h, func() { h.AI().Desires().Clear() })

	w.hit(t, private, 10)
	if !i6cWants(h, i6cLong2ID, me) {
		t.Fatal("saint wants no second long range DD magic on its private's attacker")
	}
	w.draw.Store(80)
	w.onQueue(t, h, func() { h.AI().Desires().Clear() })
	w.hit(t, private, 10)
	if !i6cWants(h, i6cRangeDebuffID, me) {
		t.Fatal("saint wants no range debuff on its private's attacker on a draw of 80")
	}

	w.draw.Store(0)
	mate := w.spawn(t, i6cMateID, 300, 100)
	w.ticks(t, 8)
	w.onQueue(t, h, func() { h.AI().Desires().Clear(); h.AI().Hates().Clear() })
	w.hit(t, mate, 10)
	if !i6cWants(h, i6cLong1ID, me) {
		t.Fatal("settled saint wants no first long range DD magic on its clan member's attacker")
	}
}

// TestPartyLeaderWizardSaintKeepsItsDistance runs the saint's skill-finished
// and move-finished hooks, every draw 0, with the character on its hate
// list. A cast that ends with the character 50 away makes it flee the
// character (its flee desire, weight 1000000, outweighs the rest); one that
// ends with the character 300 away wants its first long range DD magic on
// it, and so does a walk that ends.
func TestPartyLeaderWizardSaintKeepsItsDistance(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		dist int
		walk bool
	}{{"near cast", 50, false}, {"far cast", 300, false}, {"walk", 300, true}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual6c(t, raidBoot{}, i6cSaintTemplate(0))
			me := w.player.ObjectID()
			h := w.spawn(t, i6cSaintID, row.dist, 0)
			w.srv.InstallNPCCastSeam(t, h, i6cSaintSkills(0))
			w.onQueue(t, h, func() { h.AI().Hates().Add(w.player, 100) })
			if _, ok := castDesireOf(h, i6cLong1ID); ok {
				t.Fatal("setup: the saint wants its DD magic before it acts")
			}

			if row.walk {
				x, y, z := h.Position()
				to := location.Location{X: x + 100, Y: y, Z: z}
				if !h.AI().AddMoveToDesire(to, 1e9) {
					t.Fatalf("AddMoveToDesire(%+v) = false, want the walk queued", to)
				}
				for i := 0; !h.IsMoving(); i++ {
					if i == 5 {
						t.Fatalf("no walk to %+v after five cycles", to)
					}
					cycle(t, w.srv)
				}
				for i := 0; h.IsMoving(); i++ {
					if i >= int(20*time.Second/move.PositionUpdateInterval) {
						t.Fatal("walk never arrived")
					}
					w.srv.TickPositions()
				}
				w.srv.Settle(t)
				if !i6cWants(h, i6cLong1ID, me) {
					t.Fatal("saint wants no first long range DD magic on the character after its walk")
				}
				return
			}

			w.onQueue(t, h, func() {
				script.NPCOf(h).AddCastDesire(script.NPCOf(h), modelskill.Ref{ID: i6cSelfRangeBuffID, Level: 1}, 1e9)
			})
			for i := 0; ; i++ {
				if i == 20 {
					t.Fatalf("the saint %d away never answered the end of its cast", row.dist)
				}
				w.ticks(t, 1)
				w.srv.Advance(t, time.Second)
				if row.dist < 100 && h.AI().CurrentIntention() == actorai.IntentionFlee {
					return
				}
				if row.dist >= 100 && i6cWants(h, i6cLong1ID, me) {
					return
				}
			}
		})
	}
}
