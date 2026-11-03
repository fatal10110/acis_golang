package banditstronghold

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// BanditStronghold class. The reference names the overweight refusal by
// its SystemMessageId constant, which the port writes as its id; it writes
// the siege date with a SimpleDateFormat pattern, the port with Go's
// layout, and parses a link's number as a 32-bit int, as the port asks of
// strconv. It logs a link with no number, which the port does not, and
// throws where the port panics: a clan owning another hall, its items
// taken, holds no registration.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"script.siegablehall.BanditStronghold"},
		"number 1118",
		`string "yyyy-MM-dd HH:mm:ss"`, `string "2006-01-02 15:04:05"`, "number 32",
		`string "Invalid NPC index in event: "`,
		`string "banditstronghold: clan %d owns another hall and holds no registration"`,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
