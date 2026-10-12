package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

// renameNPCID is the NPC template whose name //set name may not give a
// player; gameservertest.FolkTemplate names it "Folk".
const renameNPCID = 31357

// renameSword is the harness's non-stackable "Sword", which the item log
// records.
const renameSword = 30

// incorrectName reports whether frames are exactly INCORRECT_NAME_TRY_AGAIN.
func incorrectName(frames [][]byte) bool {
	return len(frames) == 1 && frames[0][0] == serverpackets.OpcodeSystemMessage &&
		binary.LittleEndian.Uint32(frames[0][1:]) == serverpackets.SystemMessageIncorrectNameTryAgain
}

// shownName returns the object id and name a UserInfo or CharInfo frame
// shows.
func shownName(frame []byte) (int32, string) {
	r := wire.NewReader(frame[1:])
	for range 4 { // x, y, z, heading or boat
		r.ReadInt32()
	}
	return r.ReadInt32(), r.ReadString()
}

// storedName reads objID's stored char_name once every queued write has
// landed.
func storedName(t *testing.T, srv *gameservertest.Server, objID int32) string {
	t.Helper()
	srv.FlushPersistence(t)
	var name string
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT char_name FROM characters WHERE obj_Id = ?", objID).Scan(&name); err != nil {
		t.Fatalf("read char_name of %d: %v", objID, err)
	}
	return name
}

// TestAdminSetPlayerName pins //set name on a player (AdminEditChar.java
// admin_set name, Player case): a name that is not 1 to 16 ASCII letters
// or digits, an NPC's name or any character's, online or not and in any
// case, answers INCORRECT_NAME_TRY_AGAIN and changes nothing. A free name
// renames the player: it and the players around it are shown the name,
// it is stored, the GM is told, and the player is found by its new name
// only.
func TestAdminSetPlayerName(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{gameservertest.FolkTemplate("Folk", renameNPCID)})))
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	srv.SeedCharacterFor(t, "player3", "Sleeper", 1, 0)
	drain(t, gm)
	selectPlayer(t, gm, user, userID)

	for _, name := range []string{
		"Bad_Name",          // not a letter or digit
		"Abcdefghijklmnopq", // 17 characters
		"fOLK",              // an NPC's name
		"admin",             // an online character's, the GM's
		"PLAYER",            // the player's own
		"sleeper",           // an offline character's
	} {
		if frames := exchange(t, gm, encodeBuildCmd("set name "+name)); !incorrectName(frames) {
			t.Fatalf("//set name %s frames = %x, want INCORRECT_NAME_TRY_AGAIN alone", name, testsupport.FrameOpcodes(frames))
		}
	}
	if frames := settle(t, user); len(frames) != 0 {
		t.Fatalf("player frames after refused renames = %x, want none", testsupport.FrameOpcodes(frames))
	}
	if got := onlineCharacter(t, srv, userID).CharacterName(); got != "Player" {
		t.Fatalf("player name after refused renames = %q, want Player", got)
	}

	gmFrames := append(exchange(t, gm, encodeBuildCmd("set name Renamed7")), settle(t, gm)...)
	charInfos, rest := split(gmFrames)
	if len(charInfos) != 1 {
		t.Fatalf("GM frames after //set name = %x, want one CharInfo", testsupport.FrameOpcodes(gmFrames))
	}
	if id, name := shownName(charInfos[0]); id != userID || name != "Renamed7" {
		t.Fatalf("GM CharInfo = %d %q, want %d Renamed7", id, name, userID)
	}
	ops := testsupport.FrameOpcodes(gmFrames)
	if slices.Index(ops, serverpackets.OpcodeCharInfo) > slices.Index(ops, serverpackets.OpcodeSystemMessage) {
		t.Fatalf("GM frames after //set name = %x, want the CharInfo before the message", ops)
	}
	assertTexts(t, rest, "You successfully set your target's name to Renamed7.")

	frames := settle(t, user)
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("player frames after //set name = %x, want UserInfo", testsupport.FrameOpcodes(frames))
	}
	if id, name := shownName(frames[0]); id != userID || name != "Renamed7" {
		t.Fatalf("player UserInfo = %d %q, want %d Renamed7", id, name, userID)
	}
	if got := onlineCharacter(t, srv, userID).CharacterName(); got != "Renamed7" {
		t.Fatalf("player name = %q, want Renamed7", got)
	}
	if got := storedName(t, srv, userID); got != "Renamed7" {
		t.Fatalf("stored char_name = %q, want Renamed7", got)
	}
	if p, ok := srv.State.PlayerByName("renamed7"); !ok || p.ObjectID() != userID {
		t.Fatal("the player is not found by its new name")
	}
	if _, ok := srv.State.PlayerByName("Player"); ok {
		t.Fatal("the player is still found by its old name")
	}

	// The new name is taken; the old one is free again.
	if frames := exchange(t, gm, encodeBuildCmd("set name renamed7")); !incorrectName(frames) {
		t.Fatalf("//set name to the new name frames = %x, want INCORRECT_NAME_TRY_AGAIN", testsupport.FrameOpcodes(frames))
	}
	gmFrames = append(exchange(t, gm, encodeBuildCmd("set name Player")), settle(t, gm)...)
	charInfos, rest = split(gmFrames)
	if len(charInfos) != 1 {
		t.Fatalf("GM frames after //set name Player = %x, want one CharInfo", testsupport.FrameOpcodes(gmFrames))
	}
	assertTexts(t, rest, "You successfully set your target's name to Player.")
	settle(t, user)
	if p, ok := srv.State.PlayerByName("player"); !ok || p.ObjectID() != userID {
		t.Fatal("the player is not found by its old name again")
	}
	if got := storedName(t, srv, userID); got != "Player" {
		t.Fatalf("stored char_name = %q, want Player", got)
	}
}

