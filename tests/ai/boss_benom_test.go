package ai

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/benom"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/raid"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// Benom, the exit teleporter it leaves, Rune castle's Dungeon Gatekeeper
// and Rune castle.
const (
	benomID         = int32(29054)
	benomCubicID    = int32(29055)
	benomGatekeeper = int32(35506)
	runeCastleID    = 8
)

// Benom's skills, as its template lists them, and the Instant Move its
// skill-finished hook knows by id.
const (
	benomPhysical    = modelskill.ID(4993)
	benomMagic       = modelskill.ID(4994)
	benomCancel      = modelskill.ID(4995)
	benomRangeCancel = modelskill.ID(4996)
	benomInstantMove = modelskill.ID(4222)
)

// benomThrone is where a siege moves Benom; benomPoint the second and
// third points its cancels send their target to.
var (
	benomThrone = location.Location{X: 11563, Y: -49152, Z: -537}
	benomPoint1 = location.Location{X: 14878, Y: -51339, Z: 1024}
	benomPoint2 = location.Location{X: 15674, Y: -49970, Z: 864}
)

// benomSkills are Benom's fixture skills: its physical special and magic,
// one-target attacks, and its two cancels and the Instant Move, cast on
// itself.
func benomSkills() []modelskill.Definition {
	return []modelskill.Definition{
		ind1Skill(benomPhysical, 2000, false), ind1Skill(benomMagic, 2000, false),
		ind1Skill(benomCancel, 0, true), ind1Skill(benomRangeCancel, 0, true), ind1Skill(benomInstantMove, 0, true),
	}
}

// bootBenom boots Benom's and its gatekeeper's behaviors beside the
// character, Benom a level 20 raid boss listing its skills by type, every
// draw 0 until a test queues others, the gatekeeper's pages each naming
// itself (the greeting with its link), and extra. The scenarios of testdata/scenarios cover Benom's
// shouts, its hate and casts out of a siege, and the gatekeeper's page and
// teleport out of a siege; these tests cover what the scenario format cannot
// hold: script events, Benom's script memory and timers, its cancels and
// Instant Move, the exit teleporter, and a siege being fought.
func bootBenom(t *testing.T, extra ...gameservertest.Option) *ind7World {
	t.Helper()
	paths := []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.individual.Monster.MonsterAI",
		"script.ai.individual.Monster.RaidBoss.RaidBossStandard",
		"script.ai.boss.benom.Benom",
		"script.ai.boss.benom.DungeonTeleporter",
		"test.Ind1Director",
	}
	catalog := script.Catalog{
		paths[0]: defaultnpc.New, paths[1]: monster.New, paths[2]: raid.BossStandard,
		paths[3]: benom.Benom, paths[4]: benom.DungeonTeleporter,
		paths[5]: func() script.Script { return script.Script{} },
	}
	var list []script.Listing
	for _, p := range paths {
		list = append(list, script.Listing{Path: p})
	}
	kinds := map[int32]script.NPCKind{benomID: script.KindHostile, benomGatekeeper: script.KindFolk}
	tmpl := rbTemplate(benomID, 20, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypePhysicalSpecial: benomPhysical, npc.SkillTypeDDMagic: benomMagic,
		npc.SkillTypeSelfRangeCancelA1: benomCancel, npc.SkillTypeSelfRangeCancelA2: benomRangeCancel,
	})
	tmpl.AggroRange = 1000 // a raid boss sees as far as its aggro range
	draws := &ind1Draws{}
	w := bootRaid(t, raidBoot{opts: append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(benomSkills())),
		gameservertest.WithScriptSkills(benomSkills()...),
		gameservertest.WithHTMLPages(map[string]string{
			"doormen/35506.htm":   `<html><body>35506.htm <a action="bypass -h npc_%objectId%_Quest DungeonTeleporter">down</a></body></html>`,
			"doormen/35506-2.htm": "<html><body>35506-2.htm</body></html>",
		}),
	}, extra...)}, tmpl, gameservertest.FolkTemplate("Folk", int(benomCubicID)), gameservertest.FolkTemplate("CastleDoorman", int(benomGatekeeper)))
	return &ind7World{ind1World: &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}}
}

