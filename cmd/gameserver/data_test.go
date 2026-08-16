package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/rs/zerolog"
)

// TestLogUnsupportedSkillEffectsWarnsOnAnUnmappedCoreEffect is #1517's
// regression case for its "log it once at load" ask: before #1517, a
// coreKinds gap like the pre-fix Signet family degraded every affected
// cast to a silent no-op with nothing in the logs. This proves the boot
// warning actually fires for an unmapped name and stays quiet for a real
// one, so a future coreKinds gap doesn't repeat that silent-degradation
// pattern undetected.
func TestLogUnsupportedSkillEffectsWarnsOnAnUnmappedCoreEffect(t *testing.T) {
	table := skill.NewTable([]skill.Definition{
		{
			ID:    9999,
			Level: 1,
			Name:  "Definitely Not A Real Effect Kind",
			Effects: []skill.EffectTemplate{
				{Name: "ThisEffectKindDoesNotExist"},
			},
		},
	})

	var buf bytes.Buffer
	logUnsupportedSkillEffects(table, zerolog.New(&buf))

	got := buf.String()
	if !strings.Contains(got, "ThisEffectKindDoesNotExist") {
		t.Fatalf("log output = %q, want a warning naming the unmapped effect", got)
	}
}

func TestLogUnsupportedSkillEffectsStaysQuietForAKnownCoreEffect(t *testing.T) {
	table := skill.NewTable([]skill.Definition{
		{
			ID:      9998,
			Level:   1,
			Name:    "Real Buff",
			Effects: []skill.EffectTemplate{{Name: "Buff"}},
		},
	})

	var buf bytes.Buffer
	logUnsupportedSkillEffects(table, zerolog.New(&buf))

	if got := buf.String(); got != "" {
		t.Fatalf("log output = %q, want no warning for a recognized core effect", got)
	}
}

// TestLogUnsupportedSkillEffectsStaysQuietForAnUnresolvedTableName covers
// the #1516 loader defect (an unresolved "#table" reference name): that's
// a separate, already-tracked bug, not a coreKinds gap, so this helper
// must not warn about it.
func TestLogUnsupportedSkillEffectsStaysQuietForAnUnresolvedTableName(t *testing.T) {
	table := skill.NewTable([]skill.Definition{
		{
			ID:      9997,
			Level:   1,
			Name:    "Unresolved Table Ref",
			Effects: []skill.EffectTemplate{{Name: "#effectname1"}},
		},
	})

	var buf bytes.Buffer
	logUnsupportedSkillEffects(table, zerolog.New(&buf))

	if got := buf.String(); got != "" {
		t.Fatalf("log output = %q, want no warning for the #1516 unresolved-table-name defect", got)
	}
}
