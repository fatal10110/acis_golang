package manager

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// linkedToDeadMaster returns the privates in state still linked to a dead
// master.
func linkedToDeadMaster(state *world.State) []*npc.Hostile {
	var out []*npc.Hostile
	for _, obj := range state.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.IsPrivate() {
			if m := h.Master(); m != nil && m.Dead() {
				out = append(out, h)
			}
		}
	}
	return out
}

// placingIDs hands out the ids of the NPCs placed, running hook first: a
// seam between a private's request and its link to its master.
type placingIDs struct {
	idAllocator
	hook func()
}

func (p *placingIDs) NextID() (int32, error) {
	p.hook()
	return p.idAllocator.NextID()
}

// TestPrivateWhoseMasterDiesMeanwhileIsLetGo pins #5156 in order: a
// private asked for while its master is alive, whose master dies while it
// is placed, after the death let the master's privates go, is let go too,
// so it follows no life of the master. One asked for a master already
// dead, as its own dying hooks do, is linked to it as before.
func TestPrivateWhoseMasterDiesMeanwhileIsLetGo(t *testing.T) {
	w := newScriptRespawnWorld(t)
	tmpl, _ := w.npcs.templates.Get(2)
	w.npcs.ids = &placingIDs{idAllocator: w.npcs.ids, hook: func() { w.master.Kill(nil) }}
	private, err := w.npcs.createPrivate(w.master, tmpl, nil, 0, 0, [3]int32{}, 0)
	if err != nil || private == nil {
		t.Fatalf("createPrivate() = %v, %v, want a private", private, err)
	}
	if !w.master.Dead() {
		t.Fatal("the master is alive, want it dead while its private was placed")
	}
	if m := private.Master(); m != nil {
		t.Fatalf("the late private's master = %d, want none: its master died meanwhile", m.ObjectID())
	}
	if !private.IsPrivate() {
		t.Fatal("the late private is no private, want one let go")
	}
	if got := linkedToDeadMaster(w.state); len(got) != 0 {
		t.Fatalf("%d privates linked to the dead master, want none", len(got))
	}

	after, err := w.npcs.createPrivate(w.master, tmpl, nil, 0, 0, [3]int32{}, 0)
	if err != nil || after == nil {
		t.Fatalf("createPrivate() for the dead master = %v, %v, want a private", after, err)
	}
	if m := after.Master(); m == nil || m.ObjectID() != w.master.ObjectID() {
		t.Fatalf("the dead master's private's master = %v, want the master", m)
	}
}

// TestPrivateCreatedWhileItsMasterDiesIsLetGo pins #5156 under a race: a
// script creating a private for a master that dies meanwhile on another
// goroutine never leaves the private linked to the dead master unless the
// death had let the master's privates go before the private was asked for,
// whichever of the two links or lets go first.
func TestPrivateCreatedWhileItsMasterDiesIsLetGo(t *testing.T) {
	for range 300 {
		w := newScriptRespawnWorld(t)
		tmpl, _ := w.npcs.templates.Get(2)
		// A master whose death had not let its privates go as the
		// private is placed had not when the private was asked for.
		var letGoAtPlacing bool
		w.npcs.ids = &placingIDs{idAllocator: w.npcs.ids, hook: func() { letGoAtPlacing = w.master.PrivatesLetGo() }}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			w.master.Kill(nil)
		}()
		var (
			private   npc.PartyMember
			createErr error
		)
		go func() {
			defer wg.Done()
			<-start
			private, createErr = w.npcs.createPrivate(w.master, tmpl, nil, 0, 0, [3]int32{}, 0)
		}()
		close(start)
		wg.Wait()
		if createErr != nil || private == nil {
			t.Fatalf("createPrivate() = %v, %v, want a private", private, createErr)
		}
		if m := private.Master(); !letGoAtPlacing && m != nil && m.Dead() {
			t.Fatal("a private asked for before its master's death let its privates go is linked to the dead master")
		}
		for _, m := range w.master.Minions() {
			if !npc.SameNPC(m, private) && m.Master() != nil {
				t.Fatal("the master's declared private is still linked to the dead master")
			}
		}
	}
}

// partyDiedHooks runs onPartyDied for every party-died hook raised; every
// other hook does nothing.
type partyDiedHooks struct {
	npc.ScriptHooks
	onPartyDied func(caller, called npc.PartyMember)
}

