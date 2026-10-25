package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestSecondclassMatchesReferenceFingerprints compares the package with the reference
// base class: literals and engine calls. The player memo's presence test
// and write are the Player handle's Memo and SetMemo; the census rows of
// MemoSet are shared with the quest state's variables.
func TestSecondclassMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("secondclass", []string{"quest.SecondClassQuest"},
		"call MemoSet.containsKey", "call MemoSet.set")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
