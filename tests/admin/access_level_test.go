package admin

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Name and title colors of the shipped access levels.
const (
	userColor        = 0xFFFFFF
	userTitleColor   = 0xFFFF77
	adminColor       = 0xCC6600
	testGMLevel      = 2 // "Test GM": no transactions, not a GM
	generalGMLevel   = 3
	generalGMColor   = 0xCC3333
	bannedCharLevel  = -1
	bannedAcctLevel  = -100
	accessPollPeriod = 20 * time.Millisecond
)

// userInfoAccess is what a UserInfo frame shows of its character's access
// level.
type userInfoAccess struct {
	gm                    bool
	title                 string
	nameColor, titleColor int32
}

func decodeUserInfoAccess(t *testing.T, frame []byte) userInfoAccess {
	t.Helper()
	if frame[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("opcode = %#x, want UserInfo", frame[0])
	}
	r := wire.NewReader(frame[1:])
	for range 5 { // x, y, z, heading, object id
		r.ReadInt32()
	}
	r.ReadString()
	for range 4 { // race, sex, class, level
		r.ReadInt32()
	}
	r.ReadInt64() // exp
	// 6 attributes, 4 HP/MP values, sp, weight, weight limit, bonus slots,
	// the paperdoll's object and item ids.
	for range 6 + 4 + 4 + 2*17 {
		r.ReadInt32()
	}
	for range 14 + 2 + 12 + 2 + 4 { // augmentation shorts and the two ids
		r.ReadUint16()
	}
	for range 12 + 8 { // combat stats, speeds
		r.ReadInt32()
	}
	for range 4 { // speed and attack multipliers, collision
		r.ReadFloat64()
	}
	for range 3 { // hair style, hair color, face
		r.ReadInt32()
	}
	gm := r.ReadInt32() != 0
	title := r.ReadString()
	if r.Err() != nil {
		t.Fatalf("decode UserInfo: %v", r.Err())
	}
	n := len(frame)
	// The tail: name color, running, pledge class, pledge type, title
	// color, cursed weapon stage.
	return userInfoAccess{
		gm:         gm,
		title:      title,
		nameColor:  int32(binary.LittleEndian.Uint32(frame[n-21:])),
		titleColor: int32(binary.LittleEndian.Uint32(frame[n-8:])),
	}
}

// charInfoColors returns a CharInfo frame's name and title colors.
func charInfoColors(t *testing.T, frame []byte) (name, title int32) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeCharInfo {
		t.Fatalf("opcode = %#x, want CharInfo", frame[0])
	}
	// The tail: name color, heading, pledge class, pledge type, title
	// color, cursed weapon stage.
	n := len(frame)
	return int32(binary.LittleEndian.Uint32(frame[n-24:])), int32(binary.LittleEndian.Uint32(frame[n-8:]))
}

// settle returns c's frames received up to now: a barrier sent after them.
func settle(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	return testsupport.SyncBarrierFrames(t, c, func() { c.Send(encodeManorBarrier()) }, serverpackets.OpcodeExtended)
}

// onlyUserInfo requires frames to hold exactly one UserInfo and returns
// what it shows.
func onlyUserInfo(t *testing.T, frames [][]byte) userInfoAccess {
	t.Helper()
	if len(frames) != 1 {
		t.Fatalf("frames = %x, want one UserInfo", testsupport.FrameOpcodes(frames))
	}
	return decodeUserInfoAccess(t, frames[0])
}

// split separates CharInfo frames from the rest, leaving out the
// RelationChanged that follows each.
func split(frames [][]byte) (charInfos, rest [][]byte) {
	for i, f := range frames {
		switch {
		case f[0] == serverpackets.OpcodeCharInfo:
			charInfos = append(charInfos, f)
		case f[0] == serverpackets.OpcodeRelationChanged && i > 0 && frames[i-1][0] == serverpackets.OpcodeCharInfo:
			// The relation that follows every CharInfo refresh.
		default:
			rest = append(rest, f)
		}
	}
	return charInfos, rest
}

