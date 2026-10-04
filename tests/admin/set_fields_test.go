package admin

import (
	"encoding/binary"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// respawnDelay is how long //set class and //set sex keep the player off
// the grid (AdminEditChar.java, ThreadPool.schedule(..., 4000)).
const respawnDelay = 4 * time.Second

// bootSetAdmin boots the GM with the class list page, then brings "Player"
// (account player2) into the world next to it.
func bootSetAdmin(t *testing.T) (srv *gameservertest.Server, gm, user *testsupport.ScriptedClient, gmID, userID int32) {
	t.Helper()
	srv, gmID = bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, "charclasses.htm")))
	gm = srv.Client
	enterWorld(t, gm)
	user, userID = addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	return srv, gm, user, gmID, userID
}

// hasOpcodes reports whether frames hold want in that order, other frames
// between them allowed.
func hasOpcodes(frames [][]byte, want ...byte) bool {
	got := testsupport.FrameOpcodes(frames)
	for _, op := range want {
		i := slices.Index(got, op)
		if i < 0 {
			return false
		}
		got = got[i+1:]
	}
	return true
}

// deletes reports whether frames hold a DeleteObject of id.
func deletes(frames [][]byte, id int32) bool {
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeDeleteObject && int32(binary.LittleEndian.Uint32(f[1:])) == id {
			return true
		}
	}
	return false
}

// TestAdminSetClass pins //set class (AdminEditChar.java admin_set class):
// the player is taken off the grid drawn held, told it is gone, and 4 s
// later comes back in its new class, stored, released and shown no longer
// held, before the GM is told. A malformed id or the id one past the last
// class opens the class list, an id outside the classes answers nothing,
// a reserved id and the player's own class are refused.
func TestAdminSetClass(t *testing.T) {
	t.Parallel()
	srv, gm, user, gmID, userID := bootSetAdmin(t)
	target := onlineCharacter(t, srv, userID)
	selectPlayer(t, gm, user, userID)

	assertPage(t, exchange(t, gm, encodeBuildCmd("set class")), "Class Selection Menu")
	assertPage(t, exchange(t, gm, encodeBuildCmd("set class x")), "Class Selection Menu")
	assertPage(t, exchange(t, gm, encodeBuildCmd("set class 119")), "Class Selection Menu")
	for _, id := range []string{"-1", "120"} {
		if frames := exchange(t, gm, encodeBuildCmd("set class "+id)); len(frames) != 0 {
			t.Fatalf("//set class %s frames = %x, want none", id, testsupport.FrameOpcodes(frames))
		}
	}
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set class 58")), "You tried to set an invalid class for Player.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set class 0")), "Player is already a(n) Human Fighter.")
	if frames := settle(t, user); len(frames) != 0 {
		t.Fatalf("player frames after refused //set class = %x, want none", testsupport.FrameOpcodes(frames))
	}

	sent := exchange(t, gm, encodeBuildCmd("set class 1"))
	frames := settle(t, user)
	if !hasOpcodes(frames, serverpackets.OpcodeUserInfo, serverpackets.OpcodeDeleteObject) || !deletes(frames, userID) || !deletes(frames, gmID) {
		t.Fatalf("player frames at //set class = %x, want UserInfo, its own DeleteObject, then the GM's", testsupport.FrameOpcodes(frames))
	}
	gmFrames := append(sent, settle(t, gm)...)
	if !hasOpcodes(gmFrames, serverpackets.OpcodeCharInfo, serverpackets.OpcodeDeleteObject) || !deletes(gmFrames, userID) || len(messages(gmFrames)) != 0 {
		t.Fatalf("GM frames at //set class = %x, want CharInfo then the player's DeleteObject, no message yet", testsupport.FrameOpcodes(gmFrames))
	}
	if target.AbnormalEffect()&modelskill.AbnormalHold2 == 0 || target.ClassID() != 0 {
		t.Fatalf("player abnormal %#x class %d while off the grid, want held and still class 0", target.AbnormalEffect(), target.ClassID())
	}
	if onlineCharacter(t, srv, gmID).Knows(target) {
		t.Fatal("the GM still knows the player taken off the grid")
	}

	srv.Advance(t, respawnDelay)
	frames = settle(t, user)
	if !hasOpcodes(frames, serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeCharInfo,
		serverpackets.OpcodeHennaInfo, serverpackets.OpcodeUserInfo, serverpackets.OpcodeUserInfo, serverpackets.OpcodeActionFailed) {
		t.Fatalf("player frames after the delay = %x, want MagicSkillUse, CLASS_TRANSFER, the GM's CharInfo, HennaInfo, UserInfo twice, ActionFailed", testsupport.FrameOpcodes(frames))
	}
	if last := frames[len(frames)-1][0]; last != serverpackets.OpcodeActionFailed {
		t.Fatalf("player frames after the delay = %x, want ActionFailed last", testsupport.FrameOpcodes(frames))
	}
	gmFrames = settle(t, gm)
	charInfos, rest := split(gmFrames)
	if len(charInfos) != 3 {
		t.Fatalf("GM frames after the delay = %x, want the player discovered, then its CharInfo twice", testsupport.FrameOpcodes(gmFrames))
	}
	assertTexts(t, messages(rest), "You successfully set Player class to Warrior.")
	if target.ClassID() != 1 || target.BaseClassID() != 1 || target.AbnormalEffect()&modelskill.AbnormalHold2 != 0 {
		t.Fatalf("player class %d base %d abnormal %#x, want class and base 1, no hold", target.ClassID(), target.BaseClassID(), target.AbnormalEffect())
	}
	if !onlineCharacter(t, srv, gmID).Knows(target) {
		t.Fatal("the GM does not know the player back on the grid")
	}
	storedColumn(t, srv, userID, "classid", "1")
	storedColumn(t, srv, userID, "base_class", "1")

	// A selection that is no player answers nothing.
	monster := srv.SpawnHostileNPCAt(t, location.Location{X: spawnX + 40, Y: spawnY, Z: spawnZ})
	drain(t, gm)
	exchange(t, gm, encodeAction(monster.ObjectID()))
	if frames := exchange(t, gm, encodeBuildCmd("set class 2")); len(frames) != 0 {
		t.Fatalf("//set class on a monster frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
}

// TestAdminSetSex pins //set sex (AdminEditChar.java admin_set sex): the
// player's own sex and a value that names none are refused; another sex is
// taken through the same 4 s off-grid respawn as //set class, then stored.
func TestAdminSetSex(t *testing.T) {
	t.Parallel()
	srv, gm, user, _, userID := bootSetAdmin(t)
	target := onlineCharacter(t, srv, userID)
	selectPlayer(t, gm, user, userID)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("set sex")), "Usage: //set sex <sex>")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set sex girl")), "Usage: //set sex <sex>")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set sex Male")), "Player's sex is already defined as MALE.")

	exchange(t, gm, encodeBuildCmd("set sex fEmale"))
	settle(t, user)
	if target.Sex() != player.SexMale || target.AbnormalEffect()&modelskill.AbnormalHold2 == 0 {
		t.Fatalf("player sex %v abnormal %#x before the delay, want still male and held", target.Sex(), target.AbnormalEffect())
	}
	settle(t, gm)
	srv.Advance(t, respawnDelay)
	frames := settle(t, user)
	if !hasOpcodes(frames, serverpackets.OpcodeCharInfo, serverpackets.OpcodeUserInfo, serverpackets.OpcodeUserInfo, serverpackets.OpcodeActionFailed) {
		t.Fatalf("player frames after the delay = %x, want the GM's CharInfo, UserInfo twice, ActionFailed", testsupport.FrameOpcodes(frames))
	}
	assertTexts(t, messages(settle(t, gm)), "You successfully set Player gender to FEMALE.")
	if target.Sex() != player.SexFemale || target.AbnormalEffect()&modelskill.AbnormalHold2 != 0 {
		t.Fatalf("player sex %v abnormal %#x, want female and no hold", target.Sex(), target.AbnormalEffect())
	}
	storedColumn(t, srv, userID, "sex", "1")
}

