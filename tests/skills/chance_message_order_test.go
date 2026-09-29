package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The passive ON_HIT chance skill below always fires on the holder's
// landed melee hit and triggers procOrderTriggered on the monster hit.
const (
	procOrderPassive   = 3207
	procOrderTriggered = 5146
	// procKillPower is far above the fixture monster's 1000 HP, so the
	// proc kills it and its damage report stands apart from the melee
	// hit's.
	procKillPower = 1_000_000
)

// procKillFrames is where a killing proc's frames sit in the holder's log:
// the proc's damage report, the monster's zero-HP status, the exp message
// and the monster's Die.
type procKillFrames struct{ dealt, status, reward, die int }

// procKillingSkill boots a player holding an always-firing ON_HIT chance
// skill that triggers triggered, has its melee hit set the proc off against
// the fixture monster until the proc kills it, and returns where the kill's
// frames sit in the player's log.
func procKillingSkill(t *testing.T, triggered modelskill.Definition) procKillFrames {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: procOrderPassive, Level: 1, Activation: modelskill.ActivationPassive, Target: modelskill.TargetSelf,
				SkillType: "BUFF", ChanceType: "ON_HIT", ActivationChance: -1,
				TriggeredID: int(triggered.ID), TriggeredLevel: triggered.Level,
			},
			triggered,
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, procOrderPassive, 1)
	startInWorld(t, c)
	// Magic rolls never fail or crit; every other roll comes up zero, so
	// the melee hit lands.
	setPlayerRollSource(t, srv, objID, func(n int) int {
		if n == 10000 {
			return 500
		}
		return 0
	})
	px, py, pz := srv.PlayerPosition(t, objID)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: px + 20, Y: py, Z: pz})
	drainUntilQuiet(t, c)

	startMelee(t, srv, objID, hostile.ObjectID())
	srv.AdvanceUntil(t, "the killing proc", hostile.Dead)

	log := readFrameLog(c)
	got := procKillFrames{
		dealt: log.index(func(frame []byte) bool {
			r, ok := systemMessage(frame, serverpackets.SystemMessageYouDidS1Dmg)
			return ok && r.ReadInt32() == 1 && r.ReadInt32() == serverpackets.SystemMessageParamNumber && r.ReadInt32() >= 1000
		}),
		status: log.index(func(frame []byte) bool {
			hp, ok := statusValue(frame, hostile.ObjectID(), serverpackets.StatusCurrentHP)
			return ok && hp == 0
		}),
		reward: log.index(isSystemMessage(serverpackets.SystemMessageYouEarnedS1ExpAndS2SP)),
		die:    log.index(objectFrame(serverpackets.OpcodeDie, hostile.ObjectID())),
	}
	if got.dealt < 0 || got.status < 0 || got.reward < 0 || got.die < 0 {
		t.Fatalf("holder frames: proc YOU_DID_S1_DMG at %d, zero-HP StatusUpdate at %d, exp message at %d, Die at %d; want all present",
			got.dealt, got.status, got.reward, got.die)
	}
	return got
}

// TestChanceProcMDamKillReportsTheDamageBeforeTheKill: an MDAM chance proc
// that kills the fixture monster reports its damage before the monster's
// zero-HP status, the exp message and its Die, the way a cast MDAM does
// (issue #2777).
func TestChanceProcMDamKillReportsTheDamageBeforeTheKill(t *testing.T) {
	t.Parallel()
	got := procKillingSkill(t, modelskill.Definition{
		ID: procOrderTriggered, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		SkillType: "MDAM", Power: procKillPower, Magic: true, Offensive: true,
	})
	if !(got.dealt < got.status && got.status < got.reward && got.reward < got.die) {
		t.Fatalf("holder frame order: proc YOU_DID_S1_DMG %d, zero-HP StatusUpdate %d, exp message %d, Die %d; want that order",
			got.dealt, got.status, got.reward, got.die)
	}
}

// TestChanceProcPDamKillReportsTheDamageAfterTheKill: a PDAM chance proc
// reports its damage after applying it, so the holder reads the kill's
// frames first, the way a cast PDAM does (issue #2777).
func TestChanceProcPDamKillReportsTheDamageAfterTheKill(t *testing.T) {
	t.Parallel()
	got := procKillingSkill(t, modelskill.Definition{
		ID: procOrderTriggered, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		SkillType: "PDAM", Power: procKillPower, Offensive: true,
	})
	if !(got.status < got.reward && got.reward < got.die && got.die < got.dealt) {
		t.Fatalf("holder frame order: zero-HP StatusUpdate %d, exp message %d, Die %d, proc YOU_DID_S1_DMG %d; want that order",
			got.status, got.reward, got.die, got.dealt)
	}
}
