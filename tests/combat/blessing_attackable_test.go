package combat

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// blessedDuel is a level-30 player with karma (the default client) beside a
// level-10 player under Blessing of Protection.
type blessedDuel struct {
	srv          *gameservertest.Server
	pk, victim   *scriptedClient
	pkID, victID int32
}

// bootBlessedDuel boots the karma player and the blessed victim, both in the
// world and quiet. victimKarma makes the victim attackable without force;
// a non-zero skillID is taught to the karma player before it enters.
func bootBlessedDuel(t *testing.T, arena bool, victimKarma int, skillID int, opts ...gameservertest.Option) blessedDuel {
	t.Helper()
	opts = append(opts, gameservertest.WithCharacter("Pk", 30, 0), gameservertest.WithWantChars(1))
	if arena {
		opts = append(opts, gameservertest.WithZones(arenaEverywhere(t)))
	}
	srv := gameservertest.Boot(t, opts...)
	d := blessedDuel{srv: srv, pk: srv.Client, pkID: srv.SoleObjectID(t)}
	ctx := context.Background()
	ch, err := srv.Chars.Get(ctx, d.pkID)
	if err != nil {
		t.Fatalf("load Pk: %v", err)
	}
	ch.KarmaPoints = 500
	if err := srv.Chars.Save(ctx, ch.SaveState()); err != nil {
		t.Fatalf("save Pk: %v", err)
	}
	if skillID != 0 {
		seedKnownSkill(t, srv, d.pkID, skillID, 1)
	}
	d.victID = seedPlayer(t, srv, "blessed", "Blessed", 10, victimKarma)
	d.victim = srv.DialClient(t, "blessed", 1)
	startInWorld(t, d.victim)
	startInWorld(t, d.pk)
	victim, ok := srv.State.Player(d.victID)
	if !ok {
		t.Fatal("victim missing from world state")
	}
	landEffect(t, victim.(effectHolder), "ProtectionBlessing")
	drainUntilQuiet(t, d.victim)
	drainUntilQuiet(t, d.pk)
	return d
}

func (d blessedDuel) victimHPCP(t *testing.T) int {
	t.Helper()
	return d.srv.PlayerCurrentHP(t, d.victID) + d.srv.PlayerCurrentCP(t, d.victID)
}

// TestQueuedAttackOnBlessedPlayerDoesNotLand has the karma player swing at a
// monster and, mid-swing, click attack on a blessed player 20 levels below
// it. The click is queued behind the swing, past the new-intention gate.
// When the swing ends and the queued attack runs, outside a PvP zone the
// blessed player cannot be attacked: no swing starts and no damage lands.
// Inside an arena the queued attack swings and lands.
func TestQueuedAttackOnBlessedPlayerDoesNotLand(t *testing.T) {
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
			// The victim's own karma makes a plain click an attack.
			d := bootBlessedDuel(t, tt.arena, 100, 0)
			if !d.srv.DrivesClock() {
				t.Skip("holding a swing open needs the driven clock")
			}
			hostile := d.srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, d.pk)

			targetHostile(t, d.pk, hostile.ObjectID())
			d.pk.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
			assertAutoAttackStart(t, d.pk, d.pkID)
			assertAttackBy(t, d.pk, d.pkID)

			full := d.victimHPCP(t)
			selectPlayerTarget(t, d.pk, d.victID)
			d.pk.Send(encodeAction(d.victID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
			for _, f := range readQuiet(d.pk) {
				if f[0] == serverpackets.OpcodeSystemMessage && wireReader(f[1:]).ReadInt32() == int32(serverpackets.SystemMessageTargetIncorrect) {
					t.Fatal("mid-swing click was refused up front; it must be queued behind the swing")
				}
			}

			if tt.arena {
				d.srv.AdvanceUntil(t, "the queued attack landing on the blessed player", func() bool {
					return d.victimHPCP(t) < full
				})
				return
			}
			assertNoAttackBy(t, d.srv, d.pk, d.pkID, 3*time.Second, "queued attack on the blessed player")
			if got := d.victimHPCP(t); got != full {
				t.Fatalf("blessed player HP+CP = %d after the queued attack, want untouched %d", got, full)
			}
		})
	}
}

// TestForcedOffensiveCastOnBlessedPlayer has the karma player cast an
// offensive skill with ctrl at a blessed player 20 levels below it. Outside
// a PvP zone the cast is refused as an invalid target; inside an arena it
// starts.
func TestForcedOffensiveCastOnBlessedPlayer(t *testing.T) {
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
			d := bootBlessedDuel(t, tt.arena, 0, 42, gameservertest.WithSkills(combatPersistence(t, offensiveKillSkillDefs())))
			selectPlayerTarget(t, d.pk, d.victID)
			drainUntilQuiet(t, d.pk)

			if tt.arena {
				castKillSkill(t, d.srv, d.pk, d.pkID, d.victID, true)
				return
			}
			full := d.victimHPCP(t)
			d.pk.Send(encodeRequestMagicSkillUse(42, true, false))
			frames := readQuiet(d.pk)
			if frameIndex(frames, serverpackets.OpcodeSystemMessage, int32(serverpackets.SystemMessageInvalidTarget)) < 0 || frameIndex(frames, serverpackets.OpcodeMagicSkillUse, 0) >= 0 {
				t.Fatalf("forced cast on a blessed player = %v, want INVALID_TARGET and no cast", opcodes(frames))
			}
			if got := d.victimHPCP(t); got != full {
				t.Fatalf("blessed player HP+CP = %d after the refused cast, want untouched %d", got, full)
			}
		})
	}
}
