package ai

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
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

// The NPC ids of the sixth individual behavior batch's tests. Elcroki and
// its silhouettes are datapack ids; PartyLeaderWizard and
// PartyLeaderWizardDD2 bind no id of their own, so the tests bind each to
// a fixture id, as the children that bind ids will.
const (
	plwLeaderID     = int32(29501) // PartyLeaderWizard (fixture binding)
	plwSummonerID   = int32(29502) // PartyLeaderWizard (fixture binding)
	plwClanLeaderID = int32(29503) // PartyLeaderWizard (fixture binding)
	plwDD2NearID    = int32(29504) // PartyLeaderWizardDD2 (fixture binding)
	plwDD2BrokeID   = int32(29505) // PartyLeaderWizardDD2 (fixture binding)
	plwDD2ClanID    = int32(29506) // PartyLeaderWizardDD2 (fixture binding)
	plwDD2PartyID   = int32(29507) // PartyLeaderWizardDD2 (fixture binding)
	plwPrivateID    = int32(29510) // no behavior
	plwClanMateID   = int32(29511) // no behavior
	plwListenerID   = int32(29512) // eventListener
	elcrokiID       = int32(22214) // Elcroki
	silhouetteOneID = int32(22209) // no behavior here
	silhouetteTwoID = int32(22212) // no behavior here
)

// The skills the batch's NPCs list by type.
const (
	wLongDDID    = modelskill.ID(9350)
	wShortDDID   = modelskill.ID(9351)
	elLongDDID   = modelskill.ID(9352)
	elShortDDID  = modelskill.ID(9353)
	elBuffID     = modelskill.ID(9354)
	elDebuffID   = modelskill.ID(9355)
	plwMuteID    = modelskill.ID(9356)
	plwAggroBuff = modelskill.ID(9357)
	plwPlainBuff = modelskill.ID(9358)
)

// boundTo returns ctor's script bound to id.
func boundTo(ctor func() script.Script, id ...int32) func() script.Script {
	return func() script.Script {
		s := ctor()
		s.NPCs = id
		return s
	}
}

// eventListener says every script event its NPC hears as
// "event <id> <arg1>", so a test sees through packets the events the
// batch broadcasts.
func eventListener() script.Script {
	return script.Script{Behavior: true, NPCs: []int32{plwListenerID}, Hooks: script.Hooks{
		OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
			e.NPC.Say(fmt.Sprintf("event %d %d", e.EventID, e.Arg1))
		},
	}}
}

