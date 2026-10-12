package npcs

import (
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Npc.onBypassFeedback (Npc.java:1135-1205) answers
// TerritoryStatus, Chat <n> (showChatWindow(player, val), which sends the
// page then ActionFailed) and Link <path> for every NPC, a guard's and a
// friendly monster's included; RequestBypassToServer then releases the
// client once more, unless the handler threw (Link with no path).
// Guard.getHtmlPath (Guard.java:33-41) reads data/html/guard/<id>[-<val>].htm
// with no existence check; a friendly monster reads Npc.getHtmlPath
// (Npc.java:1569-1582): data/html/default/<id>[-<val>].htm when it exists,
// else data/html/npcdefault.htm.

const (
	dialogGuardID    = 30039
	dialogFriendlyID = 30995
)

// hostileDialogPages are a guard's and a friendly monster's pages.
func hostileDialogPages() map[string]string {
	pages := dialogPages()
	pages["guard/30039-1.htm"] = `<html><body>Guard one %objectId%</body></html>`
	pages["guard/info.htm"] = `<html><body>Guard info %objectId%</body></html>`
	pages["default/30995-1.htm"] = `<html><body>Friend one %objectId%</body></html>`
	pages["territorystatus.htm"] = statusPage
	pages["territorynoclan.htm"] = noClanPage
	return pages
}

// guardCastles is Gludio listing the guard among its NPCs.
func guardCastles(t *testing.T) *castledata.Table {
	t.Helper()
	gludio, err := castledata.NewCastle(castledata.CastleAttrs{
		ID: taxCastleID, Alias: "gludio_castle", Name: "Gludio Castle", CircletID: 6838,
		Tax: residence.Tax{Rate: taxCastleTax}, NPCs: []int{dialogGuardID},
	}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	table, err := castledata.NewTable([]*castledata.Castle{gludio})
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func (w *folkWorld) spawnTalkingHostile(t *testing.T, kind string, npcID, dx int) *npc.Hostile {
	t.Helper()
	tmpl := &npc.Template{
		ID: npcID, TemplateID: npcID, Type: kind, Level: 20, HPMax: 1000, AtkSpd: 300,
		RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
	h := w.srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: w.at.X + dx, Y: w.at.Y, Z: w.at.Z})
	drainUntilQuiet(t, w.c)
	return h
}

// hostileCommand sends the dialog command to h from the page admitting
// every command and returns the answer.
func (w *folkWorld) hostileCommand(t *testing.T, h *npc.Hostile, command string) [][]byte {
	t.Helper()
	w.openAnyNpcPage(t)
	return w.bypass(t, "npc_"+strconv.Itoa(int(h.ObjectID()))+"_"+command)
}

// assertHostileAnswer checks frames are exactly want and that their page,
// if any, is html as the client receives it, with %objectId% naming h: a
// cached page ends with a newline, the missing-page notice does not.
func assertHostileAnswer(t *testing.T, what string, frames [][]byte, want []byte, h *npc.Hostile, html string) {
	t.Helper()
	if got := opcodes(frames); string(got) != string(want) {
		t.Fatalf("%s: answer = %x, want %x", what, got, want)
	}
	frame, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		return
	}
	html = strings.ReplaceAll(html, "%objectId%", strconv.Itoa(int(h.ObjectID())))
	if !strings.Contains(html, "My html is missing") {
		html += "\n"
	}
	if objectID, got, _ := htmlMessage(t, frame); objectID != h.ObjectID() || got != html {
		t.Fatalf("%s: page = %d %q, want %d %q", what, objectID, got, h.ObjectID(), html)
	}
}

// TestHostileDialogChatAndLink pins Chat and Link at a guard and at a
// friendly monster: a chat page opens with the chat window's ActionFailed
// and the dispatcher's; a guard's missing page reads as the missing-page
// notice, a friendly monster's falls back to the generic page; Link opens
// its page with the dispatcher's ActionFailed alone, a path climbing out
// with ".." only releases the client, and Link with no path answers
// nothing.
func TestHostileDialogChatAndLink(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, hostileDialogPages(), noBypassReuse)
	guard := w.spawnTalkingHostile(t, "Guard", dialogGuardID, 30)
	friend := w.spawnTalkingHostile(t, "FriendlyMonster", dialogFriendlyID, 50)
	pageTwice := []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}
	pageOnce := []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}

	assertHostileAnswer(t, "guard Chat 1", w.hostileCommand(t, guard, "Chat 1"), pageTwice, guard, `<html><body>Guard one %objectId%</body></html>`)
	assertHostileAnswer(t, "guard Chat 2", w.hostileCommand(t, guard, "Chat 2"), pageTwice, guard,
		"<html><body>My html is missing:<br>data/html/guard/30039-2.htm</body></html>")
	assertHostileAnswer(t, "friend Chat 1", w.hostileCommand(t, friend, "Chat 1"), pageTwice, friend, `<html><body>Friend one %objectId%</body></html>`)
	assertHostileAnswer(t, "friend Chat 2", w.hostileCommand(t, friend, "Chat 2"), pageTwice, friend, "<html><body>Nothing to say %objectId%</body></html>")
	assertHostileAnswer(t, "guard Link", w.hostileCommand(t, guard, "Link guard/info.htm "), pageOnce, guard, `<html><body>Guard info %objectId%</body></html>`)
	assertHostileAnswer(t, "guard Link ..", w.hostileCommand(t, guard, "Link ../guard/info.htm"), releaseOnly, guard, "")
	assertHostileAnswer(t, "guard bare Link", w.hostileCommand(t, guard, "Link"), nil, guard, "")
}

// TestHostileDialogTerritoryStatus pins TerritoryStatus at a guard of an
// owned castle, matched in any case: the territory page of its castle,
// then the dispatcher's ActionFailed. A friendly monster of no castle only
// releases the client.
func TestHostileDialogTerritoryStatus(t *testing.T) {
	t.Parallel()
	opts := append(withTaxCastle(t, "Talker", true), noBypassReuse)
	opts[0] = gameservertest.WithCastles(guardCastles(t))
	w := bootFolkWorld(t, hostileDialogPages(), opts...)
	taxCastle(t, w.srv, true)
	guard := w.spawnTalkingHostile(t, "Guard", dialogGuardID, 30)
	friend := w.spawnTalkingHostile(t, "FriendlyMonster", dialogFriendlyID, 50)

	want := strings.NewReplacer("%clanLeaderName%", "Talker", "%clanName%", taxClanName, "%taxPercent%", "10",
		"%townName%", taxTownName, "%territory%", taxKingdom).Replace(statusPage)
	assertHostileAnswer(t, "guard", w.hostileCommand(t, guard, "territorystatus"),
		[]byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}, guard, want)
	assertHostileAnswer(t, "friend", w.hostileCommand(t, friend, "TerritoryStatus"), releaseOnly, friend, "")
}
