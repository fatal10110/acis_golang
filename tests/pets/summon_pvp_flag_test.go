package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// summonPvPScene is a karma-free, unflagged owner whose wolf knows a strike,
// next to an innocent player the owner has selected, with a PvP flag
// tracker wired in.
type summonPvPScene struct {
	h        *petWorld
	pet      *summon.Actor
	victimID int32
	victim   interface{ Dead() bool }
	owner    interface {
		PvPFlagState() task.PvPFlagState
		Karma() int
	}
	flags *task.PvPFlags
}

// bootSummonPvPScene boots the scene with the lethal wolfStrike, or, when
// lethal is false, a harmless offensive debuff in its place.
func bootSummonPvPScene(t *testing.T, lethal bool) *summonPvPScene {
	t.Helper()
	strike := wolfStrike()
	if !lethal {
		strike.SkillType, strike.Power = "DEBUFF", 0
	}
	flags := task.NewPvPFlags(task.DefaultPvPFlagOptions(), nil)
	h, pet, _ := bootWolfStrikerWith(t, strike, gameservertest.WithAITask(), gameservertest.WithPvPFlags(flags))
	victimID := h.srv.SeedCharacterFor(t, "player2", "Victim", 5, 0).ID
	vc := h.srv.DialClient(t, "player2", 1)
	startInWorld(t, vc)
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, vc)
	victim, ok := h.srv.State.Player(victimID)
	if !ok {
		t.Fatal("victim missing from world state")
	}
	owner, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from world state")
	}
	h.targetPlayer(t, victimID)
	return &summonPvPScene{
		h: h, pet: pet, victimID: victimID, flags: flags,
		victim: victim.(interface{ Dead() bool }),
		owner: owner.(interface {
			PvPFlagState() task.PvPFlagState
			Karma() int
		}),
	}
}

// strike forces the wolf's strike on the victim and runs it until done
// reports the hit has landed.
func (s *summonPvPScene) strike(t *testing.T, done func() bool) {
	t.Helper()
	s.h.client.Send(encodeRequestActionUse(wolfStrikeAction, true))
	requireSummonStrikeStarted(t, readUntilOpcode(t, s.h.client, serverpackets.OpcodeActionFailed, "strike ActionFailed"), s.pet, s.victimID)
	s.h.srv.AdvanceUntil(t, "strike landing on the victim", done)
	s.h.srv.Settle(t)
}

// attack forces the wolf's auto-attack on the victim, every swing rolled
// with roll, and runs it until done reports the hit has landed.
func (s *summonPvPScene) attack(t *testing.T, roll func(int) int, done func() bool) {
	t.Helper()
	setSummonRoll(t, s.h.srv, s.h.ownerID, s.pet, roll)
	s.h.client.Send(encodeRequestActionUse(petAttackAction, true))
	s.h.srv.AdvanceUntil(t, "pet hit landing on the victim", done)
	s.h.srv.Settle(t)
}

func (s *summonPvPScene) ownerFlagged() bool { return s.owner.PvPFlagState() != task.PvPFlagNone }

func (s *summonPvPScene) requireOwner(t *testing.T, wantKarma bool, wantFlag task.PvPFlagState, tracked int) {
	t.Helper()
	if karma := s.owner.Karma(); (karma > 0) != wantKarma {
		t.Fatalf("owner karma = %d, want karma %v", karma, wantKarma)
	}
	if state := s.owner.PvPFlagState(); state != wantFlag {
		t.Fatalf("owner PvP flag = %v, want %v", state, wantFlag)
	}
	if n := s.flags.Len(); n != tracked {
		t.Fatalf("PvP flag task tracks %d players, want %d", n, tracked)
	}
}

// TestSummonSkillOnFlaggedPlayerFlagsOwner forces the wolf's offensive
// debuff on a karma-free player once that player is PvP-flagged: no debuff
// may be cast on a player outside a clan war who is neither flagged nor a
// PKer, CTRL or not (Playable.canCastOffensiveSkillOnPlayable). A summon's
// cast flags its acting player once the skill has run
// (CreatureCast.callSkill): the owner ends flagged, without karma.
func TestSummonSkillOnFlaggedPlayerFlagsOwner(t *testing.T) {
	t.Parallel()
	s := bootSummonPvPScene(t, false)
	s.victim.(interface{ UpdatePvPFlag(task.PvPFlagState) }).UpdatePvPFlag(task.PvPFlagOn)
	drainUntilQuiet(t, s.h.client)
	s.strike(t, s.ownerFlagged)
	if s.victim.Dead() {
		t.Fatal("the harmless debuff killed the victim")
	}
	s.requireOwner(t, false, task.PvPFlagOn, 1)
}

// TestSummonSkillPKKillLeavesOwnerFlagged has the wolf's strike kill the
// innocent player the owner follows, its main target. The kill makes the owner a PKer, which resets its flag,
// but the strike flags the owner once its effects have run, so the owner
// ends with both karma and the flag.
func TestSummonSkillPKKillLeavesOwnerFlagged(t *testing.T) {
	t.Parallel()
	s := bootSummonPvPScene(t, true)
	s.h.followPlayer(t, s.victimID)
	s.strike(t, s.victim.Dead)
	if !s.victim.Dead() {
		t.Fatal("victim alive after the lethal strike")
	}
	s.requireOwner(t, true, task.PvPFlagOn, 1)
}

// TestSummonHitOnInnocentPlayerFlagsOwner forces the wolf's auto-attack on
// the innocent player: a summon's hit flags its acting player before its
// damage, even a missed one (CreatureAttack.onHitTimer). Every swing
// misses, so the victim lives.
func TestSummonHitOnInnocentPlayerFlagsOwner(t *testing.T) {
	t.Parallel()
	s := bootSummonPvPScene(t, false)
	s.attack(t, func(int) int { return 999 }, s.ownerFlagged)
	if s.victim.Dead() {
		t.Fatal("a missed swing killed the victim")
	}
	s.requireOwner(t, false, task.PvPFlagOn, 1)
}

// TestSummonHitPKKillLeavesOwnerUnflagged has the wolf's hit kill the
// innocent player. The hit flags the owner before its damage, then the kill
// makes the owner a PKer and resets the flag: the owner ends with karma and
// no flag.
func TestSummonHitPKKillLeavesOwnerUnflagged(t *testing.T) {
	t.Parallel()
	s := bootSummonPvPScene(t, false)
	s.attack(t, landNoCrit(), s.victim.Dead)
	if !s.victim.Dead() {
		t.Fatal("victim alive after the pet's lethal hit")
	}
	s.requireOwner(t, true, task.PvPFlagNone, 0)
}
