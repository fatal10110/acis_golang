package clan

import (
	"context"
	"database/sql"
	"os"
	"slices"
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

// siegeClanID is the founder's clan in the siege world: level 5, no
// castle.
const siegeClanID int32 = 0x7f000101

// gludioMessengerID is Sir Tyron, Gludio Castle's siege messenger
// (type SiegeNpc, in Gludio's castles.xml npc list).
const gludioMessengerID = 35104

// bootSiegeWorld boots the shipped castles with their sieges running, the
// founder leading siegeClanID and the recruit in no clan, both in the
// world.
func bootSiegeWorld(t *testing.T) *clanWorld {
	t.Helper()
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	pages := map[string]string{}
	for _, name := range []string{"01.htm", "02.htm", "03.htm"} {
		data, err := os.ReadFile(datapack.Path(t, "data", "html", "siege", name))
		if err != nil {
			t.Fatalf("read siege/%s: %v", name, err)
		}
		pages["siege/"+name] = string(data)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Founder", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCastles(castles),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, q := range []string{
				"UPDATE characters SET clanid = ? WHERE char_name = 'Founder'",
				"INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) SELECT ?, 'Siegers', 5, obj_Id FROM characters WHERE char_name = 'Founder'",
			} {
				if _, err := db.ExecContext(context.Background(), q, siegeClanID); err != nil {
					t.Fatalf("seed siege clan: %v", err)
				}
			}
		}),
	)
	w := &clanWorld{srv: srv, leader: srv.Client, leaderID: srv.SoleObjectID(t)}
	w.memberID = srv.SeedCharacterFor(t, "player2", "Recruit", 1, 0).ID
	w.member = srv.DialClient(t, "player2", 1)
	startInWorld(t, w.leader)
	startInWorld(t, w.member)
	drainFrames(t, w.leader)
	x, y, z := srv.PlayerPosition(t, w.leaderID)
	w.at = location.Location{X: x, Y: y, Z: z}
	return w
}

func encodeSiegeRequest(opcode byte, fields ...int32) []byte {
	w := wire.NewPacketWriter(opcode)
	for _, f := range fields {
		w.WriteInt32(f)
	}
	return w.Bytes()
}

// siegeInfo decodes a SiegeInfo frame's castle id, lord flag, owner id and
// owner clan name.
func siegeInfo(t *testing.T, frame []byte) (castle, lord, owner int32, ownerName string) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSiegeInfo {
		t.Fatalf("opcode = %#x, want SiegeInfo", frame[0])
	}
	r := wire.NewReader(frame[1:])
	return r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadString()
}

// siegeListClans decodes a siege list frame's residence id and its clan
// ids, in order.
func siegeListClans(t *testing.T, frame []byte, defenders bool) (int32, []int32) {
	t.Helper()
	r := wire.NewReader(frame[1:])
	id := r.ReadInt32()
	r.ReadInt32()
	r.ReadInt32()
	r.ReadInt32()
	n := r.ReadInt32()
	r.ReadInt32()
	var clans []int32
	for range n {
		clans = append(clans, r.ReadInt32())
		r.ReadString()
		r.ReadString()
		r.ReadInt32()
		r.ReadInt32()
		if defenders {
			r.ReadInt32()
		}
		r.ReadInt32()
		r.ReadString()
		r.ReadString()
		r.ReadInt32()
	}
	if err := r.Err(); err != nil {
		t.Fatalf("decode siege list: %v", err)
	}
	return id, clans
}

