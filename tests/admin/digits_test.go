package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestAdminIntegerArgumentReadsUnicodeDigits pins the admin commands'
// shared integer reader to the reference's Integer.parseInt
// (AdminAdmin.java:167 for //msg): fullwidth and Arabic-Indic digits read
// as their value (a Java probe on OpenJDK 21.0.11, recorded in #3091,
// prints Integer.parseInt("１２") and Integer.parseInt("١٢") as 12), and a
// fullwidth letter is no number, so the usage line answers.
func TestAdminIntegerArgumentReadsUnicodeDigits(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	c := srv.Client
	enterWorld(t, c)

	for _, arg := range []string{"１０９", "١٠٩", "1٠９"} {
		frames := exchange(t, c, encodeBuildCmd("msg "+arg))
		if len(frames) != 1 {
			t.Fatalf("//msg %s frames = %d, want 1", arg, len(frames))
		}
		assertStatic(t, frames[0], serverpackets.SystemMessageInvalidTarget)
	}
	assertTexts(t, exchange(t, c, encodeBuildCmd("msg ａ")), "Usage: //msg sysMsgId")
}