// messages keeps the SystemMessage frames of frames.
func messages(frames [][]byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			out = append(out, f)
		}
	}
	return out
}

func encodeTradeRequest(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeTradeRequest)
	w.WriteInt32(objectID)
	return w.Bytes()
}

// storedAccessLevel waits for objID's stored access level to become want.
func storedAccessLevel(t *testing.T, srv *gameservertest.Server, objID int32, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var got int
	for time.Now().Before(deadline) {
		if err := srv.DB.QueryRowContext(context.Background(), "SELECT accesslevel FROM characters WHERE obj_Id = ?", objID).Scan(&got); err != nil {
			t.Fatalf("read access level: %v", err)
		}
		if got == want {
			return
		}
		time.Sleep(accessPollPeriod)
	}
	t.Fatalf("stored access level of %d = %d, want %d", objID, got, want)
}

// accountAccessLevel waits for the login server's access level of account
// to become want.
func accountAccessLevel(t *testing.T, srv *gameservertest.Server, account string, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var got int
	for time.Now().Before(deadline) {
		if err := srv.DB.QueryRowContext(context.Background(), "SELECT access_level FROM accounts WHERE login = ?", account).Scan(&got); err != nil {
			t.Fatalf("read account access level: %v", err)
		}
		if got == want {
			return
		}
		time.Sleep(accessPollPeriod)
	}
	t.Fatalf("account %s access level = %d, want %d", account, got, want)
}

// leftWorld waits for the player called name to leave the world.
func leftWorld(t *testing.T, srv *gameservertest.Server, name string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := srv.State.PlayerByName(name); !ok {
			return
		}
		time.Sleep(accessPollPeriod)
	}
	t.Fatalf("%s still in the world", name)
}

func seedAccount(t *testing.T, srv *gameservertest.Server, login string) {
	t.Helper()
	if _, err := srv.DB.ExecContext(context.Background(), "INSERT INTO accounts (login) VALUES (?)", login); err != nil {
		t.Fatalf("seed account %s: %v", login, err)
	}
}

