package lifecycle

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestGroundItemWithUnloadedTemplateSkippedAtBoot saves two ground items at
// shutdown, one of a template the next boot no longer loads (what a reload
// removing the template leaves on the ground). The next boot still comes up,
// restores the other item and logs the skipped one.
func TestGroundItemWithUnloadedTemplateSkippedAtBoot(t *testing.T) {
	t.Parallel()
	withOrphan := gameservertest.ItemTemplates().All()
	withOrphan = append(withOrphan, &item.Template{ID: unloadedTemplateID, Name: "Reloaded Away", Kind: item.KindEtcItem, Duration: -1, EtcItem: &item.EtcItemDetail{}})
	srv := gameservertest.Boot(t, gameservertest.WithWantChars(0), gameservertest.WithItemTemplates(item.NewTable(withOrphan)))
	srv.SeedGroundItem(t, 0, item.AdenaID, 40, spawnX, spawnY, spawnZ)
	srv.SeedGroundItem(t, 0, unloadedTemplateID, 1, spawnX+20, spawnY, spawnZ)
	srv.Shutdown(t)

	var adena, orphan int32
	for _, row := range groundRows(t, srv) {
		switch row.TemplateID {
		case item.AdenaID:
			adena = row.ObjectID
		case unloadedTemplateID:
			orphan = row.ObjectID
		}
	}
	if adena == 0 || orphan == 0 {
		t.Fatalf("items_on_ground rows after shutdown = %+v, want the adena and the orphan", groundRows(t, srv))
	}

	srv2 := gameservertest.Boot(t, gameservertest.WithWantChars(0), gameservertest.WithCapturedLog())
	if _, ok := srv2.State.Object(adena); !ok {
		t.Fatal("second boot did not restore the adena")
	}
	if _, ok := srv2.State.Object(orphan); ok {
		t.Fatal("second boot spawned the ground item with no loaded template")
	}
	if got := srv2.GroundItems.Len(); got != 1 {
		t.Fatalf("second boot restored GroundItems.Len() = %d, want 1", got)
	}
	if !strings.Contains(srv2.LogText(), "skipped a saved item with no loaded template") {
		t.Fatal("no log line for the skipped ground item")
	}
}
