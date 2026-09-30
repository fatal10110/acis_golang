package combat

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// indexOfSystemMessage finds the first SystemMessage frame at or after from
// carrying messageID, or -1.
func indexOfSystemMessage(frames [][]byte, from int, messageID int) int {
	for i := from; i < len(frames); i++ {
		if frames[i][0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		if wireReader(frames[i][1:]).ReadInt32() == int32(messageID) {
			return i
		}
	}
	return -1
}

// TestPlayerKillSendsDieBeforeDeathCosts pins the client-visible order of a
// player kill: the victim's Die reaches both clients before the killer's PK
// karma update, and on the victim before its charge reset and experience
// loss UserInfo, which keep that relative order. Values and persistence are the ones
// the death-cost and PK suites already pin.
func TestPlayerKillSendsDieBeforeDeathCosts(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithSeed(seedExperiencedCharacter(1500, 0)),
		gameservertest.WithSkills(combatPersistence(t, killSkillDefs())),
		gameservertest.WithLevels(deathLossTable(t)),
		gameservertest.WithAllowDelevel(true),
	)
	c := srv.Client
	victimID := srv.SoleObjectID(t)
	startInWorld(t, c)

	killerChar := srv.SeedCharacterFor(t, "killer", "Killer", 5, 0)
	seedKnownSkill(t, srv, killerChar.ID, 42, 1)
	killer := srv.DialClient(t, "killer", 1)
	startInWorld(t, killer)

	obj, ok := srv.State.Player(victimID)
	if !ok {
		t.Fatal("victim missing from world state")
	}
	charged, ok := obj.(interface{ IncreaseCharges(count, max int) bool })
	if !ok {
		t.Fatalf("world victim %T does not expose IncreaseCharges", obj)
	}
	if !charged.IncreaseCharges(2, 7) {
		t.Fatal("victim charges were not raised")
	}
	drainUntilQuiet(t, killer)
	drainUntilQuiet(t, c)

	killPrimaryClient(t, srv, killer, killerChar.ID, victimID)

	victimFrames := readQuiet(c)
	die := indexOf(victimFrames, 0, serverpackets.OpcodeDie, victimID)
	if die < 0 {
		t.Fatal("victim never received its own Die")
	}
	if early := indexOf(victimFrames[:die], 0, serverpackets.OpcodeEtcStatusUpdate, -1); early >= 0 {
		t.Fatalf("charge-reset EtcStatusUpdate at frame %d preceded Die at %d", early, die)
	}
	charges := indexOf(victimFrames, die+1, serverpackets.OpcodeEtcStatusUpdate, -1)
	if charges < 0 {
		t.Fatal("charge-reset EtcStatusUpdate never followed Die")
	}
	// The experience loss is a negative experience add: UserInfo, and no
	// EXP_DECREASED_BY_S1 (Player.java:2925, PlayerStatus.java:478-485).
	if lost := indexOfSystemMessage(victimFrames, 0, serverpackets.SystemMessageExpDecreasedByS1); lost >= 0 {
		t.Fatalf("death sent EXP_DECREASED_BY_S1 at frame %d; the reference sends none", lost)
	}
	if lost := indexOf(victimFrames, charges+1, serverpackets.OpcodeUserInfo, -1); lost < 0 {
		t.Fatal("experience-loss UserInfo never followed the charge reset")
	}

	killerFrames := readQuiet(killer)
	die = indexOf(killerFrames, 0, serverpackets.OpcodeDie, victimID)
	if die < 0 {
		t.Fatal("killer never received the victim's Die")
	}
	if early := indexOfSystemMessage(killerFrames[:die], 0, serverpackets.SystemMessageYourKarmaHasBeenChangedToS1); early >= 0 {
		t.Fatalf("killer karma message at frame %d preceded Die at %d", early, die)
	}
	if karma := indexOfSystemMessage(killerFrames, die+1, serverpackets.SystemMessageYourKarmaHasBeenChangedToS1); karma < 0 {
		t.Fatal("killer karma message never followed Die")
	}

	logoutPersisted(t, srv, c)
	logoutPersisted(t, srv, killer)
	if ch, err := srv.Chars.Get(context.Background(), victimID); err != nil || ch.Exp != 1300 {
		t.Fatalf("persisted victim = %+v, %v; want exp 1300", ch, err)
	}
	if ch, err := srv.Chars.Get(context.Background(), killerChar.ID); err != nil || ch.PKKills != 1 || ch.KarmaPoints != 240 {
		t.Fatalf("persisted killer = %+v, %v; want 1 PK kill and karma 240", ch, err)
	}
}