// npcNameTitle returns an NpcInfo frame's object id, name and title.
func npcNameTitle(t *testing.T, frame []byte) (int32, string, string) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("opcode = %#x, want NpcInfo", frame[0])
	}
	r := wire.NewReader(frame[1:])
	id := r.ReadInt32()
	for range 17 { // template id .. the 8 speeds
		r.ReadInt32()
	}
	for range 4 { // move and attack speed multipliers, collision
		r.ReadFloat64()
	}
	for range 3 { // right hand, chest, left hand
		r.ReadInt32()
	}
	r.ReadBytes(5) // name above, running, in combat, alike dead, summoned
	return id, r.ReadString(), r.ReadString()
}

// TestAdminSetNPCNameTitle pins //set name and //set title on an NPC
// (AdminEditChar.java admin_set name|title, Npc case): the NPC takes the
// word as given, every player around it is shown its NpcInfo again, and
// the GM is told. Renaming a player is not ported yet (#3399) and releases
// the client; a missing value answers the usage.
func TestAdminSetNPCNameTitle(t *testing.T) {
	t.Parallel()
	srv, gm, user, _, userID := bootSetAdmin(t)
	folk := srv.SpawnFolkNPCAt(t, &npc.Template{
		ID: 31357, TemplateID: 31357, Type: "Folk", Name: "Leandro", Title: "Gatekeeper", Level: 1, HPMax: 100,
		UsingServerSideName: true, UsingServerSideTitle: true,
	}, location.Location{X: spawnX + 40, Y: spawnY, Z: spawnZ})
	drain(t, gm)
	drain(t, user)
	exchange(t, gm, encodeAction(folk.ObjectID()))
	settle(t, gm)
	settle(t, user)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("set name")), "Usage: //set name <name>")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set title")), "Usage: //set title <title>")

	sent := exchange(t, gm, encodeBuildCmd("set name Leandro_The_Very_Long_Name more"))
	if got := testsupport.FrameOpcodes(sent); !slices.Equal(got, []byte{serverpackets.OpcodeNPCInfo, serverpackets.OpcodeSystemMessage}) {
		t.Fatalf("GM frames after //set name = %x, want NpcInfo, the message", got)
	}
	if id, name, title := npcNameTitle(t, sent[0]); id != folk.ObjectID() || name != "Leandro_The_Very_Long_Name" || title != "Gatekeeper" {
		t.Fatalf("NpcInfo = %d %q %q, want %d with the new name and the old title", id, name, title, folk.ObjectID())
	}
	assertTexts(t, sent[1:], "You successfully set your target's name to Leandro_The_Very_Long_Name.")
	if frames := settle(t, user); len(frames) != 1 || frames[0][0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("player frames after //set name = %x, want the NpcInfo", testsupport.FrameOpcodes(frames))
	}

	sent = exchange(t, gm, encodeBuildCmd("set title Keeper_Of_The_Old_Gate"))
	if got := testsupport.FrameOpcodes(sent); !slices.Equal(got, []byte{serverpackets.OpcodeNPCInfo, serverpackets.OpcodeSystemMessage}) {
		t.Fatalf("GM frames after //set title = %x, want NpcInfo, the message", got)
	}
	if _, name, title := npcNameTitle(t, sent[0]); name != "Leandro_The_Very_Long_Name" || title != "Keeper_Of_The_Old_Gate" {
		t.Fatalf("NpcInfo name %q title %q, want both set, the title untrimmed", name, title)
	}
	assertTexts(t, sent[1:], "You successfully set your target's title to Keeper_Of_The_Old_Gate.")
	settle(t, user)
	if got := folk.CharacterName(); got != "Leandro_The_Very_Long_Name" {
		t.Fatalf("NPC name = %q, want the new one", got)
	}

	// A player: not ported yet (#3399).
	selectPlayer(t, gm, user, userID)
	if got := testsupport.FrameOpcodes(exchange(t, gm, encodeBuildCmd("set name Renamed"))); !slices.Equal(got, []byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("//set name on a player frames = %x, want ActionFailed", got)
	}
	if strings.Contains(onlineCharacter(t, srv, userID).Name, "Renamed") {
		t.Fatal("//set name renamed the player")
	}
}

