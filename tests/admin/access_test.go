package admin

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAdminCommandAccess pins the gate both admin entries share
// (RequestBypassToServer.java:49-72, SendBypassBuildCmd.java:28-54) against
// the shipped accessLevels.xml and adminCommands.xml: an unknown command
// tells only a game master it does not exist, a known one refuses a level
// without its rights, each entry with its own wording, and a level reaches
// a command through its child levels.
func TestAdminCommandAccess(t *testing.T) {
	t.Parallel()
	t.Run("user", func(t *testing.T) {
		t.Parallel()
		srv, _ := bootAdmin(t, userLevel)
		c := srv.Client
		enterWorld(t, c)

		assertTexts(t, exchange(t, c, encodeBuildCmd("admin")), "You don't have the access right to use this command.")
		assertTexts(t, exchange(t, c, encodeBypass("admin_admin")), "You don't have the access rights to use this command.")
		assertTexts(t, exchange(t, c, encodeBypass("admin_teleport 1 2 3")), "You don't have the access rights to use this command.")
		// An unknown command answers a non-GM nothing; neither entry leaves
		// a client action pending.
		assertTexts(t, exchange(t, c, encodeBuildCmd("nosuchcommand")))
		assertTexts(t, exchange(t, c, encodeBypass("admin_nosuchcommand")))
	})
	t.Run("admin", func(t *testing.T) {
		t.Parallel()
		srv, _ := bootAdmin(t, adminLevel)
		c := srv.Client
		enterWorld(t, c)

		assertTexts(t, exchange(t, c, encodeBuildCmd("nosuchcommand x")), "The command nosuchcommand doesn't exist.")
		assertTexts(t, exchange(t, c, encodeBypass("admin_nosuchcommand")), "The command nosuchcommand doesn't exist.")
		// Command words are matched as spelled: the table's admin_admin is
		// not admin_ADMIN.
		assertTexts(t, exchange(t, c, encodeBuildCmd("ADMIN")), "The command ADMIN doesn't exist.")
		// The typed text is trimmed before its word is read.
		assertPage(t, exchange(t, c, encodeBuildCmd("  admin  ")), "<title>Main menu</title>")
		assertPage(t, exchange(t, c, encodeBypass("admin_admin")), "<title>Main menu</title>")
		assertPage(t, exchange(t, c, encodeBuildCmd("admin 2")), "<title>Game menu</title>")
		assertPage(t, exchange(t, c, encodeBuildCmd("admin game")), "<title>Game menu</title>")
		// A named panel with no page falls back to the main one.
		assertPage(t, exchange(t, c, encodeBuildCmd("admin nosuchpanel")), "<title>Main menu</title>")
		assertPage(t, exchange(t, c, encodeBuildCmd("link teleports.htm")), "<title>Teleport</title>")
		assertPage(t, exchange(t, c, encodeBuildCmd("link")), "<title>Main menu</title>")
		// A missing page is announced as such.
		assertPage(t, exchange(t, c, encodeBuildCmd("link nosuchpage.htm")), "My html is missing:<br>data/html/admin/nosuchpage.htm")

		frames := exchange(t, c, encodeBuildCmd("msg 109"))
		if len(frames) != 1 {
			t.Fatalf("//msg 109 frames = %d, want 1", len(frames))
		}
		assertStatic(t, frames[0], serverpackets.SystemMessageInvalidTarget)
		assertTexts(t, exchange(t, c, encodeBuildCmd("msg x")), "Usage: //msg sysMsgId")
		assertTexts(t, exchange(t, c, encodeBuildCmd("msg")), "Usage: //msg sysMsgId")

		// A command the table lists that this server does not run yet logs
		// the gap and releases the client.
		frames = exchange(t, c, encodeBuildCmd("heal"))
		if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("//heal frames = %x, want ActionFailed", frames)
		}
	})
	t.Run("master reaches admin commands through its child level", func(t *testing.T) {
		t.Parallel()
		srv, _ := bootAdmin(t, masterLevel)
		c := srv.Client
		enterWorld(t, c)
		assertPage(t, exchange(t, c, encodeBuildCmd("admin")), "<title>Main menu</title>")
	})
}

// syncBuffer is a log sink the test reads while the server writes it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestAdminCommandAudit pins the gmaudit record of a command run: who ran
// what, as the entry received it, on which target. A refused command is
// not audited.
func TestAdminCommandAudit(t *testing.T) {
	t.Parallel()
	var audit syncBuffer
	srv, objID := bootAdmin(t, adminLevel, gameservertest.WithGMAudit(zerolog.New(&audit)))
	c := srv.Client
	enterWorld(t, c)
	exchange(t, c, encodeBuildCmd(" admin 2 "))
	exchange(t, c, encodeBypass("admin_admin"))
	exchange(t, c, encodeAction(objID))
	exchange(t, c, encodeBuildCmd("link main_menu.htm"))

	got := audit.String()
	for _, want := range []string{
		fmt.Sprintf("Admin [%d] used 'admin 2' command on: none", objID),
		fmt.Sprintf("Admin [%d] used 'admin_admin' command on: none", objID),
		fmt.Sprintf("Admin [%d] used 'link main_menu.htm' command on: Admin", objID),
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("audit log = %s, want a record %q", got, want)
		}
	}
	if n := strings.Count(got, "\n"); n != 3 {
		t.Fatalf("audit records = %d, want 3:\n%s", n, got)
	}
}

// TestAdminCommandAuditSkipsRefusal pins that a command refused for want of
// rights leaves no audit record.
func TestAdminCommandAuditSkipsRefusal(t *testing.T) {
	t.Parallel()
	var audit syncBuffer
	srv, _ := bootAdmin(t, userLevel, gameservertest.WithGMAudit(zerolog.New(&audit)))
	enterWorld(t, srv.Client)
	exchange(t, srv.Client, encodeBuildCmd("admin"))
	if got := audit.String(); got != "" {
		t.Fatalf("audit log after a refused command = %q, want empty", got)
	}
}