// benomSays renders, in order, the NpcSay frames among frames.
func benomSays(t *testing.T, frames [][]byte, roles map[int32]string) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeNpcSay {
			continue
		}
		line, err := scriptcontract.Packet(f, roles)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, line)
	}
	return out
}

// benomShout renders the shout of the NpcString id by the NPC of npcID
// playing role.
func benomShout(role string, npcID, id int32) string {
	return fmt.Sprintf("S NpcSay obj=%s type=1 npc=%d text=%s", role, 1000000+npcID, scriptcontract.Quote(script.NpcString(id)))
}

// benomMemory renders Benom's script memory: its siege mark, its
// remembered point, and its marked count and hunting mark.
func benomMemory(h *npc.Hostile) string {
	return fmt.Sprintf("ai0=%d home=%d,%d,%d quest0=%d quest1=%d",
		ind1Slot(h, script.SlotIntAI0), ind1Slot(h, script.SlotIntAI1), ind1Slot(h, script.SlotIntAI2), ind1Slot(h, script.SlotIntAI3),
		ind1Slot(h, npc.IntQuest0), ind1Slot(h, npc.IntQuest1))
}

// wantBenomAt fails unless h stands on at's ground point.
func wantBenomAt(t *testing.T, when string, h *npc.Hostile, at location.Location) {
	t.Helper()
	if p := positionOf(h); p.X != at.X || p.Y != at.Y {
		t.Fatalf("%s: Benom stands at %d,%d, want %d,%d", when, p.X, p.Y, at.X, at.Y)
	}
}

// benomCubics returns the live exit teleporters.
func (w *ind7World) benomCubics() []*npc.Folk {
	var out []*npc.Folk
	for _, obj := range w.srv.State.Objects() {
		if f, ok := obj.(*npc.Folk); ok && int32(f.Instance.Template.ID) == benomCubicID {
			out = append(out, f)
		}
	}
	return out
}

// benomDies kills h with a hit of the character and returns the frames the
// character was sent.
func (w *ind7World) benomDies(t *testing.T, h *npc.Hostile) [][]byte {
	t.Helper()
	w.srv.ReadQueued(t, w.srv.Client)
	w.hit(t, h, 1<<20)
	if !h.Dead() {
		t.Fatal("Benom survived the lethal hit")
	}
	// The dying hooks run 3 s after the death.
	w.srv.Advance(t, 3500*time.Millisecond)
	return w.srv.ReadQueued(t, w.srv.Client)
}

// place teleports the character to at, from the director script, and lets
// its client appear there: the NPCs around see it arrive.
func (w *ind7World) place(t *testing.T, at location.Location) {
	t.Helper()
	id := w.player.ObjectID()
	w.srv.RunScript(t, id, "Ind1Director", func(_ *script.Script, p *script.Player) {
		p.TeleportTo(script.Loc{X: at.X, Y: at.Y, Z: at.Z}, 0)
	})
	w.srv.Settle(t)
	w.srv.Client.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	if x, y, _ := w.srv.PlayerPosition(t, id); x != at.X || y != at.Y {
		t.Fatalf("setup: the character stands at %d,%d, want %d,%d", x, y, at.X, at.Y)
	}
}

