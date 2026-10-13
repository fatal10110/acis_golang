package summon

import "testing"

// TestSummonReadsSettingsInForce pins that a summon reads its base buff
// slots from its settings each time, so settings put in force after it was
// called reach it.
func TestSummonReadsSettingsInForce(t *testing.T) {
	settings := &fixedSettings{maxBuffs: 20}
	servitor := mustServitor(t, ServitorConfig{ObjectID: 1, Settings: settings})
	if got := servitor.MaxBuffCount(); got != 20 {
		t.Fatalf("MaxBuffCount() = %d, want 20", got)
	}
	settings.maxBuffs = 24
	if got := servitor.MaxBuffCount(); got != 24 {
		t.Fatalf("MaxBuffCount() after the settings changed = %d, want 24", got)
	}
}
