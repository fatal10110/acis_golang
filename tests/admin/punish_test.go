package admin

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Punishment levels as the characters row stores them (PunishmentType
// ordinals).
const (
	punishNone = 0
	punishChat = 1
	punishJail = 2
)

// Where the jail holds a player and where it releases one (AdminPunish's
// UPDATE_JAIL / UPDATE_UNJAIL, Punishment.setType).
var (
	jailAt   = [3]int32{-114356, -249645, -2984}
	floranAt = [3]int32{17836, 170178, -3507}
	sayAll   = int32(0)
	sayTell  = int32(2)
)

// Sounds of a chat ban starting and ending.
const (
	chatBanStartSound = "systemmsg_e.346"
	chatBanEndSound   = "systemmsg_e.345"
)

// Reference ids of the punishment system messages.
const (
	chattingProhibited = 147
	targetIsChatBanned = 1079
	noUnstuckPetition  = 1043
)

func encodeSay(text string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSay2)
	w.WriteString(text)
	w.WriteInt32(sayAll)
	return w.Bytes()
}

func encodeWhisper(text, target string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSay2)
	w.WriteString(text)
	w.WriteInt32(sayTell)
	w.WriteString(target)
	return w.Bytes()
}

func encodeUserCommand(id int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestUserCommand)
	w.WriteInt32(id)
	return w.Bytes()
}

func encodeRestartPoint(kind int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestRestartPoint)
	w.WriteInt32(kind)
	return w.Bytes()
}

// punishPages returns the shipped jail pages, keyed for the page cache.
func punishPages(t *testing.T) map[string]string {
	t.Helper()
	pages := map[string]string{}
	for _, name := range []string{"jail_in.htm", "jail_out.htm"} {
		raw, err := os.ReadFile(datapack.Path(t, "data", "html", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		pages[name] = string(raw)
	}
	return pages
}

// shippedJailZones loads the shipped jail zones alone.
func shippedJailZones(t *testing.T) *zone.Index {
	t.Helper()
	raw, err := os.ReadFile(datapack.Path(t, "data", "xml", "zones", "JailZone.xml"))
	if err != nil {
		t.Fatalf("read JailZone.xml: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "JailZone.xml"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := gamexml.LoadZones(dir)
	if err != nil {
		t.Fatalf("load jail zones: %v", err)
	}
	return index
}

// bootPunish boots the GM "Admin" with the jail pages and zones.
func bootPunish(t *testing.T) *gameservertest.Server {
	t.Helper()
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithHTMLPages(punishPages(t)),
		gameservertest.WithZones(shippedJailZones(t)))
	return srv
}

// storedPunishment reads objID's stored punishment.
func storedPunishment(t *testing.T, srv *gameservertest.Server, objID int32) (level int, timer int64) {
	t.Helper()
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT punish_level, punish_timer FROM characters WHERE obj_Id = ?", objID).Scan(&level, &timer); err != nil {
		t.Fatalf("read punishment: %v", err)
	}
	return level, timer
}

// waitPunishment waits for objID's stored punishment level to become
// level, and returns the stored timer.
func waitPunishment(t *testing.T, srv *gameservertest.Server, objID int32, level int) int64 {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, timer := storedPunishment(t, srv, objID)
		if got == level {
			return timer
		}
		if time.Now().After(deadline) {
			t.Fatalf("stored punishment of %d = %d, want %d", objID, got, level)
		}
		time.Sleep(accessPollPeriod)
	}
}

// storedPosition reads objID's stored position.
func storedPosition(t *testing.T, srv *gameservertest.Server, objID int32) [3]int32 {
	t.Helper()
	var at [3]int32
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT x, y, z FROM characters WHERE obj_Id = ?", objID).Scan(&at[0], &at[1], &at[2]); err != nil {
		t.Fatalf("read position: %v", err)
	}
	return at
}

func setPunishment(t *testing.T, srv *gameservertest.Server, objID int32, level int, timer int64) {
	t.Helper()
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET punish_level = ?, punish_timer = ? WHERE obj_Id = ?", level, timer, objID); err != nil {
		t.Fatalf("seed punishment: %v", err)
	}
}

// etcBlocked returns the third field of an EtcStatusUpdate: the
// refusal/chat-ban flag.
func etcBlocked(t *testing.T, frame []byte) int32 {
	t.Helper()
	if frame[0] != serverpackets.OpcodeEtcStatusUpdate || len(frame) != 1+7*4 {
		t.Fatalf("frame %x, want a 29-byte EtcStatusUpdate", frame)
	}
	return int32(binary.LittleEndian.Uint32(frame[1+2*4:]))
}

