package admin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The onPlayerEnter notices of a game master's login modes.
const (
	invulnerableNotice = "Entering world in Invulnerable mode."
	invisibleNotice    = "Entering world in Invisible mode."
	refusalNotice      = "Entering world in Refusal mode."
)

func encodeFriendInvite(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestFriendInvite)
	w.WriteString(name)
	return w.Bytes()
}

// gmNotices returns the login-mode notices frames carry, in order.
func gmNotices(frames [][]byte) []string {
	var out []string
	for i := range frames {
		if text, ok := texts(frames[i : i+1])[0]; ok && strings.HasPrefix(text, "Entering world in ") {
			out = append(out, text)
		}
	}
	return out
}

// requireNotices requires frames to carry exactly the login-mode notices
// want, in order.
func requireNotices(t *testing.T, frames [][]byte, want ...string) {
	t.Helper()
	got := gmNotices(frames)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("login-mode notices = %q, want %q", got, want)
	}
}

// adminDataRestricting loads the shipped admin tables with commands moved
// to the Master level (8), out of an Admin's (7) reach.
func adminDataRestricting(t *testing.T, commands ...string) *admin.Data {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"accessLevels.xml", "adminCommands.xml"} {
		raw, err := os.ReadFile(datapack.Path(t, "data", "xml", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text := string(raw)
		for _, cmd := range commands {
			old := `name="` + cmd + `" accessLevel="7"`
			if !strings.Contains(text, old) && name == "adminCommands.xml" {
				t.Fatalf("adminCommands.xml has no %s", old)
			}
			text = strings.ReplaceAll(text, old, `name="`+cmd+`" accessLevel="8"`)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	data, err := gamexml.LoadAdminData(dir)
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	return data
}

// TestGMLoginModes pins a game master's login modes (EnterWorld.java:78-91,
// Player.onPlayerEnter): with GMStartupInvulnerable, GMStartupInvisible and
// GMStartupBlockAll on, the GM's block-everything EtcStatusUpdate goes out
// right ahead of the burst's macro list, its three notices follow the
// punishment reminder's position ahead of the reuse timers, it is
// invulnerable, the players around are shown it drawn invisible and do
// not know it, and a friend invitation to it is refused naming it.
func TestGMLoginModes(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithGMStartupModes(true, true, true))
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)

	frames := burst(t, srv.Client)
	etc, macros := index(frames, serverpackets.OpcodeEtcStatusUpdate), index(frames, serverpackets.OpcodeSendMacroList)
	if etc < 0 || macros != etc+1 {
		t.Fatalf("burst %x: first EtcStatusUpdate at %d, macro list at %d; want the EtcStatusUpdate right before it", testsupport.FrameOpcodes(frames), etc, macros)
	}
	if got := etcBlocked(t, frames[etc]); got != 1 {
		t.Fatalf("early EtcStatusUpdate blocked = %d, want 1", got)
	}
	requireNotices(t, frames, invulnerableNotice, invisibleNotice, refusalNotice)
	shortcuts, coolTime := index(frames, serverpackets.OpcodeShortCutInit), index(frames, serverpackets.OpcodeSkillCoolTime)
	if at := textAt(frames, invulnerableNotice); at < shortcuts || textAt(frames, refusalNotice) > coolTime {
		t.Fatalf("burst %x: notices at %d..%d, want between ShortCutInit (%d) and SkillCoolTime (%d)", testsupport.FrameOpcodes(frames), at, textAt(frames, refusalNotice), shortcuts, coolTime)
	}
	drain(t, srv.Client)

	gm := onlineCharacter(t, srv, gmID)
	if !gm.Invul() || !gm.Invisible() || !gm.BlockingAll() {
		t.Fatalf("GM invul=%v invisible=%v blocking all=%v, want all on", gm.Invul(), gm.Invisible(), gm.BlockingAll())
	}
	seen := settle(t, user)
	var shown int
	for _, f := range seen {
		if f[0] != serverpackets.OpcodeCharInfo {
			continue
		}
		if id, invisible := charInfoHidden(t, f); id == gmID {
			shown++
			if invisible != 1 {
				t.Fatalf("player shown the GM with CharInfo invisible byte %d, want 1", invisible)
			}
		}
	}
	if shown == 0 {
		t.Fatalf("player frames %x, want the GM drawn invisible", testsupport.FrameOpcodes(seen))
	}
	if onlineCharacter(t, srv, userID).Knows(gm) {
		t.Fatal("a player knows a GM that logged in invisible")
	}

	refused := exchange(t, user, encodeFriendInvite("Admin"))
	if len(refused) == 0 {
		t.Fatal("friend invitation to the GM answered nothing")
	}
	if id, name := systemText(t, refused[0]); id != serverpackets.SystemMessageS1BlockedEverything || name != "Admin" {
		t.Fatalf("friend invitation to the GM = %d %q, want S1_BLOCKED_EVERYTHING Admin", id, name)
	}
}

// TestGMLoginModesOff pins the shipped GMStartup* defaults and the GM gate:
// a GM logs in with no mode and no notice, and a player that is not a GM
// gets none of the modes even with all of them on.
func TestGMLoginModesOff(t *testing.T) {
	t.Parallel()
	t.Run("defaults", func(t *testing.T) {
		t.Parallel()
		srv, gmID := bootAdmin(t, adminLevel)
		frames := burst(t, srv.Client)
		requireNotices(t, frames)
		if etc := index(frames, serverpackets.OpcodeEtcStatusUpdate); etc+1 == index(frames, serverpackets.OpcodeSendMacroList) {
			t.Fatalf("burst %x: an EtcStatusUpdate ahead of the macro list without GMStartupBlockAll", testsupport.FrameOpcodes(frames))
		}
		drain(t, srv.Client)
		if gm := onlineCharacter(t, srv, gmID); gm.Invul() || gm.Invisible() || gm.BlockingAll() {
			t.Fatalf("GM invul=%v invisible=%v blocking all=%v, want all off", gm.Invul(), gm.Invisible(), gm.BlockingAll())
		}
	})
	t.Run("not a GM", func(t *testing.T) {
		t.Parallel()
		srv, objID := bootAdmin(t, userLevel, gameservertest.WithGMStartupModes(true, true, true))
		frames := burst(t, srv.Client)
		requireNotices(t, frames)
		drain(t, srv.Client)
		if c := onlineCharacter(t, srv, objID); c.Invul() || c.Invisible() || c.BlockingAll() {
			t.Fatalf("player invul=%v invisible=%v blocking all=%v, want all off", c.Invul(), c.Invisible(), c.BlockingAll())
		}
	})
}

// TestGMLoginModesAccess pins the access gate on the invulnerable and
// invisible modes: a GM whose level may not use //invul and //hide logs in
// with neither, while GMStartupBlockAll, which has no gate, still applies.
func TestGMLoginModesAccess(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithAdmin(adminDataRestricting(t, "admin_invul", "admin_hide")),
		gameservertest.WithGMStartupModes(true, true, true))
	frames := burst(t, srv.Client)
	requireNotices(t, frames, refusalNotice)
	drain(t, srv.Client)
	gm := onlineCharacter(t, srv, gmID)
	if gm.Invul() || gm.Invisible() || !gm.BlockingAll() {
		t.Fatalf("GM invul=%v invisible=%v blocking all=%v, want only blocking all", gm.Invul(), gm.Invisible(), gm.BlockingAll())
	}
}
