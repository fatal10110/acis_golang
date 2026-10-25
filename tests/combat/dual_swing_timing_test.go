package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// dualSwordID is a two-handed dual sword the shared catalog does not have:
// the catalog sword made a dual weapon.
const dualSwordID int32 = 9901

// dualSwordTemplates is the shared catalog plus dualSwordID.
func dualSwordTemplates(t *testing.T) *item.Table {
	t.Helper()
	var templates []*item.Template
	var sword *item.Template
	for _, tmpl := range gameservertest.ItemTemplates().All() {
		if tmpl.ID == dualSwordID {
			t.Fatalf("the shared catalog already has item %d", dualSwordID)
		}
		if tmpl.ID == midSwingSwordID {
			sword = tmpl
		}
		templates = append(templates, tmpl)
	}
	if sword == nil {
		t.Fatalf("the shared catalog has no sword %d", midSwingSwordID)
	}
	dual := *sword
	weapon := *sword.Weapon
	weapon.Type = item.WeaponDual
	dual.ID, dual.Name, dual.Slot, dual.Weapon = dualSwordID, "Dual Sword", item.SlotLRHand, &weapon
	return item.NewTable(append(templates, &dual))
}

// TestDualSwingLandsAtQuarterAndHalfAndFinishesAtThreeQuarters pins a
// player's dual swing on the driven clock (CreatureAttack.doAttack hands
// doAttackHitByDual half the attack time, CreatureAttack.java:326, which
// halves it again for the first landing and the step after it,
// CreatureAttack.java:392-394, and onHitTimer's DUAL branch lands the second
// hit and finishes a step apart, CreatureAttack.java:164-177): one Attack
// carries both hits, each with the soulshot flag of the charged weapon, the
// hits land at attackTime/4 and attackTime/2, and the attack intention swings
// again only when the swing finishes at 3*attackTime/4.
func TestDualSwingLandsAtQuarterAndHalfAndFinishesAtThreeQuarters(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(dualSwordTemplates(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	if !srv.DrivesClock() {
		t.Skip("timing a swing's landings needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	dual := srv.GiveItem(t, objID, dualSwordID, 1)
	srv.GiveItem(t, objID, autoSoulshotID, 10)
	startInWorld(t, c)
	c.Send(encodeRequestAutoSoulShot(autoSoulshotID, 1))
	drainUntilQuiet(t, c)
	c.Send(encodeUseItem(dual, false))
	drainUntilQuiet(t, c)

	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 30, Y: hostileY, Z: hostileZ})
	// A pinned P.Def. keeps the monster standing through the charged
	// critical hits.
	hostile.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpSet, Value: 200}})
	drainUntilQuiet(t, c)
	var attackMillis int
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		if got := pc.AttackType(); got != item.WeaponDual {
			t.Errorf("AttackType() with the dual sword = %v, want %v", got, item.WeaponDual)
		}
		// Every roll at zero lands every hit, as a critical, so both hits
		// carry the soulshot flag.
		pc.SetRollSource(func(int) int { return 0 })
		attackMillis = formulas.TimeBetweenAttacks(pc.AttackSpeed())
	})
	// Java halves the int attack time twice, each rounding down.
	step := time.Duration(attackMillis/2/2) * time.Millisecond

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	// The read stops at the Attack, so the clock stands at the swing's start.
	swing := readUntilOwnAttack(t, c, objID, "the dual swing")
	r := wireReader(swing[1:])
	r.ReadInt32() // attacker
	if got := r.ReadInt32(); got != hostile.ObjectID() {
		t.Fatalf("first hit target = %d, want %d", got, hostile.ObjectID())
	}
	r.ReadInt32() // damage
	firstFlags := r.ReadUint8()
	r.ReadInt32() // x
	r.ReadInt32() // y
	r.ReadInt32() // z
	if extra := r.ReadUint16(); extra != 1 {
		t.Fatalf("Attack extra hits = %d, want 1 (two hits per dual swing)", extra)
	}
	if got := r.ReadInt32(); got != hostile.ObjectID() {
		t.Fatalf("second hit target = %d, want %d", got, hostile.ObjectID())
	}
	r.ReadInt32() // damage
	secondFlags := r.ReadUint8()
	for i, flags := range []uint8{firstFlags, secondFlags} {
		if flags&attack.HitSoulshot == 0 || flags&attack.HitMiss != 0 {
			t.Fatalf("hit %d flags = %#x, want a landed soulshot hit", i+1, flags)
		}
	}

	landings, swings := dualSwingTraffic(t, srv, c, objID)
	if landings != 0 || swings != 0 {
		t.Fatalf("at the swing's start: landings = %d, swings = %d; want 0, 0", landings, swings)
	}
	for i, want := range []struct {
		at       string
		landings int
		swings   int
	}{
		{"attackTime/4", 1, 0},
		{"attackTime/2", 1, 0},
		{"3*attackTime/4", 0, 1},
	} {
		srv.Advance(t, step-time.Millisecond)
		if landings, swings := dualSwingTraffic(t, srv, c, objID); landings != 0 || swings != 0 {
			t.Fatalf("1ms before %s: landings = %d, swings = %d; want 0, 0", want.at, landings, swings)
		}
		srv.Advance(t, time.Millisecond)
		if landings, swings := dualSwingTraffic(t, srv, c, objID); landings != want.landings || swings != want.swings {
			t.Fatalf("step %d, at %s: landings = %d, swings = %d; want %d, %d", i+1, want.at, landings, swings, want.landings, want.swings)
		}
	}
}

// readUntilOwnAttack reads until objID's Attack and returns it.
func readUntilOwnAttack(t *testing.T, c *scriptedClient, objID int32, what string) []byte {
	t.Helper()
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(3 * time.Second)
		if frame == nil {
			t.Fatalf("%s never arrived", what)
		}
		if attackFrameBy(frame, objID) {
			return frame
		}
	}
	t.Fatalf("%s not found within 100 frames", what)
	return nil
}

// dualSwingTraffic reads what the server has queued to c without letting
// time pass, and counts the player's hit landings (each ends in its damage
// number or a miss) and its new swings.
func dualSwingTraffic(t *testing.T, srv *gameservertest.Server, c *scriptedClient, objID int32) (landings, swings int) {
	t.Helper()
	for _, frame := range srv.ReadQueued(t, c) {
		switch {
		case attackFrameBy(frame, objID):
			swings++
		case frame[0] == serverpackets.OpcodeSystemMessage:
			switch wireReader(frame[1:]).ReadInt32() {
			case serverpackets.SystemMessageYouDidS1Dmg, serverpackets.SystemMessageMissedTarget:
				landings++
			}
		}
	}
	return landings, swings
}