// TestBenomAppearsAndLeavesItsCubic covers Benom's created and dying hooks
// out of a siege (phases 1 and 4). It appears shouting its challenge,
// without the raid boss music or checks its parent's created hook would
// start, and remembers where it stands; its siege mark, marked count and
// hunting mark start at 0. Dying, it shouts its last line and leaves the
// exit teleporter at the dungeon's entrance, no siege event having moved
// it.
func TestBenomAppearsAndLeavesItsCubic(t *testing.T) {
	t.Parallel()
	w := bootBenom(t)
	w.draws.set(99)
	s := w.spawnFrames(t, benomID, 400, 0)
	h := s.h
	roles := map[int32]string{h.ObjectID(): "benom"}
	if got, want := benomSays(t, s.frames, roles), []string{benomShout("benom", benomID, 1010623)}; !slices.Equal(got, want) {
		t.Fatalf("NpcSay frames as Benom appears = %q, want %q", got, want)
	}
	if got := ind8Pick(s.frames, serverpackets.OpcodePlaySound); len(got) != 0 {
		t.Fatalf("PlaySound frames as Benom appears = %x, want none", got)
	}
	at := positionOf(h)
	if got, want := benomMemory(h), fmt.Sprintf("ai0=0 home=%d,%d,%d quest0=0 quest1=0", at.X, at.Y, at.Z); got != want {
		t.Fatalf("memory as Benom appears = %s, want %s", got, want)
	}
	w.srv.Advance(t, 61*time.Second)
	wantBenomAt(t, "a minute later, no raid boss check run", h, at)

	frames := w.benomDies(t, h)
	if got, want := benomSays(t, frames, roles), []string{benomShout("benom", benomID, 1010626)}; !slices.Equal(got, want) {
		t.Fatalf("NpcSay frames as Benom dies = %q, want %q", got, want)
	}
	cubics := w.benomCubics()
	if len(cubics) != 1 {
		t.Fatalf("exit teleporters after Benom's death = %d, want 1", len(cubics))
	}
	if x, y, _ := cubics[0].Position(); x != 12589 || y != -49044 {
		t.Fatalf("exit teleporter at %d,%d, want 12589,-49044", x, y)
	}
}

// TestBenomSiegeMovesItToItsThrone covers script event 10100 and Benom's
// two timers (phase 2), then its death after a siege (phase 4). The event
// moves Benom to its throne, takes its first argument as its siege mark and
// sets its hunting mark. 5 s later it remembers where it then stands (its
// parent's check of the same timer, out of its territory, draws 1 of 2 and
// 4 of 5: nothing). Every 2 minutes, strayed out of its territory and out
// of a fight, on a draw of 0 of 2 it drops its hate, goes back to its
// throne and wants to walk there at 1000000. Dying after a siege moved it,
// it shouts but leaves no exit teleporter.
func TestBenomSiegeMovesItToItsThrone(t *testing.T) {
	t.Parallel()
	w := bootBenom(t)
	w.draws.set(99)
	h := w.spawn(t, benomID, 400, 0)
	roles := map[int32]string{h.ObjectID(): "benom"}

	w.sendEvent(t, h, 10100, 1, 0)
	wantBenomAt(t, "after script event 10100", h, benomThrone)

	w.teleport(t, h, 100, 0)
	w.srv.Advance(t, 5*time.Second)
	at := positionOf(h)
	if got, want := benomMemory(h), fmt.Sprintf("ai0=1 home=%d,%d,%d quest0=0 quest1=1", benomThrone.X+100, benomThrone.Y, at.Z); got != want {
		t.Fatalf("memory 5 s after script event 10100 = %s, want %s", got, want)
	}
	wantBenomAt(t, "5 s after script event 10100, draw 1 of 2", h, location.Location{X: benomThrone.X + 100, Y: benomThrone.Y})

	w.draws.set(0)
	w.srv.Advance(t, 115*time.Second)
	wantBenomAt(t, "2 minutes after script event 10100, draw 0 of 2", h, benomThrone)
	d, ok := desire(h, actorai.IntentionMoveTo)
	if !ok || d.Weight != 1000000 || d.Location.X != benomThrone.X || d.Location.Y != benomThrone.Y {
		t.Fatalf("move desire 2 minutes after script event 10100 = %+v (%v), want the throne at 1000000", d, ok)
	}

	w.teleport(t, h, 100, 0)
	w.srv.Advance(t, 120*time.Second)
	wantBenomAt(t, "4 minutes after script event 10100", h, benomThrone)

	w.place(t, location.Location{X: benomThrone.X - 2000, Y: benomThrone.Y, Z: benomThrone.Z})
	frames := w.benomDies(t, h)
	if got, want := benomSays(t, frames, roles), []string{benomShout("benom", benomID, 1010626)}; !slices.Equal(got, want) {
		t.Fatalf("NpcSay frames as Benom dies after a siege = %q, want %q", got, want)
	}
	if got := len(w.benomCubics()); got != 0 {
		t.Fatalf("exit teleporters after Benom's death after a siege = %d, want none", got)
	}
}

