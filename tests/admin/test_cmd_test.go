package admin

import "testing"

// TestAdminTest pins AdminTest.java: //test answers its usage, whatever
// follows it.
func TestAdminTest(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("test")), "Usage : //test ...")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("test skill 1")), "Usage : //test ...")
	assertTexts(t, exchange(t, gm, encodeBypass("admin_test")), "Usage : //test ...")
}
