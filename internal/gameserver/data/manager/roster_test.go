//go:build integration

package manager

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// sequentialIDs is a minimal idAllocator for tests: ids count up from a
// fixed start with no reuse, since these tests never delete and re-create
// enough objects for id reuse to matter.
type sequentialIDs struct{ next int32 }

func (s *sequentialIDs) NextID() (int32, error) {
	s.next++
	return s.next, nil
}

func humanFighterTemplate(t *testing.T) *player.TemplateTable {
	t.Helper()
	tmpl := &player.Template{
		ID:        0,
		BaseLevel: 1,
		HPTable:   []float64{80},
		MPTable:   []float64{30},
		CPTable:   []float64{32},
		Spawns:    []location.Location{{X: 10, Y: 20, Z: 30}},
		Items: []player.StarterItem{
			{ItemID: 1146, Count: 1, Equipped: true},  // chest
			{ItemID: 1147, Count: 1, Equipped: true},  // legs
			{ItemID: 10, Count: 1, Equipped: false},   // dagger, unequipped
			{ItemID: 2369, Count: 1, Equipped: true},  // sword, equipped
			{ItemID: 5588, Count: 1, Equipped: false}, // etc item
		},
	}
	table, err := player.NewTemplateTable(map[int]*player.Template{0: tmpl})
	if err != nil {
		t.Fatalf("build template table: %v", err)
	}
	return table
}

func starterItemTable() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: 1146, Kind: item.KindArmor, Slot: item.SlotChest},
		{ID: 1147, Kind: item.KindArmor, Slot: item.SlotLegs},
		{ID: 10, Kind: item.KindWeapon, Slot: item.SlotRHand},
		{ID: 2369, Kind: item.KindWeapon, Slot: item.SlotRHand},
		{ID: 5588, Kind: item.KindEtcItem, Slot: item.SlotNone},
	})
}

func newTestRoster(t *testing.T, deleteAfter time.Duration, now func() time.Time) (*Roster, *sql.CharacterStore, *sql.ItemStore) {
	t.Helper()
	db := sqltest.NewDB(t)
	characters := sql.NewCharacterStore(db)
	items := sql.NewItemStore(db)
	roster := NewRoster(characters, items, humanFighterTemplate(t), starterItemTable(), &sequentialIDs{next: 0x10000000}, deleteAfter, now)
	return roster, characters, items
}

