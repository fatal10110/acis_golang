package admin

import "testing"

// TestAdminSetAccessOfflineRange pins #5129: an offline //set access level
// the stored column cannot hold is refused with an answer and leaves the
// row as it was; the column's bounds are stored.
func TestAdminSetAccessOfflineRange(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	offline := srv.SeedCharacterFor(t, "offline", "Offline", 1, 0)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("set access Offline 8388608")), "Access level 8388608 is out of range; Offline's access level is unaltered.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set access Offline -8388609")), "Access level -8388609 is out of range; Offline's access level is unaltered.")
	srv.FlushPersistence(t)
	if got := readAccessLevel(t, srv, offline.ID); got != userLevel {
		t.Fatalf("stored access level after refused levels = %d, want %d", got, userLevel)
	}

	for _, level := range []struct {
		arg  string
		want int
	}{{"8388607", 1<<23 - 1}, {"-8388608", -1 << 23}} {
		assertTexts(t, exchange(t, gm, encodeBuildCmd("set access Offline "+level.arg)), "Offline's access level is now set to "+level.arg+".")
		srv.FlushPersistence(t)
		if got := readAccessLevel(t, srv, offline.ID); got != level.want {
			t.Fatalf("stored access level = %d, want %d", got, level.want)
		}
	}
}
