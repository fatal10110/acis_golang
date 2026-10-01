package network

import (
	"context"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	tradebook "github.com/fatal10110/acis_golang/internal/gameserver/trade"
)

// gateProbe records, at each point a handler's operation has to cover,
// whether an operation was open (task.ItemInstances.OperationOpen). The
// persister hook fires inside the handler's mutation and the item store's
// write inside applyPersistActions, after the rows were bound and took their
// places: the link has no persistence worker, so the write runs inline.
type gateProbe struct {
	instances *task.ItemInstances
	// covered, when set, replaces OperationOpen as what a probed point has
	// to see.
	covered   func() bool
	mu        sync.Mutex
	recording bool
	mutations []bool
	writes    []bool
}

func (p *gateProbe) start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.recording = true
}

func (p *gateProbe) record(into *[]bool) {
	open := p.instances.OperationOpen()
	if p.covered != nil {
		open = p.covered()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.recording {
		*into = append(*into, open)
	}
}

// assertCovered fails unless the handler mutated at least one probed row and
// wrote, with an operation open at every mutation and at every write.
func (p *gateProbe) assertCovered(t *testing.T) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.mutations) == 0 || len(p.writes) == 0 {
		t.Fatalf("probed %d mutations and %d writes, want both: the handler did not run its operation", len(p.mutations), len(p.writes))
	}
	for n, open := range p.mutations {
		if !open {
			t.Fatalf("mutation %d ran outside the operation the probe wants: another write could land that leg before it is bound", n)
		}
	}
	for n, open := range p.writes {
		if !open {
			t.Fatalf("write %d was reserved outside the operation the probe wants: another write could land a leg between them", n)
		}
	}
}

// probingPersister registers the row as the link's own persister does and
// probes the gate from inside the mutation that called it.
type probingPersister struct {
	inner item.Persister
	probe *gateProbe
}

func (p probingPersister) Persist(inst *item.Instance) {
	p.probe.record(&p.probe.mutations)
	p.inner.Persist(inst)
}

type probingItemStore struct {
	itemStore
	probe *gateProbe
}

func (s probingItemStore) WriteBatch(ctx context.Context, batch item.FlushBatch) error {
	s.probe.record(&s.probe.writes)
	return s.itemStore.WriteBatch(ctx, batch)
}

// discardFlusher stands in for the tick's store: these tests run no tick.
type discardFlusher struct{}

func (discardFlusher) Flush(context.Context, item.FlushBatch) error { return nil }

// newGateProbe gives link a real item persistence task and an item store that
// probes it.
func newGateProbe(link *GameClientLink) *gateProbe {
	instances := task.NewItemInstances(discardFlusher{}, link.itemTemplates, nil, link.itemWrites, zerolog.Nop())
	link.itemInstances = instances
	probe := &gateProbe{instances: instances}
	link.items = probingItemStore{itemStore: link.items, probe: probe}
	return probe
}

// addProbedStack gives inv a stack whose mutations probe the gate.
func addProbedStack(link *GameClientLink, probe *gateProbe, ownerID int32, inst *item.Instance) {
	inst.BindPersister(probingPersister{inner: link.itemPersister(ownerID), probe: probe})
}

