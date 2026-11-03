package feature

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestMissqueenMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The player memo's presence
// test and write are the Player handle's Memo and SetMemo; the census rows
// of MemoSet are shared with the quest state's variables. The class tier
// is the Player handle's ClassLevel, which reads the class itself.
func TestMissqueenMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("missqueen", []string{"script.feature.MissQueen"},
		"call MemoSet.containsKey", "call MemoSet.set", "call Player.getClassId")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestMissqueenPagesAreInTheDatapack requires every page the feature names to be a
// page of its datapack directory.
func TestMissqueenPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "missqueen", "feature/MissQueen")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
