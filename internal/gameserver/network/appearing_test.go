package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestGameClientLinkAppearingSendsUserInfo(t *testing.T) {
	c, chars, _, state := newLinkedGameClientWithSkillsSeed(t, nil, func(chars *fakeCharStore, _ *fakeItemStore) {
		seedSelectableCharacter(t, chars, "player1", "Newbie", 1, 0)
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)
	obj, ok := state.Player(chars.soleObjectID(t))
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
