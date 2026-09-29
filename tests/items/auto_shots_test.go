package items

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// autoSpiritshotID is the shared catalog's no-grade spiritshot, whose charge
// visual is skill 2047.
const (
	autoSpiritshotID     int32 = 2509
	autoSpiritshotVisual int32 = 2047
)

// castShots is what one magic cast showed about the caster's spiritshot:
// whether the cast recharged the weapon (ENABLED_SPIRITSHOT, then the charge
// MagicSkillUse), and whether that came after the cast's MagicSkillLaunched.
type castShots struct {
	launched   bool
	enabled    bool
	chargeCast bool
}

// TestAutoSpiritshotsRechargeAfterEachMagicCast pins
// CreatureCast.onMagicFinalizer (CreatureCast.java:299-306) with
// Player.rechargeShots (Player.java:5072-5098): a magic skill's cast
// finalizer charges the weapon again from the auto-use spiritshots, after
// the cast's MagicSkillLaunched, which sends ENABLED_SPIRITSHOT and the
// charge MagicSkillUse and consumes one shot. Each cast spends the charge
// the previous one put back. Once the stack is gone the auto-use entry is
// dropped silently.
func TestAutoSpiritshotsRechargeAfterEachMagicCast(t *testing.T) {
	t.Parallel()
	const healSkillID = 1011
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: healSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		Magic: true, HitTime: 500, StaticHitTime: true, SkillType: "HEAL", Power: 1,
	}}), gamesql.NewCharacterSkillStore(db))
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(skills),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, healSkillID, 1); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	weapon := srv.GiveItem(t, objID, 30, 1)
	shots := srv.GiveItem(t, objID, autoSpiritshotID, 3)
	startInWorld(t, c)

	// Auto use goes on with no weapon in hand, then the equip charges the
	// sword from it: three shots cover the equip and the first two casts.
	c.Send(encodeRequestAutoSoulShot(autoSpiritshotID, 1))
	drainUntilQuiet(t, c)
	c.Send(encodeUseItem(weapon, false))
	drainUntilQuiet(t, c)
	var got int
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { got = pc.Inventory().ItemByObjectID(shots).Count })
	if got != 2 {
		t.Fatalf("spiritshots after the equip charge = %d, want 2", got)
	}

	want := []castShots{
		{launched: true, enabled: true, chargeCast: true},
		{launched: true, enabled: true, chargeCast: true},
		// The second cast's recharge spent the last shot.
		{launched: true},
	}
	for i, w := range want {
		c.Send(encodeRequestMagicSkillUse(healSkillID))
		if got := readCastShots(t, c, objID, healSkillID); got != w {
			t.Fatalf("cast %d = %+v, want %+v", i+1, got, w)
		}
	}

	charged, autoOn, held := false, false, -1
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		charged = pc.ChargedShot(item.ShotSpirit)
		autoOn = pc.AutoSoulShotEnabled(autoSpiritshotID)
		if inst := pc.Inventory().ItemByTemplateID(autoSpiritshotID); inst != nil {
			held = inst.Count
		}
	})
	if charged {
		t.Fatal("weapon still charged after the third cast spent the last recharge")
	}
	if autoOn {
		t.Fatal("auto use of the spent spiritshot is still on")
	}
	if held >= 0 {
		t.Fatalf("spiritshot stack still held with %d, want it spent", held)
	}
}

// readCastShots reads one cast of skillID by objID through its end, two
// seconds of clock, and reports its spiritshot traffic. A spent stack drops
// its auto use without an ExAutoSoulShot.
func readCastShots(t *testing.T, c *testsupport.ScriptedClient, objID, skillID int32) castShots {
	t.Helper()
	var got castShots
	for end := c.Now().Add(2 * time.Second); c.Now().Before(end); {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			continue
		}
		r := wire.NewReader(frame[1:])
		switch frame[0] {
		case serverpackets.OpcodeMagicSkillLaunched:
			if r.ReadInt32() == objID && r.ReadInt32() == skillID {
				got.launched = true
			}
		case serverpackets.OpcodeSystemMessage:
			if r.ReadInt32() != serverpackets.SystemMessageEnabledSpiritshot {
				continue
			}
			if !got.launched || got.enabled {
				t.Fatalf("ENABLED_SPIRITSHOT out of place: %+v", got)
			}
			got.enabled = true
		case serverpackets.OpcodeMagicSkillUse:
			caster, _, skill := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
			if caster != objID || skill != autoSpiritshotVisual {
				continue
			}
			if !got.enabled || got.chargeCast {
				t.Fatalf("charge MagicSkillUse out of place: %+v", got)
			}
			got.chargeCast = true
		case serverpackets.OpcodeExtended:
			if r.ReadUint16() == serverpackets.OpcodeExAutoSoulShot {
				t.Fatal("ExAutoSoulShot sent; a spent stack drops auto use silently")
			}
		}
	}
	return got
}

// TestUseBeastSoulshotNotEnoughWithAutoDisablesAuto pins BeastSoulShots'
// failed consume (BeastSoulShots.java:52-56): a beast soulshot stack that
// cannot pay the servitor's per-hit count while auto-enabled turns auto use
// off (ExAutoSoulShot off, then AUTO_USE_OF_S1_CANCELLED) instead of
// reporting the shortage, and the click still gets its ActionFailed.
func TestUseBeastSoulshotNotEnoughWithAutoDisablesAuto(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	shot := srv.GiveItem(t, objID, 6645, 4)
	startInWorld(t, c)

	servitor, err := summon.NewServitor(summon.ServitorConfig{
		ObjectID: srv.NewObjectID(),
		Level:    44,
		Stats:    summon.CombatStats{MaxHP: 500, MaxMP: 200, SSCount: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	servitor.SetQueue(srv.PlayerQueue(t, objID))
	srv.State.AddSummon(objID, servitor)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestAutoSoulShot(6645, 1))
	assertExAutoSoulShot(t, c.Read(), 6645, true)
	assertSystemMessageItem(t, c.Read(), serverpackets.SystemMessageUseOfItemWillBeAuto, 6645)
	drainUntilQuiet(t, c)

	c.Send(encodeUseItem(shot, false))
	assertExAutoSoulShot(t, c.Read(), 6645, false)
	assertSystemMessageItem(t, c.Read(), serverpackets.SystemMessageAutoUseOfItemCancelled, 6645)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "failed consume")
	if reply := c.ReadWithTimeout(300 * time.Millisecond); reply != nil {
		t.Fatalf("unexpected extra reply %x, want none (not-enough suppressed while auto-enabled)", reply)
	}
	if inst := mustFindItem(t, srv, objID, shot); inst.Count != 4 {
		t.Fatalf("beast soulshot count after the failed consume = %d, want 4", inst.Count)
	}
}
