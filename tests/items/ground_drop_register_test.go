package items

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// spawnPicker stands at a drop point and, the moment a ground item becomes
// visible to it, tries the start of a pickup: the claim, then the untrack a
// winning pickup does before its item leaves the world. It also records the
// loot owner the item carried when it was first seen.
type spawnPicker struct {
	world.Presence
	ground *task.GroundItems

	mu      sync.Mutex
	won     []*grounditem.Item
	ownerAt map[int32]int32
}

func (p *spawnPicker) ObjectID() int32  { return 1 }
func (p *spawnPicker) Kind() actor.Kind { return actor.KindNPC }
func (p *spawnPicker) Forget(world.Tracked) {
}

func (p *spawnPicker) Discover(obj world.Tracked) {
	ground, ok := obj.(*grounditem.Item)
	if !ok {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ownerAt[ground.ObjectID()] = ground.Instance.Snapshot().OwnerID
	if ground.Claim() {
		p.ground.Remove(ground)
		p.won = append(p.won, ground)
	}
}

// TestDropRegistersBeforeItIsVisible: a drop is claimed, loot-protected and
// registered for cleanup before it becomes visible (#5096; owner-approved
// divergence, the fix the issue's Scope names). A pickup that sees it
// appear loses its claim instead of taking an item whose cleanup entry is
// then registered behind its back, which no tick could expire and the
// shutdown save would write while a player holds it, and it sees the loot
// owner already set. Once the drop is in place a pickup takes it and the
// registry and the save hold nothing for it.
func TestDropRegistersBeforeItIsVisible(t *testing.T) {
	t.Parallel()
	state := world.New()
	g := task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Minute, PlayerDroppedMultiplier: 1}, nil)
	picker := &spawnPicker{ground: g, ownerAt: map[int32]int32{}}
	state.Spawn(picker, 100, 100, 0, 0)

	tmpl := &item.Template{ID: 57, Stackable: true}
	ground, err := grounditem.New(item.Instance{ObjectID: 900101, TemplateID: 57, Count: 1}, tmpl)
	if err != nil {
		t.Fatalf("grounditem.New: %v", err)
	}
	g.Drop(ground, task.DropOptions{X: 100, Y: 100, ProtectOwnerID: 4242, ProtectFor: time.Minute})
	for _, won := range picker.won {
		state.Despawn(won)
	}

	if owner, seen := picker.ownerAt[ground.ObjectID()]; !seen || owner != 4242 {
		t.Fatalf("loot owner seen as the drop appeared = %d (seen %v), want 4242", owner, seen)
	}
	if len(picker.won) != 0 {
		t.Fatalf("a pickup claimed the drop while it was being placed; registry left with %d entries, save with %d rows",
			g.Len(), len(g.Snapshots(nil)))
	}
	if g.Len() != 1 || len(g.Snapshots(nil)) != 1 {
		t.Fatalf("placed drop: tracked %d, saved %d, want 1 and 1", g.Len(), len(g.Snapshots(nil)))
	}

	if !ground.Claim() {
		t.Fatal("placed drop refused a pickup's claim")
	}
	g.Remove(ground)
	state.Despawn(ground)
	if g.Len() != 0 || len(g.Snapshots(nil)) != 0 {
		t.Fatalf("picked drop: tracked %d, saved %d, want none", g.Len(), len(g.Snapshots(nil)))
	}
}

// TestConcurrentDropsAndPickupsLeaveNoPickedEntry interleaves many drops with
// pickups on other goroutines that take each item as soon as it shows up:
// once every item is picked, the registry and the shutdown save hold none of
// them (#5096).
func TestConcurrentDropsAndPickupsLeaveNoPickedEntry(t *testing.T) {
	t.Parallel()
	state := world.New()
	g := task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Minute, PlayerDroppedMultiplier: 1}, nil)
	tmpl := &item.Template{ID: 57, Stackable: true}

	const drops = 400
	const pickers = 4
	items := make([]*grounditem.Item, drops)
	for i := range items {
		ground, err := grounditem.New(item.Instance{ObjectID: int32(910000 + i), TemplateID: 57, Count: 1}, tmpl)
		if err != nil {
			t.Fatalf("grounditem.New: %v", err)
		}
		items[i] = ground
	}

	taken := make([]atomic.Bool, drops)
	var wg sync.WaitGroup
	for p := 0; p < pickers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i, ground := range items {
				// Take the item as soon as it is visible; a lost claim
				// retries, as a click repeated on a placed drop does.
				for !taken[i].Load() {
					if _, ok := state.Object(ground.ObjectID()); ok && ground.Claim() {
						g.Remove(ground)
						state.Despawn(ground)
						taken[i].Store(true)
					}
				}
			}
		}()
	}
	for i, ground := range items {
		g.Drop(ground, task.DropOptions{X: 10 * i, Y: 10 * i, PlayerDropped: true})
	}
	wg.Wait()

	if g.Len() != 0 {
		t.Fatalf("registry holds %d entries for picked items, want none", g.Len())
	}
	if snaps := g.Snapshots(nil); len(snaps) != 0 {
		t.Fatalf("save holds %d picked items, want none", len(snaps))
	}
}

// TestPickedItemNeverSavedOrRestoredTwice: an item a player picked up is not
// written to items_on_ground at shutdown, and a row a save left behind for
// an object id the items table holds, or a second row for one object id,
// is not restored to the ground beside it (#5096).
func TestPickedItemNeverSavedOrRestoredTwice(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	startInWorld(t, c)
	srv.SeedGroundItem(t, 0, 30, 1, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, c)
	picked := soleGroundObjectID(t, srv)
	pickedSnap := srv.GroundItems.Snapshots(nil)[0]

	c.Send(encodeAction(picked, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "pickup release")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeGetItem, "GetItem")
	drainUntilQuiet(t, c)
	if n := carriedCount(t, srv, objID, 30); n != 1 {
		t.Fatalf("carried weapons after the pickup = %d, want 1", n)
	}
	srv.FlushItems(t)

	srv.SeedGroundItem(t, 0, 30, 1, spawnX+40, spawnY, spawnZ)
	lying := srv.GroundItems.Snapshots(nil)
	if len(lying) != 1 || lying[0].ObjectID == picked {
		t.Fatalf("save after the pickup = %+v, want only the item still lying", lying)
	}

	store := gamesql.NewGroundItemStore(srv.DB)
	// The rows an earlier shutdown save wrote while the picked item still
	// had a stale cleanup entry: it and the item still lying.
	if err := store.Save(ctx, append([]item.GroundSnapshot{pickedSnap}, lying...)); err != nil {
		t.Fatalf("save ground items: %v", err)
	}
	rows, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("load ground items: %v", err)
	}
	if len(rows) != 1 || rows[0].ObjectID != lying[0].ObjectID {
		t.Fatalf("restored rows = %+v, want only object %d", rows, lying[0].ObjectID)
	}

	templates := item.NewTable([]*item.Template{groundItem(t, srv, lying[0].ObjectID).Template})
	restored := task.NewGroundItems(world.New(), task.DefaultGroundItemOptions(), nil)
	if err := restored.Load(append(rows, rows[0]), templates, zerolog.Nop()); err != nil {
		t.Fatalf("restore ground items: %v", err)
	}
	if restored.Len() != 1 || len(restored.Snapshots(nil)) != 1 {
		t.Fatalf("restored %d items, saved back %d, want 1 and 1", restored.Len(), len(restored.Snapshots(nil)))
	}
}
