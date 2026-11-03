package sirra

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// deaconPageSuffixes are the endings the deacon appends to szName to name
// a page: the fragments are no pages of their own, the names they make are.
var deaconPageSuffixes = []string{"_01.htm", "_02.htm", "_03.htm", "_05.htm", "_06.htm"}

// TestDeaconPagesAreInTheDatapack requires every page the package names,
// the deacon's, to be a page of the deacon's datapack directory, the
// pages it names as szName and a suffix included.
func TestDeaconPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, ".", "ai/boss/sirra/FreyaDeacon", deaconPageSuffixes...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
	for _, suffix := range deaconPageSuffixes {
		if key := "ai/boss/sirra/FreyaDeacon/" + szName + suffix; !indexed(idx, key) {
			t.Errorf("page %s is not in the script page index", key)
		}
	}
}

// indexed reports whether key is a page of idx.
func indexed(idx scriptpages.Index, key string) bool {
	_, ok := idx[key]
	return ok
}