// individual6Scripts are the behavior chain down to WizardBase, the
// batch's classes in scripts.xml order, and the event listener.
func individual6Scripts() ([]script.Listing, script.Catalog) {
	entries := []struct {
		path string
		ctor func() script.Script
	}{
		{"script.ai.individual.DefaultNpc", defaultnpc.New},
		{"script.ai.individual.Monster.MonsterAI", monster.New},
		{"script.ai.individual.Monster.WizardBase.WizardBase", wizard.Base},
		{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.Elcroki", wizard.Elcroki},
		{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizard", boundTo(wizard.PartyLeader, plwLeaderID, plwSummonerID, plwClanLeaderID)},
		{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardDD2", boundTo(wizard.PartyLeaderDD2, plwDD2NearID, plwDD2BrokeID, plwDD2ClanID, plwDD2PartyID)},
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

// individual6World is one character beside the batch's fixture NPCs, every
// script draw the current value of draw (n-1 for a draw of [0, n) it is not
// inside).
type individual6World struct {
	*warriorWorld
	draw *atomic.Int32
}

func bootIndividual6(t *testing.T, extra raidBoot, templates ...*npc.Template) *individual6World {
	t.Helper()
	list, catalog := individual6Scripts()
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

// plwTemplate is a fixture party leader wizard of id with ai as its AI
// parameters, declaring privates of the fixture private, each respawning
// after 240 s.
func plwTemplate(id int32, ai npc.AIParams, privates int) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	tmpl.Name = "PartyLeaderWizard"
	for range privates {
		tmpl.Privates = append(tmpl.Privates, npc.PrivateEntry{NpcID: int(plwPrivateID), Weight: 1, RespawnDelay: 240 * time.Second})
	}
	return tmpl
}

// inClan puts tmpl in the batch's test clan, with a clan range of 500.
func inClan(tmpl *npc.Template) *npc.Template {
	tmpl.Clans, tmpl.ClanRange = []string{"wizard_clan"}, 500
	return tmpl
}

// hateListOf returns h's hate list entry of the character.
func (w *individual6World) hateListOf(h *npc.Hostile) float64 {
	return h.AI().Hates().Hate(w.player)
}

// setPlayerHP sets the character's HP to share of its maximum.
func (w *individual6World) setPlayerHP(t *testing.T, share float64) {
	t.Helper()
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		c := w.player.(player.CharacterHolder).PlayerCharacter()
		c.SetHP(c.MaxHPValue() * share)
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
}

// castOn reports whether h wants to cast skill id at target with weight
// 1000000.
func castOn(h *npc.Hostile, id modelskill.ID, target int32) bool {
	d, ok := castDesireOf(h, id)
	return ok && d.Weight == 1000000 && d.FinalTarget != nil && d.FinalTarget.ObjectID() == target
}

// harmlessSkill is a one-target offensive magic of id that deals no damage
// and lands no effect, so a test's NPCs can cast it at the character for
// as long as the test runs.
func harmlessSkill(id modelskill.ID) modelskill.Definition {
	return modelskill.Definition{
		ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		Offensive: true, Magic: true, SkillType: "DEBUFF", CastRange: 600, HitTime: 500, StaticHitTime: true, StaticReuse: true,
	}
}

// livePrivates returns master's privates that are not dead, sorted by
// object id, failing unless there are want of them.
func livePrivates(t *testing.T, master *npc.Hostile, want int) []*npc.Hostile {
	t.Helper()
	var out []*npc.Hostile
	for _, m := range master.Minions() {
		if h := m.(*npc.Hostile); !h.Dead() {
			out = append(out, h)
		}
	}
	if len(out) != want {
		t.Fatalf("master %d live privates = %d, want %d", master.NpcID(), len(out), want)
	}
	slices.SortFunc(out, func(a, b *npc.Hostile) int { return int(a.ObjectID() - b.ObjectID()) })
	return out
}

// TestPartyLeaderWizardCreatesItsPrivates runs PartyLeaderWizard's two
// private triggers. With no SummonPrivateRate it creates its two declared
// privates as it spawns, with weight point 10 and int slot 2 set. With a
// SummonPrivateRate of 30 it creates none at spawn; a hit drawing 30 (not
// under the rate) still creates none, a hit drawing 0 creates both and
// says the first of its summoning lines (1000294), and a later hit
// creates no more.
func TestPartyLeaderWizardCreatesItsPrivates(t *testing.T) {
	t.Parallel()
	w := bootIndividual6(t, raidBoot{},
		plwTemplate(plwLeaderID, nil, 2),
		plwTemplate(plwSummonerID, npc.AIParams{"SummonPrivateRate": "30"}, 2),
		warriorTemplate(plwPrivateID, nil))

	leader := w.spawn(t, plwLeaderID, 400, 0)
	privatesOf(t, leader, 2)
	if got, want := [2]int32{leader.Scratch().Int(npc.IntWeightPoint), leader.Scratch().Int(npc.IntAI2)}, [2]int32{10, 1}; got != want {
		t.Fatalf("leader weight point and int slot 2 = %v, want %v", got, want)
	}

	summoner := w.spawn(t, plwSummonerID, 400, 200)
	privatesOf(t, summoner, 0)
	if got := summoner.Scratch().Int(npc.IntAI2); got != 0 {
		t.Fatalf("summoner int slot 2 = %d, want 0", got)
	}
	w.draw.Store(30)
	w.hit(t, summoner, 10)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), summoner.ObjectID()); len(got) != 0 {
		t.Fatalf("summoner lines on a hit drawing 30 = %q, want none", got)
	}
	privatesOf(t, summoner, 0)

	w.draw.Store(0)
	w.hit(t, summoner, 10)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), summoner.ObjectID()); !slices.Equal(got, []string{script.NpcString(1000294)}) {
		t.Fatalf("summoner lines on the summoning hit = %q, want the 1000294 line", got)
	}
	privatesOf(t, summoner, 2)
	if got := summoner.Scratch().Int(npc.IntAI2); got != 1 {
		t.Fatalf("summoner int slot 2 after summoning = %d, want 1", got)
	}

	w.hit(t, summoner, 10)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), summoner.ObjectID()); len(got) != 0 {
		t.Fatalf("summoner lines on a later hit = %q, want none", got)
	}
	privatesOf(t, summoner, 2)
}