func TestRoster_Create(t *testing.T) {
	roster, _, items := newTestRoster(t, DefaultDeleteAfter, nil)

	c, outcome, err := roster.Create("acct1", CreateRequest{
		Name: "Newbie", ClassID: 0, Race: 0, Sex: player.SexMale,
		HairStyle: 1, HairColor: 0, Face: 0,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if outcome != CreateOK {
		t.Fatalf("Create() outcome = %v, want CreateOK", outcome)
	}
	if c.Name != "Newbie" || c.ClassID != 0 || c.Race != player.RaceHuman {
		t.Fatalf("Create() character = %+v", c)
	}
	if c.Position != (location.Location{X: 10, Y: 20, Z: 30}) {
		t.Errorf("Position = %+v, want template spawn", c.Position)
	}

	granted, err := items.ListByOwner(c.ObjectID)
	if err != nil {
		t.Fatalf("ListByOwner() unexpected error: %v", err)
	}
	if len(granted) != 5 {
		t.Fatalf("granted %d items, want 5", len(granted))
	}

	byTemplate := map[int32]*item.Instance{}
	for _, inst := range granted {
		byTemplate[inst.TemplateID] = inst
	}
	if inst := byTemplate[1146]; inst == nil || inst.Location != item.LocationPaperdoll || inst.LocationData != 10 {
		t.Errorf("chest = %+v, want equipped at paperdoll position 10", inst)
	}
	if inst := byTemplate[10]; inst == nil || inst.Location != item.LocationInventory {
		t.Errorf("dagger = %+v, want unequipped in inventory", inst)
	}
	if inst := byTemplate[2369]; inst == nil || inst.Location != item.LocationPaperdoll || inst.LocationData != 7 {
		t.Errorf("sword = %+v, want equipped at paperdoll position 7 (RHAND)", inst)
	}
	if inst := byTemplate[5588]; inst == nil || inst.Location != item.LocationInventory {
		t.Errorf("etc item = %+v, want inventory (never equips)", inst)
	}
}

func TestRoster_Create_InvalidName(t *testing.T) {
	roster, _, _ := newTestRoster(t, DefaultDeleteAfter, nil)

	_, outcome, err := roster.Create("acct1", CreateRequest{
		Name: "bad name!", ClassID: 0, Race: 0, Sex: player.SexMale,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if outcome != CreateInvalidName {
		t.Fatalf("Create() outcome = %v, want CreateInvalidName", outcome)
	}
}

func TestRoster_Create_NameTaken(t *testing.T) {
	roster, _, _ := newTestRoster(t, DefaultDeleteAfter, nil)

	req := CreateRequest{Name: "Newbie", ClassID: 0, Race: 0, Sex: player.SexMale}
	if _, outcome, err := roster.Create("acct1", req); err != nil || outcome != CreateOK {
		t.Fatalf("first Create() = outcome %v, err %v", outcome, err)
	}

	_, outcome, err := roster.Create("acct2", CreateRequest{Name: "newbie", ClassID: 0, Race: 0, Sex: player.SexMale})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if outcome != CreateNameTaken {
		t.Fatalf("Create() outcome = %v, want CreateNameTaken", outcome)
	}
}

func TestRoster_Create_TooManyCharacters(t *testing.T) {
	roster, _, _ := newTestRoster(t, DefaultDeleteAfter, nil)

	for i := 0; i < MaxCharactersPerAccount; i++ {
		name := string(rune('A'+i)) + "char"
		if _, outcome, err := roster.Create("acct1", CreateRequest{Name: name, ClassID: 0, Race: 0, Sex: player.SexMale}); err != nil || outcome != CreateOK {
			t.Fatalf("Create(%q) = outcome %v, err %v", name, outcome, err)
		}
	}

	_, outcome, err := roster.Create("acct1", CreateRequest{Name: "OneTooMany", ClassID: 0, Race: 0, Sex: player.SexMale})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if outcome != CreateTooManyCharacters {
		t.Fatalf("Create() outcome = %v, want CreateTooManyCharacters", outcome)
	}
}

func TestRoster_Create_NonRootClassRejected(t *testing.T) {
	roster, _, _ := newTestRoster(t, DefaultDeleteAfter, nil)

	_, outcome, err := roster.Create("acct1", CreateRequest{Name: "Newbie", ClassID: 999, Race: 0, Sex: player.SexMale})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if outcome != CreateRejected {
		t.Fatalf("Create() outcome = %v, want CreateRejected", outcome)
	}
}

func TestRoster_List_ExpiresPastDeadline(t *testing.T) {
	fixedNow := time.UnixMilli(2_000_000_000_000)
	roster, characters, items := newTestRoster(t, DefaultDeleteAfter, func() time.Time { return fixedNow })

	c, outcome, err := roster.Create("acct1", CreateRequest{Name: "Newbie", ClassID: 0, Race: 0, Sex: player.SexMale})
	if err != nil || outcome != CreateOK {
		t.Fatalf("Create() = outcome %v, err %v", outcome, err)
	}

	// Schedule deletion in the past relative to fixedNow.
	if err := characters.SetDeleteAt(c.ObjectID, fixedNow.UnixMilli()-1000); err != nil {
		t.Fatalf("SetDeleteAt() unexpected error: %v", err)
	}

	got, err := roster.List("acct1")
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("List() = %v, want empty (expired character purged)", got)
	}

	if _, err := characters.Get(c.ObjectID); err == nil {
		t.Error("Get() after expiry: want ErrCharacterNotFound, got nil error")
	}
	remainingItems, err := items.ListByOwner(c.ObjectID)
	if err != nil {
		t.Fatalf("ListByOwner() unexpected error: %v", err)
	}
	if len(remainingItems) != 0 {
		t.Errorf("items after expiry purge = %v, want none", remainingItems)
	}
}

func TestRoster_List_NotYetExpired(t *testing.T) {
	fixedNow := time.UnixMilli(2_000_000_000_000)
	roster, characters, _ := newTestRoster(t, DefaultDeleteAfter, func() time.Time { return fixedNow })

	c, _, err := roster.Create("acct1", CreateRequest{Name: "Newbie", ClassID: 0, Race: 0, Sex: player.SexMale})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if err := characters.SetDeleteAt(c.ObjectID, fixedNow.UnixMilli()+1000); err != nil {
		t.Fatalf("SetDeleteAt() unexpected error: %v", err)
	}

	got, err := roster.List("acct1")
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List() = %d characters, want 1 (not yet expired)", len(got))
	}
}

func TestRoster_MarkForDeletion_AndRestore(t *testing.T) {
	fixedNow := time.UnixMilli(1_700_000_000_000)
	roster, characters, _ := newTestRoster(t, DefaultDeleteAfter, func() time.Time { return fixedNow })

	c, _, err := roster.Create("acct1", CreateRequest{Name: "Newbie", ClassID: 0, Race: 0, Sex: player.SexMale})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	if err := roster.MarkForDeletion(c.ObjectID); err != nil {
		t.Fatalf("MarkForDeletion() unexpected error: %v", err)
	}
	got, err := characters.Get(c.ObjectID)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	want := fixedNow.Add(DefaultDeleteAfter).UnixMilli()
	if got.DeleteAt != want {
		t.Errorf("DeleteAt = %d, want %d", got.DeleteAt, want)
	}

	if err := roster.Restore(c.ObjectID); err != nil {
		t.Fatalf("Restore() unexpected error: %v", err)
	}
	got, err = characters.Get(c.ObjectID)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if got.DeleteAt != 0 {
		t.Errorf("DeleteAt after restore = %d, want 0", got.DeleteAt)
	}
}

func TestRoster_MarkForDeletion_Immediate(t *testing.T) {
	roster, characters, items := newTestRoster(t, 0, nil)

	c, _, err := roster.Create("acct1", CreateRequest{Name: "Newbie", ClassID: 0, Race: 0, Sex: player.SexMale})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	if err := roster.MarkForDeletion(c.ObjectID); err != nil {
		t.Fatalf("MarkForDeletion() unexpected error: %v", err)
	}

	if _, err := characters.Get(c.ObjectID); err == nil {
		t.Error("Get() after immediate deletion: want ErrCharacterNotFound, got nil error")
	}
	remaining, err := items.ListByOwner(c.ObjectID)
	if err != nil {
		t.Fatalf("ListByOwner() unexpected error: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("items after immediate deletion = %v, want none", remaining)
	}
}
