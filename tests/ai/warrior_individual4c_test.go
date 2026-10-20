package ai

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids the scenarios of the batch's third cut spawn (#189 PR 3).
const (
	ind4cBufferID     = int32(22131) // PartyPrivatePriestBuffer
	ind4cSlowID       = int32(20964) // PartyPrivateSlowTypeMagic
	ind4cSlow2ID      = int32(22091) // PartyPrivateSlowType2
	ind4cSplitID      = int32(22087) // PartyPrivateSplit
	ind4cGuardID      = int32(29005) // QueenAntPrivateRoyalGuardAnt
	ind4cBombID       = int32(21437) // RoyalRushPartyPrivateBomb
	ind4cHateID       = int32(21435) // RoyalRushPartyPrivateHate
	ind4cFollowerID   = int32(29995) // PartyPrivateCoupleFollower, bound by the test
	ind4cSilhouetteID = int32(29996) // the followers' silhouette
	ind4cFollowerPath = "test.PartyPrivateCoupleFollower"
)

// The skills the cut's fixture NPCs list by type beside ind4Template's.
const (
	ind4cBuff1Skill    = modelskill.ID(9711)
	ind4cBuff2Skill    = modelskill.ID(9712)
	ind4cSlowSkill     = modelskill.ID(9713)
	ind4cBombSkill     = modelskill.ID(9714)
	ind4cSelfBuffSkill = modelskill.ID(9715)
	ind4cMobHateSkill  = modelskill.ID(9716)
	ind4cBuff1Stack    = "ind4c_buff1"
)

// ind4cTemplate is ind4Template listing also two buffs, a slow, a bomb, a
// self buff and a hate skill by type.
func ind4cTemplate(id int32, clan bool) *npc.Template {
	tmpl := ind4Template(id, clan)
	for t, s := range map[npc.SkillType]modelskill.ID{
		npc.SkillTypeBuff1: ind4cBuff1Skill, npc.SkillTypeBuff2: ind4cBuff2Skill,
		npc.SkillTypeDDMagicSlow: ind4cSlowSkill, npc.SkillTypeBomb: ind4cBombSkill,
		npc.SkillTypeSelfBuff: ind4cSelfBuffSkill, npc.SkillTypeMobHate: ind4cMobHateSkill,
	} {
		tmpl.SkillsByType[t] = modelskill.Ref{ID: s, Level: 1}
	}
	return tmpl
}

// ind4cBuff is a fixture buff of id: a one-target spell on a friend
// reaching 2000, landing no effect.
func ind4cBuff(id modelskill.ID) modelskill.Definition {
	d := ind1Skill(id, 2000, false)
	d.Offensive, d.Magic, d.SkillType, d.Power = false, false, "BUFF", 0
	return d
}

// ind4cHeldBuff1 is the first fixture buff as the scripts read it, whose
// effect lasts 600 s.
func ind4cHeldBuff1() modelskill.Definition {
	d := ind4cBuff(ind4cBuff1Skill)
	d.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 600, Icon: true, StackType: ind4cBuff1Stack, StackOrder: 1}}
	return d
}

// ind4cSkills are ind4Skills with the cut's buffs, slow, bomb, self buff
// and hate skill.
func ind4cSkills() *modelskill.Table {
	return modelskill.NewTable(append(ind4Skills().All(),
		ind4cBuff(ind4cBuff1Skill), ind4cBuff(ind4cBuff2Skill), ind1Skill(ind4cSlowSkill, 2000, false),
		ind1Skill(ind4cBombSkill, 0, true), ind1Skill(ind4cSelfBuffSkill, 0, true), ind1Skill(ind4cMobHateSkill, 2000, false)))
}

// ind4cCast is a queued cast desire: its skill, the object id of its
// target and its weight.
type ind4cCast struct {
	skill  modelskill.ID
	target int32
	weight float64
}

// wantCastList fails unless h's queued cast desires are exactly want, in
// any order: wantCasts for an NPC that may want one skill on two targets.
func wantCastList(t *testing.T, when string, h *npc.Hostile, want ...ind4cCast) {
	t.Helper()
	var got []ind4cCast
	for _, d := range h.AI().Desires().Snapshot() {
		if d.Kind == actorai.IntentionCast && d.FinalTarget != nil {
			got = append(got, ind4cCast{d.Skill.ID, d.FinalTarget.ObjectID(), d.Weight})
		}
	}
	order := func(a, b ind4cCast) int {
		return cmp.Or(cmp.Compare(a.skill, b.skill), cmp.Compare(a.target, b.target))
	}
	slices.SortFunc(got, order)
	slices.SortFunc(want, order)
	if !slices.Equal(got, want) {
		t.Fatalf("%s: cast desires = %+v, want %+v", when, got, want)
	}
}

