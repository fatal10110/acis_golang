package skills

import (
	"bytes"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// relaxSkillID is Relax, shaped like skill 226 in the datapack: a toggle
// whose two-second tick drains 2 MP while the player sits.
const relaxSkillID int32 = 226

func relaxSkills() []modelskill.Definition {
	return append(gateSkills(), modelskill.Definition{
		ID: modelskill.ID(relaxSkillID), Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf,
		MPConsume: 2, SkillType: "CONT",
		Effects: []modelskill.EffectTemplate{{Name: "Relax", Count: 0x7fffffff, Time: 2, Value: 2}},
	})
}

func bootRelaxer(t *testing.T) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, relaxSkills())),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	for _, def := range relaxSkills() {
		seedKnownSkill(t, srv, objID, int(def.ID), 1)
	}
	startInWorld(t, c)
	return srv, c, objID
}

// TestRelaxEndsWithinOnePointOfFullHP pins #4930: Relax's tick ends it when
// HP plus one exceeds max HP (EffectRelax.onActionTime), so fractional HP
// half a point short of full already ends it with SKILL_DEACTIVATED_HP_FULL,
// while HP a point and a half short keeps it draining.
func TestRelaxEndsWithinOnePointOfFullHP(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		short float64
		ends  bool
	}{
		{name: "half a point short", short: 0.5, ends: true},
		{name: "a point and a half short", short: 1.5, ends: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, c, objID := bootRelaxer(t)
			if !srv.DrivesClock() {
				t.Skip("needs the driven clock to run the first Relax tick before any regeneration")
			}
			var pc *player.Character
			onPlayerQueue(t, srv, objID, func(p *player.Character) {
				pc = p
				p.SetHP(p.MaxHPValue() - tc.short)
			})
			drainUntilQuiet(t, c)

			c.Send(encodeRequestMagicSkillUse(relaxSkillID, false, false))
			readMatching(t, c, time.Second, "Relax sit-down", isWaitType(serverpackets.WaitSitting))
			drainUntilQuiet(t, c)
			var mpBefore float64
			onPlayerQueue(t, srv, objID, func(p *player.Character) { mpBefore = p.MPValue() })

			srv.Advance(t, 2100*time.Millisecond)
			srv.TickEffects()
			frames := c.ReadQueued()

			hpFull := slices.ContainsFunc(frames, func(f []byte) bool {
				return f[0] == serverpackets.OpcodeSystemMessage &&
					bytes.Equal(f[1:5], []byte{serverpackets.SystemMessageSkillDeactivatedHPFull, 0, 0, 0})
			})
			var on bool
			var mp float64
			onPlayerQueue(t, srv, objID, func(p *player.Character) {
				_, on = p.EffectList().ActiveBySkillID(int(relaxSkillID))
				mp = p.MPValue()
			})
			if hpFull != tc.ends || on == tc.ends {
				t.Fatalf("HP %.1f of %.1f after the first tick: HP-full message %v, Relax on %v; want message %v, on %v",
					pc.HP(), pc.MaxHPValue(), hpFull, on, tc.ends, !tc.ends)
			}
			if !tc.ends && mp != mpBefore-2 {
				t.Fatalf("MP %.1f after the first tick, want %.1f drained by 2", mp, mpBefore)
			}
		})
	}
}

// TestFakeDeathSendsOnlyPostureFrames pins #4931: switching Fake Death on
// lies the player down with ChangeWaitType alone (Player.startFakeDeath),
// and switching it off gets up with ChangeWaitType then Revive
// (Player.stopFakeDeath). Neither refreshes the player's appearance: no
// UserInfo or CharInfo. Both requests also show the toggle cast
// (PlayerCast.doToggleCast), and the icon refresh of the switch-off comes
// before the effect's exit.
func TestFakeDeathSendsOnlyPostureFrames(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootPostureCaster(t)
	lieDown, _ := fakeDeathDelays(t, srv, objID)

	c.Send(encodeRequestMagicSkillUse(fakeDeathSkillID, false, false))
	start := opcodeList(c.ReadQueued())
	wantStart := []byte{
		serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeStatusUpdate, serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeChangeWaitType, serverpackets.OpcodeAbnormalStatusUpdate,
	}
	if !bytes.Equal(start, wantStart) {
		t.Fatalf("Fake Death start frames %x, want %x", start, wantStart)
	}
	srv.Advance(t, lieDown)
	// On the wall clock the lie-down timer can fire just after the delay
	// advanced by; a request made while the player is still lying down is
	// deferred behind an ActionFailed (#5247).
	awaitPostureSettled(t, srv, objID)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(fakeDeathSkillID, false, false))
	end := opcodeList(c.ReadQueued())
	wantEnd := []byte{
		serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeAbnormalStatusUpdate,
		serverpackets.OpcodeChangeWaitType, serverpackets.OpcodeRevive,
	}
	if !bytes.Equal(end, wantEnd) {
		t.Fatalf("Fake Death end frames %x, want %x", end, wantEnd)
	}
}
