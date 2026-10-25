package ai

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/core"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Core, the private it leads and the teleport cube it leaves.
const (
	coreID      = int32(29006)
	coreGuardID = int32(29993)
	coreCubeID  = int32(31842)
)

// bootCore boots Core's behavior beside a character, with Core leading
// two privates that come back only when asked, and every script draw the
// highest its bound allows. The scenarios of testdata/scenarios cover its
// lines, sounds and skill desires; these tests cover what the scenario
// format cannot place or show: its template privates and the cubes it
// spawns.
func bootCore(t *testing.T) *warriorWorld {
	t.Helper()
	boss := warriorTemplate(coreID, nil)
	boss.Type, boss.Name, boss.CanMove = "GrandBoss", "Core", false
	boss.Privates = []npc.PrivateEntry{
		{NpcID: int(coreGuardID), Weight: 1},
		{NpcID: int(coreGuardID), Weight: 1},
	}
	private := warriorTemplate(coreGuardID, nil)
	private.CanMove = false
	kinds := map[int32]script.NPCKind{coreID: script.KindHostile, coreGuardID: script.KindHostile, coreCubeID: script.KindFolk}
	list := []script.Listing{{Path: "script.ai.individual.DefaultNpc"}, {Path: "script.ai.boss.core.Core"}}
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc": defaultnpc.New,
		"script.ai.boss.core.Core":        core.Core,
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{boss, private, gameservertest.FolkTemplate("Folk", int(coreCubeID))})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithScriptRand(func(n int) int { return n - 1 }),
		gameservertest.WithNPCScripts(kinds, list, catalog),
	)
	c := srv.Client
	c.Send(encodeGameStart())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("no CharSelected within 100 frames")
		}
		if c.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	srv.ReadQueued(t, c)
	id := srv.SoleObjectID(t)
	x, y, z := srv.PlayerPosition(t, id)
	obj, ok := srv.State.Object(id)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	return &warriorWorld{srv: srv, at: location.Location{X: x, Y: y, Z: z}, player: obj.(attackable.Combatant)}
}

// coreCubes returns where each teleport cube in the world stands.
func coreCubes(w *warriorWorld) []string {
	var out []string
	for _, obj := range w.srv.State.Objects() {
		if f, ok := obj.(*npc.Folk); ok && int32(f.Instance.Template.ID) == coreCubeID && !f.Decayed() {
			out = append(out, placeOf(f))
		}
	}
	return out
}

// TestCoreBringsItsPrivatesBack covers Core's created and party-died
// hooks: it appears leading its two privates, and a private dying while
// Core lives is asked back after 280 s plus a draw of 40, here the highest
// (39): 319 s. Once Core is dead, its other dying private is asked nothing.
func TestCoreBringsItsPrivatesBack(t *testing.T) {
	t.Parallel()
	w := bootCore(t)
	boss := w.spawn(t, coreID, 400, 0)
	privates := privatesOf(t, boss, 2)

	before := time.Now()
	w.kill(t, privates[0])
	after := time.Now()
	key := w.armedPrivateRespawn(t)
	deadline, ok := w.srv.NpcRespawns.Deadline(key)
	if !ok {
		t.Fatalf("no deadline for the armed respawn %s", key)
	}
	const delay = 319 * time.Second
	if deadline.Before(before.Add(delay)) || deadline.After(after.Add(delay)) {
		t.Fatalf("respawn due %s after the death, want %s", deadline.Sub(before), delay)
	}

	w.kill(t, boss)
	w.kill(t, privates[1])
	if got := w.armedPrivateRespawn(t); got != key {
		t.Fatalf("respawn armed for %s after Core died, want only the first private's %s", got, key)
	}
}

// TestCoreLeavesTwoCubes covers the spawns of Core's dying hook: 3 s after
// its death it leaves a teleport cube at 16502,110165,-6394 and another at
// 18948,110166,-6397, each placed 20 above the ground height there (the
// fixture world has no geodata, so 20 above the asked z), and each leaving
// the world 900 s after it appeared.
func TestCoreLeavesTwoCubes(t *testing.T) {
	t.Parallel()
	w := bootCore(t)
	boss := w.spawn(t, coreID, 400, 0)

	w.onQueue(t, boss, func() { script.NPCOf(boss).DoDie(script.PlayerOf(w.player)) })
	w.srv.ReadQueued(t, w.srv.Client)
	if got := coreCubes(w); len(got) != 0 {
		t.Fatalf("cubes at the death = %q, want none before the dying hook", got)
	}
	w.srv.Advance(t, 3*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	want := []string{"16502,110165,-6374 heading 0", "18948,110166,-6377 heading 0"}
	got := coreCubes(w)
	if len(got) != len(want) || !(got[0] == want[0] && got[1] == want[1] || got[0] == want[1] && got[1] == want[0]) {
		t.Fatalf("cubes after the dying hook = %q, want %q", got, want)
	}

	w.srv.Advance(t, 899*time.Second)
	if got := coreCubes(w); len(got) != 2 {
		t.Fatalf("cubes 899 s after they appeared = %q, want both", got)
	}
	w.srv.Advance(t, time.Second)
	if got := coreCubes(w); len(got) != 0 {
		t.Fatalf("cubes 900 s after they appeared = %q, want none", got)
	}
}