// ind4cTopDesireTarget sets h's top desire target to the character, or
// empties it.
func (w *ind1World) ind4cTopDesireTarget(t *testing.T, h *npc.Hostile, on bool) {
	t.Helper()
	w.onQueue(t, h, func() {
		if on {
			script.NPCOf(h).SetTopDesireTarget(script.PlayerOf(w.player))
		} else {
			script.NPCOf(h).SetTopDesireTarget(nil)
		}
	})
}

// TestPartyPrivatePriestBufferBuffsItsParty ports
// PartyPrivatePriestBuffer's created, timer and see-creature hooks for two
// buffers of one master beside another master's two. Seeing others only
// when it looks around, a buffer wants nothing before its 20 s timer first
// fires. That look wants each buff its template lists (BUFF1 and BUFF2;
// the other four types list none) on its master and on the other buffer
// of its master, weight 1000000 each, and nothing on the other master or
// its privates. With the first buff's effect on its master, the next look
// wants only the second on the master.
func TestPartyPrivatePriestBufferBuffsItsParty(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(ind4cHeldBuff1())}},
		ind4Leader(ind4cBufferID, 2), ind4cTemplate(ind4cBufferID, false))
	leader := w.spawn(t, leaderID, 400, 0)
	privates := privatesOf(t, leader, 2)
	buffer, fellow := privates[0], privates[1]
	privatesOf(t, w.spawn(t, leaderID, 400, 150), 2)
	w.srv.InstallNPCCastSeam(t, buffer, ind4cSkills())
	w.clearDesires(t, buffer)
	w.ticks(t, 2)
	wantCastList(t, "before the first look", buffer)

	w.srv.Advance(t, 20*time.Second)
	lid, fid := leader.ObjectID(), fellow.ObjectID()
	wantCastList(t, "after the 20 s look", buffer,
		ind4cCast{ind4cBuff1Skill, lid, 1000000}, ind4cCast{ind4cBuff2Skill, lid, 1000000},
		ind4cCast{ind4cBuff1Skill, fid, 1000000}, ind4cCast{ind4cBuff2Skill, fid, 1000000})

	me := w.player.ObjectID()
	if !w.srv.RunScript(t, me, "Ind1Director", func(s *script.Script, _ *script.Player) {
		s.ApplyEffects(modelskill.Ref{ID: ind4cBuff1Skill, Level: 1}, script.NPCOf(buffer), script.NPCOf(leader))
	}) {
		t.Fatal("the effect landing panicked")
	}
	w.srv.AdvanceUntil(t, "the first buff on the master", func() bool {
		var level int32
		w.srv.RunScript(t, me, "Ind1Director", func(s *script.Script, _ *script.Player) {
			level = s.AbnormalLevel(script.NPCOf(leader), modelskill.Ref{ID: ind4cBuff1Skill, Level: 1})
		})
		return level > 0
	})
	w.clearDesires(t, buffer)
	w.srv.Advance(t, 40*time.Second)
	wantCastList(t, "after a look with the first buff on the master", buffer,
		ind4cCast{ind4cBuff2Skill, lid, 1000000},
		ind4cCast{ind4cBuff1Skill, fid, 1000000}, ind4cCast{ind4cBuff2Skill, fid, 1000000})
}

// TestPartyPrivatePriestBufferSparesItsPartyWhileAttacking ports the
// intention gate of PartyPrivatePriestBuffer's see-creature hook: a buffer
// attacking the character when its 20 s look finds its party wants no
// buff on any of them.
func TestPartyPrivatePriestBufferSparesItsPartyWhileAttacking(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4cBufferID, 2), ind4cTemplate(ind4cBufferID, false))
	leader := w.spawn(t, leaderID, 400, 0)
	buffer := privatesOf(t, leader, 2)[0]
	ind4bTakeUpAttack(t, w, buffer)
	w.srv.InstallNPCCastSeam(t, buffer, ind4cSkills())
	w.srv.Advance(t, 20*time.Second)
	if got := buffer.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("setup: intention at the look = %v, want attack", got)
	}
	wantCastList(t, "at the look while attacking", buffer)
}

// TestPartyPrivateSlowTypeMagicSlowsDistantAttackers ports
// PartyPrivateSlowTypeMagic's attacked hook: a hit from more than 200 away
// with a draw under 70 makes it want its slow on the attacker with weight
// 1000000; a draw of 70, or a hit from 150, wants none.
func TestPartyPrivateSlowTypeMagicSlowsDistantAttackers(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		dx   int
		draw int
		slow bool
	}{{"far", 300, 0, true}, {"far, draw 70", 300, 70, false}, {"near", 150, 0, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4cTemplate(ind4cSlowID, false))
			n := w.spawn(t, ind4cSlowID, row.dx, 0)
			w.srv.InstallNPCCastSeam(t, n, ind4cSkills())
			if a0, a1 := ind1Slot(n, script.SlotIntAI0), ind1Slot(n, script.SlotIntAI1); a0 != 0 || a1 != 0 {
				t.Fatalf("created i_ai0, i_ai1 = %d, %d, want 0, 0", a0, a1)
			}
			w.draws.set(row.draw)
			w.hit(t, n, 10)
			var want []ind4cCast
			if row.slow {
				want = append(want, ind4cCast{ind4cSlowSkill, w.player.ObjectID(), 1000000})
			}
			wantCastList(t, "after the hit", n, want...)
		})
	}
}