// TestSiegeRegistrationWindow drives the siege window packets on Gludio
// Castle, held by the NPCs (RequestJoinSiege, RequestSiegeAttackerList,
// RequestSiegeDefenderList): a player without the siege privilege is told
// 794 and nothing else; the founder registers its clan to attack and sees
// the window again, the registration stored and listed; a second request
// is refused with 638 and defending a castle no clan holds with 649, each
// followed by the window; leaving drops the stored row; a list of a castle
// id naming nothing answers nothing.
func TestSiegeRegistrationWindow(t *testing.T) {
	t.Parallel()
	w := bootSiegeWorld(t)

	w.member.Send(encodeSiegeRequest(clientpackets.OpcodeRequestJoinSiege, 1, 1, 1))
	if got := drainFrames(t, w.member); !slices.Equal(opcodes(got), []byte{serverpackets.OpcodeSystemMessage}) || messages(t, got)[0] != serverpackets.SystemMessageNotAuthorizedToDoThat {
		t.Fatalf("clanless request answered %x %v", opcodes(got), messages(t, got))
	}

	join := func(attacker, joining int32) [][]byte {
		t.Helper()
		w.leader.Send(encodeSiegeRequest(clientpackets.OpcodeRequestJoinSiege, 1, attacker, joining))
		frames := drainFrames(t, w.leader)
		last := frames[len(frames)-1]
		if castle, lord, owner, name := siegeInfo(t, last); castle != 1 || lord != 0 || owner != 0 || name != "NPC" {
			t.Fatalf("SiegeInfo = castle %d lord %d owner %d %q", castle, lord, owner, name)
		}
		return frames[:len(frames)-1]
	}
	attackers := func() []int32 {
		t.Helper()
		w.leader.Send(encodeSiegeRequest(clientpackets.OpcodeRequestSiegeAttackerList, 1))
		frames := drainFrames(t, w.leader)
		if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeSiegeAttackerList {
			t.Fatalf("attacker list answer = %x", opcodes(frames))
		}
		id, clans := siegeListClans(t, frames[0], false)
		if id != 1 {
			t.Fatalf("attacker list castle = %d", id)
		}
		return clans
	}
	stored := func() int64 {
		t.Helper()
		w.srv.FlushPersistence(t)
		return queryInt(t, w, "SELECT COUNT(*) FROM siege_clans WHERE castle_id = 1 AND clan_id = ? AND type = 'ATTACKER'", siegeClanID)
	}

	if got := attackers(); len(got) != 0 {
		t.Fatalf("attackers before registering = %v", got)
	}
	if before := join(1, 1); len(before) != 0 {
		t.Fatalf("registration answered %x before the window", opcodes(before))
	}
	if got := attackers(); !slices.Equal(got, []int32{siegeClanID}) {
		t.Fatalf("attackers = %#x, want the founder's clan", got)
	}
	if stored() != 1 {
		t.Fatal("registration not stored")
	}
	if got := messages(t, join(1, 1)); !slices.Equal(got, []int{siege.MsgAlreadyRequested}) {
		t.Fatalf("second registration told %v, want 638", got)
	}
	if got := messages(t, join(0, 1)); !slices.Equal(got, []int{siege.MsgDefenderSideFull}) {
		t.Fatalf("defending an NPC castle told %v, want 649", got)
	}
	if before := join(1, 0); len(before) != 0 {
		t.Fatalf("leaving answered %x before the window", opcodes(before))
	}
	if got := attackers(); len(got) != 0 {
		t.Fatalf("attackers after leaving = %#x", got)
	}
	if stored() != 0 {
		t.Fatal("registration still stored after leaving")
	}

	w.leader.Send(encodeSiegeRequest(clientpackets.OpcodeRequestSiegeDefenderList, 99))
	if got := drainFrames(t, w.leader); len(got) != 0 {
		t.Fatalf("defender list of no castle answered %x", opcodes(got))
	}
	w.leader.Send(encodeSiegeRequest(clientpackets.OpcodeRequestSiegeDefenderList, 1))
	frames := drainFrames(t, w.leader)
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeSiegeDefenderList {
		t.Fatalf("defender list answer = %x", opcodes(frames))
	}
	if id, clans := siegeListClans(t, frames[0], true); id != 1 || len(clans) != 0 {
		t.Fatalf("defender list = castle %d clans %#x", id, clans)
	}
}