// TestPartyLeaderWizardHatesForItsPartyAndClan pins PartyLeaderWizard's
// hate list formulas on a level 1 leader, every draw 0. A clan member's
// hit before the leader has lived 7 AI cycles adds nothing. Once it has, a
// clan member's hit of 10 enters the character at hate ratio 0 * 10 * 30
// * 10 + 300 = 300, and another adds 0. A hit of 10 on its private adds
// the party formula 10 / 8 * 10 * the private's weight point 1 * 10 =
// 125. A hit of 10 on the leader adds its attacked formula, 10 / 8 * 100
// = 125 (its own clan call adds 0).
func TestPartyLeaderWizardHatesForItsPartyAndClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual6(t, raidBoot{},
		inClan(plwTemplate(plwClanLeaderID, nil, 1)),
		warriorTemplate(plwPrivateID, nil),
		inClan(warriorTemplate(plwClanMateID, nil)))
	leader := w.spawn(t, plwClanLeaderID, 400, 0)
	private := privatesOf(t, leader, 1)[0]
	mate := w.spawn(t, plwClanMateID, 400, 150)

	w.hit(t, mate, 10)
	if got := w.hateListOf(leader); got != 0 {
		t.Fatalf("leader hate list entry after a clan hit at life time %d = %v, want none", leader.LifeTime(), got)
	}
	w.ticks(t, 8)
	if leader.LifeTime() <= 7 {
		t.Fatalf("leader life time = %d, want past 7", leader.LifeTime())
	}
	for i, want := range []float64{300, 300} {
		w.hit(t, mate, 10)
		if got := w.hateListOf(leader); got != want {
			t.Fatalf("leader hate list entry after clan hit %d = %v, want %v", i+1, got, want)
		}
	}
	w.hit(t, private, 10)
	if got := w.hateListOf(leader); got != 425 {
		t.Fatalf("leader hate list entry after a hit on its private = %v, want 300 + 125", got)
	}
	w.hit(t, leader, 10)
	if got := w.hateListOf(leader); got != 550 {
		t.Fatalf("leader hate list entry after a hit on itself = %v, want 425 + 125", got)
	}
}

// TestPartyLeaderWizardShoutsTargetAndDeath runs a ShoutTarget leader with
// a ShoutMsg4, every draw 0. A hit of 10 by the character under 40% of its
// HP enters it at 425 as the most hated, so the leader names it in the
// first of its target lines (1000291), restarts its hate list at
// 10 / 8 * 1000 + 300 = 1550, keeps the character's id as its flag and
// broadcasts event 10002 with its own id; WizardBase then broadcasts its
// shout-target event 10016 with the character's id. The leader's death
// shouts its ShoutMsg4 line.
func TestPartyLeaderWizardShoutsTargetAndDeath(t *testing.T) {
	t.Parallel()
	w := bootIndividual6(t, raidBoot{},
		plwTemplate(plwLeaderID, npc.AIParams{"ShoutTarget": "1", "ShoutMsg4": "1000403"}, 0),
		warriorTemplate(plwListenerID, nil))
	leader := w.spawn(t, plwLeaderID, 400, 0)
	listener := w.spawn(t, plwListenerID, 400, 100)
	w.setPlayerHP(t, 0.3)

	w.hit(t, leader, 10)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got := says(frames, leader.ObjectID()); !slices.Equal(got, []string{script.NpcString(1000291, "Hunter")}) {
		t.Fatalf("leader lines on the shouting hit = %q, want the 1000291 line naming Hunter", got)
	}
	want := []string{fmt.Sprintf("event 10002 %d", leader.ObjectID()), fmt.Sprintf("event 10016 %d", w.player.ObjectID())}
	if got := says(frames, listener.ObjectID()); !slices.Equal(got, want) {
		t.Fatalf("events heard = %q, want %q", got, want)
	}
	if got := w.hateListOf(leader); got != 1550 {
		t.Fatalf("leader hate list entry after the shout = %v, want 1550", got)
	}
	if got := leader.Scratch().Int(npc.IntFlag); got != w.player.ObjectID() {
		t.Fatalf("leader flag = %d, want the character's id %d", got, w.player.ObjectID())
	}

	w.onQueue(t, leader, func() { script.NPCOf(leader).DoDie(nil) })
	w.srv.Advance(t, 3*time.Second)
	shouts := framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeNpcSay, leader.ObjectID())
	if len(shouts) != 1 || shouts[0][5] != 1 {
		t.Fatalf("leader death lines = %d, want one shout", len(shouts))
	}
	if got := says(shouts, leader.ObjectID()); !slices.Equal(got, []string{script.NpcString(1000403)}) {
		t.Fatalf("leader death shout = %q, want the 1000403 line", got)
	}
}

