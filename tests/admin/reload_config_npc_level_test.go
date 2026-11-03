package admin

import (
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const levelMonsterID = 31941

// npcInfoTitles decodes the title of every NpcInfo for objID in frames.
func npcInfoTitles(frames [][]byte, objID int32) []string {
	var titles []string
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeNPCInfo {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != objID {
			continue
		}
		for range 17 { // npc id .. walk speeds
			r.ReadInt32()
		}
		for range 4 { // speed multipliers, collision radius and height
			r.ReadFloat64()
		}
		for range 3 { // right hand, chest, left hand
			r.ReadInt32()
		}
		for range 5 { // name shown .. summon animation
			r.ReadUint8()
		}
		r.ReadString()
		titles = append(titles, r.ReadString())
	}
	return titles
}

// refreshedTitle has the monster h show itself again and returns the title
// of the NpcInfo c is sent.
func refreshedTitle(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, h *npc.Hostile) string {
	t.Helper()
	onNPCQueue(t, h, h.UpdateAbnormalEffect)
	srv.Settle(t)
	titles := npcInfoTitles(c.ReadQueued(), h.ObjectID())
	if len(titles) != 1 {
		t.Fatalf("refresh sent %d NpcInfo titles %q, want one", len(titles), titles)
	}
	return titles[0]
}

// levelMonster spawns an aggressive level 33 monster titled "Elite" beside
// objID.
func levelMonster(t *testing.T, srv *gameservertest.Server, objID int32) *npc.Hostile {
	t.Helper()
	x, y, z := srv.PlayerPosition(t, objID)
	tmpl := &npc.Template{
		ID: levelMonsterID, TemplateID: levelMonsterID, Type: "Monster", Level: 33, HPMax: 1000, AtkSpd: 300,
		CollisionRadius: 8, CollisionHeight: 20, AggroRange: 300, Title: "Elite", UsingServerSideTitle: true,
	}
	return srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: x - 100, Y: y, Z: z})
}

// TestAdminReloadConfigShowNPCLevel: once //reload config turns
// ShowNpcLevel on, the next NpcInfo a monster is shown with starts with its
// level, as the reference reads SHOW_NPC_LVL on each NpcInfo; the one sent
// before keeps its plain title until then.
func TestAdminReloadConfigShowNPCLevel(t *testing.T) {
	t.Parallel()
	reloaded := liveSettings()
	reloaded.ShowNPCLevel = true
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithDataReloads(fixedConfig(reloaded)))
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)
	h := levelMonster(t, srv, gmID)
	srv.Settle(t)
	if got := npcInfoTitles(gm.ReadQueued(), h.ObjectID()); len(got) != 1 || got[0] != "Elite" {
		t.Fatalf("boot NpcInfo titles = %q, want [Elite]", got)
	}

	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload config")), configReloaded)
	if got := refreshedTitle(t, srv, gm, h); got != "Lv 33* Elite" {
		t.Fatalf("reloaded NpcInfo title = %q, want %q", got, "Lv 33* Elite")
	}
}

// TestAdminReloadConfigShowNPCLevelRaceFree: NpcInfo built while another
// game master's //reload config turns ShowNpcLevel on and off carry either
// title whole.
func TestAdminReloadConfigShowNPCLevelRaceFree(t *testing.T) {
	t.Parallel()
	on := liveSettings()
	on.ShowNPCLevel = true
	var calls atomic.Int32
	srv, gmID := bootAdmin(t, adminLevel, flipReloads(&calls, liveSettings(), on))
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)
	h := levelMonster(t, srv, gmID)
	srv.Settle(t)
	drain(t, gm)
	gm2, _ := addPlayer(t, srv, "gm2", "Reloader", adminLevel)
	drain(t, gm)

	const rounds = 20
	sent := reloadsInBackground(gm2, rounds)
	for i := range 10 {
		if got := refreshedTitle(t, srv, gm, h); got != "Elite" && got != "Lv 33* Elite" {
			t.Fatalf("refresh %d NpcInfo title = %q, want %q or %q", i, got, "Elite", "Lv 33* Elite")
		}
	}
	awaitReloads(t, srv, &calls, rounds, sent)
}
