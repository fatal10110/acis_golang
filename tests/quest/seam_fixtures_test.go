package quest

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// bootSeamQuest boots a character of level with pages, runs before (seeds
// that must precede the selection) and enters the world.
func bootSeamQuest(t *testing.T, level int, pages map[string]string, before func(srv *gameservertest.Server, objID int32), opts ...gameservertest.Option) *dialogWorld {
	t.Helper()
	all := map[string]string{"test/any.htm": anyBypassPage}
	for k, v := range pages {
		all[k] = v
	}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Seeker", level, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithHTMLPages(all),
	}, opts...)...)
	objID := srv.SoleObjectID(t)
	if before != nil {
		before(srv, objID)
	}
	enterWorld(t, srv)
	x, y, z := srv.PlayerPosition(t, objID)
	return &dialogWorld{srv: srv, player: objID, at: location.Location{X: x, Y: y, Z: z}, roles: map[int32]string{}, npcs: map[string]int32{}}
}

// questPages stands in for a quest's pages: each names its file and carries
// the links listed for it.
func questPages(quest string, links map[string][]string) map[string]string {
	pages := map[string]string{}
	for file, events := range links {
		html := "<html><body>" + file
		for _, ev := range events {
			html += `<a action="bypass -h Quest ` + quest + ` ` + ev + `">go</a>`
		}
		pages["script/quest/"+quest+"/"+file] = html + "</body></html>"
	}
	return pages
}

// chatPages gives each NPC a chat window linking to its quests.
func chatPages(pages map[string]string, ids ...int) map[string]string {
	for _, id := range ids {
		pages[fmt.Sprintf("default/%d.htm", id)] = `<html><body>chat <a action="bypass -h npc_%objectId%_Quest">Quest</a></body></html>`
	}
	return pages
}

// questWindow talks to the NPC obj and opens its quest window.
func (w *dialogWorld) questWindow(t *testing.T, obj int32) []string {
	t.Helper()
	w.interact(t, obj)
	return w.bypass(t, fmt.Sprintf("npc_%d_Quest", obj))
}

// shows reports whether lines show the page file.
func shows(lines []string, file string) bool {
	return slices.ContainsFunc(lines, func(l string) bool {
		return strings.HasPrefix(l, "S NpcHtmlMessage ") && strings.Contains(l, "<html><body>"+file)
	})
}

// scriptLines renders the system messages and sounds among frames, as the
// goldens write packets.
func scriptLines(t *testing.T, frames [][]byte) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage && f[0] != serverpackets.OpcodePlaySound {
			continue
		}
		line, err := scriptcontract.Packet(f, nil)
		if err != nil {
			t.Fatalf("frame %x: %v", f, err)
		}
		out = append(out, line)
	}
	return out
}

// itemMessage is the rendered system message id naming the item itemID.
func itemMessage(id int, itemID int32) string {
	return "S SystemMessage id=" + strconv.Itoa(id) + " 3:" + strconv.Itoa(int(itemID))
}

// onNPCQueue runs fn on the hostile NPC h's queue and waits for it.
func onNPCQueue(t *testing.T, h *npc.Hostile, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !h.Queue().Post(func() {
		defer close(done)
		fn()
	}) {
		t.Fatal("npc queue is closed")
	}
	<-done
}

// playerCombatant returns the online player objID as the world tracks it.
func playerCombatant(t *testing.T, srv *gameservertest.Server, objID int32) attackable.Combatant {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("player %d is not in the world", objID)
	}
	c, ok := obj.(attackable.Combatant)
	if !ok {
		t.Fatalf("player %d (%T) is not a combatant", objID, obj)
	}
	return c
}

func encodeMoveBackwardToLocation(target, origin location.Location) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	for _, v := range []int{target.X, target.Y, target.Z, origin.X, origin.Y, origin.Z} {
		w.WriteInt32(int32(v))
	}
	w.WriteInt32(1)
	return w.Bytes()
}

// walkTo walks the player from where it stands to target and waits until
// it arrives, returning every frame sent meanwhile.
func (w *dialogWorld) walkTo(t *testing.T, target location.Location) [][]byte {
	t.Helper()
	w.srv.Client.Send(encodeMoveBackwardToLocation(target, w.at))
	w.srv.AdvanceUntil(t, fmt.Sprintf("walk arrival at %+v", target), func() bool {
		x, y, z := w.srv.PlayerPosition(t, w.player)
		return x == target.X && y == target.Y && z == target.Z
	})
	w.at = target
	return w.srv.ReadQueued(t, w.srv.Client)
}