// TestPartyPrivateSlowTypeMagicSlowsWhatItFights ports
// PartyPrivateSlowTypeMagic's timer hook. A hit starts its 20 s timer
// "2001". Fired while it attacks, with i_ai1 clear, a draw under 50 makes
// it want its slow on its most hated creature, weight 1000000, and the
// timer starts again. A hit by its most hated creature while i_ai0 is set
// sets i_ai1 instead of starting the timer, and the next firing then wants
// no slow but clears i_ai1, so the one after slows again.
func TestPartyPrivateSlowTypeMagicSlowsWhatItFights(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4cSlowID, 1), ind4cTemplate(ind4cSlowID, false))
	leader := w.spawn(t, leaderID, 300, 0)
	n := privatesOf(t, leader, 1)[0]
	ind4bTakeUpAttack(t, w, n)
	w.srv.InstallNPCCastSeam(t, n, ind4cSkills())
	w.clearDesires(t, n)
	slow := ind4cCast{ind4cSlowSkill, w.player.ObjectID(), 1000000}

	w.srv.Advance(t, 20*time.Second)
	if got := n.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("setup: intention at the timer = %v, want attack", got)
	}
	wantCastList(t, "after the first firing", n, slow)

	w.onQueue(t, n, func() { script.NPCOf(n).SetInt(script.SlotIntAI0, 1) })
	w.draws.set(70)
	w.hit(t, n, 1)
	if got := ind1Slot(n, script.SlotIntAI1); got != 1 {
		t.Fatalf("i_ai1 after a hit by its most hated with i_ai0 set = %d, want 1", got)
	}
	w.draws.set(0)
	w.clearDesires(t, n)
	w.srv.Advance(t, 20*time.Second)
	wantCastList(t, "after the firing with i_ai1 set", n)
	if got := ind1Slot(n, script.SlotIntAI1); got != 0 {
		t.Fatalf("i_ai1 after the firing = %d, want 0", got)
	}
	w.srv.Advance(t, 20*time.Second)
	wantCastList(t, "after the next firing", n, slow)
}

// TestPartyPrivateSlowTypeMagicResetsAtPeace ports the peace branch of
// PartyPrivateSlowTypeMagic's timer hook: fired while it neither attacks
// nor casts, timer "2001" that a hit by its master started clears i_ai0
// and i_ai1, wants no slow and does not start again.
func TestPartyPrivateSlowTypeMagicResetsAtPeace(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4cSlowID, 1), ind4cTemplate(ind4cSlowID, false))
	leader := w.spawn(t, leaderID, 300, 0)
	n := privatesOf(t, leader, 1)[0]
	w.srv.InstallNPCCastSeam(t, n, ind4cSkills())
	// A hit by its master, no player or summon, starts the timer but no
	// fight.
	w.onQueue(t, leader, func() { n.TakeDamage(1, leader) })
	w.clearDesires(t, n)
	set := func() {
		w.onQueue(t, n, func() {
			script.NPCOf(n).SetInt(script.SlotIntAI0, 7)
			script.NPCOf(n).SetInt(script.SlotIntAI1, 7)
		})
	}
	set()
	w.draws.set(0)
	w.srv.Advance(t, 20*time.Second)
	if got := n.AI().CurrentIntention(); got == actorai.IntentionAttack || got == actorai.IntentionCast {
		t.Fatalf("setup: intention at the timer = %v, want neither attack nor cast", got)
	}
	if a0, a1 := ind1Slot(n, script.SlotIntAI0), ind1Slot(n, script.SlotIntAI1); a0 != 0 || a1 != 0 {
		t.Fatalf("i_ai0, i_ai1 after the firing at peace = %d, %d, want 0, 0", a0, a1)
	}
	wantCastList(t, "after the firing at peace", n)
	set()
	w.srv.Advance(t, 40*time.Second)
	if a0, a1 := ind1Slot(n, script.SlotIntAI0), ind1Slot(n, script.SlotIntAI1); a0 != 7 || a1 != 7 {
		t.Fatalf("i_ai0, i_ai1 40 s later = %d, %d, want 7, 7: the timer did not start again", a0, a1)
	}
}