// TestPvPKillCreditFollowsDie pins the PvP branch of the same order: killing
// a flagged, karma-free victim resends the killer's UserInfo for its new PvP
// count only after the victim's Die.
func TestPvPKillCreditFollowsDie(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithSeed(seedExperiencedCharacter(1500, 0)),
		gameservertest.WithSkills(combatPersistence(t, killSkillDefs())),
	)
	c := srv.Client
	victimID := srv.SoleObjectID(t)
	startInWorld(t, c)

	killerChar := srv.SeedCharacterFor(t, "killer", "Killer", 5, 0)
	seedKnownSkill(t, srv, killerChar.ID, 42, 1)
	killer := srv.DialClient(t, "killer", 1)
	startInWorld(t, killer)

	obj, ok := srv.State.Player(victimID)
	if !ok {
		t.Fatal("victim missing from world state")
	}
	flagged, ok := obj.(interface{ UpdatePvPFlag(task.PvPFlagState) })
	if !ok {
		t.Fatalf("world victim %T does not expose UpdatePvPFlag", obj)
	}
	flagged.UpdatePvPFlag(task.PvPFlagOn)
	drainUntilQuiet(t, killer)
	drainUntilQuiet(t, c)

	killPrimaryClient(t, srv, killer, killerChar.ID, victimID)

	killerFrames := readQuiet(killer)
	die := indexOf(killerFrames, 0, serverpackets.OpcodeDie, victimID)
	if die < 0 {
		t.Fatal("killer never received the victim's Die")
	}
	if credit := indexOf(killerFrames, die+1, serverpackets.OpcodeUserInfo, -1); credit < 0 {
		t.Fatal("killer PvP-credit UserInfo never followed Die")
	}

	logoutPersisted(t, srv, killer)
	if ch, err := srv.Chars.Get(context.Background(), killerChar.ID); err != nil || ch.PvPKills != 1 || ch.KarmaPoints != 0 {
		t.Fatalf("persisted killer = %+v, %v; want 1 PvP kill and no karma", ch, err)
	}
}

// TestMonsterKillDeathPenaltyFollowsKarmaLoss pins the death-penalty step's
// place at the end of a monster kill: it runs after Die and after the
// death's karma loss, so its karma gate sees the karma that loss left. With
// no chance roll configured, a victim still carrying karma gains the
// penalty level (after Die), while one whose last karma the loss erased
// does not. The victim dies underwater, and its breath gauge is reset only
// after all of that.
func TestMonsterKillDeathPenaltyFollowsKarmaLoss(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		karma     int
		wantKarma int32
		wantLevel int
	}{
		// 200 lost exp / KarmaModifier 2 / 15 = 6 karma lost.
		{name: "karma left", karma: 240, wantKarma: 234, wantLevel: 1},
		{name: "karma erased", karma: 5, wantKarma: 0, wantLevel: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			form, err := zone.NewCuboid(-1_000, 1_000, -1_000, 1_000, -1_000, 150)
			if err != nil {
				t.Fatal(err)
			}
			zones := zone.NewIndex()
			zones.Add(zone.NewWater(1, form))
			srv := gameservertest.Boot(t,
				gameservertest.WithWantChars(1),
				gameservertest.WithZones(zones),
				gameservertest.WithWater(nil),
				gameservertest.WithSeed(seedExperiencedCharacter(1500, tc.karma)),
				gameservertest.WithLevels(deathLossTable(t)),
				gameservertest.WithAllowDelevel(true),
				gameservertest.WithRateKarmaExpLost(1.0),
			)
			c := srv.Client
			victimID := srv.SoleObjectID(t)
			// Entering the world already underwater interleaves the swim
			// state's frames with the EnterWorld burst, so only drain it.
			c.Send(encodeRequestGameStart(0))
			c.Send(encodeEnterWorld())
			drainUntilQuiet(t, c)
			hostile := srv.SpawnHostileNPC(t)
			drainUntilQuiet(t, c)

			obj, ok := srv.State.Player(victimID)
			if !ok {
				t.Fatal("victim missing from world state")
			}
			victim := obj.(interface {
				CurrentHP() int
				SetCP(float64)
				SetSpawnProtection(bool)
				ReduceHP(float64, attackable.Combatant, modelskill.Definition)
			})
			victim.SetSpawnProtection(false)
			victim.SetCP(0)
			victim.ReduceHP(float64(victim.CurrentHP()), hostile, modelskill.Definition{})
			srv.Settle(t)

			frames := readQuiet(c)
			die := indexOf(frames, 0, serverpackets.OpcodeDie, victimID)
			if die < 0 {
				t.Fatal("victim never received its own Die")
			}
			karma := indexOfSystemMessage(frames, die+1, serverpackets.SystemMessageYourKarmaHasBeenChangedToS1)
			if karma < 0 {
				t.Fatal("karma-loss message never followed Die")
			}
			r := wireReader(frames[karma][1:])
			r.ReadInt32() // message id
			if params := r.ReadInt32(); params != 1 {
				t.Fatalf("karma-loss message params = %d, want 1", params)
			}
			r.ReadInt32() // param type
			if got := r.ReadInt32(); got != tc.wantKarma {
				t.Fatalf("karma-loss message value = %d, want %d", got, tc.wantKarma)
			}
			penalty := indexOfSystemMessage(frames, 0, serverpackets.SystemMessageDeathPenaltyLevelS1Added)
			switch {
			case tc.wantLevel == 0 && penalty >= 0:
				t.Fatalf("death-penalty message at frame %d, want none once the karma loss erased karma", penalty)
			case tc.wantLevel > 0 && penalty < karma:
				t.Fatalf("death-penalty message at frame %d, want it after the karma loss at %d", penalty, karma)
			}
			if gauge := indexOf(frames, 0, serverpackets.OpcodeSetupGauge, -1); gauge < max(karma, penalty) {
				t.Fatalf("breath-gauge reset at frame %d, want it after the karma loss at %d and death penalty at %d", gauge, karma, penalty)
			}

			logoutPersisted(t, srv, c)
			ch, err := srv.Chars.Get(context.Background(), victimID)
			if err != nil || ch.KarmaPoints != int(tc.wantKarma) || ch.DeathPenaltyLevel() != tc.wantLevel {
				t.Fatalf("persisted victim = %+v, %v; want karma %d and death-penalty level %d", ch, err, tc.wantKarma, tc.wantLevel)
			}
		})
	}
}

