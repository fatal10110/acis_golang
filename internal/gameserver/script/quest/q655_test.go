package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ655MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference's reward
// method has no caller (the tamed beast gives the clan leader his crystal
// itself), so its calls and its crystal count are not ported; the date
// pattern is Go's layout here.
func TestQ655MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q655", []string{"quest.Q655_AGrandPlanForTamingWildBeasts"},
		"call Quest.checkClanLeaderCondition", "call Quest.dropItemsAlways", "call QuestState.getPlayer", "number 10",
		`string "yyyy-MM-dd HH:mm:ss"`, `string "2006-01-02 15:04:05"`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ655PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ655PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q655", "quest/Q655_AGrandPlanForTamingWildBeasts")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
