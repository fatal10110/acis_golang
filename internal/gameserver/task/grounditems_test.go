package task

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestGroundItemOptionsFromProperties(t *testing.T) {
	props, err := config.ParseString(`
AutoDestroyHerbTime = 15
AutoDestroyItemTime = 600
AutoDestroyEquipableItemTime = 0
AutoDestroySpecialItemTime = 57-0,5575-5
PlayerDroppedItemMultiplier = 3
`)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	got, err := GroundItemOptionsFromProperties(props)
	if err != nil {
		t.Fatalf("GroundItemOptionsFromProperties() error = %v", err)
	}
	if got.HerbAutoDestroy != 15*time.Second || got.ItemAutoDestroy != 10*time.Minute || got.EquipableAutoDestroy != 0 {
		t.Fatalf("durations = %+v", got)
	}
	if got.SpecialAutoDestroy[57] != 0 || got.SpecialAutoDestroy[5575] != 5*time.Second {
		t.Fatalf("special destroy map = %+v", got.SpecialAutoDestroy)
	}
	if got.PlayerDroppedMultiplier != 3 {
		t.Fatalf("PlayerDroppedMultiplier = %d, want 3", got.PlayerDroppedMultiplier)
	}
}

func TestGroundItemsDropAndExpire(t *testing.T) {
	now := time.UnixMilli(100_000)
	state := world.New()
	items := NewGroundItems(state, GroundItemOptions{
		ItemAutoDestroy:         10 * time.Second,
		PlayerDroppedMultiplier: 1,
	}, func() time.Time { return now })

	ground := testGroundItem(t, item.Instance{ObjectID: 1, TemplateID: 10, Count: 1}, &item.Template{ID: 10, Kind: item.KindEtcItem})
	items.Drop(ground, DropOptions{X: 100, Y: 200, Z: -50})

	if !ground.Visible() {
		t.Fatal("ground item is not visible after Drop")
	}
	if got, ok := state.Object(1); !ok || got != ground {
		t.Fatalf("state.Object(1) = %v, %v; want ground item", got, ok)
	}

	now = now.Add(9 * time.Second)
	items.Tick()
	if !ground.Visible() {
		t.Fatal("ground item despawned before its destroy time")
	}

	now = now.Add(time.Second)
	items.Tick()
	if ground.Visible() {
		t.Fatal("ground item is still visible after destroy time")
	}
	if _, ok := state.Object(1); ok {
		t.Fatal("state.Object(1) still exists after destroy time")
	}
	if got := items.Len(); got != 0 {
		t.Fatalf("Len() = %d, want 0 after expiry", got)
	}
}

func TestGroundItemsDropAndExpireNotifiesNearbyObservers(t *testing.T) {
	now := time.UnixMilli(300_000)
	state := world.New()
	items := NewGroundItems(state, GroundItemOptions{
		ItemAutoDestroy:         time.Second,
		PlayerDroppedMultiplier: 1,
	}, func() time.Time { return now })

	a := newGroundItemObserver(100)
	b := newGroundItemObserver(200)
	state.Spawn(a, 0, 0, 0, 0)
	state.Spawn(b, 100, 100, 0, 0)
	a.take()
	b.take()

	ground := testGroundItem(t, item.Instance{ObjectID: 1, TemplateID: 10, Count: 1}, &item.Template{ID: 10, Kind: item.KindEtcItem})
	items.Drop(ground, DropOptions{X: 50, Y: 50, Z: 0})

	if got, want := a.take(), []string{"100 discover 1"}; !slices.Equal(got, want) {
		t.Fatalf("observer a after drop = %v, want %v", got, want)
	}
	if got, want := b.take(), []string{"200 discover 1"}; !slices.Equal(got, want) {
		t.Fatalf("observer b after drop = %v, want %v", got, want)
	}

	now = now.Add(time.Second)
	items.Tick()

	if got, want := a.take(), []string{"100 forget 1"}; !slices.Equal(got, want) {
		t.Fatalf("observer a after expiry = %v, want %v", got, want)
	}
	if got, want := b.take(), []string{"200 forget 1"}; !slices.Equal(got, want) {
		t.Fatalf("observer b after expiry = %v, want %v", got, want)
	}
}

