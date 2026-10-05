package clan

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestSubunitRenameReadsUnicodeDigits renames sub-units by an id spelled in
// non-ASCII digits. The reference's VillageMaster reads the id with
// Integer.valueOf, which reads any Basic Multilingual Plane decimal digit (a
// Java probe on OpenJDK 21.0.11, recorded in #3091, prints
// Integer.parseInt("１２") and Integer.parseInt("١٢") as 12): a fullwidth
// id of a unit the clan lacks is answered as missing, an Arabic-Indic id
// renames the academy, and a fullwidth letter is no number, so nothing
// answers. The shipped pages fix the id in the link, so the master's page
// here also carries a link whose id is typed.
func TestSubunitRenameReadsUnicodeDigits(t *testing.T) {
	pages := subunitPages(t)
	key := "villagemaster/" + strconv.Itoa(masterID) + ".htm"
	pages[key] += `<a action="bypass -h npc_%objectId%_rename_pledge $id $name">Rename</a>`
	w, _ := bootSubunitWorld(t, 1, seedSubunitClan(t, academySeed), gameservertest.WithHTMLPages(pages))

	wantMessage(t, "rename of a missing unit", w.masterCommandBy(t, w.leader, "rename_pledge １００ Other"), serverpackets.SystemMessageS1, "Pledge doesn't exist.")
	for _, f := range w.masterCommandBy(t, w.leader, "rename_pledge ａ Other") {
		if f[0] == serverpackets.OpcodeSystemMessage {
			id, params := sysMsg(t, f)
			t.Fatalf("rename by a fullwidth letter answered message %d %v, want nothing", id, params)
		}
	}

	w.masterCommandBy(t, w.leader, "rename_pledge -١ Scholars")
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT COUNT(*) FROM clan_subpledges WHERE clan_id = ? AND sub_pledge_id = -1 AND name = 'Scholars'`, subunitClanID); got != 1 {
		t.Fatalf("academy rows renamed Scholars = %d, want 1", got)
	}
}
