package feature

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestShadowweaponMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestShadowweaponMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("shadowweapon", []string{"script.feature.ShadowWeapon"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestShadowweaponPagesAreInTheDatapack requires every page the feature names to be a
// page of its datapack directory.
func TestShadowweaponPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "shadowweapon", "feature/ShadowWeapon")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