// TestAccessLevelShownAtLogin pins Player.setAccessLevel as the restore
// runs it (Player.java:4087, 3896-3930): a level above 0 takes the access
// level's name as title, shown from CharSelected on, and the access level
// colors the name and title.
func TestAccessLevelShownAtLogin(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
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
	if title := r.ReadString(); title != "Admin" {
		t.Fatalf("CharSelected title = %q, want Admin", title)
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	for range 100 {
		f := c.Read()
		if f[0] != serverpackets.OpcodeUserInfo {
			continue
		}
		got := decodeUserInfoAccess(t, f)
		want := userInfoAccess{gm: true, title: "Admin", nameColor: adminColor, titleColor: adminColor}
		if got != want {
			t.Fatalf("EnterWorld UserInfo = %+v, want %+v", got, want)
		}
		return
	}
	t.Fatal("no UserInfo within 100 EnterWorld frames")
}

// TestAdminSetAccess pins //set access (AdminEditChar.java:139-197) and the
// runtime Player.setAccessLevel (Player.java:3896-3930): the target's
// access level, damage and transaction rights, title, colors and GM-list
// membership follow at once, it gets UserInfo and its observers CharInfo,
// the level is stored, and an offline character's stored level changes.
func TestAdminSetAccess(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)

	// The 2-argument form names an online character.
	sent := exchange(t, gm, encodeBuildCmd("set access Player 2"))
	got := onlyUserInfo(t, settle(t, user))
	if want := (userInfoAccess{title: "Test GM", nameColor: userColor, titleColor: userTitleColor}); got != want {
		t.Fatalf("UserInfo after //set access 2 = %+v, want %+v", got, want)
	}
	charInfos, rest := split(append(sent, settle(t, gm)...))
	if len(charInfos) != 1 {
		t.Fatalf("GM frames = %x, want one CharInfo of Player", testsupport.FrameOpcodes(charInfos))
	}
	assertTexts(t, rest, "Player's access level is now set to 2.")
	storedAccessLevel(t, srv, userID, testGMLevel)
	// Test GM may not trade.
	frames := exchange(t, user, encodeTradeRequest(gmID))
	if len(frames) != 1 {
		t.Fatalf("trade request frames = %x, want NOT_AUTHORIZED", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageNotAuthorizedToDoThat)

	// The 1-argument form acts on the selected player: a GM level puts it
	// on the GM list.
	exchange(t, gm, encodeAction(userID))
	settle(t, user)
	sent = exchange(t, gm, encodeBuildCmd("set access 7"))
	got = onlyUserInfo(t, settle(t, user))
	if want := (userInfoAccess{gm: true, title: "Admin", nameColor: adminColor, titleColor: adminColor}); got != want {
		t.Fatalf("UserInfo after //set access 7 = %+v, want %+v", got, want)
	}
	charInfos, rest = split(append(sent, settle(t, gm)...))
	if len(charInfos) != 1 {
		t.Fatalf("GM frames = %x, want one CharInfo of Player", testsupport.FrameOpcodes(charInfos))
	}
	if name, title := charInfoColors(t, charInfos[0]); name != adminColor || title != adminColor {
		t.Fatalf("CharInfo colors = %#x %#x, want %#x", name, title, adminColor)
	}
	assertTexts(t, rest, "Player's access level is now set to 7.")
	assertGMs(t, exchange(t, user, encodeGmList()), "Admin", "Player")

	// Back to the user level: off the GM list, transactions allowed, the
	// title kept.
	exchange(t, gm, encodeBuildCmd("set access Player 0"))
	got = onlyUserInfo(t, settle(t, user))
	if want := (userInfoAccess{title: "Admin", nameColor: userColor, titleColor: userTitleColor}); got != want {
		t.Fatalf("UserInfo after //set access 0 = %+v, want %+v", got, want)
	}
	settle(t, gm)
	storedAccessLevel(t, srv, userID, userLevel)
	assertGMs(t, exchange(t, user, encodeGmList()), "Admin")
	frames = exchange(t, user, encodeTradeRequest(gmID))
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage && int(binary.LittleEndian.Uint32(f[1:])) == serverpackets.SystemMessageNotAuthorizedToDoThat {
			t.Fatalf("trade request refused at the user level: %x", testsupport.FrameOpcodes(frames))
		}
	}
	settle(t, gm)

	// An offline character's stored level changes; an unknown name is
	// reported, missing space included.
	offline := srv.SeedCharacterFor(t, "player3", "Offline", 1, 0)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set access Offline 3")), "Offline's access level is now set to 3.")
	storedAccessLevel(t, srv, offline.ID, generalGMLevel)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set access Offline 3")), "Offline's access level is now set to 3.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set access Nobody 3")), "Nobodycouldn't be found - its access level is unaltered.")

	assertTexts(t, exchange(t, gm, encodeBuildCmd("set")),
		"Usage: //set <access|class|color|exp|karma|level>",
		"Usage: //set <name|noble|rec|sex|sp|tcolor|title>")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set access x")), "Usage: //set access <level> | <name> <level>")
	if frames := exchange(t, gm, encodeBuildCmd("set access")); len(frames) != 0 {
		t.Fatalf("//set access frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
	frames = exchange(t, gm, encodeBuildCmd("set sex female"))
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("//set sex frames = %x, want ActionFailed", testsupport.FrameOpcodes(frames))
	}
}

// TestAdminSetAccessSelf pins //set access without a selection acting on
// the GM itself, its colors following at once.
func TestAdminSetAccessSelf(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, masterLevel)
	gm := srv.Client
	enterWorld(t, gm)
	frames := exchange(t, gm, encodeBuildCmd("set access 3"))
	if len(frames) != 2 {
		t.Fatalf("frames = %x, want UserInfo then the report", testsupport.FrameOpcodes(frames))
	}
	got := decodeUserInfoAccess(t, frames[0])
	if want := (userInfoAccess{title: "General GM", nameColor: generalGMColor, titleColor: generalGMColor}); got != want {
		t.Fatalf("UserInfo = %+v, want %+v", got, want)
	}
	assertTexts(t, frames[1:], "Admin's access level is now set to 3.")
	storedAccessLevel(t, srv, gmID, generalGMLevel)
	var title string
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT title FROM characters WHERE obj_Id = ?", gmID).Scan(&title); err != nil {
		t.Fatalf("read title: %v", err)
	}
	if title != "General GM" {
		t.Fatalf("stored title = %q, want General GM", title)
	}
}

