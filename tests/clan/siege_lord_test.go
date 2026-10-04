package clan

import (
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// bootGludioLordWorld boots the shipped castles with their sieges running:
// the founder leads Knights, the lord of Gludio Castle (id 1), and the
// rival leads Rivals, both level 5 and in the world, plus extra
// statements.
func bootGludioLordWorld(t *testing.T, stmts ...string) *clanWorld {
	t.Helper()
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Founder", 10, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCastles(castles),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithHTMLPages(siegePages(t)),
		seedRival(t),
		seedAllianceClans(t, append([]string{
			`UPDATE clan_data SET hasCastle = 1 WHERE clan_id = ` + itoa(knightsClanID),
		}, stmts...)...),
	)
	w := &clanWorld{srv: srv, leader: srv.Client, leaderID: srv.SoleObjectID(t), memberID: rivalLeaderID}
	w.member = srv.DialClient(t, "player2", 1)
	startInWorld(t, w.leader)
	startInWorld(t, w.member)
	drainFrames(t, w.leader)
	x, y, z := srv.PlayerPosition(t, w.leaderID)
	w.at = location.Location{X: x, Y: y, Z: z}
	return w
}

// siegeDefenderSides decodes a SiegeDefenderList frame into its clan ids
// and their sides (1 owner, 2 pending, 3 approved), in order.
func siegeDefenderSides(t *testing.T, frame []byte) (clans, sides []int32) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSiegeDefenderList {
		t.Fatalf("opcode = %#x, want SiegeDefenderList", frame[0])
	}
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != 1 {
		t.Fatalf("defender list castle = %d, want 1", id)
	}
	r.ReadInt32()
	r.ReadInt32()
	r.ReadInt32()
	n := r.ReadInt32()
	r.ReadInt32()
	for range n {
		clans = append(clans, r.ReadInt32())
		r.ReadString()
		r.ReadString()
		r.ReadInt32()
		r.ReadInt32()
		sides = append(sides, r.ReadInt32())
		r.ReadInt32()
		r.ReadString()
		r.ReadString()
		r.ReadInt32()
	}
	if err := r.Err(); err != nil {
		t.Fatalf("decode defender list: %v", err)
	}
	return clans, sides
}

// TestSiegeLordConfirmsWaitingDefenders drives RequestConfirmSiegeWaitingList
// on Gludio Castle, held by Knights, with Rivals waiting to defend it: the
// rival, leading a clan that holds no castle, approving its own request is
// answered nothing and changes nothing; the lord's approval moves Rivals
// from pending (side 2) to defender (side 3), shown at once in the
// defender list and stored; the lord's refusal drops Rivals from the list
// and from storage.
func TestSiegeLordConfirmsWaitingDefenders(t *testing.T) {
	t.Parallel()
	w := bootGludioLordWorld(t)
	stored := func() (string, int64) {
		t.Helper()
		w.srv.FlushPersistence(t)
		n := queryInt(t, w, "SELECT COUNT(*) FROM siege_clans WHERE castle_id = 1 AND clan_id = ?", rivalsClanID)
		if n == 0 {
			return "", 0
		}
		var side string
		if err := w.srv.DB.QueryRowContext(t.Context(), "SELECT type FROM siege_clans WHERE castle_id = 1 AND clan_id = ?", rivalsClanID).Scan(&side); err != nil {
			t.Fatal(err)
		}
		return side, n
	}
	confirm := func(c interface{ Send([]byte) }, approved int32) {
		c.Send(encodeSiegeRequest(clientpackets.OpcodeRequestConfirmSiegeWaitingList, 1, rivalsClanID, approved))
	}

	w.member.Send(encodeSiegeRequest(clientpackets.OpcodeRequestJoinSiege, 1, 0, 1))
	if got := messages(t, drainFrames(t, w.member)); len(got) != 0 {
		t.Fatalf("defender request refused with %v", got)
	}
	if side, _ := stored(); side != "PENDING" {
		t.Fatalf("stored side after the request = %q, want PENDING", side)
	}

	confirm(w.member, 1)
	if got := drainFrames(t, w.member); len(got) != 0 {
		t.Fatalf("non-lord approval answered %x", opcodes(got))
	}
	if side, _ := stored(); side != "PENDING" {
		t.Fatalf("stored side after a non-lord approval = %q, want PENDING", side)
	}

	confirm(w.leader, 1)
	frames := drainFrames(t, w.leader)
	if len(frames) != 1 {
		t.Fatalf("lord approval answered %x, want one SiegeDefenderList", opcodes(frames))
	}
	clans, sides := siegeDefenderSides(t, frames[0])
	if i := slices.Index(clans, rivalsClanID); i < 0 || sides[i] != serverpackets.SiegeDefenderApproved {
		t.Fatalf("defender list after approval = clans %#x sides %v, want Rivals on side 3", clans, sides)
	}
	if side, _ := stored(); side != "DEFENDER" {
		t.Fatalf("stored side after approval = %q, want DEFENDER", side)
	}

	confirm(w.leader, 0)
	frames = drainFrames(t, w.leader)
	if len(frames) != 1 {
		t.Fatalf("lord refusal answered %x, want one SiegeDefenderList", opcodes(frames))
	}
	if clans, _ := siegeDefenderSides(t, frames[0]); slices.Contains(clans, rivalsClanID) {
		t.Fatalf("defender list after refusal = %#x, still listing Rivals", clans)
	}
	if _, n := stored(); n != 0 {
		t.Fatal("refused defender still stored")
	}
}

