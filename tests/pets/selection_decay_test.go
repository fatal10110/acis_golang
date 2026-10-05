package pets

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Player.restore (Player.java:4144-4150) relinks the pet
// World.getPet still holds on the same restore that loads the inventory, so a
// corpse decay cannot settle against an owner whose rows the restore has
// already read but whose pet it has not yet taken over. aCis revision in the
// outer repo.

// signalingDecay is the production decay effect, reporting each corpse it
// decayed once the decay is over.
type signalingDecay struct {
	*corpseDecay
	decayed chan int32
}

func (d signalingDecay) Decay(actor task.DecayActor) {
	d.corpseDecay.Decay(actor)
	d.decayed <- actor.ObjectID()
}

// newSignalingDecay is newCorpseDecay whose decays report on decayed.
func newSignalingDecay(t *testing.T) signalingDecay {
	t.Helper()
	d := signalingDecay{corpseDecay: &corpseDecay{now: time.Unix(1_700_000_000, 0)}, decayed: make(chan int32, 4)}
	decay, err := task.NewDecay(d, d.clock)
	if err != nil {
		t.Fatalf("task.NewDecay: %v", err)
	}
	d.task = decay
	return d
}

// TestPetCorpseDecayingDuringItsOwnersSelection: an owner left its pet dead,
// carrying 40 adena, and selects its character just as the corpse's 20
// minutes run out: the corpse decays after the selection has read the owner's
// item rows and before it registers the owner. The decay settles with the
// selected session, as it does for an owner already back in the world: the
// session holds the pet's adena and no collar, and every item row of the owner
// is one the session holds.
func TestPetCorpseDecayingDuringItsOwnersSelection(t *testing.T) {
	t.Parallel()
	decay := newSignalingDecay(t)
	var armed atomic.Bool
	var held atomic.Int32
	hold := func(int32) {
		if !armed.Load() {
			return
		}
		held.Add(1)
		decay.mu.Lock()
		decay.now = decay.now.Add(2 * time.Second)
		decay.mu.Unlock()
		if err := decay.task.Tick(); err != nil {
			t.Errorf("decay tick inside the selection: %v", err)
			return
		}
		select {
		case <-decay.decayed:
		case <-time.After(10 * time.Second):
			t.Error("pet corpse did not decay inside the selection")
		}
	}
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithDecay(decay.task), gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithSelectionHold(hold),
	}, seedItem{TemplateID: item.AdenaID, Count: 40})
	srv := h.srv
	decay.attach(srv.State)
	wolf, _ := h.spawnWolf(t)
	h.giveToPet(t, h.seededItem(t, item.AdenaID), 40)
	killPet(t, h, wolf)

	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	decay.passAndTick(t, srv, 1199*time.Second)
	if _, ok := srv.State.Object(wolf.ObjectID()); !ok {
		t.Fatal("pet corpse decayed before its 20 minutes were up")
	}

	armed.Store(true)
	h.client.Send(encodeRequestGameStart(0))
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelected, "CharSelected")
	armed.Store(false)
	if got := held.Load(); got != 1 {
		t.Fatalf("selection hold ran %d times, want once", got)
	}
	if _, ok := srv.State.Object(wolf.ObjectID()); ok {
		t.Fatal("pet corpse still in the world after its decay")
	}
	h.client.Send(encodeEnterWorld())
	readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "end of the EnterWorld burst")
	srv.Settle(t)

	inv := h.ownerInventory(t)
	if inst := inv.ItemByObjectID(h.collarID); inst != nil {
		t.Fatalf("session still holds the decayed pet's collar %+v", inst.Snapshot())
	}
	if inst := inv.ItemByTemplateID(item.AdenaID); inst == nil || inst.Snapshot().Count != 40 {
		t.Fatal("session does not hold the decayed pet's 40 adena")
	}
	if got := h.ownerItemCount(t, wolfCollarID); got != 0 {
		t.Fatalf("collar rows after the decay = %d, want none", got)
	}
	if got := h.ownerItemCount(t, item.AdenaID); got != 40 {
		t.Fatalf("owner adena rows after the decay = %d, want the pet's 40", got)
	}
	if got := h.collarItemCount(t, item.AdenaID); got != 0 {
		t.Fatalf("adena still saved under the collar = %d, want none", got)
	}
	rows, err := srv.Items.ListByOwner(petCtx(), h.ownerID)
	if err != nil {
		t.Fatalf("list owner items: %v", err)
	}
	for _, row := range rows {
		if row.Location != item.LocationInventory && row.Location != item.LocationPaperdoll {
			continue
		}
		if inv.ItemByObjectID(row.ObjectID) == nil {
			t.Fatalf("item row %d (template %d, count %d) is not in the session's inventory", row.ObjectID, row.TemplateID, row.Count)
		}
	}
	srv.FlushPersistence(t)
	if _, ok, err := srv.Pets.Get(context.Background(), h.collarID); err != nil || ok {
		t.Fatalf("pets row after decay: present=%v err=%v, want it deleted", ok, err)
	}
}
