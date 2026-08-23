package combat

import (
	"context"
	"testing"

	playermodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// killSkillDefs is one physical single-target skill able to one-shot the
// fixture monster, plus its RECALL sibling used by the karma guards.
func killSkillDefs() []modelskill.Definition {
	return []modelskill.Definition{
		{
			ID: 42, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
			MPInitialConsume: 2, MPConsume: 3, SkillType: "PDAM", Power: 1_000_000,
		},
	}
}

// levelTableFor is a table whose thresholds sit low enough that the kill
// reward promotes the character exactly once (into level 6) and never
// further.
func levelTableFor(t *testing.T) *playermodel.LevelTable {
	t.Helper()
	table, err := playermodel.NewLevelTable(map[int]playermodel.Level{
		1: {RequiredExpToLevelUp: 0},
		2: {RequiredExpToLevelUp: 1},
		3: {RequiredExpToLevelUp: 2},
		4: {RequiredExpToLevelUp: 3},
		5: {RequiredExpToLevelUp: 4},
		6: {RequiredExpToLevelUp: 5},
		7: {RequiredExpToLevelUp: 1_000_000_000},
	})
	if err != nil {
		t.Fatalf("build level table: %v", err)
	}
	return table
}

// readExpSpGain scans frames until the exp/SP gain SystemMessage arrives and
// asserts its payload against the awarded amounts.
func readExpSpGain(t *testing.T, c *scriptedClient, exp int64, sp int) {
	t.Helper()
	for i := 0; i < 50; i++ {
		frame := c.ReadWithTimeout(readQuietWindow)
		if frame == nil {
			t.Fatalf("exp/SP gain message for %d/%d never arrived", exp, sp)
		}
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wireReader(frame[1:])
		id := r.ReadInt32()
		switch id {
		case int32(serverpackets.SystemMessageYouEarnedS1ExpAndS2SP):
			if params := r.ReadInt32(); params != 2 {
				t.Fatalf("gain message params = %d, want 2", params)
			}
			gotType := r.ReadInt32()
			gotExp := r.ReadInt32()
			gotType2 := r.ReadInt32()
			gotSP := r.ReadInt32()
			if gotType != serverpackets.SystemMessageParamNumber || gotType2 != serverpackets.SystemMessageParamNumber {
				t.Fatalf("gain message param types = %d/%d, want numbers", gotType, gotType2)
			}
			if gotExp != int32(exp) || gotSP != int32(sp) {
				t.Fatalf("gain message = exp %d sp %d, want exp %d sp %d", gotExp, gotSP, exp, sp)
			}
			return
		case int32(serverpackets.SystemMessageEarnedS1Experience):
			t.Fatalf("got experience-only gain message, want combined exp %d / sp %d", exp, sp)
		default:
			continue
		}
	}
	t.Fatal("exp/SP gain message not found within 50 frames")
}

// TestKillNPCPaysExpAndSp walks the reward chain end to end: killing the
// monster reports the exact awarded amounts on the wire and persists them.
func TestKillNPCPaysExpAndSp(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t, killSkillDefs())),
		gameservertest.WithLevels(levelTableFor(t)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 42, 1)
	startInWorld(t, c)
	hostile := spawnRewardedNPC(t, srv, 5000, 25)
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(42, false, false))
	readCastStartFrames(t, c, objID, 42, 1, 500, 60_000, hostile.ObjectID())

	waitFor(t, "monster death", func() bool { return hostile.CurrentHP() <= 0 })

	wantExp, wantSp := playermodel.KillRewardExpAndSp(5000, 25, 1, 1, 5-1)
	if wantExp <= 0 || wantSp <= 0 {
		t.Fatalf("oracle reward = %d exp / %d sp, want positive amounts", wantExp, wantSp)
	}
	readExpSpGain(t, c, wantExp, wantSp)

	// Logout persists the character; the reward must survive the round-trip.
	c.Send(encodeLogout())
	var saved *playermodel.Character
	waitFor(t, "persisted exp", func() bool {
		ch, err := srv.Chars.Get(context.Background(), objID)
		if err == nil && ch.Exp >= wantExp {
			saved = ch
			return true
		}
		return false
	})
	if saved.SP < wantSp {
		t.Fatalf("persisted SP = %d, want at least %d", saved.SP, wantSp)
	}
}

// TestKillNPCLevelUpRefreshesSkills pins the level-up leg of the same flow:
// a reward that crosses a threshold announces the new level and re-derives
// the character's SkillList.
func TestKillNPCLevelUpRefreshesSkills(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t, killSkillDefs())),
		gameservertest.WithLevels(levelTableFor(t)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 42, 1)
	startInWorld(t, c)
	hostile := spawnRewardedNPC(t, srv, 5000, 25)
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(42, false, false))
	readCastStartFrames(t, c, objID, 42, 1, 500, 60_000, hostile.ObjectID())
	waitFor(t, "monster death", func() bool { return hostile.CurrentHP() <= 0 })

	var sawSkillList bool
	for i := 0; i < 50 && !sawSkillList; i++ {
		frame := c.ReadWithTimeout(readQuietWindow)
		if frame == nil {
			break
		}
		if frame[0] == serverpackets.OpcodeSkillList {
			sawSkillList = true
		}
	}
	if !sawSkillList {
		t.Fatal("no SkillList after the level-up reward")
	}

	// Logout persists the character; the promoted level must survive.
	c.Send(encodeLogout())
	waitFor(t, "persisted level", func() bool {
		ch, err := srv.Chars.Get(context.Background(), objID)
		return err == nil && ch.CharLevel == 6
	})
}
