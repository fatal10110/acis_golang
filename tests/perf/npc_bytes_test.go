package perf

import (
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const idleBytesMonsterID = 31941

// TestIdleNPCBytes spawns ACIS_PERF_NPCS idle monsters through the NPC
// population's production spawn path (cast skills loaded, as the game
// server loads them) and reports the live heap each one holds: the heap in
// use after a full collection, before and after the spawns, over the count.
// No AI or other task ticks, so every NPC stays as idle as one no player
// ever saw. With ACIS_PERF_PROFILE_DIR set it also writes idle_npcs.pprof,
// the heap profile with the NPCs live.
func TestIdleNPCBytes(t *testing.T) {
	count := envInt(t, "ACIS_PERF_NPCS", 0)
	if count <= 0 {
		t.Skip("set ACIS_PERF_NPCS to measure the idle NPC heap")
	}
	if os.Getenv("ACIS_PERF_PROFILE_DIR") != "" {
		// Record every allocation, so the profile attributes each NPC's
		// small objects to their sites.
		defer func(rate int) { runtime.MemProfileRate = rate }(runtime.MemProfileRate)
		runtime.MemProfileRate = 1
	}
	monster := &npc.Template{
		ID: idleBytesMonsterID, TemplateID: idleBytesMonsterID, Type: "Monster", Level: 20, HPMax: 1000, MPMax: 100,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		Skills: map[int]int{4136: 1},
	}
	cast := modelskill.Definition{
		ID: 4136, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		SkillType: "BUFF", CastRange: 600, HitTime: 500, StaticHitTime: true, StaticReuse: true,
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{monster})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable([]modelskill.Definition{cast})),
	)
	// Warm the shared per-template state with one NPC first.
	if _, err := srv.NpcSpawns.SpawnFixedHostile(monster, 0, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	before := liveHeap()
	spawned := make([]*npc.Hostile, count)
	for i := range spawned {
		h, err := srv.NpcSpawns.SpawnFixedHostile(monster, 200+i%100*40, 200+i/100*40, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		spawned[i] = h
	}
	after := liveHeap()
	if dir := os.Getenv("ACIS_PERF_PROFILE_DIR"); dir != "" {
		writeHeapProfile(t, filepath.Join(dir, "idle_npcs.pprof"))
	}
	runtime.KeepAlive(spawned)
	t.Logf("idle NPCs=%d heap_in_use_delta=%d bytes_per_npc=%d", count, after-before, (after-before)/uint64(count))
}

// liveHeap is the heap in use after a full collection.
func liveHeap() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

func writeHeapProfile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := pprof.Lookup("heap").WriteTo(f, 0); err != nil {
		t.Fatal(err)
	}
}