// TestAdminSetAccessBans pins //set access below 0: the character is
// banned, disconnected, and refused at its next selection.
func TestAdminSetAccessBans(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)

	exchange(t, gm, encodeBuildCmd("set access Player -1"))
	expectKicked(t, user)
	storedAccessLevel(t, srv, userID, bannedCharLevel)
}

// TestSelectionRereadsBan pins the selection's ban check against the row
// read at selection: a ban stored after the character list was sent still
// refuses the selection, silently.
func TestSelectionRereadsBan(t *testing.T) {
	t.Parallel()
	srv, objID := bootAdmin(t, userLevel)
	c := srv.Client
	setAccessLevel(t, srv, objID, bannedCharLevel)
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(0)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	c.Send(w.Bytes())
	if f := c.ReadWithTimeout(500 * time.Millisecond); f != nil {
		t.Fatalf("selection of a banned character answered %#x, want nothing", f[0])
	}
}

// TestAdminGMOff pins //gmoff (AdminAdmin.java:111-136): the GM drops to
// the user level, leaving the GM list and the admin commands, and gets its
// level back when the timer ends.
func TestAdminGMOff(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, _ := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)

	frames := exchange(t, gm, encodeBuildCmd("gmoff 0"))
	// The 0-minute timer may already have run.
	for len(frames) < 4 {
		frames = append(frames, gm.Read())
	}
	if len(frames) != 4 {
		t.Fatalf("frames = %x, want UserInfo, report, UserInfo, report", testsupport.FrameOpcodes(frames))
	}
	off := decodeUserInfoAccess(t, frames[0])
	if want := (userInfoAccess{title: "Admin", nameColor: userColor, titleColor: userTitleColor}); off != want {
		t.Fatalf("UserInfo off = %+v, want %+v", off, want)
	}
	assertTexts(t, frames[1:2], "You no longer have GM status, but will be rehabilitated after 0 minutes.")
	on := decodeUserInfoAccess(t, frames[2])
	if want := (userInfoAccess{gm: true, title: "Admin", nameColor: adminColor, titleColor: adminColor}); on != want {
		t.Fatalf("UserInfo back = %+v, want %+v", on, want)
	}
	assertTexts(t, frames[3:], "Your previous access level has been rehabilitated.")
	drain(t, user)
	assertGMs(t, exchange(t, user, encodeGmList()), "Admin")

	// A timer it cannot read is the default minute.
	frames = exchange(t, gm, encodeBuildCmd("gmoff soon"))
	if len(frames) != 3 {
		t.Fatalf("frames = %x, want notice, UserInfo, report", testsupport.FrameOpcodes(frames))
	}
	assertTexts(t, frames[:1], "Invalid timer set for //gm ; default time is used.")
	assertTexts(t, frames[2:], "You no longer have GM status, but will be rehabilitated after 1 minutes.")
	drain(t, user)
	assertNoGM(t, exchange(t, user, encodeGmList()))
	assertTexts(t, exchange(t, gm, encodeBuildCmd("gmlist")), "You don't have the access right to use this command.")
}