// TestPartyLeaderWizardRespawnsADeadPrivate runs PartyLeaderWizard's
// party-died hook: a private's death, with the leader alive, asks its
// spawn to bring it back after its declared delay.
func TestPartyLeaderWizardRespawnsADeadPrivate(t *testing.T) {
	t.Parallel()
	w := bootIndividual6(t, raidBoot{}, plwTemplate(plwLeaderID, nil, 1), warriorTemplate(plwPrivateID, nil))
	leader := w.spawn(t, plwLeaderID, 400, 0)
	private := privatesOf(t, leader, 1)[0]
	w.kill(t, private)
	w.armedPrivateRespawn(t)
}

// TestPartyLeaderWizardTimers runs PartyLeaderWizard's 1003 and 1002
// timers, every draw 0. A hit on a silenced leader sets its int slot 0 and
// arms 1003 for 10 s; while the silence lasts it re-arms. Once the silence
// is gone, 1003 clears the slot and runs the attacked hook again for its
// most hated with a damage of 100: 425 + 100 / 8 * 100 = 1675. Every 10 s,
// 1002 drops the creatures 1000 or more away from its hate list.
func TestPartyLeaderWizardTimers(t *testing.T) {
	t.Parallel()
	w := bootIndividual6(t, raidBoot{}, plwTemplate(plwLeaderID, nil, 0))
	h := w.spawn(t, plwLeaderID, 400, 0)
	mute, err := effect.New(effect.Skill{ID: plwMuteID, Level: 1}, modelskill.EffectTemplate{Name: "Mute", Time: 600})
	if err != nil {
		t.Fatalf("effect.New(Mute): %v", err)
	}
	mute.Effector, mute.Effected = h, h
	w.onQueue(t, h, func() { h.EffectList().Add(mute) })

	w.hit(t, h, 10)
	if got := h.Scratch().Int(npc.IntAI0); got != 1 {
		t.Fatalf("silenced leader int slot 0 = %d, want 1", got)
	}
	w.srv.Advance(t, 10*time.Second)
	if got := w.hateListOf(h); got != 425 {
		t.Fatalf("hate list entry while silenced = %v, want 425", got)
	}
	w.onQueue(t, h, func() { h.EffectList().Remove(mute) })
	w.srv.Advance(t, 10*time.Second)
	if got := h.Scratch().Int(npc.IntAI0); got != 0 {
		t.Fatalf("freed leader int slot 0 = %d, want 0", got)
	}
	if got := w.hateListOf(h); got != 1675 {
		t.Fatalf("hate list entry after the timer's attacked call = %v, want 1675", got)
	}

	x, y, z := h.Position()
	w.instantTeleport(t, location.Location{X: x - 1200, Y: y, Z: z})
	if !h.Knows(w.player) {
		t.Fatal("the leader no longer knows the character")
	}
	w.srv.Advance(t, 10*time.Second)
	if got := w.hateListOf(h); got != 0 {
		t.Fatalf("hate list entry of a character 1200 away after 1002 = %v, want none", got)
	}
}

