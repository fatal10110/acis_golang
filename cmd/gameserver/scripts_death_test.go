package main

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q230"
	"github.com/rs/zerolog"
)

// TestManifestComparisonCatchesMissingDeathTrigger: Test of the Summoner
// with its death hook dropped no longer records "on death", and the
// comparison names it beside the missing own hook.
func TestManifestComparisonCatchesMissingDeathTrigger(t *testing.T) {
	const path = "quest.Q230_TestOfTheSummoner"
	noDeath := script.Catalog{path: func() script.Script {
		s := q230.New()
		s.Hooks.OnDeath = nil
		return s
	}}
	noTemplates := func(int32) (script.NPCKind, bool) { return script.KindOther, false }
	got := manifestMismatches(t, []script.Listing{{Path: path}}, noDeath, script.Config{KindOf: noTemplates, Log: zerolog.Nop()})
	if want := path + ": on , reference death"; !slices.Contains(got, want) {
		t.Fatalf("mismatches %q, want %q among them", got, want)
	}
}