// soundFile returns a PlaySound frame's file.
func soundFile(t *testing.T, frame []byte) string {
	t.Helper()
	if frame[0] != serverpackets.OpcodePlaySound {
		t.Fatalf("opcode = %#x, want PlaySound", frame[0])
	}
	r := wire.NewReader(frame[1:])
	r.ReadInt32()
	return r.ReadString()
}

// only keeps the frames of frames with one of opcodes.
func only(frames [][]byte, opcodes ...byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if slices.Contains(opcodes, f[0]) {
			out = append(out, f)
		}
	}
	return out
}

// assertChatBanStart requires frames, a player's, to be the start of a chat
// ban: EtcStatusUpdate with the flag set, notice, sound.
func assertChatBanStart(t *testing.T, frames [][]byte, notice string) {
	t.Helper()
	if len(frames) != 3 {
		t.Fatalf("chat ban frames = %x, want EtcStatusUpdate, notice, PlaySound", testsupport.FrameOpcodes(frames))
	}
	if got := etcBlocked(t, frames[0]); got != 1 {
		t.Fatalf("EtcStatusUpdate chat ban flag = %d, want 1", got)
	}
	assertTexts(t, frames[1:2], notice)
	if got := soundFile(t, frames[2]); got != chatBanStartSound {
		t.Fatalf("sound = %q, want %q", got, chatBanStartSound)
	}
}

// assertChatBanEnd requires frames, a player's, to be the end of a chat
// ban: EtcStatusUpdate with the flag clear, notice, sound.
func assertChatBanEnd(t *testing.T, frames [][]byte) {
	t.Helper()
	if len(frames) != 3 {
		t.Fatalf("chat ban end frames = %x, want EtcStatusUpdate, notice, PlaySound", testsupport.FrameOpcodes(frames))
	}
	if got := etcBlocked(t, frames[0]); got != 0 {
		t.Fatalf("EtcStatusUpdate chat ban flag = %d, want 0", got)
	}
	assertTexts(t, frames[1:2], "Chatting is now available.")
	if got := soundFile(t, frames[2]); got != chatBanEndSound {
		t.Fatalf("sound = %q, want %q", got, chatBanEndSound)
	}
}

// assertRefused requires frames to be the one parameterless system message
// id.
func assertRefused(t *testing.T, frames [][]byte, id int) {
	t.Helper()
	if len(frames) != 1 {
		t.Fatalf("frames = %x, want system message %d alone", testsupport.FrameOpcodes(frames), id)
	}
	assertStatic(t, frames[0], id)
}

// TestAdminChatBan pins //ban chat and //unban chat on an online player
// (AdminPunish.java:92-108, 237-251; Punishment.setType CHAT and NONE): the
// player sees the ban start and end, its chat and whispers to it are
// refused meanwhile (Say2.java:89-93, ChatTell.java:32-36), and the
// punishment row follows.
func TestAdminChatBan(t *testing.T) {
	t.Parallel()
	srv := bootPunish(t)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("ban chat")), "Usage: //ban chat [name duration].")
	assertRefused(t, exchange(t, gm, encodeBuildCmd("ban chat Admin")), serverpackets.SystemMessageCannotUseOnYourself)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("ban chat Player 5")), "Player is chat banned for 5 minutes.")
	assertChatBanStart(t, settle(t, user), "Chatting has been suspended for 5 minute(s).")
	if timer := waitPunishment(t, srv, userID, punishChat); timer != 5*60000 {
		t.Fatalf("stored timer = %d, want 300000", timer)
	}
	assertTexts(t, exchange(t, gm, encodeBuildCmd("ban chat Player")), "Player is already chat banned, and can't receive another punishment.")

	assertRefused(t, exchange(t, user, encodeSay("hello")), chattingProhibited)
	assertRefused(t, exchange(t, user, encodeWhisper("hello", "Admin")), chattingProhibited)
	// A game master's whisper is refused too.
	assertRefused(t, exchange(t, gm, encodeWhisper("hello", "Player")), targetIsChatBanned)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("unban chat Player")), "Player's chat ban has been lifted.")
	assertChatBanEnd(t, settle(t, user))
	// The lift keeps what was left of the timer.
	if timer := waitPunishment(t, srv, userID, punishNone); timer <= 0 || timer > 5*60000 {
		t.Fatalf("stored timer after the lift = %d, want what was left of 300000", timer)
	}
	assertTexts(t, exchange(t, gm, encodeBuildCmd("unban chat Player")), "Player isn't currently chat banned.")
	if says := only(exchange(t, user, encodeSay("free")), serverpackets.OpcodeCreatureSay); len(says) != 1 {
		t.Fatalf("chat after the lift = %d CreatureSay, want 1", len(says))
	}

	// Without end, by the selected player.
	exchange(t, gm, encodeAction(userID))
	settle(t, user)
	assertTexts(t, messages(exchange(t, gm, encodeBuildCmd("ban chat"))), "Player is chat banned.")
	assertChatBanStart(t, settle(t, user), "Chatting has been suspended.")
	if timer := waitPunishment(t, srv, userID, punishChat); timer != 0 {
		t.Fatalf("stored timer = %d, want 0", timer)
	}
}