// TestPartyLeaderWizardSeesBuffs runs PartyLeaderWizard's see-spell hook: a
// self buff of the character with 100 aggro points enters it in the hate
// list of a level 1 leader at 100 / 8 * 75 = 937.5; a buff with none adds
// nothing.
func TestPartyLeaderWizardSeesBuffs(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	buff := func(id modelskill.ID, aggro int) modelskill.Definition {
		return modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true, AggroPoints: aggro,
		}
	}
	defs := []modelskill.Definition{buff(plwAggroBuff, 100), buff(plwPlainBuff, 0)}
	table := modelskill.NewTable(append([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}}, defs...))
	w := bootIndividual6(t, raidBoot{
		opts: []gameservertest.Option{
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(defs...),
		},
		ready: func(srv *gameservertest.Server, objID int32) {
			for _, d := range defs {
				if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(d.ID), 1); err != nil {
					t.Fatalf("seed skill %d: %v", d.ID, err)
				}
			}
		},
	}, plwTemplate(plwLeaderID, nil, 0))
	h := w.spawn(t, plwLeaderID, 150, 0)

	castOwnSkill(w.warriorWorld, plwPlainBuff)
	w.srv.Advance(t, 2*time.Second)
	if got := w.hateListOf(h); got != 0 {
		t.Fatalf("hate list entry after a buff with no aggro points = %v, want none", got)
	}
	castOwnSkill(w.warriorWorld, plwAggroBuff)
	w.srv.AdvanceUntil(t, "the buff's hate", func() bool { return w.hateListOf(h) != 0 })
	if got := w.hateListOf(h); got != 937.5 {
		t.Fatalf("hate list entry after a buff with 100 aggro points = %v, want 937.5", got)
	}
}

// elcrokiTemplate is a fixture Elcroki of level 1 with the datapack's AI
// parameters (aggressive type 1, silhouettes 22209 and 22212, a long
// range skill distance of 100, broadcasts within 300) and the four skill
// types it reads.
func elcrokiTemplate(ai npc.AIParams) *npc.Template {
	params := npc.AIParams{"silhouette1": "22209", "silhouette2": "22212", "LongRangeSkillDist": "100", "BroadCastRange": "300", "max_spawn_privates": "5"}
	for k, v := range ai {
		params[k] = v
	}
	tmpl := warriorTemplate(elcrokiID, params)
	tmpl.Name = "Elroki"
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypeLongRangeDDMagic1: {ID: elLongDDID, Level: 1},
		npc.SkillTypeSelfRangeDDMagic1: {ID: elShortDDID, Level: 1},
		npc.SkillTypeSelfRangeBuff1:    {ID: elBuffID, Level: 1},
		npc.SkillTypeSelfRangeDebuff1:  {ID: elDebuffID, Level: 1},
	}
	return tmpl
}

// elcrokiSkills are the skills a fixture Elcroki casts.
func elcrokiSkills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{
		harmlessSkill(elLongDDID), harmlessSkill(elShortDDID), npcSkill(elBuffID, 1, false), npcSkill(elDebuffID, 1, false),
	})
}