// TestPartyPrivateSlowTypeMagicAnswersItsClan ports
// PartyPrivateSlowTypeMagic's clan-attacked hook, every draw 0: a settled
// clan member that is not attacking wants its slow on another member's
// attacker with weight 1000000; a young one wants none.
func TestPartyPrivateSlowTypeMagicAnswersItsClan(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		settle int
		slow   bool
	}{{"settled member", 8, true}, {"young member", 0, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4cTemplate(ind4cSlowID, true))
			victim := w.spawn(t, ind4cSlowID, 1500, 0)
			member := w.spawn(t, ind4cSlowID, 1500, 200)
			w.srv.InstallNPCCastSeam(t, member, ind4cSkills())
			w.ticks(t, row.settle)
			w.clearDesires(t, member)
			w.hit(t, victim, 10)
			var want []ind4cCast
			if row.slow {
				want = append(want, ind4cCast{ind4cSlowSkill, w.player.ObjectID(), 1000000})
			}
			wantCastList(t, "member after the hit", member, want...)
		})
	}
}

// TestPartyPrivateSlowType2DrawsTwiceToSlow ports PartyPrivateSlowType2's
// attacked hook: hit from more than 200 away, it draws for its slow on the
// attacker before its parent does, so two draws under 70 want it with
// weight 2000000, one with 1000000, and none, like a hit from 150, none.
func TestPartyPrivateSlowType2DrawsTwiceToSlow(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		dx     int
		draws  []int
		weight float64
	}{
		{"both draws pass", 300, []int{0, 0}, 2000000},
		{"its own draw fails", 300, []int{70, 0}, 1000000},
		{"its parent's draw fails", 300, []int{0, 70}, 1000000},
		{"both draws fail", 300, []int{70, 70}, 0},
		{"near", 150, []int{0, 0}, 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4cTemplate(ind4cSlow2ID, false))
			n := w.spawn(t, ind4cSlow2ID, row.dx, 0)
			w.srv.InstallNPCCastSeam(t, n, ind4cSkills())
			w.draws.set(70, row.draws...)
			w.hit(t, n, 10)
			var want []ind4cCast
			if row.weight > 0 {
				want = append(want, ind4cCast{ind4cSlowSkill, w.player.ObjectID(), row.weight})
			}
			wantCastList(t, "after the hit", n, want...)
		})
	}
}

// ind4cCallPrivate places an NPC of id dx, dy from the character as
// master's private, with spawn parameters params, through the director
// script, and returns it.
func (w *ind1World) ind4cCallPrivate(t *testing.T, master *npc.Hostile, id int32, dx, dy int, params script.Params) *npc.Hostile {
	t.Helper()
	var placed *script.NPC
	if !w.srv.RunScript(t, w.player.ObjectID(), "Ind1Director", func(s *script.Script, _ *script.Player) {
		placed = s.Spawner().CreateOnePrivateEx(script.NPCOf(master), id, script.Loc{X: w.at.X + dx, Y: w.at.Y + dy, Z: w.at.Z}, 0, params)
	}) || placed == nil {
		t.Fatalf("call %d: no private placed", id)
	}
	w.srv.Settle(t)
	for _, m := range master.Minions() {
		if m.ObjectID() == placed.ObjectID() {
			return m.(*npc.Hostile)
		}
	}
	t.Fatalf("call %d: private %d is not among its master's", id, placed.ObjectID())
	return nil
}

// TestPartyPrivateSplitTakesUpItsTarget ports PartyPrivateSplit's created
// and see-creature hooks. Spawned with the character as its second spawn
// parameter, a split keeps the character in c_ai0 and as its top desire
// target, and on seeing it wants its first physical special on it for a
// draw under 50, the second otherwise, weight 1000000, and to attack it,
// hating it by 200 more than the 500 it spawned with. Spawned with no creature there, it keeps none and
// wants nothing of the character it sees.
func TestPartyPrivateSplitTakesUpItsTarget(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name    string
		target  bool
		draw    int
		special modelskill.ID
	}{{"draw 0", true, 0, ind4Special1Skill}, {"draw 50", true, 50, ind4Special2Skill}, {"no target", false, 0, 0}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4cSplitID, 0), ind4cTemplate(ind4cSplitID, false))
			leader := w.spawn(t, leaderID, 200, 0)
			me := w.player.ObjectID()
			params := script.Params{P1: 1000}
			if row.target {
				params.P2 = me
			}
			w.draws.set(row.draw)
			split := w.ind4cCallPrivate(t, leader, ind4cSplitID, 150, 0, params)
			var c0, top int32
			w.onQueue(t, split, func() {
				n := script.NPCOf(split)
				if c := n.Slot(script.SlotCreature0); c != nil {
					c0 = c.ObjectID()
				}
				if c := n.TopDesireTarget(); c != nil {
					top = c.ObjectID()
				}
			})
			if want := params.P2; c0 != want || top != want {
				t.Fatalf("created c_ai0, top desire target = %d, %d, want %d, %d", c0, top, want, want)
			}
			// Its first spawn parameter 1000 makes its chain's created hook
			// want to attack that creature by 500.
			if got, want := split.AggroHate(w.player), float64(0); row.target && got != 500 || !row.target && got != want {
				t.Fatalf("created hate of the character = %v, want 500 with it as the target, else 0", got)
			}
			w.srv.InstallNPCCastSeam(t, split, ind4cSkills())

			w.ticks(t, 2)
			if !row.target {
				wantCastList(t, "after seeing the character", split)
				if got := split.AggroHate(w.player); got != 0 {
					t.Fatalf("hate of the character with no target = %v, want 0", got)
				}
				return
			}
			wantCastList(t, "after seeing the character", split, ind4cCast{row.special, me, 1000000})
			if got := split.AggroHate(w.player); got != 700 {
				t.Fatalf("hate of the character it sees = %v, want 700: 500 and 200 for the sight", got)
			}
		})
	}
}

