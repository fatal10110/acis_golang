package npcs

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	regenMonsterID = 31901
	regenRaidID    = 31902
	regenFolkID    = 31903
)

// npcRegenRates are the HP and MP regeneration rates of a monster, a raid
// boss and a civilian NPC spawned through the server's NPC population.
type npcRegenRates struct {
	monsterHP, monsterMP, raidHP, raidMP, folkHP, folkMP float64
}

// spawnRegenNPCs boots a server with extra and spawns a monster, a raid
// boss and a civilian NPC of the same base regeneration (3 HP, 1.5 MP)
// through its NPC population, then reads their rates.
func spawnRegenNPCs(t *testing.T, extra ...gameservertest.Option) npcRegenRates {
	t.Helper()
	monster := &npc.Template{
		ID: regenMonsterID, TemplateID: regenMonsterID, Type: "Monster", Level: 1, HPMax: 1000, MPMax: 100,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		HPRegen: 3, MPRegen: 1.5,
	}
	raid := raidTemplate(regenRaidID)
	raid.MPMax, raid.HPRegen, raid.MPRegen = 100, 3, 1.5
	folk := folkTemplate("Folk", regenFolkID)
	folk.HPRegen, folk.MPRegen = 3, 1.5
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{monster, raid, folk})),
		gameservertest.WithNpcSpawns(nil),
	}, extra...)...)
	if err := srv.NpcSpawns.SpawnFixed(monster, 1000, 0, 0, 0, 0); err != nil {
		t.Fatalf("spawn the monster: %v", err)
	}
	if err := srv.NpcSpawns.SpawnFixed(raid, 2000, 0, 0, 0, 0); err != nil {
		t.Fatalf("spawn the raid boss: %v", err)
	}
	f, err := srv.NpcSpawns.SpawnFixedFolk(folk, 3000, 0, 0, 0)
	if err != nil {
		t.Fatalf("spawn the civilian: %v", err)
	}
	hostile := func(id int) *npc.Hostile {
		for _, obj := range srv.State.Objects() {
			if h, ok := obj.(*npc.Hostile); ok && h.NpcID() == id {
				return h
			}
		}
		t.Fatalf("npc %d not spawned", id)
		return nil
	}
	m, r := hostile(regenMonsterID), hostile(regenRaidID)
	if m.RaidRelated() || !r.RaidRelated() {
		t.Fatalf("monster raid related %v, raid boss %v; want false, true", m.RaidRelated(), r.RaidRelated())
	}
	return npcRegenRates{
		monsterHP: m.HPRegenRate(), monsterMP: m.MPRegenRate(),
		raidHP: r.HPRegenRate(), raidMP: r.MPRegenRate(),
		folkHP: f.HPRegenRate(), folkMP: f.MPRegenRate(),
	}
}

// TestRegenMultipliersScaleNPCRegen: players.properties' HpRegenMultiplier
// and MpRegenMultiplier multiply the base regeneration of every NPC that is
// not raid related, a monster and a civilian alike, before the stat
// (CreatureStatus.getRegenHp/getRegenMp). A raid boss takes the raid
// multipliers in their place, so the plain ones leave its rates alone.
func TestRegenMultipliersScaleNPCRegen(t *testing.T) {
	t.Parallel()
	// Each subtest boots its own server; they run in order so the second
	// compares against the shipped rates the first reads.
	var base, got npcRegenRates
	t.Run("shipped", func(t *testing.T) { base = spawnRegenNPCs(t) })
	t.Run("multiplied", func(t *testing.T) { got = spawnRegenNPCs(t, gameservertest.WithRegenMultipliers(2, 3, 5)) })
	if t.Failed() {
		return
	}
	if base.monsterHP <= 0 || base.monsterMP <= 0 || base.folkHP <= 0 || base.folkMP <= 0 || base.raidHP <= 0 || base.raidMP <= 0 {
		t.Fatalf("shipped rates = %+v, want every rate positive", base)
	}
	want := npcRegenRates{
		monsterHP: base.monsterHP * 2, monsterMP: base.monsterMP * 3,
		raidHP: base.raidHP, raidMP: base.raidMP,
		folkHP: base.folkHP * 2, folkMP: base.folkMP * 3,
	}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	if !near(got.monsterHP, want.monsterHP) || !near(got.monsterMP, want.monsterMP) ||
		!near(got.raidHP, want.raidHP) || !near(got.raidMP, want.raidMP) ||
		!near(got.folkHP, want.folkHP) || !near(got.folkMP, want.folkMP) {
		t.Fatalf("rates at multipliers 2/3 = %+v, want %+v (shipped %+v)", got, want, base)
	}
}
