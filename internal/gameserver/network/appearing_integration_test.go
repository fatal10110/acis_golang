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

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
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

	c.send(encodeSingleOpcode(clientpackets.OpcodeAppearing))

	reply := c.read()
	if reply[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("Appearing opcode = %#x, want UserInfo (%#x)", reply[0], serverpackets.OpcodeUserInfo)
	}
	if live.Teleporting() {
		t.Fatal("Teleporting() = true after Appearing")
	}
}