func TestGroundItemsPlayerDroppedMultiplierZeroDisablesExpiry(t *testing.T) {
	now := time.UnixMilli(200_000)
	state := world.New()
	items := NewGroundItems(state, GroundItemOptions{
		ItemAutoDestroy:         time.Second,
		PlayerDroppedMultiplier: 0,
	}, func() time.Time { return now })
	ground := testGroundItem(t, item.Instance{ObjectID: 1, TemplateID: 10, Count: 1}, &item.Template{ID: 10, Kind: item.KindEtcItem})

	items.Drop(ground, DropOptions{PlayerDropped: true})
	now = now.Add(time.Hour)
	items.Tick()

	if !ground.Visible() {
		t.Fatal("player-dropped item despawned with multiplier 0")
	}
}

func TestGroundItemsLoadAndSaveSnapshots(t *testing.T) {
	now := time.UnixMilli(1_000_000)
	state := world.New()
	templates := item.NewTable([]*item.Template{{ID: 10, Kind: item.KindEtcItem}})
	items := NewGroundItems(state, GroundItemOptions{}, func() time.Time { return now })

	rows := []item.GroundSnapshot{{
		Instance:       item.Instance{ObjectID: 1, TemplateID: 10, Count: 2, EnchantLevel: 3},
		X:              10,
		Y:              20,
		Z:              30,
		TimeLeftMillis: 5_000,
	}}
	if err := items.Load(rows, templates); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := items.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}

	now = now.Add(2 * time.Second)
	saved := items.Snapshots(nil)
	if len(saved) != 1 {
		t.Fatalf("Snapshots() len = %d, want 1", len(saved))
	}
	if saved[0].TimeLeftMillis != 3_000 {
		t.Fatalf("saved TimeLeftMillis = %d, want 3000", saved[0].TimeLeftMillis)
	}
	if saved[0].X != 10 || saved[0].Y != 20 || saved[0].Z != 30 {
		t.Fatalf("saved position = %+v", saved[0])
	}
}

func TestGroundItemsLoadSetsManaLeftDefault(t *testing.T) {
	now := time.UnixMilli(1_000_000)
	state := world.New()
	ordinaryTmpl := &item.Template{ID: 10, Kind: item.KindEtcItem, Duration: -1}
	shadowTmpl := &item.Template{ID: 20, Kind: item.KindWeapon, Duration: 300}
	templates := item.NewTable([]*item.Template{ordinaryTmpl, shadowTmpl})
	items := NewGroundItems(state, GroundItemOptions{}, func() time.Time { return now })

	rows := []item.GroundSnapshot{
		{Instance: item.Instance{ObjectID: 1, TemplateID: 10}},
		{Instance: item.Instance{ObjectID: 2, TemplateID: 20}},
	}
	if err := items.Load(rows, templates); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	ordinary, ok := state.Object(1)
	if !ok {
		t.Fatal("ordinary item not spawned")
	}
	if got, want := ordinary.(*grounditem.Item).Instance.ManaLeft, ordinaryTmpl.InitialManaLeft(); got != want {
		t.Fatalf("restored ordinary item ManaLeft = %d, want %d", got, want)
	}

	shadow, ok := state.Object(2)
	if !ok {
		t.Fatal("shadow item not spawned")
	}
	if got, want := shadow.(*grounditem.Item).Instance.ManaLeft, shadowTmpl.InitialManaLeft(); got != want {
		t.Fatalf("restored shadow item ManaLeft = %d, want %d", got, want)
	}
}

func testGroundItem(t *testing.T, inst item.Instance, tmpl *item.Template) *grounditem.Item {
	t.Helper()
	ground, err := grounditem.New(inst, tmpl)
	if err != nil {
		t.Fatalf("grounditem.New() error = %v", err)
	}
	return ground
}

type groundItemObserver struct {
	world.Presence

	id int32

	mu     sync.Mutex
	events []string
}

func newGroundItemObserver(id int32) *groundItemObserver {
	return &groundItemObserver{id: id}
}

func (o *groundItemObserver) ObjectID() int32 { return o.id }

func (o *groundItemObserver) Discover(obj world.Tracked) {
	o.record("discover", obj)
}

func (o *groundItemObserver) Forget(obj world.Tracked) {
	o.record("forget", obj)
}

func (o *groundItemObserver) record(verb string, obj world.Tracked) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, fmt.Sprintf("%d %s %d", o.id, verb, obj.ObjectID()))
}

func (o *groundItemObserver) take() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := o.events
	o.events = nil
	return out
}
