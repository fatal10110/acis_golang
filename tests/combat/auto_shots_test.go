package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// autoSoulshotID is the shared catalog's no-grade soulshot, whose charge
// visual is skill 2150; the catalog sword it charges spends one per hit.
const (
	autoSoulshotID     int32 = 1463
	autoSoulshotVisual int32 = 2150
)

// swingShots is what one of the player's swings showed about its soulshot:
// whether the Attack carried the charge, and whether the hit recharged the
// weapon (ENABLED_SOULSHOT, then the charge MagicSkillUse) ahead of its own
// damage feedback.
type swingShots struct {
	charged    bool
	enabled    bool
	chargeCast bool
	landed     bool
}

// TestAutoSoulshotsRechargeEverySwing pins CreatureAttack.onHitTimer
// (CreatureAttack.java:134-145) with Player.rechargeShots
// (Player.java:5072-5098): every swing's hit timer spends the landed
// swing's soulshot, then charges the weapon again from the auto-use
// soulshots, which sends ENABLED_SOULSHOT and the charge MagicSkillUse
// ahead of the hit's damage feedback and consumes one shot. Once the stack
// is gone the auto-use entry is dropped silently and swings go uncharged.
func TestAutoSoulshotsRechargeEverySwing(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	sword := srv.GiveItem(t, objID, midSwingSwordID, 1)
	shots := srv.GiveItem(t, objID, autoSoulshotID, 3)
	startInWorld(t, c)

	// Auto use goes on with no weapon in hand, then the equip charges the
	// sword from it: the first swing starts charged and three shots cover
	// the equip and the first two swings.
	c.Send(encodeRequestAutoSoulShot(autoSoulshotID, 1))
	drainUntilQuiet(t, c)
	c.Send(encodeUseItem(sword, false))
	drainUntilQuiet(t, c)
	var got int
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { got = pc.Inventory().ItemByObjectID(shots).Count })
	if got != 2 {
		t.Fatalf("soulshots after the equip charge = %d, want 2", got)
	}

	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 30, Y: hostileY, Z: hostileZ})
	// The monster's P.Def. is pinned so four charged critical swings leave
	// it standing.
	hostile.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpSet, Value: 20}})
	drainUntilQuiet(t, c)
	attacker := livePlayer(t, srv, objID)
	done := make(chan struct{})
	if !srv.PlayerQueue(t, objID).Post(func() {
		defer close(done)
		// Every roll at zero lands every hit, as a critical; the recharges'
		// own draws cannot shift it into a miss.
		attacker.(interface{ SetRollSource(func(int) int) }).SetRollSource(func(int) int { return 0 })
	}) {
		t.Fatal("player queue closed")
	}
	<-done

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	swings := readSwingShots(t, c, objID, 4)

	want := []swingShots{
		{charged: true, enabled: true, chargeCast: true, landed: true},
		{charged: true, enabled: true, chargeCast: true, landed: true},
		// The second swing's recharge spent the last shot: the third swing
		// is charged, and its hit finds no stack to charge from.
		{charged: true, landed: true},
		{landed: true},
	}
	for i := range want {
		if swings[i] != want[i] {
			t.Fatalf("swing %d = %+v, want %+v (all swings %+v)", i+1, swings[i], want[i], swings)
		}
	}
	held, autoOn := -1, false
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		if inst := pc.Inventory().ItemByTemplateID(autoSoulshotID); inst != nil {
			held = inst.Count
		}
		autoOn = pc.AutoSoulShotEnabled(autoSoulshotID)
	})
	if held >= 0 {
		t.Fatalf("soulshot stack still held with %d after four swings, want it spent", held)
	}
	if autoOn {
		t.Fatal("auto use of the spent soulshot is still on")
	}
}

// readSwingShots reads n of the player's swings and the shot traffic each
// swing's hit produced. A spent stack drops its auto use without an
// ExAutoSoulShot.
func readSwingShots(t *testing.T, c *scriptedClient, objID int32, n int) []swingShots {
	t.Helper()
	var swings []swingShots
	current := func(what string) *swingShots {
		if len(swings) == 0 {
			t.Fatalf("%s before the player's first Attack", what)
		}
		return &swings[len(swings)-1]
	}
	for end := c.Now().Add(30 * time.Second); c.Now().Before(end); {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			continue
		}
		r := wireReader(frame[1:])
		switch frame[0] {
		case serverpackets.OpcodeAttack:
			if r.ReadInt32() != objID {
				continue
			}
			r.ReadInt32() // target
			r.ReadInt32() // damage
			flags := r.ReadUint8()
			swings = append(swings, swingShots{charged: flags&attack.HitSoulshot != 0})
		case serverpackets.OpcodeSystemMessage:
			switch r.ReadInt32() {
			case serverpackets.SystemMessageEnabledSoulshot:
				s := current("ENABLED_SOULSHOT")
				if s.landed || s.enabled {
					t.Fatalf("ENABLED_SOULSHOT out of place in swing %d: %+v", len(swings), *s)
				}
				s.enabled = true
			case serverpackets.SystemMessageYouDidS1Dmg:
				s := current("damage feedback")
				s.landed = true
				if len(swings) == n {
					return swings
				}
			}
		case serverpackets.OpcodeMagicSkillUse:
			caster, _, skill := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
			if caster != objID || skill != autoSoulshotVisual {
				continue
			}
			s := current("charge MagicSkillUse")
			if !s.enabled || s.landed || s.chargeCast {
				t.Fatalf("charge MagicSkillUse out of place in swing %d: %+v", len(swings), *s)
			}
			s.chargeCast = true
		case serverpackets.OpcodeExtended:
			if r.ReadUint16() == serverpackets.OpcodeExAutoSoulShot {
				t.Fatalf("ExAutoSoulShot sent in swing %d; a spent stack drops auto use silently", len(swings))
			}
		}
	}
	t.Fatalf("read %d of %d swings within 30s: %+v", len(swings), n, swings)
	return nil
}

func encodeRequestAutoSoulShot(itemID, typ int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestAutoSoulShot)
	w.WriteInt32(itemID)
	w.WriteInt32(typ)
	return w.Bytes()
}