// TestBenomCancelsAfterASiege covers Benom's see-creature, attacked and
// skill-finished hooks after a siege moved it (phase 3). Seeing the
// character, it wants to attack it at 200 and, on a draw of 0 of 3, marks
// it. A hit draws its cancel chance (0 of 625): it wants its cancel on
// itself, unchecked; the cast done, Benom, the cancel's target, is sent to
// the point drawn (2 of 12). Under a third of its HP, a hit draws its range
// cancel chance after the cancel's (99, then 0), then from over 300 its
// magic chance and its physical special chance (99 each); the range cancel
// done, Benom is sent to the point drawn (1 of 12), where the marked
// character stands within 250 of it: Benom is sent away again (1 of 12)
// and its hate of the character drops; its marks are cleared. Its Instant
// Move then brings it back to where it stood 5 s after the siege moved it,
// dropping its hate.
func TestBenomCancelsAfterASiege(t *testing.T) {
	t.Parallel()
	w := bootBenom(t)
	w.draws.set(99)
	h := w.spawn(t, benomID, 400, 0)
	self := h.ObjectID()
	w.sendEvent(t, h, 10100, 1, 0)
	w.srv.Advance(t, 5*time.Second)
	wantBenomAt(t, "5 s after script event 10100", h, benomThrone)

	w.place(t, location.Location{X: benomThrone.X - 100, Y: benomThrone.Y, Z: benomThrone.Z})
	// Benom looks around at most once every 30 s.
	w.srv.Advance(t, 30*time.Second)
	w.draws.set(99, 0)
	w.onQueue(t, h, func() { script.NPCOf(h).LookNeighbor() })
	if got := w.attackWeight(h); got != 200 {
		t.Fatalf("attack desire on the character seen = %v, want 200", got)
	}
	if got := ind1Slot(h, npc.IntQuest0); got != 1 {
		t.Fatalf("marked count after seeing the character = %d, want 1", got)
	}

	w.draws.set(99, 0, 99)
	w.hit(t, h, 22)
	w.wantCasts(t, "after the hit, draw 0 of 625", h, map[modelskill.ID]ind1Cast{benomCancel: {self, 1000000}})
	w.draws.set(2)
	w.castQueued(t, h)
	wantBenomAt(t, "after its cancel, draw 2 of 12", h, benomPoint2)

	w.place(t, location.Location{X: benomPoint1.X - 100, Y: benomPoint1.Y, Z: benomPoint1.Z})
	w.setHPShare(t, h, 0.3)
	w.draws.set(99, 99, 0, 99, 99)
	w.hit(t, h, 22)
	w.wantCasts(t, "after the hit under a third of its HP", h, map[modelskill.ID]ind1Cast{benomRangeCancel: {self, 1000000}})
	if got := h.AggroHate(w.player); got <= 0 {
		t.Fatalf("hate of the character before the range cancel = %v, want some", got)
	}
	w.draws.set(1)
	w.castQueued(t, h)
	wantBenomAt(t, "after its range cancel, draws 1 and 1 of 12", h, benomPoint1)
	if got := h.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of the character after the range cancel = %v, want none", got)
	}
	if got := ind1Slot(h, npc.IntQuest0); got != 0 {
		t.Fatalf("marked count after the range cancel = %d, want 0", got)
	}

	w.onQueue(t, h, func() {
		script.NPCOf(h).AddCastDesireUnchecked(script.NPCOf(h), modelskill.Ref{ID: benomInstantMove, Level: 1}, 1000000)
	})
	w.castQueued(t, h)
	wantBenomAt(t, "after its Instant Move", h, benomThrone)
	if got := h.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of the character after the Instant Move = %v, want none", got)
	}
}

