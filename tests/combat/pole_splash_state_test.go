package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// splashPoleID is a pole the shared catalog does not have.
const splashPoleID int32 = 9902

func splashPoleTemplates(t *testing.T) *item.Table {
	t.Helper()
	templates := gameservertest.ItemTemplates().All()
	for _, tmpl := range templates {
		if tmpl.ID == splashPoleID {
			t.Fatalf("the shared catalog already has item %d", splashPoleID)
		}
	}
	return item.NewTable(append(templates, &item.Template{
		ID: splashPoleID, Name: "Pole", Kind: item.KindWeapon, Slot: item.SlotLRHand,
		Duration: -1, Destroyable: true, DefaultAction: item.ActionEquip,
		Weapon: &item.WeaponDetail{Type: item.WeaponPole},
	}))
}

// TestPoleHitOnASplashTargetKilledInFlight pins a pole swing whose splash
// target a third party killed before the hit (CreatureAttack.doHit has no
// per-target check): the player still reads the damage feedback of both
// hits.
func TestPoleHitOnASplashTargetKilledInFlight(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(splashPoleTemplates(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	if !srv.DrivesClock() {
		t.Skip("holding a swing open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	pole := srv.GiveItem(t, objID, splashPoleID, 1)
	startInWorld(t, c)
	c.Send(encodeUseItem(pole, false))
	drainUntilQuiet(t, c)

	primary := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 30, Y: hostileY, Z: hostileZ})
	splash := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY + 10, Z: hostileZ})
	// A pinned P.Def. keeps the main target standing through the hit.
	primary.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpSet, Value: 200}})
	drainUntilQuiet(t, c)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		if got := pc.AttackType(); got != item.WeaponPole {
			t.Errorf("AttackType() with the pole = %v, want %v", got, item.WeaponPole)
		}
		pc.AddStatFuncs([]effect.Mod{{Stat: stat.AttackCountMax, Op: effect.OpSet, Value: 3}})
		// Every hit lands, none critical.
		pc.SetRollSource(func(int) int { return 99 })
	})
	drainUntilQuiet(t, c)
	hitAt := time.Duration(playerAttackMillis(t, srv, objID)/2) * time.Millisecond

	targetHostile(t, c, primary.ObjectID())
	c.Send(encodeAction(primary.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	swing := readUntilOwnAttack(t, c, objID, "the pole swing")
	r := wireReader(swing[1:])
	r.ReadInt32() // attacker
	r.ReadInt32() // first target
	r.ReadInt32() // damage
	r.ReadUint8() // flags
	r.ReadInt32() // x
	r.ReadInt32() // y
	r.ReadInt32() // z
	if extra := r.ReadUint16(); extra != 1 {
		t.Fatalf("Attack extra hits = %d, want 1 (the splash target)", extra)
	}
	if got := r.ReadInt32(); got != splash.ObjectID() {
		t.Fatalf("splash hit target = %d, want %d", got, splash.ObjectID())
	}
	if !splash.Die(nil, nil) {
		t.Fatal("splash.Die() = false, want the third-party kill")
	}

	srv.Advance(t, hitAt)
	if got, want := tallySwing(t, srv, c, objID), (swingTally{damage: 2}); got != want {
		t.Fatalf("at the hit: %+v, want %+v (both hits reported)", got, want)
	}
}