// TestElcrokiCallsSilhouettesOnSightAndAsTheyDie runs Elcroki's
// see-creature and party-died hooks, every draw 0. Seeing the character
// 150 away with an empty hate list, it places two silhouette1 NPCs at its
// own x and y (draws of 0), each told (1000, the character), counts them
// in int slots 3 and 4, wants its long range DD magic (farther than 100)
// and its self range one on the character, and enters it in its hate
// list; with the list no longer empty it calls no more. In combat, the
// death of one silhouette brings two more (int slot 3 back to 1, then 3;
// slot 4 to 4), told the top desire target; the next death, with slot 3 at
// 2, brings none.
func TestElcrokiCallsSilhouettesOnSightAndAsTheyDie(t *testing.T) {
	t.Parallel()
	w := bootIndividual6(t, raidBoot{}, elcrokiTemplate(npc.AIParams{"ag_type": "1"}), warriorTemplate(silhouetteOneID, nil), warriorTemplate(silhouetteTwoID, nil))
	h := w.spawn(t, elcrokiID, 150, 0)
	w.srv.InstallNPCCastSeam(t, h, elcrokiSkills())
	x, y, _ := h.Position()

	w.ticks(t, 1)
	silhouettes := privatesOf(t, h, 2)
	for _, s := range silhouettes {
		sx, sy, _ := s.Position()
		if int32(s.Instance.Template.ID) != silhouetteOneID || sx != x || sy != y {
			t.Errorf("silhouette %d at %d,%d, want a %d at %d,%d", s.Instance.Template.ID, sx, sy, silhouetteOneID, x, y)
		}
		if got := [2]int32{s.Scratch().Int(npc.IntParam1), s.Scratch().Int(npc.IntParam2)}; got != [2]int32{1000, w.player.ObjectID()} {
			t.Errorf("silhouette spawn parameters = %v, want 1000 and the character", got)
		}
	}
	if got := [2]int32{h.Scratch().Int(npc.IntAI3), h.Scratch().Int(npc.IntAI4)}; got != [2]int32{2, 2} {
		t.Fatalf("int slots 3 and 4 = %v, want 2 2", got)
	}
	for _, id := range []modelskill.ID{elLongDDID, elShortDDID} {
		if !castOn(h, id, w.player.ObjectID()) {
			t.Fatalf("Elcroki wants no skill %d on the character it sees, weight 1000000", id)
		}
	}
	if w.hateListOf(h) == 0 {
		t.Fatal("Elcroki did not enter the character it sees in its hate list")
	}
	w.ticks(t, 2)
	privatesOf(t, h, 2)

	var top int32
	w.onQueue(t, h, func() {
		h.SetInCombat(true)
		if c := script.NPCOf(h).TopDesireTarget(); c != nil {
			top = c.ObjectID()
		}
	})
	w.kill(t, silhouettes[0])
	if got := [2]int32{h.Scratch().Int(npc.IntAI3), h.Scratch().Int(npc.IntAI4)}; got != [2]int32{3, 4} {
		t.Fatalf("int slots 3 and 4 after a silhouette death = %v, want 3 4", got)
	}
	live := livePrivates(t, h, 3)
	for _, s := range live[1:] {
		want := [2]int32{0, 0}
		if top != 0 {
			want = [2]int32{1000, top}
		}
		if got := [2]int32{s.Scratch().Int(npc.IntParam1), s.Scratch().Int(npc.IntParam2)}; got != want {
			t.Errorf("new silhouette spawn parameters = %v, want %v", got, want)
		}
	}
	w.kill(t, live[0])
	livePrivates(t, h, 2)
	if got := h.Scratch().Int(npc.IntAI3); got != 2 {
		t.Fatalf("int slot 3 after a death at 3 = %d, want 2", got)
	}
}

// TestElcrokiAttackedBuffsCursesAndStrikes runs Elcroki's attacked and
// skill-finished hooks, every draw 0. A hit of 10 at full HP broadcasts
// event 10002 with its id (draw 0 under ProbCond4 30), runs the parent
// hooks twice (the hate list gains 425, then 125), and wants its long
// range DD magic on the character 150 away. Under 60% of its HP it wants
// its self range buff on itself, under 30% also its self range debuff.
// Once the buff's cast ends, int quest slot 0 is set and it wants its self
// range DD magic on its most hated; a later hit under 60% wants no buff.
func TestElcrokiAttackedBuffsCursesAndStrikes(t *testing.T) {
	t.Parallel()
	w := bootIndividual6(t, raidBoot{}, elcrokiTemplate(nil), warriorTemplate(plwListenerID, nil))
	h := w.spawn(t, elcrokiID, 150, 0)
	listener := w.spawn(t, plwListenerID, 150, 100)
	w.srv.InstallNPCCastSeam(t, h, elcrokiSkills())

	w.hit(t, h, 10)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), listener.ObjectID()); !slices.Equal(got, []string{fmt.Sprintf("event 10002 %d", h.ObjectID())}) {
		t.Fatalf("events heard = %q, want event 10002 from Elcroki", got)
	}
	if got := w.hateListOf(h); got != 550 {
		t.Fatalf("hate list entry after one hit = %v, want 425 + 125 from the two parent calls", got)
	}
	if !castOn(h, elLongDDID, w.player.ObjectID()) {
		t.Fatal("Elcroki wants no long range DD magic on the character 150 away")
	}
	if _, ok := castDesireOf(h, elBuffID); ok {
		t.Fatal("Elcroki wants its buff at full HP")
	}

	// The attacked hooks read the HP the hit has not taken yet: each share
	// shows at the hit after the one that brings it.
	maxHP := int(h.MaxHPValue())
	w.hit(t, h, maxHP/2-10)
	w.hit(t, h, 1)
	if !castOn(h, elBuffID, h.ObjectID()) {
		t.Fatal("Elcroki under 60% of its HP wants no self range buff on itself")
	}
	if _, ok := castDesireOf(h, elDebuffID); ok {
		t.Fatal("Elcroki at half its HP wants its debuff")
	}
	w.hit(t, h, maxHP/4)
	w.hit(t, h, 1)
	if !castOn(h, elDebuffID, h.ObjectID()) {
		t.Fatal("Elcroki under 30% of its HP wants no self range debuff on itself")
	}

	for i := 0; h.Scratch().Int(npc.IntQuest0) != 1; i++ {
		if i == 20 {
			t.Fatal("Elcroki's buff cast never ended")
		}
		w.ticks(t, 1)
		w.srv.Advance(t, time.Second)
	}
	if _, ok := castDesireOf(h, elShortDDID); !ok {
		t.Fatal("Elcroki wants no self range DD magic on its most hated after a cast")
	}
	w.onQueue(t, h, func() { script.NPCOf(h).RemoveAllAttackDesire() })
	w.hit(t, h, 1)
	if _, ok := castDesireOf(h, elBuffID); ok {
		t.Fatal("Elcroki wants its buff again after it landed")
	}
}