// TestDungeonGatekeeperDuringASiege covers the gatekeeper (phases 2 and 5)
// while Rune castle's siege is fought: its created hook sets its first
// memory slot; it greets with its page; asked to teleport, it answers its refusal page and sends no
// one down; script event 10101 makes it shout, and 10100 does nothing.
func TestDungeonGatekeeperDuringASiege(t *testing.T) {
	t.Parallel()
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	const clanID = 0x70000041
	w := bootBenom(t,
		gameservertest.WithCastles(castles),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, q := range []struct {
				q    string
				args []any
			}{
				{"UPDATE characters SET clanid = ? WHERE char_name = 'Hunter'", []any{clanID}},
				{`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
					SELECT ?, 'Raiders', 5, obj_Id FROM characters WHERE char_name = 'Hunter'`, []any{clanID}},
				{"INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (?, ?, 'ATTACKER')", []any{runeCastleID, clanID}},
			} {
				if _, err := db.ExecContext(context.Background(), q.q, q.args...); err != nil {
					t.Fatalf("seed clans: %s: %v", q.q, err)
				}
			}
		}),
	)
	live, err := w.srv.NpcSpawns.AddSpawn(benomGatekeeper, w.at.X+60, w.at.Y, w.at.Z, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn the gatekeeper: %v", err)
	}
	gk := live.(*npc.Folk)
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	if got := script.NPCOf(gk).Int(script.SlotIntAI0); got != 1 {
		t.Fatalf("gatekeeper's first memory slot = %d, want 1", got)
	}
	rune, ok := w.srv.Sieges.Get(runeCastleID)
	if !ok {
		t.Fatal("no Rune castle siege")
	}
	rune.Start()
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)

	var talk [][]byte
	for range 2 {
		w.srv.Client.Send(encodeFolkAction(gk.ObjectID(), w.at))
		w.srv.Settle(t)
		talk = append(talk, w.srv.ReadQueued(t, w.srv.Client)...)
	}
	if got := managerHTML(talk); len(got) != 1 || !strings.Contains(got[0], "35506.htm") {
		t.Fatalf("pages on talking to the gatekeeper during the siege = %q, want its greeting", got)
	}
	x, y, _ := w.srv.PlayerPosition(t, w.player.ObjectID())
	w.srv.Client.Send(encodeManagerBypass(fmt.Sprintf("npc_%d_Quest DungeonTeleporter", gk.ObjectID())))
	w.srv.Settle(t)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got := managerHTML(frames); !slices.Equal(got, []string{"<html><body>35506-2.htm</body></html>\n"}) {
		t.Fatalf("pages asked to teleport during the siege = %q, want the refusal", got)
	}
	if got := ind8Pick(frames, serverpackets.OpcodeTeleportToLocation); len(got) != 0 {
		t.Fatalf("TeleportToLocation frames during the siege = %x, want none", got)
	}
	if nx, ny, _ := w.srv.PlayerPosition(t, w.player.ObjectID()); nx != x || ny != y {
		t.Fatalf("the character moved to %d,%d during the siege, want %d,%d", nx, ny, x, y)
	}

	roles := map[int32]string{gk.ObjectID(): "gatekeeper"}
	for _, row := range []struct {
		event int32
		want  []string
	}{
		{10100, nil},
		{10101, []string{benomShout("gatekeeper", benomGatekeeper, 1010632)}},
	} {
		if !w.srv.RunScript(t, w.player.ObjectID(), "Ind1Director", func(s *script.Script, _ *script.Player) {
			s.SendScriptEvent(script.NPCOf(gk), row.event, 1, 0)
		}) {
			t.Fatal("the script event panicked")
		}
		w.srv.Settle(t)
		if got := benomSays(t, w.srv.ReadQueued(t, w.srv.Client), roles); !slices.Equal(got, row.want) {
			t.Fatalf("NpcSay frames on script event %d = %q, want %q", row.event, got, row.want)
		}
	}
}