// TestSiegeJoinRefusedWhileDissolving has the leader of Rivals, whose
// dissolution is pending, ask to attack Gludio Castle: it is told 1114
// alone, with no siege window after it, and nothing is stored.
func TestSiegeJoinRefusedWhileDissolving(t *testing.T) {
	t.Parallel()
	w := bootGludioLordWorld(t, `UPDATE clan_data SET dissolving_expiry_time = 4102444800000 WHERE clan_id = `+itoa(rivalsClanID))

	w.member.Send(encodeSiegeRequest(clientpackets.OpcodeRequestJoinSiege, 1, 1, 1))
	frames := drainFrames(t, w.member)
	if !slices.Equal(opcodes(frames), []byte{serverpackets.OpcodeSystemMessage}) || messages(t, frames)[0] != siege.MsgDissolutionInProgress {
		t.Fatalf("dissolving clan's request answered %x %v, want 1114 alone", opcodes(frames), messages(t, frames))
	}
	w.srv.FlushPersistence(t)
	if n := queryInt(t, w, "SELECT COUNT(*) FROM siege_clans WHERE clan_id = ?", rivalsClanID); n != 0 {
		t.Fatalf("dissolving clan stored %d siege rows", n)
	}
}

// TestSiegeMessengerLordPages has the lord of Gludio Castle talk to its
// siege messenger: it reads siege/01.htm outside the siege and
// siege/03.htm while the siege runs, each followed by ActionFailed and
// never the siege window.
func TestSiegeMessengerLordPages(t *testing.T) {
	t.Parallel()
	w := bootGludioLordWorld(t)
	pages := siegePages(t)
	messenger := w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("SiegeNpc", gludioMessengerID), location.Location{X: w.at.X + 30, Y: w.at.Y, Z: w.at.Z})
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	talk := func(want string) {
		t.Helper()
		w.leader.Send(encodeAction(messenger.ObjectID(), w.at))
		drainFrames(t, w.leader)
		w.leader.Send(encodeAction(messenger.ObjectID(), w.at))
		frames := drainFrames(t, w.leader)
		if _, ok := firstOpcode(frames, serverpackets.OpcodeSiegeInfo); ok {
			t.Fatalf("the lord got the siege window, want %s", want)
		}
		i := slices.Index(opcodes(frames), serverpackets.OpcodeNpcHtmlMessage)
		if i < 0 || i+1 >= len(frames) || frames[i+1][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("lord talk answered %x, want the page then ActionFailed", opcodes(frames))
		}
		r := wire.NewReader(frames[i][1:])
		if id, html := r.ReadInt32(), r.ReadString(); id != messenger.ObjectID() || strings.TrimSpace(html) != strings.ReplaceAll(pages[want], "\r\n", "\n") {
			t.Fatalf("lord page = npc %d %q, want npc %d %s %q", id, html, messenger.ObjectID(), want, pages[want])
		}
	}

	talk("siege/01.htm")

	w.member.Send(encodeSiegeRequest(clientpackets.OpcodeRequestJoinSiege, 1, 1, 1))
	drainFrames(t, w.member)
	gludio, ok := w.srv.Sieges.Get(1)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	gludio.Start()
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	if !gludio.InProgress() {
		t.Fatal("siege not in progress")
	}
	talk("siege/03.htm")
}