// TestElcrokiGoesHomeWhenLeftAlone runs Elcroki's no-desire hook: healthy
// and out of combat, with silhouettes counted, it broadcasts event 11039,
// clears int slots 3 and 4, and wants to walk back to its spawn point with
// weight 30.
func TestElcrokiGoesHomeWhenLeftAlone(t *testing.T) {
	t.Parallel()
	w := bootIndividual6(t, raidBoot{}, elcrokiTemplate(nil), warriorTemplate(plwListenerID, nil))
	h := w.spawn(t, elcrokiID, 700, 0)
	spawnX, spawnY, _ := h.Position()
	w.onQueue(t, h, func() {
		h.Scratch().SetInt(npc.IntAI3, 2)
		h.Scratch().SetInt(npc.IntAI4, 3)
	})
	w.teleport(t, h, 200, 0)
	listener := w.spawn(t, plwListenerID, 900, 100)

	frames := w.ticks(t, 2)
	if got := says(frames, listener.ObjectID()); len(got) == 0 || got[0] != "event 11039 0" {
		t.Fatalf("events heard = %q, want event 11039 0", got)
	}
	if got := [2]int32{h.Scratch().Int(npc.IntAI3), h.Scratch().Int(npc.IntAI4)}; got != [2]int32{0, 0} {
		t.Fatalf("int slots 3 and 4 = %v, want 0 0", got)
	}
	if d, ok := desire(h, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location.X != spawnX || d.Location.Y != spawnY {
		t.Fatalf("Elcroki move desire = %+v (%v), want weight 30 to %d,%d", d, ok, spawnX, spawnY)
	}
}

// dd2Template is a fixture PartyLeaderWizardDD2 of id listing its two DD
// magics by type.
func dd2Template(id int32, privates int) *npc.Template {
	tmpl := plwTemplate(id, nil, privates)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypeWLongRangeDDMagic:  {ID: wLongDDID, Level: 1},
		npc.SkillTypeWShortRangeDDMagic: {ID: wShortDDID, Level: 1},
	}
	return tmpl
}

// dd2Skills are a fixture DD2's DD magics, each costing mp MP.
func dd2Skills(mp int) *modelskill.Table {
	long, short := harmlessSkill(wLongDDID), harmlessSkill(wShortDDID)
	long.MPConsume, short.MPConsume = mp, mp
	return modelskill.NewTable([]modelskill.Definition{long, short})
}

