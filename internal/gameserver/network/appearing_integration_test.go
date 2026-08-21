//go:build integration

package network

import (
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestGameClientLinkAppearingSendsUserInfo(t *testing.T) {
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, nil, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 1, 0)
	}, 1)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)
	obj, ok := state.Player(sqlSoleObjectID(t, chars))
	if !ok {
		t.Fatal("entered player missing from world")
	}
	live, ok := obj.(*livePlayer)
	if !ok {
		t.Fatalf("world player type = %T, want *livePlayer", obj)
	}
	live.SetTeleporting(true)

	c.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))

	reply := c.Read()
	if reply[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("Appearing opcode = %#x, want UserInfo (%#x)", reply[0], serverpackets.OpcodeUserInfo)
	}
	if live.Teleporting() {
		t.Fatal("Teleporting() = true after Appearing")
	}
}

// TestGameClientLinkAppearingUnflaggedStillSendsUserInfo pins issue #1634:
// a live client can send opcode 0x30 outside the restart-point teleport
// window (Teleporting() already false), and Java only ever returns UserInfo
// for that case (Appearing.java:17-24). Go must keep answering UserInfo
// without treating the packet as a teleport completion.
func TestGameClientLinkAppearingUnflaggedStillSendsUserInfo(t *testing.T) {
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, nil, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 1, 0)
	}, 1)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)
	obj, ok := state.Player(sqlSoleObjectID(t, chars))
	if !ok {
		t.Fatal("entered player missing from world")
	}
	live, ok := obj.(*livePlayer)
	if !ok {
		t.Fatalf("world player type = %T, want *livePlayer", obj)
	}
	if live.Teleporting() {
		t.Fatal("Teleporting() = true before Appearing, want false")
	}

	c.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))

	reply := c.Read()
	if reply[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("Appearing opcode = %#x, want UserInfo (%#x)", reply[0], serverpackets.OpcodeUserInfo)
	}
	if live.Teleporting() {
		t.Fatal("Teleporting() = true after an unflagged Appearing")
	}
}
