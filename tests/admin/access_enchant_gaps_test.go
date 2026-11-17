package admin

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// undefinedLevel is an access level the shipped table does not define.
const undefinedLevel = 77

// TestAccessLevelUndefinedAtLogin: a character whose stored access level the
// table does not define (AdminData.getAccessLevel finds none and
// Player.setAccessLevel resets it to the user level) plays as a user: from
// CharSelected on the title is the user level's name, the name and title
// take the user colors, it is no game master and is on no GM list. The
// level above 0 still sets the title. The row keeps the stored level until
// the character's first save, which writes the user level and its title
// (storeCharBase writes the resolved getAccessLevel().getLevel()).
func TestAccessLevelUndefinedAtLogin(t *testing.T) {
	t.Parallel()
	srv, objID := bootAdmin(t, undefinedLevel)
	c := srv.Client
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(0)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	c.Send(w.Bytes())
	var selected []byte
	for range 10 {
		if f := c.Read(); f[0] == serverpackets.OpcodeCharSelected {
			selected = f
			break
		}
	}
	if selected == nil {
		t.Fatal("no CharSelected")
	}
	r := wire.NewReader(selected[1:])
	r.ReadString()
	r.ReadInt32()
	if title := r.ReadString(); title != "User" {
		t.Fatalf("CharSelected title = %q, want the user level's name", title)
	}

	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	var got *userInfoAccess
	for _, f := range c.ReadQueued() {
		if f[0] == serverpackets.OpcodeUserInfo && got == nil {
			info := decodeUserInfoAccess(t, f)
			got = &info
		}
	}
	if got == nil {
		t.Fatal("no UserInfo at EnterWorld")
	}
	if want := (userInfoAccess{title: "User", nameColor: userColor, titleColor: userTitleColor}); *got != want {
		t.Fatalf("EnterWorld UserInfo = %+v, want %+v", *got, want)
	}
	assertNoGM(t, exchange(t, c, encodeGmList()))

	if got := storedNow(t, srv, objID, "accesslevel"); got != strconv.Itoa(undefinedLevel) {
		t.Fatalf("stored access level before the first save = %s, want %d", got, undefinedLevel)
	}
	srv.TickAutosave(t)
	if got := storedNow(t, srv, objID, "accesslevel"); got != "0" {
		t.Fatalf("stored access level after the first save = %s, want the user level 0", got)
	}
	if got := storedNow(t, srv, objID, "title"); got != "User" {
		t.Fatalf("stored title after the first save = %q, want User", got)
	}
}

// TestAdminSetAccessRowWaitsForSave: //set access on an online player
// changes memory only (Player.setAccessLevel; storeCharBase writes
// accesslevel and title), so the player's row keeps its level and title
// until the next save of that character, which then stores both.
func TestAdminSetAccessRowWaitsForSave(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	titleBefore := storedNow(t, srv, userID, "title")

	sent := exchange(t, gm, encodeBuildCmd("set access Player 3"))
	_, rest := split(append(sent, settle(t, gm)...))
	assertTexts(t, rest, "Player's access level is now set to 3.")
	settle(t, user)
	if got := storedNow(t, srv, userID, "accesslevel"); got != "0" {
		t.Fatalf("stored access level after the command = %s, want 0 until the next save", got)
	}
	if got := storedNow(t, srv, userID, "title"); got != titleBefore {
		t.Fatalf("stored title after the command = %q, want %q until the next save", got, titleBefore)
	}

	srv.TickAutosave(t)
	if got := storedNow(t, srv, userID, "accesslevel"); got != "3" {
		t.Fatalf("stored access level after the save = %s, want 3", got)
	}
	if got := storedNow(t, srv, userID, "title"); got != "General GM" {
		t.Fatalf("stored title after the save = %q, want General GM", got)
	}
}

// TestAdminEnchantSelectedPlayerInventoryUpdate: //enchant on a selected
// player queues the changed sword's InventoryUpdate on the enchanted
// player's inventory, so the player's client gets it with the next
// inventory tick and the GM's gets none. This differs from the reference,
// which queues it on the GM's inventory (setEnchantLevel(enchant, player)
// with the GM as actor); the recipient is the owner decision of #5042.
func TestAdminEnchantSelectedPlayerInventoryUpdate(t *testing.T) {
	t.Parallel()
	srv, player, _, playerID, _, playerSword := bootEnchantRig(t)
	gm := srv.Client
	exchange(t, gm, encodeAction(playerID))
	drain(t, player)
	drain(t, gm)

	gm.Send(encodeBuildCmd("enchant rhand 7"))
	if got := readText(t, gm); got != "Player's Sword enchant was modified from 0 to 7." {
		t.Fatalf("GM report = %q", got)
	}
	srv.InventoryUpdates.Tick()

	updates := func(c *testsupport.ScriptedClient) [][]byte {
		t.Helper()
		var out [][]byte
		for _, f := range settle(t, c) {
			if f[0] == serverpackets.OpcodeInventoryUpdate {
				out = append(out, f)
			}
		}
		return out
	}
	forPlayer := updates(player)
	if len(forPlayer) != 1 {
		t.Fatalf("enchanted player's InventoryUpdates = %d, want 1", len(forPlayer))
	}
	// One entry, for the sword.
	if n := binary.LittleEndian.Uint16(forPlayer[0][1:]); n != 1 {
		t.Fatalf("InventoryUpdate holds %d entries, want 1", n)
	}
	if !bytes.Contains(forPlayer[0], binary.LittleEndian.AppendUint32(nil, uint32(playerSword))) {
		t.Fatalf("InventoryUpdate % x does not name the sword %d", forPlayer[0], playerSword)
	}
	if got := updates(gm); len(got) != 0 {
		t.Fatalf("GM's InventoryUpdates = %d, want none", len(got))
	}
	if len(updates(player)) != 0 {
		t.Fatal("a second inventory update reached the player")
	}
}