// TestSiegeStartAndEndReachThePlayers registers the founder's clan on
// Gludio's siege, then starts and ends it (Siege.startSiege, endSiege):
// the founder takes the attacker siege state and sees its UserInfo before
// the start notice, the start sound and the temporary alliance notice;
// the recruit, in no clan, only hears the start. At the end, a draw with
// no owner, both hear it and the founder's siege state clears behind the
// draw notice, before the next date is announced. The messenger opens
// the siege window outside the siege and siege/02.htm during it.
func TestSiegeStartAndEndReachThePlayers(t *testing.T) {
	t.Parallel()
	w := bootSiegeWorld(t)
	messenger := w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("SiegeNpc", gludioMessengerID), location.Location{X: w.at.X + 30, Y: w.at.Y, Z: w.at.Z})
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	talk := func() [][]byte {
		t.Helper()
		w.leader.Send(encodeAction(messenger.ObjectID(), w.at))
		drainFrames(t, w.leader)
		w.leader.Send(encodeAction(messenger.ObjectID(), w.at))
		return drainFrames(t, w.leader)
	}
	if frame, ok := firstOpcode(talk(), serverpackets.OpcodeSiegeInfo); !ok {
		t.Fatal("the messenger opened no siege window")
	} else if castle, _, _, _ := siegeInfo(t, frame); castle != 1 {
		t.Fatalf("messenger window castle = %d, want 1", castle)
	}

	w.leader.Send(encodeSiegeRequest(clientpackets.OpcodeRequestJoinSiege, 1, 1, 1))
	drainFrames(t, w.leader)
	gludio, ok := w.srv.Sieges.Get(1)
	if !ok {
		t.Fatal("no Gludio siege")
	}

	gludio.Start()
	leader, member := drainFrames(t, w.leader), drainFrames(t, w.member)
	if got := only(leader, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePlaySound); !slices.Equal(got, []byte{
		serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePlaySound, serverpackets.OpcodeSystemMessage,
	}) {
		t.Fatalf("founder at the start: %x", opcodes(leader))
	}
	if got := messages(t, leader); !slices.Equal(got, []int{siege.MsgSiegeStarted, siege.MsgTemporaryAlliance}) {
		t.Fatalf("founder start notices = %v", got)
	}
	if got := messages(t, member); !slices.Equal(got, []int{siege.MsgSiegeStarted}) {
		t.Fatalf("recruit start notices = %v", got)
	}
	if got := w.srv.PlayerSiegeState(t, w.leaderID); got != 1 {
		t.Fatalf("founder siege state = %d, want 1", got)
	}
	if !gludio.InProgress() {
		t.Fatal("siege not in progress")
	}
	frames := talk()
	if _, ok := firstOpcode(frames, serverpackets.OpcodeSiegeInfo); ok {
		t.Fatal("the messenger opened the siege window during the siege")
	}
	if page, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); !ok {
		t.Fatalf("messenger during the siege answered %x", opcodes(frames))
	} else if r := wire.NewReader(page[1:]); r.ReadInt32() != messenger.ObjectID() || r.ReadString() == "" {
		t.Fatal("messenger page malformed")
	}

	gludio.End()
	leader, member = drainFrames(t, w.leader), drainFrames(t, w.member)
	if got := messages(t, leader); !slices.Equal(got, []int{siege.MsgSiegeEnded, siege.MsgSiegeDraw, siege.MsgAnnouncedSiegeTime}) {
		t.Fatalf("founder end notices = %v", got)
	}
	if got := only(leader, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePlaySound); !slices.Equal(got, []byte{
		serverpackets.OpcodeSystemMessage, serverpackets.OpcodePlaySound, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage,
	}) {
		t.Fatalf("founder at the end: %x", opcodes(leader))
	}
	if got := messages(t, member); !slices.Equal(got, []int{siege.MsgSiegeEnded, siege.MsgSiegeDraw, siege.MsgAnnouncedSiegeTime}) {
		t.Fatalf("recruit end notices = %v", got)
	}
	if got := w.srv.PlayerSiegeState(t, w.leaderID); got != 0 {
		t.Fatalf("founder siege state after the end = %d, want 0", got)
	}
	w.srv.FlushPersistence(t)
	if n := queryInt(t, w, "SELECT COUNT(*) FROM siege_clans WHERE castle_id = 1"); n != 0 {
		t.Fatalf("siege_clans rows after the end = %d", n)
	}
	if over := queryInt(t, w, "SELECT regTimeOver = 'false' FROM castle WHERE id = 1"); over != 1 {
		t.Fatal("castle 1 registration not reopened in storage")
	}
}