func (h partyDiedHooks) PartyDied(caller, called npc.PartyMember)      { h.onPartyDied(caller, called) }
func (partyDiedHooks) SeesCreatures(int32) bool                        { return false }
func (partyDiedHooks) HostileCreated(*npc.Hostile)                     {}
func (partyDiedHooks) HostileNoDesire(*npc.Hostile)                    {}
func (partyDiedHooks) HostileDying(*npc.Hostile, attackable.Combatant) {}
func (partyDiedHooks) ClanDied(_, _, _ attackable.Combatant)           {}

// TestPrivateAskedForBeforeItsDeadMasterLetsGoIsLetGo pins #5156 for a
// private asked for on another queue while its master dies: the master is
// already dead, its death runs its party-died hooks, and lets its privates
// go before the private is linked. The private, asked for before that
// death let the privates go, is let go too, so it neither stays linked to
// the dead master nor follows its next life.
func TestPrivateAskedForBeforeItsDeadMasterLetsGoIsLetGo(t *testing.T) {
	w := newScriptRespawnWorld(t)
	tmpl, _ := w.npcs.templates.Get(2)
	placing, proceed := make(chan struct{}), make(chan struct{})
	ids := w.npcs.ids
	var (
		once      sync.Once
		master    npc.PartyMember
		private   npc.PartyMember
		createErr error
		done      = make(chan struct{})
	)
	w.npcs.scripts = partyDiedHooks{onPartyDied: func(_, called npc.PartyMember) {
		once.Do(func() {
			if !called.Dead() || called.PrivatesLetGo() {
				t.Error("the party-died hooks run before the dead master lets its privates go")
			}
			// Another queue asks for a private now, and links it only
			// once the master's death let its privates go.
			w.npcs.ids = &placingIDs{idAllocator: ids, hook: func() {
				close(placing)
				<-proceed
			}}
			go func() {
				defer close(done)
				private, createErr = w.npcs.createPrivate(called, tmpl, nil, 0, 0, [3]int32{}, 0)
			}()
			<-placing
		})
	}}
	live, err := w.npcs.AddSpawn(1, 0, 0, 0, 0, false, 0)
	if err != nil {
		t.Fatalf("AddSpawn() error: %v", err)
	}
	master = live.(*npc.Hostile)
	if err := w.npcs.CreatePrivates(master); err != nil {
		t.Fatalf("CreatePrivates() error: %v", err)
	}
	master.(*npc.Hostile).Kill(nil)
	if !master.PrivatesLetGo() {
		t.Fatal("the dead master's death has not let its privates go")
	}
	close(proceed)
	<-done
	w.npcs.ids = ids
	if createErr != nil || private == nil {
		t.Fatalf("createPrivate() = %v, %v, want a private", private, createErr)
	}
	if m := private.Master(); m != nil {
		t.Fatalf("the late private's master = %d, want none: it was asked for before its master let its privates go", m.ObjectID())
	}
	if !private.IsPrivate() {
		t.Fatal("the late private is no private, want one let go")
	}

	after, err := w.npcs.createPrivate(master, tmpl, nil, 0, 0, [3]int32{}, 0)
	if err != nil || after == nil {
		t.Fatalf("createPrivate() after the death = %v, %v, want a private", after, err)
	}
	if m := after.Master(); m == nil || m.ObjectID() != master.ObjectID() {
		t.Fatalf("a private asked for after the death let the privates go has master %v, want the dead master (#5512)", m)
	}
}

// TestMakerDeleteTakesAPrivateCreatedMeanwhile pins #5156 for an npcmaker's
// delete: a private a script creates for one of the maker's NPCs while the
// maker deletes them does not outlive the delete.
func TestMakerDeleteTakesAPrivateCreatedMeanwhile(t *testing.T) {
	for range 200 {
		f := newDespawnFixture(t, nil)
		master := f.hostiles(1)[0]
		tmpl, _ := f.templates.Get(2)
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			f.npcs.DeleteMakerNPCs("wolves")
		}()
		var createErr error
		go func() {
			defer wg.Done()
			<-start
			_, createErr = f.npcs.createPrivate(master, tmpl, nil, 0, 0, [3]int32{}, 0)
		}()
		close(start)
		wg.Wait()
		f.queues.Run()
		if createErr != nil {
			t.Fatalf("createPrivate() error: %v", createErr)
		}
		for _, obj := range f.npcObjects() {
			if h, ok := obj.(*npc.Hostile); ok && h.IsPrivate() {
				t.Fatalf("private %d of the deleted maker's NPC is still in the world", h.ObjectID())
			}
		}
		f.npcs.mu.Lock()
		live := f.npcs.liveCount
		f.npcs.mu.Unlock()
		if live != 1 {
			t.Fatalf("live NPCs after the delete = %d, want 1: the lair boss", live)
		}
	}
}