// TestAdminBanPlayer pins //ban player and //unban player
// (AdminPunish.java:110-112, 226-235, 360-405): an online character is
// moved to -1 and disconnected, an offline one's stored level changes.
func TestAdminBanPlayer(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("ban")), "Usage : //ban account|chat|player [name [time]]")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("ban player Player soon")), "Usage : //ban account|chat|player [name [time]]")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("ban player")), "Usage: //ban player [name].")
	frames := exchange(t, gm, encodeBuildCmd("ban player Admin"))
	if len(frames) != 1 {
		t.Fatalf("self ban frames = %x, want CANNOT_USE_ON_YOURSELF", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageCannotUseOnYourself)
	if frames := exchange(t, gm, encodeBuildCmd("ban nothing Player")); len(frames) != 0 {
		t.Fatalf("unknown kind frames = %x, want none", testsupport.FrameOpcodes(frames))
	}

	assertTexts(t, exchange(t, gm, encodeBuildCmd("unban player Player")), "Player player isn't actually banned.")
	// Player's leaving may reach gm before the barrier does.
	assertTexts(t, messages(exchange(t, gm, encodeBuildCmd("ban player Player"))), "Player has been banned.")
	expectKicked(t, user)
	storedAccessLevel(t, srv, userID, bannedCharLevel)
	leftWorld(t, srv, "Player")
	drain(t, gm)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("unban player")), "Usage: //unban player [name].")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("unban player Player")), "Player now has an access level of 0.")
	storedAccessLevel(t, srv, userID, userLevel)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("ban player Player")), "Player now has an access level of -1.")
	storedAccessLevel(t, srv, userID, bannedCharLevel)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("ban player Nobody")), "This Player isn't found, or the AccessLevel was unaltered.")

	assertTexts(t, exchange(t, gm, encodeBuildCmd("ban chat Player")), "Player is chat banned.")
}

// TestAdminBanAccount pins //ban account and //unban account
// (AdminPunish.java:69-90, 213-224, Punishment.java:146-149) end to end: the
// login server receives the account and level and stores them, and an
// online account's player is disconnected.
func TestAdminBanAccount(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, _ := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	seedAccount(t, srv, "player2")
	seedAccount(t, srv, "offline")

	assertTexts(t, exchange(t, gm, encodeBuildCmd("ban account")), "Usage: //ban account [name].")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("ban account offline")), "Ban request sent for account offline.")
	accountAccessLevel(t, srv, "offline", bannedAcctLevel)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("unban account offline")), "Unban request sent for account offline.")
	accountAccessLevel(t, srv, "offline", userLevel)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("unban account Player")), "Player account isn't actually banned.")
	frames := exchange(t, gm, encodeBuildCmd("unban account Admin"))
	if len(frames) != 1 {
		t.Fatalf("self unban frames = %x, want CANNOT_USE_ON_YOURSELF", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageCannotUseOnYourself)

	// The selected player's account.
	exchange(t, gm, encodeAction(srvObjectID(t, srv, "Player")))
	assertTexts(t, messages(exchange(t, gm, encodeBuildCmd("ban account"))), "player2 account is banned.")
	expectKicked(t, user)
	accountAccessLevel(t, srv, "player2", bannedAcctLevel)
	leftWorld(t, srv, "Player")
	drain(t, gm)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("unban account")), "Unban request sent for account null.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("unban chat Player")), "Player's chat ban has been lifted.")
}

// srvObjectID returns the object id of the online player called name.
func srvObjectID(t *testing.T, srv *gameservertest.Server, name string) int32 {
	t.Helper()
	p, ok := srv.State.PlayerByName(name)
	if !ok {
		t.Fatalf("%s is not online", name)
	}
	return p.ObjectID()
}
