package scenario

import (
	"sync"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// createdClass is the class a create step makes: a human fighter.
const createdClass = 0

// shippedClasses is the shipped class template table, loaded once per test
// binary.
var shippedClasses struct {
	once  sync.Once
	table *player.TemplateTable
	err   error
}

// starterItems is what the shipped human fighter template gives a created
// character.
func starterItems(t *testing.T) []player.StarterItem {
	t.Helper()
	dir := datapack.Path(t, "data", "xml", "classes")
	shippedClasses.once.Do(func() { shippedClasses.table, shippedClasses.err = gamexml.LoadPlayerTemplates(dir) })
	if shippedClasses.err != nil {
		t.Fatalf("load shipped class templates: %v", shippedClasses.err)
	}
	tmpl, ok := shippedClasses.table.Get(createdClass)
	if !ok {
		t.Fatalf("no shipped class template %d", createdClass)
	}
	return tmpl.Items
}

// createdTemplate is the fixture's human fighter template with the shipped
// one's starter items, so a created character holds what the reference's
// does.
func createdTemplate(t *testing.T) *player.Template {
	t.Helper()
	tmpl := gameservertest.ClassTemplate()
	tmpl.Items = starterItems(t)
	return tmpl
}

// starterItemIDs is the item templates a created character's starter items
// need in the catalog.
func starterItemIDs(t *testing.T) []int32 {
	t.Helper()
	var ids []int32
	for _, it := range starterItems(t) {
		ids = append(ids, int32(it.ItemID))
	}
	return ids
}
