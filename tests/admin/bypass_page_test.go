package admin

import (
	"testing"
)

// TestAdminLinkNeedsOfferingPage pins the owner-approved divergence of
// #4654: a clicked admin_ link runs only when the last validated page sent
// to the game master offered it, as an npc_ link does; any other is
// dropped with no answer. A "//command" typed in chat takes no page.
func TestAdminLinkNeedsOfferingPage(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	c := srv.Client
	enterWorld(t, c)

	if frames := exchange(t, c, encodeBypass("admin_admin 2")); len(frames) != 0 {
		t.Fatalf("admin link before any page = %x, want nothing", opcodes(frames))
	}
	assertPage(t, exchange(t, c, encodeBuildCmd("admin")), "<title>Main menu</title>")
	assertPage(t, exchange(t, c, encodeBypass("admin_admin 2")), "<title>Game menu</title>")
	// The game menu does not offer a teleport to given coordinates.
	if frames := exchange(t, c, encodeBypass("admin_teleport 1 2 3")); len(frames) != 0 {
		t.Fatalf("admin link the page did not offer = %x, want nothing", opcodes(frames))
	}
	assertPage(t, exchange(t, c, encodeBypass("admin_admin")), "<title>Main menu</title>")
}