// TestPartyLeaderWizardDD2StrikesByRange runs PartyLeaderWizardDD2's
// attacked and skill-finished hooks, every draw 0. After the parent hooks
// have entered the character in its hate list, a hit from 50 away wants
// its short range DD magic on the character and one from 300 away its long
// range one (draw 0 under 80). One that cannot pay for its magic sets int
// slot 0 and wants to attack the character with weight 1000 instead; its
// next hit adds the attack formula, 10 / 8 * 100. Each cast that ends
// wants the magic again on its most hated, so the near one casts twice.
func TestPartyLeaderWizardDD2StrikesByRange(t *testing.T) {
	t.Parallel()
	w := bootIndividual6(t, raidBoot{}, dd2Template(plwDD2NearID, 0), dd2Template(plwDD2BrokeID, 0), dd2Template(plwDD2ClanID, 0))
	near := w.spawn(t, plwDD2NearID, 50, 0)
	far := w.spawn(t, plwDD2ClanID, 300, 0)
	broke := w.spawn(t, plwDD2BrokeID, 0, 50)
	w.srv.InstallNPCCastSeam(t, near, dd2Skills(0))
	w.srv.InstallNPCCastSeam(t, far, dd2Skills(0))
	w.srv.InstallNPCCastSeam(t, broke, dd2Skills(100000))

	w.hit(t, near, 10)
	if !castOn(near, wShortDDID, w.player.ObjectID()) {
		t.Fatal("DD2 50 away wants no short range DD magic on the character")
	}
	w.hit(t, far, 10)
	if !castOn(far, wLongDDID, w.player.ObjectID()) {
		t.Fatal("DD2 300 away wants no long range DD magic on the character")
	}
	w.hit(t, broke, 10)
	if d, ok := desire(broke, actorai.IntentionAttack); broke.Scratch().Int(npc.IntAI0) != 1 || !ok || d.Weight != 1000 {
		t.Fatalf("DD2 short of MP: int slot 0 %d, attack desire %+v (%v); want 1 and weight 1000", broke.Scratch().Int(npc.IntAI0), d, ok)
	}
	w.hit(t, broke, 10)
	if d, _ := desire(broke, actorai.IntentionAttack); d.Weight != 1125 {
		t.Fatalf("DD2 short of MP attack desire after its next hit = %v, want 1125", d.Weight)
	}

	var casts int
	for i := 0; casts < 2; i++ {
		if i == 20 {
			t.Fatalf("near DD2 short range casts = %d, want 2", casts)
		}
		frames := w.ticks(t, 1)
		w.srv.Advance(t, time.Second)
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
		casts += len(framesOf(frames, serverpackets.OpcodeMagicSkillUse, near.ObjectID()))
	}
}

// TestPartyLeaderWizardDD2AnswersItsClanAndParty runs PartyLeaderWizardDD2's
// clan-attacked and party-attacked hooks, every draw 0: with an empty hate
// list, a settled DD2 whose clan member is hit wants its long range DD
// magic on the attacker 300 away, then PartyLeaderWizard's clan formula
// enters it at 300; a DD2 whose private is hit wants it too, then the
// party formula enters it at 125 + 300.
func TestPartyLeaderWizardDD2AnswersItsClanAndParty(t *testing.T) {
	t.Parallel()
	w := bootIndividual6(t, raidBoot{},
		inClan(dd2Template(plwDD2ClanID, 0)),
		dd2Template(plwDD2PartyID, 1),
		inClan(warriorTemplate(plwClanMateID, nil)),
		warriorTemplate(plwPrivateID, nil))
	clan := w.spawn(t, plwDD2ClanID, 300, 0)
	party := w.spawn(t, plwDD2PartyID, 0, 300)
	mate := w.spawn(t, plwClanMateID, 300, 100)
	private := privatesOf(t, party, 1)[0]
	w.srv.InstallNPCCastSeam(t, clan, dd2Skills(0))
	w.srv.InstallNPCCastSeam(t, party, dd2Skills(0))
	w.ticks(t, 8)
	w.onQueue(t, clan, func() { script.NPCOf(clan).RemoveAllAttackDesire() })
	w.onQueue(t, party, func() { script.NPCOf(party).RemoveAllAttackDesire() })

	w.hit(t, mate, 10)
	if !castOn(clan, wLongDDID, w.player.ObjectID()) {
		t.Fatal("settled DD2 wants no long range DD magic on its clan member's attacker")
	}
	if got := w.hateListOf(clan); got != 300 {
		t.Fatalf("DD2 hate list entry after a clan hit = %v, want 300", got)
	}

	px, py, _ := party.Position()
	if d := math.Hypot(float64(px-w.at.X), float64(py-w.at.Y)); d <= 100 {
		t.Fatalf("party DD2 stands %v from the character, want farther than 100", d)
	}
	w.hit(t, private, 10)
	if !castOn(party, wLongDDID, w.player.ObjectID()) {
		t.Fatal("DD2 wants no long range DD magic on its private's attacker")
	}
	if got := w.hateListOf(party); got != 425 {
		t.Fatalf("DD2 hate list entry after a hit on its private = %v, want 425", got)
	}
}