// TestSettleConfirmedTradeHoldsOperationFromExchangeToWrite pins the trade's
// wiring of #3034: the settlement opens its operation before Exchange moves
// either leg and keeps it open until both legs are bound and reserved, so the
// persistence tick reads neither leg in between and cannot land one alone.
func TestSettleConfirmedTradeHoldsOperationFromExchangeToWrite(t *testing.T) {
	link, _, _, _, first, second := newDirectTradeFixture(t)
	probe := newGateProbe(link)

	const giverStack, receiverStack int32 = 5400, 5401
	addProbedStack(link, probe, first.ObjectID(), first.Inventory().AddNew(item.AdenaID, 60, giverStack))
	addProbedStack(link, probe, second.ObjectID(), second.Inventory().AddNew(item.AdenaID, 40, receiverStack))

	link.handleTradeRequest(first, clientpackets.TradeRequest{ObjectID: second.ObjectID()})
	link.handleAnswerTradeRequest(second, clientpackets.AnswerTradeRequest{Response: 1})
	link.handleAddTradeItem(first, clientpackets.AddTradeItem{ObjectID: giverStack, Count: 10})
	link.handleTradeDone(context.Background(), second, clientpackets.TradeDone{Response: 1})
	ready := link.trades.Confirm(first.ObjectID())
	if ready.Status != tradebook.DoneReady {
		t.Fatalf("Confirm status = %v, want ready", ready.Status)
	}

	probe.start()
	link.settleConfirmedTrade(ready.Session, first.ObjectID())

	if got := first.Inventory().ItemByObjectID(giverStack); got == nil || got.CountValue() != 50 {
		t.Fatalf("giver stack after the trade = %+v, want 50", got)
	}
	if got := second.Inventory().ItemByObjectID(receiverStack); got == nil || got.CountValue() != 50 {
		t.Fatalf("receiver stack after the trade = %+v, want 50", got)
	}
	probe.assertCovered(t)
	if probe.instances.OperationOpen() {
		t.Fatal("the settlement left its operation open: every persistence write would wait forever")
	}
}

// TestPickupEndsOperationBeforeTheBroadcasts pins the pickup's span: it opens
// its operation before the stack absorbs the ground item and keeps it open
// through the write, but ends it before the broadcasts, the despawn and
// ItemAdded. Every persistence write waits while any operation is open, so
// work that is not the pickup's own would stall the shared persistence lanes.
func TestPickupEndsOperationBeforeTheBroadcasts(t *testing.T) {
	link, _, _, _, first, _ := newDirectTradeFixture(t)
	probe := newGateProbe(link)
	removes := &probingGroundItems{groundItemDropper: task.NewGroundItems(link.world, task.DefaultGroundItemOptions(), nil), probe: probe}
	link.groundItems = removes

	const heldStack, groundObject int32 = 5500, 5501
	addProbedStack(link, probe, first.ObjectID(), first.Inventory().AddNew(item.AdenaID, 40, heldStack))
	tmpl, ok := link.itemTemplates.Get(item.AdenaID)
	if !ok {
		t.Fatal("missing adena template")
	}
	ground, err := grounditem.New(item.Instance{ObjectID: groundObject, TemplateID: item.AdenaID, Count: 10, Location: item.LocationVoid}, tmpl)
	if err != nil {
		t.Fatalf("grounditem.New: %v", err)
	}
	x, y, z := first.Position()
	link.world.Spawn(ground, x, y, z, 0)

	probe.start()
	if !link.pickupLiveGroundItem(context.Background(), first, ground) {
		t.Fatal("pickupLiveGroundItem did not handle the ground item")
	}

	if got := first.Inventory().ItemByObjectID(heldStack); got == nil || got.CountValue() != 50 {
		t.Fatalf("held stack after the pickup = %+v, want 50", got)
	}
	probe.assertCovered(t)
	removes.mu.Lock()
	defer removes.mu.Unlock()
	if len(removes.open) != 1 {
		t.Fatalf("ground item removed %d times, want once", len(removes.open))
	}
	if removes.open[0] {
		t.Fatal("the pickup's operation was still open at the despawn: it spans work that is not the pickup's")
	}
}

// probingGroundItems probes the gate when the pickup takes the item off the
// ground, which comes after its broadcast.
type probingGroundItems struct {
	groundItemDropper
	probe *gateProbe
	mu    sync.Mutex
	open  []bool
}

func (g *probingGroundItems) Remove(ground *grounditem.Item) {
	open := g.probe.instances.OperationOpen()
	g.mu.Lock()
	g.open = append(g.open, open)
	g.mu.Unlock()
	g.groundItemDropper.Remove(ground)
}