// TestPartyPrivateSplitLeavesWithoutItsMaster ports PartyPrivateSplit's
// party-died and timer hooks. Another private's death starts nothing; its
// master's death starts its 180 s timer "1004". Its master dead and it at
// peace, the 20 s check takes it out of the world (DeleteObject): its own
// check runs before its parent's and returns, so the parent's never runs.
func TestPartyPrivateSplitLeavesWithoutItsMaster(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4cSplitID, 0), ind4cTemplate(ind4cSplitID, false))
	leader := w.spawn(t, leaderID, 400, 0)
	split := w.ind4cCallPrivate(t, leader, ind4cSplitID, 400, 100, script.Params{})
	other := w.ind4cCallPrivate(t, leader, ind4cSplitID, 400, -100, script.Params{})
	me := w.player.ObjectID()
	pending := func() bool {
		var on bool
		if !w.srv.RunScript(t, me, "PartyPrivateSplit", func(s *script.Script, _ *script.Player) {
			on = s.HasTimer("1004", script.NPCOf(split), nil)
		}) {
			t.Fatal("the timer lookup panicked")
		}
		return on
	}

	w.kill(t, other)
	if pending() {
		t.Fatal(`timer "1004" pending after another private died, want none`)
	}
	w.kill(t, leader)
	if !pending() {
		t.Fatal(`timer "1004" not pending after its master died`)
	}
	w.srv.Advance(t, 20*time.Second)
	if got := framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeDeleteObject, split.ObjectID()); len(got) != 1 {
		t.Fatalf("DeleteObject frames of the split = %d, want 1", len(got))
	}
	if w.inWorld(split) {
		t.Fatal("the split is still in the world 20 s after its master died")
	}
}

// TestQueenAntPrivateRoyalGuardAntCursesAndMutesOverLevelled ports
// QueenAntPrivateRoyalGuardAnt's attacked and see-spell hooks. Against the
// level 20 character, a level 11 guard (9 levels below, more than 8) wants
// its raid curse on the character when hit and drops its hate of it, and
// wants its raid mute when it sees the character cast, both with weight
// 1000000; a level 12 one wants neither. Its attacked hook calls no
// parent: the hit leaves no hate either way.
func TestQueenAntPrivateRoyalGuardAntCursesAndMutesOverLevelled(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		level int
		over  bool
	}{{11, true}, {12, false}} {
		t.Run(fmt.Sprintf("level %d", row.level), func(t *testing.T) {
			t.Parallel()
			tmpl := ind4cTemplate(ind4cGuardID, false)
			tmpl.Level, tmpl.CanMove = row.level, false
			w := bootIndividual4(t, raidSpellBoot(t), tmpl)
			guard := w.spawn(t, ind4cGuardID, 300, 0)
			w.srv.InstallNPCCastSeam(t, guard, modelskill.NewTable([]modelskill.Definition{raidCurseDefinition(), ind1Skill(premoMuteID, 600, false)}))
			me := w.player.ObjectID()

			w.hit(t, guard, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.over {
				want[raidCurseID] = ind1Cast{me, 1000000}
			}
			w.wantCasts(t, "after the hit", guard, want)
			if got := guard.AggroHate(w.player); got != 0 {
				t.Fatalf("hate after the hit = %v, want 0", got)
			}

			w.castSelfBuff()
			if row.over {
				w.srv.AdvanceUntil(t, "the raid mute desire", func() bool {
					_, ok := ind1Casts(guard)[premoMuteID]
					return ok
				})
				want[premoMuteID] = ind1Cast{me, 1000000}
			} else {
				w.srv.Advance(t, 2*time.Second)
			}
			w.wantCasts(t, "after the spell", guard, want)
		})
	}
}

