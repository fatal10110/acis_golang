package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestAdminMakerPagePastInt32 pins //maker with an all-digit page number
// past int32 (AdminAlternativeSpawn.java:50-54, Integer.parseInt): the
// command fails before showing a page, so it answers nothing, while the
// largest int32 page still shows the last page.
func TestAdminMakerPagePastInt32(t *testing.T) {
	c := bootMakerAdmin(t)
	for _, page := range []string{"2147483648", "99999999999999999999"} {
		if frames := exchangeZones(t, c, encodeBuildCmd("maker "+page)); len(frames) != 0 {
			t.Fatalf("//maker %s frames = %x, want none", page, testsupport.FrameOpcodes(frames))
		}
	}
	assertContains(t, pageOf(t, exchangeZones(t, c, encodeBuildCmd("maker 2147483647"))),
		"NPC Maker Check", `<font color=LEVEL>grouped_000</font>`)
}
