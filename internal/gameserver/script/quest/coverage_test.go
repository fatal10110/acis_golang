package quest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// uncheckedPackages lists the sub-packages that are exempt from one of the two
// static checks, with the reason. A package missing from both this map and the
// checks fails TestEveryQuestPackageHasStaticChecks.
var uncheckedPackages = map[string]struct {
	fingerprints bool
	pages        bool
	reason       string
}{
	"secondclass": {
		pages:  true,
		reason: "the shared second-class-change machinery names no page of its own; each quest's page checks cover its pages",
	},
	"thirdclass": {
		pages:  true,
		reason: "the shared third-class-change machinery names no page of its own; each saga's page checks cover its pages",
	},
}

// TestEveryQuestPackageHasStaticChecks walks the quest sub-packages and fails
// when one lacks a scriptfp.Check or scriptpages call in its parent-package
// test file (<pkg>_test.go) or in a test file of its own. A saga's page check
// runs against the shared thirdclass package.
func TestEveryQuestPackageHasStaticChecks(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pkg := e.Name()
		parent, own, err := testSources(pkg)
		if err != nil {
			t.Fatal(err)
		}
		exempt := uncheckedPackages[pkg]
		quoted := regexp.QuoteMeta(pkg)
		fingerprint := regexp.MustCompile(`scriptfp\.Check\(\s*"` + quoted + `"`)
		pages := regexp.MustCompile(`scriptpages\.CheckPackage\(\s*\w+,\s*"(` + quoted + `|thirdclass)"`)
		if !exempt.fingerprints && !fingerprint.MatchString(parent) && !ownFingerprint.MatchString(own) {
			t.Errorf("%s: no scriptfp.Check call in %s_test.go or %s/*_test.go", pkg, pkg, pkg)
		}
		if !exempt.pages && !pages.MatchString(parent) && !ownPages.MatchString(own) {
			t.Errorf("%s: no scriptpages.CheckPackage or CheckFiles call in %s_test.go or %s/*_test.go", pkg, pkg, pkg)
		}
	}
	for pkg, exempt := range uncheckedPackages {
		if exempt.reason == "" {
			t.Errorf("%s: exemption without a reason", pkg)
		}
		if _, err := os.Stat(pkg); err != nil {
			t.Errorf("%s: exempt package does not exist: %v", pkg, err)
		}
	}
}

// A test inside the package checks the package it runs in, named ".".
var (
	ownFingerprint = regexp.MustCompile(`scriptfp\.Check\(\s*"\."`)
	ownPages       = regexp.MustCompile(`scriptpages\.Check(Package|Files)\(\s*\w+,\s*"\."`)
)

// testSources returns the source of the parent-package test file <pkg>_test.go
// and the joined source of the package's own test files; either may be empty.
func testSources(pkg string) (parent, own string, err error) {
	files, err := filepath.Glob(filepath.Join(pkg, "*_test.go"))
	if err != nil {
		return "", "", err
	}
	var b strings.Builder
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return "", "", err
		}
		b.Write(data)
	}
	data, err := os.ReadFile(pkg + "_test.go")
	if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	return string(data), b.String(), nil
}