// TestRoyalRushPartyPrivateBombBlowsUpForItsMaster ports
// RoyalRushPartyPrivateBomb's attacked and party-attacked hooks, the hooks
// running before the hit takes its HP. A hit by its top desire target, on
// a draw under 33, wants its first physical special on it (1000000). Under
// 40% of its HP with its master alive, each of the hit's two calls (its
// attacked hook and its own party call) wants to heal the master and blow
// itself up, 1000000 each, so 2000000 for a hit by its top desire target
// and 1000000 for another hit, or a hit on its master. A draw of 33 wants
// nothing, nor does a dead master.
func TestRoyalRushPartyPrivateBombBlowsUpForItsMaster(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4cBombID, 1), ind4cTemplate(ind4cBombID, false))
	leader := w.spawn(t, leaderID, 300, 0)
	bomb := privatesOf(t, leader, 1)[0]
	w.srv.InstallNPCCastSeam(t, bomb, ind4cSkills())
	me, lid, bid := w.player.ObjectID(), leader.ObjectID(), bomb.ObjectID()
	strike := ind4cCast{ind4Special1Skill, me, 1000000}
	round := func(top bool) {
		w.clearDesires(t, bomb)
		w.ind4cTopDesireTarget(t, bomb, top)
	}

	round(true)
	w.hit(t, bomb, int(bomb.MaxHPValue()*7/10))
	wantCastList(t, "after a hit at full HP", bomb, strike)

	round(true)
	w.hit(t, bomb, 1)
	wantCastList(t, "after a hit by its top desire target under 40%", bomb,
		strike, ind4cCast{ind4HealSkill, lid, 2000000}, ind4cCast{ind4cBombSkill, bid, 2000000})

	round(false)
	w.hit(t, bomb, 1)
	wantCastList(t, "after a hit by another under 40%", bomb,
		ind4cCast{ind4HealSkill, lid, 1000000}, ind4cCast{ind4cBombSkill, bid, 1000000})

	round(false)
	w.hit(t, leader, 1)
	wantCastList(t, "after a hit on its master", bomb,
		ind4cCast{ind4HealSkill, lid, 1000000}, ind4cCast{ind4cBombSkill, bid, 1000000})

	round(true)
	w.draws.set(33)
	w.hit(t, bomb, 1)
	wantCastList(t, "after a hit with a draw of 33", bomb)

	w.draws.set(0)
	w.kill(t, leader)
	round(true)
	w.hit(t, bomb, 1)
	wantCastList(t, "after a hit with its master dead", bomb, strike)
}

// TestRoyalRushPartyPrivateHateDrawsHate ports RoyalRushPartyPrivateHate's
// created, attacked and party-attacked hooks. It spawns wanting its self
// buff on itself (1000000). A hit by its top desire target, on a draw
// under 33, wants its hate skill on it, and its own party call, on a draw
// under 10, wants it again: 2000000 for a draw of 9. A draw of 33 wants
// neither; a hit by another creature, or on its master, wants only the
// party call's 1000000.
func TestRoyalRushPartyPrivateHateDrawsHate(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNpcSpawnSkills(ind4cSkills())}}, ind4Leader(ind4cHateID, 1), ind4cTemplate(ind4cHateID, false))
	leader := w.spawn(t, leaderID, 300, 0)
	n := privatesOf(t, leader, 1)[0]
	wantCastList(t, "as it spawns", n, ind4cCast{ind4cSelfBuffSkill, n.ObjectID(), 1000000})
	me := w.player.ObjectID()

	for _, row := range []struct {
		name   string
		top    bool
		draw   int
		onHate bool
		weight float64
	}{
		{"its top desire target, draw 9", true, 9, true, 2000000},
		{"its top desire target, draw 10", true, 10, true, 1000000},
		{"its top desire target, draw 33", true, 33, true, 0},
		{"another creature, draw 0", false, 0, true, 1000000},
		{"on its master, draw 0", false, 0, false, 1000000},
		{"on its master, draw 10", false, 10, false, 0},
	} {
		w.clearDesires(t, n)
		w.ind4cTopDesireTarget(t, n, row.top)
		w.draws.set(row.draw)
		if row.onHate {
			w.hit(t, n, 1)
		} else {
			w.hit(t, leader, 1)
		}
		var want []ind4cCast
		if row.weight > 0 {
			want = append(want, ind4cCast{ind4cMobHateSkill, me, row.weight})
		}
		wantCastList(t, row.name, n, want...)
	}
}