// TestAdminSetOwnNameRenamesClanRow pins //set name without a target: the
// GM renames itself, and its clan's roster names it by the new name, as
// the reference's online member reads its player's name.
func TestAdminSetOwnNameRenamesClanRow(t *testing.T) {
	t.Parallel()
	const clanID = 268435456
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithClanSeed(func(db *sql.DB) {
		for _, q := range []string{
			`UPDATE characters SET clanid = 268435456, power_grade = 0 WHERE char_name = 'Admin'`,
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id, enabled)
				SELECT 268435456, 'Wolves', 1, obj_Id, 0 FROM characters WHERE char_name = 'Admin'`,
		} {
			if _, err := db.ExecContext(context.Background(), q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
	}))
	gm := srv.Client
	enterWorld(t, gm)

	frames := append(exchange(t, gm, encodeBuildCmd("set name Chief")), settle(t, gm)...)
	if len(frames) != 2 || frames[0][0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("GM frames after //set name = %x, want UserInfo, the message", testsupport.FrameOpcodes(frames))
	}
	if id, name := shownName(frames[0]); id != gmID || name != "Chief" {
		t.Fatalf("GM UserInfo = %d %q, want %d Chief", id, name, gmID)
	}
	assertTexts(t, frames[1:], "You successfully set your target's name to Chief.")

	cl, ok := srv.Clans.ClanOf(onlineCharacter(t, srv, gmID))
	if !ok || cl.ID() != clanID {
		t.Fatal("the GM lost its clan")
	}
	if m, ok := cl.Member(gmID); !ok || m.Name != "Chief" {
		t.Fatalf("clan roster row = %+v, want named Chief", m)
	}
	if _, ok := cl.MemberByName("Admin"); ok {
		t.Fatal("the clan roster still names the GM Admin")
	}
	if got := storedName(t, srv, gmID); got != "Chief" {
		t.Fatalf("stored char_name = %q, want Chief", got)
	}
}

// renameLogSink is the item log the server writes while the test reads it.
type renameLogSink struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *renameLogSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

// owners returns the owner names of the records with the given state
// written for ownerID.
func (s *renameLogSink) owners(t *testing.T, state string, ownerID int32) []string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(s.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var r struct {
			State   string `json:"message"`
			Owner   string `json:"owner"`
			OwnerID int32  `json:"owner_id"`
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("item log record %q: %v", line, err)
		}
		if r.State == state && r.OwnerID == ownerID {
			out = append(out, r.Owner)
		}
	}
	return out
}

func encodeAnswerTradeRequest(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAnswerTradeRequest)
	w.WriteInt32(response)
	return w.Bytes()
}

func encodeAddTradeItem(objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAddTradeItem)
	w.WriteInt32(0)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

func encodeTradeDone(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeTradeDone)
	w.WriteInt32(response)
	return w.Bytes()
}

// TestAdminSetPlayerNameDuringTrade renames a player while the GM's own
// queue settles a trade into its inventory: the item log reads the
// player's name there, on another queue than the rename's. Run under
// -race, it proves the name is read safely; the record names the player
// by one of its two names.
func TestAdminSetPlayerNameDuringTrade(t *testing.T) {
	t.Parallel()
	var sink renameLogSink
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithItemLog(zerolog.New(&sink)))
	sword := srv.GiveItem(t, gmID, renameSword, 1)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	selectPlayer(t, gm, user, userID)

	exchange(t, gm, encodeTradeRequest(userID))
	settle(t, user)
	exchange(t, user, encodeAnswerTradeRequest(1))
	settle(t, gm)
	exchange(t, gm, encodeAddTradeItem(sword, 1))
	settle(t, user)
	exchange(t, user, encodeTradeDone(1))
	settle(t, gm)

	frames := testsupport.SyncBarrierFrames(t, gm, func() {
		gm.Send(encodeBuildCmd("set name Renamed7"))
		gm.Send(encodeTradeDone(1))
		gm.Send(encodeManorBarrier())
	}, serverpackets.OpcodeExtended)
	frames = append(frames, settle(t, gm)...)
	if !slices.ContainsFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeSendTradeDone }) {
		t.Fatalf("GM frames after the last confirm = %x, want SendTradeDone", testsupport.FrameOpcodes(frames))
	}
	settle(t, user)
	if got := onlineCharacter(t, srv, userID).CharacterName(); got != "Renamed7" {
		t.Fatalf("player name = %q, want Renamed7", got)
	}
	added := sink.owners(t, "ADDED", userID)
	if len(added) != 1 || (added[0] != "Player" && added[0] != "Renamed7") {
		t.Fatalf("ADDED records for the player name %q, want one naming Player or Renamed7", added)
	}
}
