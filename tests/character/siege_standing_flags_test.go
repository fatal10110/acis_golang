package character

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// msgEnteredCombatZone is SystemMessageId.ENTERED_COMBAT_ZONE.
const msgEnteredCombatZone = 283

// standingBattlefield is a zone index holding one battlefield, not under
// siege, around the test spawn.
func standingBattlefield(t *testing.T) (*zone.Index, *zone.Siege) {
	t.Helper()
	form, err := zone.NewCuboid(-100, 100, -100, 100, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	siege := zone.NewSiege(1, form)
	zones := zone.NewIndex()
	zones.Add(siege)
	return zones, siege
}

// assertBattlefieldState fails unless the character's pvp, siege and
// no-summon-friend zone state all read inside.
func assertBattlefieldState(t *testing.T, character *player.Character, inside bool, when string) {
	t.Helper()
	if character.InPvPZone() != inside || character.InSiegeZone() != inside || character.NoSummonFriendZone() != inside {
		t.Fatalf("%s: pvp %v, siege %v, no summon friend %v; want all %v", when,
			character.InPvPZone(), character.InSiegeZone(), character.NoSummonFriendZone(), inside)
	}
}

// combatNotices returns the combat zone system messages among frames, and
// fails on a compass zone update: the switch leaves the compass to the
// player's next zone revalidation.
func combatNotices(t *testing.T, frames [][]byte) []uint32 {
	t.Helper()
	var ids []uint32
	for _, f := range frames {
		if len(f) >= 3 && bytes.Equal(f[:3], []byte{0xfe, 0x32, 0}) {
			t.Fatal("the siege switch sent a compass zone update")
		}
		if len(f) >= 5 && f[0] == serverpackets.OpcodeSystemMessage {
			if id := binary.LittleEndian.Uint32(f[1:5]); id == msgEnteredCombatZone || id == msgLeftCombatZone {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// TestSiegeSwitchReachesStandingPlayer: a siege starting around a player
// standing on its battlefield puts the player in the pvp and siege zones
// and bars summoning it, telling it it entered a combat zone; the siege
// ending lifts all three while the player still stands there, telling it
// it left, and flags no one. Neither switch sends a compass update.
func TestSiegeSwitchReachesStandingPlayer(t *testing.T) {
	t.Parallel()
	zones, siege := standingBattlefield(t)
	srv, character, _ := bootInZones(t, zones)
	assertBattlefieldState(t, character, false, "before the siege")

	siege.SetActive(true)
	assertBattlefieldState(t, character, true, "siege started")
	if got := combatNotices(t, readUntilQuiet(srv.Client)); len(got) != 1 || got[0] != msgEnteredCombatZone {
		t.Fatalf("siege start notices = %v, want [%d]", got, msgEnteredCombatZone)
	}

	siege.SetActive(false)
	assertBattlefieldState(t, character, false, "siege ended")
	if got := combatNotices(t, readUntilQuiet(srv.Client)); len(got) != 1 || got[0] != msgLeftCombatZone {
		t.Fatalf("siege end notices = %v, want [%d]", got, msgLeftCombatZone)
	}
	if character.PvPFlagState() != 0 {
		t.Fatal("the siege ending flagged the player")
	}
}

// TestSiegeStartWhileWalkingOffBattlefield: a siege starting while a player
// walks off its battlefield leaves no battlefield state on the player
// once it stands outside, however the start and the exit interleave;
// walking back on raises it once, and the siege ending lifts it.
func TestSiegeStartWhileWalkingOffBattlefield(t *testing.T) {
	t.Parallel()
	zones, siege := standingBattlefield(t)
	srv, character, objID := bootInZones(t, zones)
	spawn := location.Location{X: 10, Y: 20, Z: 30}
	outside := location.Location{X: 300, Y: 20, Z: 30}

	srv.Client.Send(encodeMoveBackwardToLocation(outside, spawn, 1))
	siege.SetActive(true)
	waitForWorldPosition(t, srv, objID, outside)
	srv.Settle(t)
	readUntilQuiet(srv.Client)
	assertBattlefieldState(t, character, false, "walked off as the siege started")

	srv.Client.Send(encodeMoveBackwardToLocation(spawn, outside, 1))
	waitForWorldPosition(t, srv, objID, spawn)
	srv.Settle(t)
	readUntilQuiet(srv.Client)
	assertBattlefieldState(t, character, true, "walked back on")

	siege.SetActive(false)
	assertBattlefieldState(t, character, false, "siege ended")
}