// TestAdminOfflinePunishment pins the offline branches of //ban chat,
// //unban chat, //jail and //unjail (AdminPunish.java:290-358): the row's
// punishment, and for the jail its position, change; an offline chat ban
// given no minutes lasts one minute; an unknown name is not found.
func TestAdminOfflinePunishment(t *testing.T) {
	t.Parallel()
	srv := bootPunish(t)
	gm := srv.Client
	enterWorld(t, gm)
	offline := srv.SeedCharacterFor(t, "offline", "Offline", 1, 0)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("ban chat Nobody 3")), "This Player isn't found.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("ban chat Offline")), "Offline is chat banned.")
	if level, timer := storedPunishment(t, srv, offline.ID); level != punishChat || timer != 60000 {
		t.Fatalf("stored punishment = %d/%d, want chat/60000", level, timer)
	}
	assertTexts(t, exchange(t, gm, encodeBuildCmd("ban chat Offline 3")), "Offline is chat banned for 3 minutes.")
	if level, timer := storedPunishment(t, srv, offline.ID); level != punishChat || timer != 180000 {
		t.Fatalf("stored punishment = %d/%d, want chat/180000", level, timer)
	}
	assertTexts(t, exchange(t, gm, encodeBuildCmd("unban chat Offline")), "Offline's chat ban has been lifted.")
	if level, timer := storedPunishment(t, srv, offline.ID); level != punishNone || timer != 0 {
		t.Fatalf("stored punishment = %d/%d, want none/0", level, timer)
	}
	assertTexts(t, exchange(t, gm, encodeBuildCmd("unban chat")), "This Player isn't found.")

	assertTexts(t, exchange(t, gm, encodeBuildCmd("jail Nobody")), "This Player isn't found.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("jail")), "This Player isn't found.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("jail Offline 10")), "Offline has been jailed for 10 minutes.")
	if level, timer := storedPunishment(t, srv, offline.ID); level != punishJail || timer != 600000 {
		t.Fatalf("stored punishment = %d/%d, want jail/600000", level, timer)
	}
	if at := storedPosition(t, srv, offline.ID); at != jailAt {
		t.Fatalf("stored position = %v, want the jail %v", at, jailAt)
	}
	assertTexts(t, exchange(t, gm, encodeBuildCmd("jail Offline")), "Offline has been jailed.")
	if level, timer := storedPunishment(t, srv, offline.ID); level != punishJail || timer != 0 {
		t.Fatalf("stored punishment = %d/%d, want jail/0", level, timer)
	}
	// Minutes that do not parse answer nothing.
	if frames := exchange(t, gm, encodeBuildCmd("jail Offline soon")); len(frames) != 0 {
		t.Fatalf("//jail with bad minutes = %x, want nothing", testsupport.FrameOpcodes(frames))
	}

	assertTexts(t, exchange(t, gm, encodeBuildCmd("unjail Nobody")), "This Player isn't found.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("unjail Offline")), "Offline has been unjailed.")
	if level, timer := storedPunishment(t, srv, offline.ID); level != punishNone || timer != 0 {
		t.Fatalf("stored punishment = %d/%d, want none/0", level, timer)
	}
	if at := storedPosition(t, srv, offline.ID); at != floranAt {
		t.Fatalf("stored position = %v, want Floran %v", at, floranAt)
	}
}

// TestAdminJail pins //jail and //unjail on an online player (AdminPunish
// .java:165-191, 255-279; Punishment.setType JAIL and NONE): the player
// gets the jail page and is taken to the jail, cannot chat, escape or
// restart elsewhere, cannot take a chat ban on top, and on release gets the
// release page and goes to Floran village.
func TestAdminJail(t *testing.T) {
	t.Parallel()
	srv := bootPunish(t)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)

	assertRefused(t, exchange(t, gm, encodeBuildCmd("jail Admin")), serverpackets.SystemMessageCannotUseOnYourself)
	assertTexts(t, messages(exchange(t, gm, encodeBuildCmd("jail Player 30"))), "Player is jailed for 30 minutes.")
	frames := settle(t, user)
	assertTexts(t, only(frames, serverpackets.OpcodeSystemMessage), "You are jailed for 30 minutes.")
	pages := only(frames, serverpackets.OpcodeNpcHtmlMessage)
	if len(pages) != 1 || !strings.Contains(htmlBody(t, pages[0]), "You have been jailed by an admin or a GM.") {
		t.Fatalf("jail frames = %x, want the jail_in page", testsupport.FrameOpcodes(frames))
	}
	if at := teleportOf(t, frames, userID); at != jailAt {
		t.Fatalf("jail teleport = %v, want %v", at, jailAt)
	}
	// The page and the notice come before the teleport.
	if i, j := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeNpcHtmlMessage }),
		slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeTeleportToLocation }); i > j {
		t.Fatalf("jail frames = %x, want the page before the teleport", testsupport.FrameOpcodes(frames))
	}
	appear(t, user)
	if timer := waitPunishment(t, srv, userID, punishJail); timer != 30*60000 {
		t.Fatalf("stored timer = %d, want 1800000", timer)
	}

	assertRefused(t, exchange(t, user, encodeSay("let me out")), chattingProhibited)
	// gm may still be told Player left its sight.
	assertRefused(t, messages(exchange(t, gm, encodeWhisper("hello", "Player"))), targetIsChatBanned)
	assertRefused(t, exchange(t, user, encodeUserCommand(52)), noUnstuckPetition)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("ban chat Player")), "Player is already jailed, and can't receive another punishment.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("unban chat Player")), "Player isn't currently chat banned.")

	exchange(t, gm, encodeAction(userID))
	assertTexts(t, messages(exchange(t, gm, encodeBuildCmd("unjail"))), "Player has been unjailed.")
	frames = settle(t, user)
	pages = only(frames, serverpackets.OpcodeNpcHtmlMessage)
	if len(pages) != 1 || !strings.Contains(htmlBody(t, pages[0]), "You're free for now.") {
		t.Fatalf("release frames = %x, want the jail_out page", testsupport.FrameOpcodes(frames))
	}
	if at := teleportOf(t, frames, userID); !near(at, floranAt, 20) {
		t.Fatalf("release teleport = %v, want within 20 of %v", at, floranAt)
	}
	appear(t, user)
	if timer := waitPunishment(t, srv, userID, punishNone); timer <= 0 || timer > 30*60000 {
		t.Fatalf("stored timer after the release = %d, want what was left of 1800000", timer)
	}
	if says := only(exchange(t, user, encodeSay("free")), serverpackets.OpcodeCreatureSay); len(says) != 1 {
		t.Fatalf("chat after the release = %d CreatureSay, want 1", len(says))
	}
}