// TestPartyPrivateCoupleFollowerCallsItsSilhouettes ports
// PartyPrivateCoupleFollower's created and party-died hooks for three
// followers of one master. They spawn with i_ai0 clear. A follower's death
// while their master lives calls nothing. Their master's death makes the
// follower above 70% of its HP with a top desire target call three of the
// silhouettes its AI parameters name, at +10 on x, +10 on y and +5 on
// both, with spawn parameters 1000 and that target, which the client sees
// appear, and set i_ai0; the follower at 70% calls none. The follower is
// still in its dead master's party as it calls, so the silhouettes join
// that party, which the dead master then lets go: they end with no
// master. Another death then calls nothing more.
func TestPartyPrivateCoupleFollowerCallsItsSilhouettes(t *testing.T) {
	t.Parallel()
	follower := ind4cTemplate(ind4cFollowerID, false)
	follower.AIParams = npc.AIParams{"silhouette": fmt.Sprint(ind4cSilhouetteID)}
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4cFollowerID, 3), follower, warriorTemplate(ind4cSilhouetteID, nil))
	leader := w.spawn(t, leaderID, 400, 0)
	privates := privatesOf(t, leader, 3)
	caller, dies, low := privates[0], privates[1], privates[2]
	for _, p := range privates {
		if got := ind1Slot(p, script.SlotIntAI0); got != 0 {
			t.Fatalf("created i_ai0 = %d, want 0", got)
		}
	}
	w.ind4cTopDesireTarget(t, caller, true)
	w.ind4cTopDesireTarget(t, low, true)
	w.setHPShare(t, low, 0.7)
	me := w.player.ObjectID()

	w.kill(t, dies)
	if got := len(w.liveNPCs(ind4cSilhouetteID)); got != 0 {
		t.Fatalf("silhouettes after a follower died = %d, want none", got)
	}

	x, y, _ := caller.Position()
	w.onQueue(t, leader, func() { script.NPCOf(leader).DoDie(nil) })
	frames := w.srv.ReadQueued(t, w.srv.Client)
	silhouettes := w.liveNPCs(ind4cSilhouetteID)
	if len(silhouettes) != 3 {
		t.Fatalf("silhouettes after their master died = %d, want 3", len(silhouettes))
	}
	var at [][2]int
	for _, s := range silhouettes {
		if m := script.NPCOf(s).Master(); m != nil {
			t.Fatalf("silhouette master = %d, want none: the dead master let its party go", m.ObjectID())
		}
		if p1, p2, p3 := ind1Slot(s, script.SlotParam1), ind1Slot(s, script.SlotParam2), ind1Slot(s, script.SlotParam3); p1 != 1000 || p2 != me || p3 != 0 {
			t.Fatalf("silhouette spawn parameters = %d, %d, %d, want 1000, %d, 0", p1, p2, p3, me)
		}
		if got := framesOf(frames, serverpackets.OpcodeNPCInfo, s.ObjectID()); len(got) != 1 {
			t.Fatalf("NpcInfo frames of a silhouette = %d, want 1", len(got))
		}
		sx, sy, _ := s.Position()
		at = append(at, [2]int{sx - x, sy - y})
	}
	slices.SortFunc(at, func(a, b [2]int) int { return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1])) })
	if want := [][2]int{{0, 10}, {5, 5}, {10, 0}}; !slices.Equal(at, want) {
		t.Fatalf("silhouette offsets = %v, want %v", at, want)
	}
	if got := ind1Slot(caller, script.SlotIntAI0); got != 1 {
		t.Fatalf("i_ai0 after the call = %d, want 1", got)
	}

	w.kill(t, low)
	if got := len(w.liveNPCs(ind4cSilhouetteID)); got != 3 {
		t.Fatalf("silhouettes after another death = %d, want still 3", got)
	}
}

// The NpcString lines the fixture splits' AI parameters name.
const (
	ind4cShoutMsg2 = 1000290
	ind4cShoutMsg3 = 1000291
)

// ind4cLine is an NpcSay line: its say type and text.
type ind4cLine struct {
	sayType int32
	text    string
}

// ind4cLines returns the lines NpcSay frames show from objectID, each
// with its say type.
func ind4cLines(frames [][]byte, objectID int32) []ind4cLine {
	var out []ind4cLine
	sayFrames := framesOf(frames, serverpackets.OpcodeNpcSay, objectID)
	for i, text := range says(frames, objectID) {
		// opcode, object id, then the say type.
		out = append(out, ind4cLine{int32(binary.LittleEndian.Uint32(sayFrames[i][5:9])), text})
	}
	return out
}

// ind4cSocialSplit boots a split private beside its master, its AI
// parameters naming social 3 for 3 s, ShoutMsg2, ShoutMsg3 and isSay as
// IsSay, so its chain starts the 10 s timer "1001" as it spawns.
func ind4cSocialSplit(t *testing.T, isSay int) (*ind1World, *npc.Hostile) {
	t.Helper()
	tmpl := ind4cTemplate(ind4cSplitID, false)
	tmpl.AIParams = npc.AIParams{
		"MoveAroundSocial2": "3", "ShoutMsg2": fmt.Sprint(ind4cShoutMsg2), "ShoutMsg3": fmt.Sprint(ind4cShoutMsg3),
		"IsSay": fmt.Sprint(isSay),
	}
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4cSplitID, 0), tmpl)
	leader := w.spawn(t, leaderID, 300, 0)
	split := w.ind4cCallPrivate(t, leader, ind4cSplitID, 300, 50, script.Params{})
	w.clearDesires(t, split)
	return w, split
}

