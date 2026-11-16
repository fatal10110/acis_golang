package admin

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestJailEndsObservation pins the owner-approved jail of a player watching
// from a viewpoint (#4845): the watch ends where the player stands, its
// client told so before the jump to the jail, and the player lands there
// visible, hurtable and free to act, with no position left to return to,
// so a later ObserverReturn does not move it out of the jail.
func TestJailEndsObservation(t *testing.T) {
	t.Parallel()
	groups, err := gamexml.LoadObserverGroups(datapack.Path(t, "data", "xml", "observerGroups.xml"))
	if err != nil {
		t.Fatalf("load observer groups: %v", err)
	}
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithHTMLPages(punishPages(t)),
		gameservertest.WithZones(shippedJailZones(t)),
		gameservertest.WithObserverGroups(groups))
	gm := srv.Client
	enterWorld(t, gm)
	// The seat's fee is paid from adena held at login.
	userID := srv.SeedCharacterFor(t, "player2", "Player", 1, 0).ID
	srv.GiveItem(t, userID, item.AdenaID, 1000)
	user := srv.DialClient(t, "player2", 1)
	enterWorld(t, user)
	x, y, z := srv.PlayerPosition(t, userID)
	// The shipped broadcasting tower, offering the colosseum's seats.
	tower := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 31031), location.Location{X: x + 50, Y: y, Z: z})
	tower.SetObserverGroups([]int{619})
	drain(t, gm)
	settle(t, user)
	srv.ShowDialogPage(t, userID, `<html><body><edit var="c"><a action="bypass -h npc_$c">Go</a></body></html>`)
	settle(t, user)

	teleportOf(t, exchange(t, user, encodeBypass("npc_"+strconv.Itoa(int(tower.ObjectID()))+"_observe 634")), userID)
	appear(t, user)
	c := onlineCharacter(t, srv, userID)
	if !c.ObserverMode() || !c.Invisible() || !c.Paralyzed() {
		t.Fatalf("before the jail: observer=%v invisible=%v paralyzed=%v, want watching", c.ObserverMode(), c.Invisible(), c.Paralyzed())
	}

	assertTexts(t, messages(exchange(t, gm, encodeBuildCmd("jail Player 30"))), "Player is jailed for 30 minutes.")
	frames := settle(t, user)
	if end, jump := index(frames, serverpackets.OpcodeObserverEnd), index(frames, serverpackets.OpcodeTeleportToLocation); end < 0 || jump < end {
		t.Fatalf("jail frames = %x, want ObserverEnd before the teleport", testsupport.FrameOpcodes(frames))
	}
	if at := teleportOf(t, frames, userID); at != jailAt {
		t.Fatalf("jail teleport = %v, want %v", at, jailAt)
	}
	appear(t, user)
	if _, saved := c.SavedLocation(); c.ObserverMode() || c.Invisible() || c.Invul() || c.Paralyzed() || saved {
		t.Fatalf("jailed: observer=%v invisible=%v invul=%v paralyzed=%v saved=%v, want a plain player",
			c.ObserverMode(), c.Invisible(), c.Invul(), c.Paralyzed(), saved)
	}

	noTeleport(t, exchange(t, user, wire.NewPacketWriter(clientpackets.OpcodeObserverReturn).Bytes()))
	if x, y, _ := srv.PlayerPosition(t, userID); !near([3]int32{int32(x), int32(y), jailAt[2]}, jailAt, 1) {
		t.Fatalf("after ObserverReturn Player stands at %d %d, want in the jail at %v", x, y, jailAt)
	}
	if timer := waitPunishment(t, srv, userID, punishJail); timer != 30*60000 {
		t.Fatalf("stored timer = %d, want 1800000", timer)
	}
}