// burst sends RequestGameStart and EnterWorld for slot 0 and returns the
// frames up to the burst's closing ActionFailed.
func burst(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(0)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	c.Send(w.Bytes())
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	var frames [][]byte
	for range 400 {
		f := c.Read()
		frames = append(frames, f)
		if f[0] == serverpackets.OpcodeActionFailed && slices.ContainsFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeSkillCoolTime }) {
			return frames
		}
	}
	t.Fatalf("no closing ActionFailed within 400 frames: %x", testsupport.FrameOpcodes(frames))
	return nil
}

// texts returns, by frame position, the plain-text system messages of
// frames.
func texts(frames [][]byte) map[int]string {
	out := map[int]string{}
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != serverpackets.SystemMessageS1 || r.ReadInt32() != 1 || r.ReadInt32() != serverpackets.SystemMessageParamText {
			continue
		}
		out[i] = r.ReadString()
	}
	return out
}

// textAt returns the position of the plain-text message text in frames, -1
// without one.
func textAt(frames [][]byte, text string) int {
	for i, got := range texts(frames) {
		if got == text {
			return i
		}
	}
	return -1
}

// near reports whether at lies within radius of want on the ground plane,
// at want's height.
func near(at, want [3]int32, radius int32) bool {
	return at[2] == want[2] && at[0] >= want[0]-radius && at[0] <= want[0]+radius && at[1] >= want[1]-radius && at[1] <= want[1]+radius
}

// index returns the position of the first frame with opcode op, -1 without
// one.
func index(frames [][]byte, op byte) int {
	return slices.IndexFunc(frames, func(f []byte) bool { return f[0] == op })
}

