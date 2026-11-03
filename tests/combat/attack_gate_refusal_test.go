package combat

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// gateAttackerAt is where the attacking player stands: 30 units east of the
// victim at playerOrigin, inside its melee reach.
var gateAttackerAt = location.Location{X: playerOrigin.X + 30, Y: playerOrigin.Y, Z: playerOrigin.Z}

// peaceZones is a zone index holding one peace zone spanning x in
// [minX, maxX] (every y and z of the test spawns).
func peaceZones(t *testing.T, minX, maxX int) *zone.Index {
	t.Helper()
	form, err := zone.NewCuboid(minX, maxX, -100_000, 100_000, -10_000, 10_000)
	if err != nil {
		t.Fatalf("peace form: %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewPeace(1, form))
	return zones
}

// refusedThinkMessages sends the attack request and reads up to the think's
// ActionFailed, returning every system message id read before it. A swing
// fails the test. It then reads until quiet: one refused think tells its
// player once and never swings.
func refusedThinkMessages(t *testing.T, c *scriptedClient, targetID int32) []int32 {
	t.Helper()
	c.Send(encodeAttackRequest(targetID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	var messages []int32
	for {
		frame := mustRead(t, c, "refused attack ActionFailed")
		if frame[0] == serverpackets.OpcodeAttack {
			t.Fatal("refused attack swung")
		}
		if frame[0] == serverpackets.OpcodeSystemMessage {
			r := wire.NewReader(frame[1:])
			id := r.ReadInt32()
			if params := r.ReadInt32(); params != 0 {
				t.Fatalf("SystemMessage %d carries %d parameters, want none", id, params)
			}
			messages = append(messages, id)
			continue
		}
		if frame[0] == serverpackets.OpcodeActionFailed {
			break
		}
	}
	for _, frame := range readQuiet(c) {
		switch frame[0] {
		case serverpackets.OpcodeAttack:
			t.Fatal("refused attack swung after ActionFailed")
		case serverpackets.OpcodeSystemMessage:
			t.Fatalf("SystemMessage %d after the refused think, want one per think", wire.NewReader(frame[1:]).ReadInt32())
		}
	}
	return messages
}

// TestForcedAttackOnPlayerRefusedByPeaceZoneOrSight force-attacks a player
// in reach (a second click on the selected player). The attack think's gate
// answers CANT_SEE_TARGET (181) when the geodata hides the target, else
// CANT_ATK_PEACEZONE (84) when the attacker stands in a peace zone, else
// TARGET_IN_PEACEZONE (85) when the target does, each once and ahead of the
// think's ActionFailed; the weapon gate runs only after them. No swing
// starts and the victim keeps its HP. With no peace zone and sight, the same
// request swings and its hit lands.
func TestForcedAttackOnPlayerRefusedByPeaceZoneOrSight(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		minX, maxX int // the peace zone's x span; equal for none
		blind      bool
		fishingRod bool
		wantMsg    int // zero: the attack swings
	}{
		{name: "no peace zone swings", minX: 0, maxX: 0},
		{name: "target in peace zone", minX: -1_000, maxX: playerOrigin.X + 10, wantMsg: serverpackets.SystemMessageTargetInPeacezone},
		{name: "attacker in peace zone", minX: gateAttackerAt.X - 10, maxX: 1_000, wantMsg: serverpackets.SystemMessageCantAtkPeacezone},
		{name: "both in peace zone", minX: -1_000, maxX: 1_000, wantMsg: serverpackets.SystemMessageCantAtkPeacezone},
		{name: "sight before peace zone", minX: -1_000, maxX: 1_000, blind: true, wantMsg: serverpackets.SystemMessageCantSeeTarget},
		{name: "peace zone before weapon", minX: -1_000, maxX: playerOrigin.X + 10, fishingRod: true, wantMsg: serverpackets.SystemMessageTargetInPeacezone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := []gameservertest.Option{
				gameservertest.WithCharacter("Victim", 20, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithItemTemplates(weaponRefusalCatalog()),
			}
			if tc.minX != tc.maxX {
				opts = append(opts, gameservertest.WithZones(peaceZones(t, tc.minX, tc.maxX)))
			}
			if tc.blind {
				opts = append(opts, gameservertest.WithGeo(sightGeo{see: false}))
			}
			srv := gameservertest.Boot(t, opts...)
			c, victimID := srv.Client, srv.SoleObjectID(t)
			attackerID := srv.SeedCharacterFor(t, "attacker", "Attacker", 20, 0).ID
			if err := srv.Chars.SetPosition(context.Background(), attackerID, gateAttackerAt, 0); err != nil {
				t.Fatalf("place attacker: %v", err)
			}
			var rod int32
			if tc.fishingRod {
				rod = srv.GiveItem(t, attackerID, babyDuckRodID, 1)
			}
			pc := srv.DialClient(t, "attacker", 1)
			startInWorld(t, c)
			startInWorld(t, pc)
			if tc.fishingRod {
				equipAndFlush(t, srv, pc, rod)
			}
			if got, _, _ := srv.PlayerPosition(t, attackerID); got != gateAttackerAt.X {
				t.Fatalf("attacker x = %d, want %d", got, gateAttackerAt.X)
			}
			drainUntilQuiet(t, c)
			drainUntilQuiet(t, pc)

			selectPlayerTarget(t, pc, victimID)
			drainUntilQuiet(t, pc)
			full := srv.PlayerCurrentHP(t, victimID) + srv.PlayerCurrentCP(t, victimID)

			if tc.wantMsg == 0 {
				// A forced attack on a player without karma swings once and
				// does not re-swing, so a random miss would leave the victim
				// untouched: pin the attacker's rolls to land.
				attacker := intakePlayerOf(t, srv, attackerID)
				onQueue(t, srv.PlayerQueue(t, attackerID), func() { attacker.SetRollSource(landNoCrit()) })
				pc.Send(encodeAttackRequest(victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
				srv.AdvanceUntil(t, "the attacker's swing landing", func() bool {
					return srv.PlayerCurrentHP(t, victimID)+srv.PlayerCurrentCP(t, victimID) < full
				})
				return
			}

			messages := refusedThinkMessages(t, pc, victimID)
			if len(messages) != 1 || messages[0] != int32(tc.wantMsg) {
				t.Fatalf("SystemMessages before ActionFailed = %v, want exactly [%d]", messages, tc.wantMsg)
			}
			if got := srv.PlayerCurrentHP(t, victimID) + srv.PlayerCurrentCP(t, victimID); got != full {
				t.Fatalf("victim HP+CP = %d after the refusal, want untouched %d", got, full)
			}
		})
	}
}

// TestAttackOnHiddenMonsterSendsCantSeeTarget attacks a monster in reach
// that the geodata hides: the attack think answers CANT_SEE_TARGET (181)
// once, ahead of its ActionFailed, and never swings.
func TestAttackOnHiddenMonsterSendsCantSeeTarget(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithGeo(sightGeo{see: false}),
	)
	c := srv.Client
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	c.Send(encodeAttackRequest(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, c, "select ValidateLocation"), serverpackets.OpcodeValidateLocation, "select ValidateLocation")
	assertFrameOpcode(t, mustRead(t, c, "MyTargetSelected"), serverpackets.OpcodeMyTargetSelected, "MyTargetSelected")
	drainUntilQuiet(t, c)

	messages := refusedThinkMessages(t, c, hostile.ObjectID())
	if len(messages) != 1 || messages[0] != serverpackets.SystemMessageCantSeeTarget {
		t.Fatalf("SystemMessages before ActionFailed = %v, want exactly [%d]", messages, serverpackets.SystemMessageCantSeeTarget)
	}
}
