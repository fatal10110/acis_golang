package admin

import (
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	limitsMonsterID = 31931
	limitsFolkID    = 31932
)

// limitsTemplates are a monster with Divine Inspiration 2 and a merchant.
func limitsTemplates() (*npc.Template, *npc.Template) {
	monster := &npc.Template{
		ID: limitsMonsterID, TemplateID: limitsMonsterID, Type: "Monster", Level: 1, HPMax: 1000, MPMax: 100,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		Skills: map[int]int{int(modelskill.DivineInspirationSkillID): 2},
	}
	folk := &npc.Template{
		ID: limitsFolkID, TemplateID: limitsFolkID, Type: "Merchant", Level: 1, HPMax: 1000, MPMax: 100,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
	return monster, folk
}

// TestAdminReloadConfigNPCLimitsReachSpawnedNPCs: //reload config puts the
// reloaded MaxBuffsAmount and MaxDriftRange in force for the monster and
// the civilian the NPC population spawned before it, and for one spawned
// after, as the reference reads them on each use. At the boot 20 and 200
// the monster holds 22 buffs (its Divine Inspiration adds 2), the merchant
// 20, and both drift 200 (scripts read the same range); at 5 and 350 they
// hold 7 and 5 and drift 350.
func TestAdminReloadConfigNPCLimitsReachSpawnedNPCs(t *testing.T) {
	t.Parallel()
	monster, folk := limitsTemplates()
	reloaded := liveSettings()
	reloaded.NPCLimits = &npc.Limits{MaxBuffs: 5, DriftRange: 350}
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{monster, folk})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithDataReloads(fixedConfig(reloaded)))
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)
	h, err := srv.NpcSpawns.SpawnFixedHostile(monster, 3000, 0, 0, 0)
	if err != nil {
		t.Fatalf("spawn the monster: %v", err)
	}
	f, err := srv.NpcSpawns.SpawnFixedFolk(folk, 3100, 0, 0, 0)
	if err != nil {
		t.Fatalf("spawn the merchant: %v", err)
	}
	drain(t, gm)
	check := func(when string, h *npc.Hostile, f *npc.Folk, hostileBuffs, folkBuffs, drift int) {
		t.Helper()
		var got [5]int
		onNPCQueue(t, h, func() {
			got[0], got[1], got[2] = h.MaxBuffCount(), h.Instance.MaxDriftRange(), script.NPCOf(h).MaxDriftRange()
		})
		got[3], got[4] = f.MaxBuffCount(), f.Instance.MaxDriftRange()
		if want := [5]int{hostileBuffs, drift, drift, folkBuffs, drift}; got != want {
			t.Fatalf("%s: monster buffs, drift, script drift, merchant buffs, drift = %v, want %v", when, got, want)
		}
	}
	check("boot", h, f, 22, 20, 200)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload config")), configReloaded)
	check("reloaded", h, f, 7, 5, 350)
	later, err := srv.NpcSpawns.SpawnFixedHostile(monster, 3200, 0, 0, 0)
	if err != nil {
		t.Fatalf("spawn a second monster: %v", err)
	}
	check("spawned after the reload", later, f, 7, 5, 350)
}

// TestAdminReloadConfigNPCLimitsRaceFree: an NPC reading its limits while
// another game master's //reload config swaps them back and forth reads
// one set whole, never the buff count of one and the drift range of the
// other.
func TestAdminReloadConfigNPCLimitsRaceFree(t *testing.T) {
	t.Parallel()
	monster, folk := limitsTemplates()
	off, on := liveSettings(), liveSettings()
	off.NPCLimits = &npc.Limits{MaxBuffs: 20, DriftRange: 200}
	on.NPCLimits = &npc.Limits{MaxBuffs: 5, DriftRange: 350}
	var calls atomic.Int32
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{monster, folk})),
		gameservertest.WithNpcSpawns(nil),
		flipReloads(&calls, off, on))
	enterWorld(t, srv.Client)
	drain(t, srv.Client)
	h, err := srv.NpcSpawns.SpawnFixedHostile(monster, 3000, 0, 0, 0)
	if err != nil {
		t.Fatalf("spawn the monster: %v", err)
	}
	gm2, _ := addPlayer(t, srv, "gm2", "Reloader", adminLevel)

	const rounds = 20
	sent := reloadsInBackground(gm2, rounds)
	for i := range 50 {
		var got npc.Limits
		onNPCQueue(t, h, func() {
			got = h.Instance.Limits.Limits()
			_ = h.MaxBuffCount() + h.Instance.MaxDriftRange()
		})
		if got != *off.NPCLimits && got != *on.NPCLimits {
			t.Fatalf("read %d: limits %+v, want %+v or %+v", i, got, *off.NPCLimits, *on.NPCLimits)
		}
	}
	awaitReloads(t, srv, &calls, rounds, sent)
}
