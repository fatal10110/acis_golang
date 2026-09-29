package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestKarmaPlayerCannotForceAttackBlessedPet has a level-30 player with
// karma force-attack a level-10 pet under Blessing of Protection of its own.
// Outside a PvP zone the pet cannot be attacked: the forced click never
// swings and no damage lands. Inside an arena the forced attack lands.
func TestKarmaPlayerCannotForceAttackBlessedPet(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		arena bool
	}{
		{"outside a PvP zone", false},
		{"inside an arena", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var extra []gameservertest.Option
			if tt.arena {
				extra = append(extra, gameservertest.WithZones(arenaZones(t)))
			}
			h := bootOwnerWithCollarOpts(t, extra)
			wolf, _ := h.spawnWolf(t)
			addPetEffect(t, wolf, "ProtectionBlessing")
			if !wolf.ProtectionBlessing() {
				t.Fatal("Blessing of Protection did not land on the pet")
			}

			pkID := h.srv.SeedCharacterFor(t, "pk", "Pk", 30, 0).ID
			ch, err := h.srv.Chars.Get(petCtx(), pkID)
			if err != nil {
				t.Fatalf("load Pk: %v", err)
			}
			ch.KarmaPoints = 500
			if err := h.srv.Chars.Save(petCtx(), ch.SaveState()); err != nil {
				t.Fatalf("save Pk: %v", err)
			}
			pk := h.srv.DialClient(t, "pk", 1)
			startInWorld(t, pk)
			landEveryHit(t, h.srv, pkID)
			drainUntilQuiet(t, h.client)
			drainUntilQuiet(t, pk)

			x, y, z := wolf.Position()
			pk.Send(encodeAction(wolf.ObjectID(), int32(x), int32(y), int32(z), false))
			drainUntilQuiet(t, pk)
			full := wolf.HP()
			// A second click on the selected pet is the forced attack.
			pk.Send(encodeAttackRequest(wolf.ObjectID(), int32(x), int32(y), int32(z), false))

			if tt.arena {
				h.srv.AdvanceUntil(t, "forced attack landing on a blessed pet inside an arena", func() bool { return wolf.HP() < full })
				return
			}
			for passed := time.Duration(0); passed < 3*time.Second; passed += 100 * time.Millisecond {
				h.srv.Advance(t, 100*time.Millisecond)
				for f := pk.ReadWithTimeout(20 * time.Millisecond); f != nil; f = pk.ReadWithTimeout(20 * time.Millisecond) {
					if frameIndex([][]byte{f}, serverpackets.OpcodeAttack, pkID) >= 0 {
						t.Fatal("karma player swung at the blessed pet")
					}
				}
			}
			if got := wolf.HP(); got != full {
				t.Fatalf("blessed pet HP = %v after the forced attack, want untouched %v", got, full)
			}
		})
	}
}
