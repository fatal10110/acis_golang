package items

import (
	"context"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// stopMagicSkillID is a known magic self buff whose long hit time holds the
// cast open while a potion is queued behind it.
const stopMagicSkillID = 1011

// stopQueueItemSkills is the Lesser Healing Potion's physical skill (2031)
// plus stopMagicSkillID.
func stopQueueItemSkills(t *testing.T) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: 248, Level: 3},
		{ID: 294, Level: 1},
		{
			ID: 2031, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "DUMMY", StaticHitTime: true, HitTime: 500, StaticReuse: true, ReuseDelay: 3000,
		},
		{
			ID: stopMagicSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", StaticHitTime: true, HitTime: 5000, StaticReuse: true, Magic: true,
		},
	}), gamesql.NewCharacterSkillStore(db))
}

// TestMuteStopDropsQueuedItemCast pins Mute stopping a magic cast with a
// potion's item cast queued behind it. CreatureCast.stop
// (CreatureCast.java:404-434) notifies FINISHED_CASTING before it clears
// _isCastingNow, so the queued CAST's PlayerAI.thinkCast (PlayerAI.java:
// 219-226) still sees a cast in flight: it goes idle and answers
// ActionFailed, then PlayerCast.stop (PlayerCast.java:381-387) answers its
// own. The potion's skill never starts and the potion is not used.
func TestMuteStopDropsQueuedItemCast(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(stopQueueItemSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	if !srv.DrivesClock() {
		t.Skip("holding a cast open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, stopMagicSkillID, 1); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	potion := srv.GiveItem(t, objID, 1060, 5)
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(stopMagicSkillID))
	assertMagicSkillUseSelf(t, c.Read(), objID, stopMagicSkillID, 1, 5000, 0)
	drainUntilQuiet(t, c)
	c.Send(encodeUseItem(potion, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued item cast")
	drainUntilQuiet(t, c)

	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		e, err := effect.New(effect.Skill{ID: 1064, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "Mute", Time: 30})
		if err != nil {
			t.Errorf("effect.New(Mute): %v", err)
			return
		}
		e.Effector, e.Effected = pc, pc
		pc.EffectList().Add(e)
	})
	var stop []byte
	for _, frame := range readUntilQuiet(t, c) {
		switch frame[0] {
		case serverpackets.OpcodeMagicSkillCanceled, serverpackets.OpcodeActionFailed, serverpackets.OpcodeMagicSkillUse:
			stop = append(stop, frame[0])
		}
	}
	want := []byte{serverpackets.OpcodeMagicSkillCanceled, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}
	if string(stop) != string(want) {
		t.Fatalf("Mute stop with a queued item cast sent opcodes %x, want %x", stop, want)
	}

	srv.Advance(t, 10*time.Second)
	for _, frame := range readUntilQuiet(t, c) {
		if frame[0] == serverpackets.OpcodeMagicSkillUse {
			t.Fatal("queued item cast started after the stop")
		}
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("a cast is in flight after the stop")
	}
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		if pc.SkillDisabled(actorcast.ReuseKey(modelskill.Definition{ID: 2031, Level: 1})) {
			t.Error("potion skill on reuse after the stop dropped it")
		}
	})
	assertItemCount(t, srv, objID, potion, 5)
}
