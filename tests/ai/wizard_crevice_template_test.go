package ai

import (
	"os"
	"path/filepath"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// TestWizardBaseCreviceOfDiminsionReadsOnlyItsSpawnEntry: WizardBase's
// attacked hook reads CreviceOfDiminsion from the NPC's spawn entry alone,
// so the parameter set only on the NPC's template leaves a creature outside
// the spawn territory hated, and the hook runs on to its soul shot
// recharge.
func TestWizardBaseCreviceOfDiminsionReadsOnlyItsSpawnEntry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(wizardDen("")), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	w := bootWizardBaseWith(t, []gameservertest.Option{gameservertest.WithNpcSpawns(table)},
		wizardBaseTemplate(wizardBaseOnlyID, npc.AIParams{"CreviceOfDiminsion": "1", "SoulShot": "1", "SoulShotRate": "100"}, 1, nil))
	live := w.liveNPCs(wizardBaseOnlyID)
	if len(live) != 1 {
		t.Fatalf("the den spawned %d wizards, want 1", len(live))
	}
	h := live[0]
	if script.NPCOf(h).InSpawnTerritory(script.PlayerOf(w.player)) {
		x, y, z := w.player.Position()
		t.Fatalf("setup: the character at %d,%d,%d stands in the den's territory", x, y, z)
	}

	w.hit(t, h, 10)
	if !h.SoulshotCharged() {
		t.Fatal("soul shot charged = false, want true: a template-only CreviceOfDiminsion must not end the hook")
	}
}
