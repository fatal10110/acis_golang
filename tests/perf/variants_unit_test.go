package perf

import "testing"

func TestVariantsFromEnvNamesEverySelection(t *testing.T) {
	t.Setenv(variantEnv, "spread, cast ,fixture")
	v := variantsFromEnv(t)
	if !v.spread || !v.cast || !v.fixture || v.geo || v.scripts || v.catalog {
		t.Fatalf("variants = %+v, want spread, cast and fixture only", v)
	}
	if got, want := v.String(), "cast,fixture,spread"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got := (variants{}).String(); got != "baseline" {
		t.Errorf("String() of no variant = %q, want baseline", got)
	}
}

func TestFixtureSizeDefaults(t *testing.T) {
	size := fixtureSizeFromEnv(t)
	if size.itemsPerClient <= 0 || size.backgroundChars <= 0 || size.backgroundItems <= 0 {
		t.Errorf("default fixture = %+v, want every part non-empty", size)
	}
}
