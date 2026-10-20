package q655

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference's reward
// method has no caller (the tamed beast gives the clan leader his crystal
// itself), so its calls and its crystal count are not ported; the date
// pattern is Go's layout here.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"quest.Q655_AGrandPlanForTamingWildBeasts"},
		"call Quest.checkClanLeaderCondition", "call Quest.dropItemsAlways", "call QuestState.getPlayer", "number 10",
		`string "yyyy-MM-dd HH:mm:ss"`, `string "2006-01-02 15:04:05"`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestPagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, ".", "quest/Q655_AGrandPlanForTamingWildBeasts")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
