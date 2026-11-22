package npc

import (
	"sync"
	"testing"
	"unsafe"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// TestHeapLayoutSizes logs the per-NPC struct sizes the heap profile
// tracks; it guards nothing on its own.
func TestHeapLayoutSizes(t *testing.T) {
	t.Logf("Hostile=%d Folk=%d Scratch=%d", unsafe.Sizeof(Hostile{}), unsafe.Sizeof(Folk{}), unsafe.Sizeof(Scratch{}))
}

// A life no skill touched keeps no spoil, seed, over-hit, escort or party
// state: every read of it, an HP loss and a respawn reset leave it unmade.
func TestColdHostileReadsMakeNoLifeState(t *testing.T) {
	h := partyHostile(t, 1, 0, &hostileMove{})
	spawnPartyWorld(t, h)

	if h.Spoiled() || h.SpoilerID() != 0 || h.Sweepable() || h.ExistingSpoilPool() != nil {
		t.Fatal("cold life reads as spoiled")
	}
	if h.Seeded() || h.SeedBlocksDrops() {
		t.Fatal("cold life reads as sown")
	}
	if h.OverhitValid(h) || h.OverhitBonus(1000) != 0 {
		t.Fatal("cold life reads as over-hit")
	}
	h.testOverhit(h, 10)
	if h.IsMaster() || h.Master() != nil || h.IsPrivate() || h.SpawnedPrivates() || len(h.Minions()) != 0 {
		t.Fatal("cold life reads as in a party")
	}
	if h.Scratch().Creature(CreatureAI0) != nil {
		t.Fatal("cold creature slot is set")
	}
	h.Scratch().SetCreature(CreatureAI0, nil)
	h.resetForSpawn()

	if h.loot.Load() != nil {
		t.Error("loot state made by a read or reset")
	}
	if h.escortLinks.p.Load() != nil {
		t.Error("escort links made by a read or reset")
	}
	if h.Scratch().membership.Load() != nil {
		t.Error("party membership made by a read")
	}
	if h.Scratch().creatures != nil {
		t.Error("creature slots made by a read or an empty write")
	}
}

// A respawn clears the spoil, seed and over-hit of the last life in place.
func TestRespawnClearsLifeLootInPlace(t *testing.T) {
	h := partyHostile(t, 1, 0, &hostileMove{})
	spawnPartyWorld(t, h)
	pool := h.SpoilPool()
	pool.Mark(7)
	pool.Put(57, 3)
	h.SeedState().Sow(7, manor.Seed{CropID: 5073, Level: 1})
	h.EnableOverhit()
	h.testOverhit(h, h.HP()+50)
	if !h.OverhitValid(h) || !h.Sweepable() || !h.Seeded() {
		t.Fatal("setup: life not spoiled, sown and over-hit")
	}

	h.resetForSpawn()

	if h.Spoiled() || h.Sweepable() || h.Seeded() || h.OverhitValid(h) || h.OverhitBonus(1000) != 0 {
		t.Fatal("respawned life kept the last life's spoil, seed or over-hit")
	}
	if h.SpoilPool() != pool {
		t.Fatal("respawn replaced the spoil state instead of clearing it")
	}
}

// Two attackers spoiling, sowing and arming an over-hit on a cold NPC at
// once share one state: one mark and one seed take, and both see the same
// pool.
func TestColdHostileConcurrentFirstWrites(t *testing.T) {
	seed := manor.Seed{CropID: 5073, Level: 1}
	for range 100 {
		h := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", Level: 20, HPMax: 1000})
		var (
			wg           sync.WaitGroup
			pools        [2]*item.SpoilPool
			marked, sown [2]bool
		)
		for i := range 2 {
			wg.Go(func() {
				pools[i] = h.SpoilPool()
				marked[i] = pools[i].Mark(int32(10 + i))
				h.EnableOverhit()
				sown[i] = h.SeedState().Sow(int32(10+i), seed)
			})
		}
		wg.Wait()
		if pools[0] != pools[1] || pools[0] != h.ExistingSpoilPool() {
			t.Fatal("attackers got different spoil states")
		}
		if marked[0] == marked[1] || sown[0] == sown[1] {
			t.Fatalf("marks taken = %v, seeds taken = %v, want exactly one each", marked, sown)
		}
		h.testOverhit(h, h.HP()+10)
		if !h.OverhitValid(h) {
			t.Fatal("over-hit armed by both attackers is not armed")
		}
	}
}

// escortParty spawns a master with three privates that hold escort slots.
func escortParty(t *testing.T) (master *Hostile, minions []*Hostile) {
	t.Helper()
	master = partyHostile(t, 1, 2, &hostileMove{})
	state := world.New()
	master.Attach(Runtime{World: state})
	state.Spawn(master, 1000, 1000, 0, 0)
	for i := range 3 {
		m := partyHostile(t, int32(2+i), 1, &hostileMove{})
		m.Attach(Runtime{World: state})
		state.Spawn(m, 1150, 1000+i*10, 0, 0)
		master.AddMinion(m)
		m.SetMaster(master)
		m.roll = func(int) int { return 0 }
		minions = append(minions, m)
	}
	return master, minions
}

// An escort step of a private reads its master's privates without
// copying them: a claim on an ordinary party allocates nothing.
func TestClaimFollowSlotDoesNotAllocate(t *testing.T) {
	master, minions := escortParty(t)
	for _, m := range minions {
		claimFollowSlot(master, m, true, false, masterLocOf(master))
	}
	m := minions[0]
	loc := masterLocOf(master)
	allocs := testing.AllocsPerRun(200, func() {
		claimFollowSlot(master, m, true, false, loc)
	})
	if allocs != 0 {
		t.Fatalf("claimFollowSlot allocs = %v, want 0", allocs)
	}
}

func masterLocOf(h *Hostile) (loc location.Location) { return h.location() }

func BenchmarkNewHostile(b *testing.B) {
	tpl := &Template{ID: 1, Type: "Monster", Level: 20, HPMax: 1000}
	b.ReportAllocs()
	for b.Loop() {
		newCombatHostile(b, 1, tpl)
	}
}

var scratchSink *Scratch

func BenchmarkNewScratch(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		scratchSink = NewScratch()
	}
}

func BenchmarkClaimFollowSlot(b *testing.B) {
	t := &testing.T{}
	master, minions := escortParty(t)
	loc := masterLocOf(master)
	b.ReportAllocs()
	for b.Loop() {
		claimFollowSlot(master, minions[0], true, false, loc)
	}
}