// fusionSkillDefs is a single-target FUSION channel (hit time 15s) and the
// Fusion buff it triggers on its target.
func fusionSkillDefs() []modelskill.Definition {
	return []modelskill.Definition{
		{
			ID: 426, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			CastRange: 400, HitTime: 15_000, ReuseDelay: 30_000, StaticHitTime: true, StaticReuse: true,
			Magic: true, SkillType: "FUSION", TriggeredID: 5104, TriggeredLevel: 1,
		},
		{
			ID: 5104, Level: 1, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Fusion", Time: 600}},
		},
	}
}

// TestFusionCastersStopBetweenDeathCostsAndPenalty pins where a monster-killed
// player's death stops another player's fusion channel on it: after Die and
// the karma loss, before the death-penalty level.
func TestFusionCastersStopBetweenDeathCostsAndPenalty(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithSeed(seedExperiencedCharacter(1500, 240)),
		gameservertest.WithSkills(combatPersistence(t, fusionSkillDefs())),
		gameservertest.WithLevels(deathLossTable(t)),
		gameservertest.WithAllowDelevel(true),
		gameservertest.WithRateKarmaExpLost(1.0),
	)
	c := srv.Client
	victimID := srv.SoleObjectID(t)
	startInWorld(t, c)

	casterChar := srv.SeedCharacterFor(t, "caster", "Caster", 5, 0)
	seedKnownSkill(t, srv, casterChar.ID, 426, 1)
	caster := srv.DialClient(t, "caster", 1)
	startInWorld(t, caster)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, caster)
	drainUntilQuiet(t, c)

	selectPlayerTarget(t, caster, victimID)
	caster.Send(encodeRequestMagicSkillUse(426, false, false))
	readUntil(t, caster, serverpackets.OpcodeMagicSkillUse, "fusion MagicSkillUse")
	drainUntilQuiet(t, caster)
	drainUntilQuiet(t, c)

	obj, ok := srv.State.Player(victimID)
	if !ok {
		t.Fatal("victim missing from world state")
	}
	victim := obj.(interface {
		CurrentHP() int
		SetCP(float64)
		SetSpawnProtection(bool)
		ReduceHP(float64, attackable.Combatant, modelskill.Definition)
	})
	victim.SetSpawnProtection(false)
	victim.SetCP(0)
	victim.ReduceHP(float64(victim.CurrentHP()), hostile, modelskill.Definition{})
	srv.Settle(t)

	frames := readQuiet(c)
	die := indexOf(frames, 0, serverpackets.OpcodeDie, victimID)
	if die < 0 {
		t.Fatal("victim never received its own Die")
	}
	karma := indexOfSystemMessage(frames, die+1, serverpackets.SystemMessageYourKarmaHasBeenChangedToS1)
	if karma < 0 {
		t.Fatal("karma-loss message never followed Die")
	}
	if early := indexOf(frames[:karma], 0, serverpackets.OpcodeMagicSkillCanceled, casterChar.ID); early >= 0 {
		t.Fatalf("fusion cancel at frame %d preceded the karma loss at %d", early, karma)
	}
	canceled := indexOf(frames, karma+1, serverpackets.OpcodeMagicSkillCanceled, casterChar.ID)
	if canceled < 0 {
		t.Fatal("fusion cancel never followed the karma loss")
	}
	if penalty := indexOfSystemMessage(frames, 0, serverpackets.SystemMessageDeathPenaltyLevelS1Added); penalty < canceled {
		t.Fatalf("death-penalty message at frame %d, want it after the fusion cancel at %d", penalty, canceled)
	}
}