// TestChatBanResumesAtLogin pins a stored chat ban across a relog
// (Player.restore → Punishment.load, onPlayerEnter → Punishment.handle):
// the login EtcStatusUpdate carries the flag, the reminder of the minutes
// left sits between the petition replay and the reuse timers, chat is
// refused, and the ban ends on the restored timer.
func TestChatBanResumesAtLogin(t *testing.T) {
	t.Parallel()
	srv := bootPunish(t)
	ch := srv.SeedCharacterFor(t, "player2", "Player", 1, 0)
	const timer = 2500
	setPunishment(t, srv, ch.ID, punishChat, timer)
	user := srv.DialClient(t, "player2", 1)

	frames := burst(t, user)
	etc := index(frames, serverpackets.OpcodeEtcStatusUpdate)
	if etc < 0 || etcBlocked(t, frames[etc]) != 1 {
		t.Fatalf("burst = %x, want an EtcStatusUpdate with the chat ban flag", testsupport.FrameOpcodes(frames))
	}
	reminder := textAt(frames, "You are still chat banned for 0 minutes.")
	if reminder < 0 {
		t.Fatalf("burst = %x, want the reminder \"You are still chat banned for 0 minutes.\"", testsupport.FrameOpcodes(frames))
	}
	if shortcuts, cool := index(frames, serverpackets.OpcodeShortCutInit), index(frames, serverpackets.OpcodeSkillCoolTime); reminder < shortcuts || reminder > cool {
		t.Fatalf("burst = %x, reminder at %d, want it after ShortCutInit (%d) and before SkillCoolTime (%d)", testsupport.FrameOpcodes(frames), reminder, shortcuts, cool)
	}
	assertRefused(t, exchange(t, user, encodeSay("hello")), chattingProhibited)

	var end [][]byte
	for len(end) < 3 {
		f := user.ReadWithTimeout(5 * time.Second)
		if f == nil {
			t.Fatalf("chat ban did not end; got %x", testsupport.FrameOpcodes(end))
		}
		switch f[0] {
		case serverpackets.OpcodeEtcStatusUpdate, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePlaySound:
			end = append(end, f)
		}
	}
	assertChatBanEnd(t, end)
	if timer := waitPunishment(t, srv, ch.ID, punishNone); timer != 0 {
		t.Fatalf("stored timer after the end = %d, want 0", timer)
	}
}

// TestJailResumesAtLogin pins a stored jail term across a relog: a player
// outside the jail is taken back within 20 of the jail point, the reminder
// rounds the minutes left (90 s reads 2), a jailed player who dies
// restarts in the jail whatever restart it asks for
// (RequestRestartPoint.java:71-72, 127-133), and one already in the jail
// stays put.
func TestJailResumesAtLogin(t *testing.T) {
	t.Parallel()
	srv := bootPunish(t)
	ch := srv.SeedCharacterFor(t, "player2", "Player", 1, 0)
	setPunishment(t, srv, ch.ID, punishJail, 90_000)
	user := srv.DialClient(t, "player2", 1)

	frames := burst(t, user)
	if textAt(frames, "You are still jailed for 2 minutes.") < 0 {
		t.Fatalf("burst = %x, want the reminder \"You are still jailed for 2 minutes.\"", testsupport.FrameOpcodes(frames))
	}
	teleport := index(frames, serverpackets.OpcodeTeleportToLocation)
	if teleport < 0 || teleport > index(frames, serverpackets.OpcodeSkillCoolTime) {
		t.Fatalf("burst = %x, want the jail teleport before SkillCoolTime", testsupport.FrameOpcodes(frames))
	}
	if _, at := teleportTo(t, frames[teleport]); !near(at, jailAt, 20) {
		t.Fatalf("jail teleport = %v, want within 20 of %v", at, jailAt)
	}
	appear(t, user)

	// Dead in jail, a town restart lands in the jail.
	srv.MarkPlayerDead(t, ch.ID)
	frames = exchange(t, user, encodeRestartPoint(0))
	if at := teleportOf(t, frames, ch.ID); !near(at, jailAt, 20) {
		t.Fatalf("restart = %v, want within 20 of the jail %v", at, jailAt)
	}
	appear(t, user)

	// Back in the jail at the next login: no teleport.
	logout(t, user)
	leftWorld(t, srv, "Player")
	srv.FlushPersistence(t)
	if at := storedPosition(t, srv, ch.ID); !near(at, jailAt, 20) {
		t.Fatalf("stored position = %v, want within 20 of the jail %v", at, jailAt)
	}
	user = srv.DialClient(t, "player2", 1)
	noTeleport(t, burst(t, user))
}