// wantSocial fails unless h's queued social desire is social 3 for 100
// (3 * 1000 / 30) with weight, or h has none for a weight of 0.
func wantSocial(t *testing.T, when string, h *npc.Hostile, weight float64) {
	t.Helper()
	d, ok := desire(h, actorai.IntentionSocial)
	if weight == 0 {
		if ok {
			t.Fatalf("%s: social desire = %+v, want none", when, d)
		}
		return
	}
	if !ok || d.ItemObjectID != 3 || d.Timer != 100 || d.Weight != weight {
		t.Fatalf("%s: social desire = %+v (%v), want social 3 for 100 with weight %v", when, d, ok, weight)
	}
}

// TestPartyPrivateSplitActsOutAtPeace ports PartyPrivateSplit's timer
// "1001" at peace, each firing drawing for the split's own social and
// ShoutMsg2 line before its parent draws for the same in the same firing.
// Both social draws under 20 want social 3 for 100, weight 50 each, so 100.
// The split's own ShoutMsg2 draw under 17 of 1000 says the line (say type
// 0, IsSay set); the parent's shouts it (type 1). Moving to a point, a
// split acts out nothing of its own: only the parent's draws, which do
// count that intention, want the social (50) and shout the line.
func TestPartyPrivateSplitActsOutAtPeace(t *testing.T) {
	t.Parallel()
	w, split := ind4cSocialSplit(t, 1)
	say := ind4cLine{serverpackets.SayTypeAll, script.NpcString(ind4cShoutMsg2)}
	shout := ind4cLine{serverpackets.SayTypeShout, script.NpcString(ind4cShoutMsg2)}
	firing := func(when string, social float64, lines []ind4cLine, draws ...int) {
		t.Helper()
		w.clearDesires(t, split)
		w.draws.set(999, draws...)
		w.srv.ReadQueued(t, w.srv.Client)
		w.srv.Advance(t, 10*time.Second)
		wantSocial(t, when, split, social)
		if got := ind4cLines(w.srv.ReadQueued(t, w.srv.Client), split.ObjectID()); !slices.Equal(got, lines) {
			t.Fatalf("%s: lines = %+v, want %+v", when, got, lines)
		}
	}

	firing("both social draws pass", 100, nil, 0, 999, 0, 999)
	firing("its own line draw passes", 0, []ind4cLine{say}, 99, 0, 99, 999)
	firing("its parent's draws pass", 50, []ind4cLine{shout}, 99, 999, 0, 0)

	w.clearDesires(t, split)
	x, y, z := split.Position()
	w.onQueue(t, split, func() { script.NPCOf(split).AddMoveToDesire(script.Loc{X: x + 3000, Y: y, Z: z}, 1000) })
	w.ticks(t, 2)
	if got := split.AI().CurrentIntention(); got != actorai.IntentionMoveTo {
		t.Fatalf("setup: intention before the firing = %v, want move to", got)
	}
	firing("moving, every draw 0", 50, []ind4cLine{shout}, 0, 0)
}

// TestPartyPrivateSplitShoutsWithoutIsSay ports the IsSay switch of
// PartyPrivateSplit's timer "1001": with IsSay 0 its own ShoutMsg2 line is
// shouted (say type 1).
func TestPartyPrivateSplitShoutsWithoutIsSay(t *testing.T) {
	t.Parallel()
	w, split := ind4cSocialSplit(t, 0)
	w.draws.set(999, 99, 0, 99, 999)
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.Advance(t, 10*time.Second)
	want := []ind4cLine{{serverpackets.SayTypeShout, script.NpcString(ind4cShoutMsg2)}}
	if got := ind4cLines(w.srv.ReadQueued(t, w.srv.Client), split.ObjectID()); !slices.Equal(got, want) {
		t.Fatalf("lines = %+v, want %+v", got, want)
	}
}

// TestPartyPrivateSplitCallsOutInAFight ports the attack branch of
// PartyPrivateSplit's timer "1001": fired while it attacks, its own draw
// under 10 says its ShoutMsg3 line (IsSay set), its parent's failing draw
// shouts nothing, and no social is wanted.
func TestPartyPrivateSplitCallsOutInAFight(t *testing.T) {
	t.Parallel()
	w, split := ind4cSocialSplit(t, 1)
	ind4bTakeUpAttack(t, w, split)
	w.clearDesires(t, split)
	w.draws.set(99, 0, 99)
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.Advance(t, 10*time.Second)
	if got := split.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("setup: intention at the firing = %v, want attack", got)
	}
	want := []ind4cLine{{serverpackets.SayTypeAll, script.NpcString(ind4cShoutMsg3)}}
	if got := ind4cLines(w.srv.ReadQueued(t, w.srv.Client), split.ObjectID()); !slices.Equal(got, want) {
		t.Fatalf("lines = %+v, want %+v", got, want)
	}
	wantSocial(t, "in the fight", split, 0)
}