// TestAdminInvul pins //invul (AdminEffects.java admin_invul): the GM's
// flag is turned to the opposite of what Invul reads, and the message says
// what Invul reads after; a GM logged in invulnerable turns it off; a level
// the table does not allow is refused. The spawn-protected read is not
// reachable from a client packet until #3400: protection ends before the
// handler runs.
func TestAdminInvul(t *testing.T) {
	t.Parallel()
	t.Run("toggle", func(t *testing.T) {
		t.Parallel()
		srv, gmID := bootAdmin(t, adminLevel)
		enterWorld(t, srv.Client)
		gm := onlineCharacter(t, srv, gmID)
		assertTexts(t, exchange(t, srv.Client, encodeBuildCmd("invul")), "You are now invulnerable.")
		if !gm.Invul() {
			t.Fatal("//invul left the GM vulnerable")
		}
		assertTexts(t, exchange(t, srv.Client, encodeBuildCmd("invul")), "You are now vulnerable.")
		if gm.Invul() {
			t.Fatal("second //invul left the GM invulnerable")
		}
	})
	t.Run("startup invulnerable", func(t *testing.T) {
		t.Parallel()
		srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithGMStartupModes(true, false, false))
		enterWorld(t, srv.Client)
		gm := onlineCharacter(t, srv, gmID)
		if !gm.Invul() {
			t.Fatal("GM did not log in invulnerable")
		}
		assertTexts(t, exchange(t, srv.Client, encodeBuildCmd("invul")), "You are now vulnerable.")
		if gm.Invul() {
			t.Fatal("//invul left the startup-invulnerable GM invulnerable")
		}
	})
	t.Run("access", func(t *testing.T) {
		t.Parallel()
		srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithAdmin(adminDataRestricting(t, "admin_invul")))
		enterWorld(t, srv.Client)
		assertTexts(t, exchange(t, srv.Client, encodeBuildCmd("invul")), "You don't have the access right to use this command.")
		if onlineCharacter(t, srv, gmID).Invul() {
			t.Fatal("a refused //invul made the GM invulnerable")
		}
	})
}
