package bossmemo

import "testing"

// TestKeysHaveOneOwner checks that no two owners share a key (#4682: the
// Heart of Warding read Frintezza's keys).
func TestKeysHaveOneOwner(t *testing.T) {
	owners := map[string]string{}
	for _, k := range []struct{ owner, key string }{
		{"AntharasSmall", AntharasSmall},
		{"AntharasNormal", AntharasNormal},
		{"AntharasLarge", AntharasLarge},
		{"Baium", Baium},
		{"Valakas", Valakas},
		{"Frintezza", Frintezza},
		{"FrintezzaFollower", FrintezzaFollower},
		{"FrintezzaTransform", FrintezzaTransform},
		{"SculptureGarden", SculptureGarden},
	} {
		if other, ok := owners[k.key]; ok {
			t.Errorf("key %q is owned by both %s and %s", k.key, other, k.owner)
		}
		owners[k.key] = k.owner
	}
}
