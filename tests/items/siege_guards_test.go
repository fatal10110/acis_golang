package items

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// Reference: Siege.startSiege, after closing the gates, spawns the castle's
// guards (Castle.spawnSiegeGuardsOrMercenaries): with an owner it starts
// the pc_siege_warfare_start(<castle id>) makers, then for each dropped
// ticket spawns a mercenary of the ticket's npcId where the ticket lies
// (no respawn), gives it the castle as residence, decays the ticket, and
// empties the castle's ticket set; with no owner it starts the
// siege_warfare_start(<castle id>) makers. A started maker spawns only
// while its spawn condition does not hold (SpawnMaker.checkHasSpawnCondition):
// the castle's siege zone active and, for pc_siege, an owner; for siege, no
// owner. endSiege and midVictory (first, whatever the owner) despawn them:
// each mercenary deleted, both spawn times stopped (Castle.
// despawnSiegeGuardsOrMercenaries); midVictory then spawns them again.
// Castle.removeOwner kills the mercenaries (npc.doDie(npc)) before the
// mid-victory it runs. MultiSpawn.doSpawn gives a spawn-time maker's NPCs
// the castle of its first parameter.

// The guards: the mercenary Gludio's sword ticket hires, and an NPC of each
// of Gludio's guard makers.
const (
	gludioSwordMercenary = 35030 // castles.xml: ticket 6038 npcId
	gludioGuard          = 35070 // siege_warfare_start(1)
	gludioOwnerGuard     = 35071 // pc_siege_warfare_start(1)
)

// guardSpawnlist declares Gludio's two guard makers, and one of Dion's
// that no Gludio siege starts.
const guardSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="gludio_siege" minZ="-1000" maxZ="1000"><node x="-500" y="-500"/><node x="500" y="-500"/><node x="500" y="500"/><node x="-500" y="500"/></territory>
	<npcmaker name="gludio_guards" territory="gludio_siege" spawnTime="siege_warfare_start(1)" maximumNpcs="2">
		<ai type="default_maker"/>
		<npc id="35070" pos="10;10;0;0" total="2" respawn="3hour"/>
	</npcmaker>
	<npcmaker name="gludio_owner_guards" territory="gludio_siege" spawnTime="pc_siege_warfare_start(1)" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="35071" pos="20;20;0;0" total="1" respawn="3hour"/>
	</npcmaker>
	<npcmaker name="dion_owner_guards" territory="gludio_siege" spawnTime="pc_siege_warfare_start(2)" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="35071" pos="30;30;0;0" total="1" respawn="3hour"/>
	</npcmaker>
</list>`

func guardTemplate(id int) *npc.Template {
	return &npc.Template{
		ID: id, TemplateID: id, Type: "SiegeGuard", Name: "Guard", Level: 50,
		HPMax: 2000, AtkSpd: 253, RunSpeed: 120, WalkSpeed: 50, CON: 43,
		CollisionRadius: 8, CollisionHeight: 24, CorpseTime: 7,
	}
}

// bootGuardWorld is the ticket world of Gludio's lord, a rival clan
// registered to attack, with Gludio's guard makers and the guard
// templates, the admin NPC info page and the shipped Gludio sword tickets
// in the lord's hands.
func bootGuardWorld(t *testing.T) (w *castleWorld, ticket int32) {
	t.Helper()
	datapack.Require(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "guards.xml"), []byte(guardSpawnlist), 0o644); err != nil {
		t.Fatal(err)
	}
	makers, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawn list: %v", err)
	}
	page, err := os.ReadFile(datapack.Path(t, "data", "html", "admin", "npcinfo", "general-2.htm"))
	if err != nil {
		t.Fatalf("read npc info page: %v", err)
	}
	w = bootTicketWorld(t, ticketSetup{ownsCastle: 1, attacker: true},
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			guardTemplate(gludioSwordMercenary), guardTemplate(gludioGuard), guardTemplate(gludioOwnerGuard),
		})),
		gameservertest.WithNpcSpawns(makers),
		gameservertest.WithHTMLPages(map[string]string{"admin/npcinfo/general-2.htm": string(page)}),
		gameservertest.WithScripts([]script.Listing{{Path: "quest." + residenceScript}}, script.Catalog{
			"quest." + residenceScript: func() script.Script { return script.Script{QuestID: 1} },
		}),
	)
	ticket = w.srv.GiveItem(t, w.leaderID, gludioSwordTicket, 1)
	w.enter(t)
	return w, ticket
}

// placeTicket has the lord place its ticket and returns the ground
// ticket's object id and where it lies.
func (w *castleWorld) placeTicket(t *testing.T, ticket int32) (ground int32, x, y int32) {
	t.Helper()
	for _, f := range w.useTicket(t, ticket) {
		if f[0] != serverpackets.OpcodeDropItem {
			continue
		}
		r := wire.NewReader(f[1:])
		r.ReadInt32() // dropper
		ground = r.ReadInt32()
		r.ReadInt32() // item id
		x, y = r.ReadInt32(), r.ReadInt32()
		drainUntilQuiet(t, w.gm)
		return ground, x, y
	}
	t.Fatal("no ticket placed")
	return 0, 0, 0
}

// guards returns the live hostile NPCs of template id in the world.
func (w *castleWorld) guards(id int) []*npc.Hostile {
	var out []*npc.Hostile
	for _, obj := range w.srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.Instance.Template.ID == id {
			out = append(out, h)
		}
	}
	return out
}

// guardCount returns how many NPCs of each guard template stand in the
// world: the mercenary, the ownerless castle's guard, the owner's guard.
func (w *castleWorld) guardCount() [3]int {
	return [3]int{len(w.guards(gludioSwordMercenary)), len(w.guards(gludioGuard)), len(w.guards(gludioOwnerGuard))}
}

// residenceScript is the script whose hook reads an NPC's castle.
const residenceScript = "Q001_LettersOfLove"

// scriptCastle returns the id of the castle a script hook reads for the
// NPC h (Script.Castle), 0 for none.
func (w *castleWorld) scriptCastle(t *testing.T, h *npc.Hostile) int {
	t.Helper()
	id := 0
	if !w.srv.RunScript(t, w.leaderID, residenceScript, func(sc *script.Script, _ *script.Player) {
		if c := sc.Castle(script.NewNPC(h)); c != nil {
			id = int(c.ID())
		}
	}) {
		t.Fatal("script invocation panicked")
	}
	return id
}

// npcResidence has the game master select the NPC id and returns the
// residence its //info 2 page names.
func (w *castleWorld) npcResidence(t *testing.T, id int32) string {
	t.Helper()
	w.gm.Send(encodeAction(id, 0, 0, 0, false))
	drainUntilQuiet(t, w.gm)
	cmd := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
	cmd.WriteString("info 2")
	w.gm.Send(cmd.Bytes())
	_, page, _ := gmAnswer(t, collectFrames(w.gm))
	_, after, ok := strings.Cut(page, "Residence :</font> ")
	if !ok {
		t.Fatalf("//info 2 page without a residence: %q", page)
	}
	name, _, _ := strings.Cut(after, "\n")
	return strings.TrimSpace(name)
}

func gludioSiegeOf(t *testing.T, w *castleWorld) *siege.Siege {
	t.Helper()
	s, ok := w.srv.Sieges.Get(1)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	return s
}

// deletedIDs returns the object ids of the DeleteObject frames among
// frames.
func deletedIDs(frames [][]byte) []int32 {
	var out []int32
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeDeleteObject {
			out = append(out, wire.NewReader(f[1:]).ReadInt32())
		}
	}
	return out
}

// TestSiegeGuardsAndMercenaries drives Gludio's siege with a sword ticket
// placed by its lord. At the start the owner's guard spawns, no ownerless
// guard, and one mercenary of the ticket's NPC stands where the ticket
// lay, Gludio's per //info 2; the ticket leaves the world and the castle's
// ticket set is empty. At mid-victory the mercenary is deleted and, the
// ticket set being empty, none comes back, while the owner's guard is
// deleted and spawns again. At the end every guard is gone.
func TestSiegeGuardsAndMercenaries(t *testing.T) {
	t.Parallel()
	w, ticket := bootGuardWorld(t)
	ground, x, y := w.placeTicket(t, ticket)
	if got := w.guardCount(); got != [3]int{} {
		t.Fatalf("guards before the siege: %v, want none", got)
	}

	gludio := gludioSiegeOf(t, w)
	gludio.Start()
	if !gludio.InProgress() {
		t.Fatal("siege not under way")
	}
	leader := collectFrames(w.leader)
	if got := w.guardCount(); got != [3]int{1, 0, 1} {
		t.Fatalf("guards at the start [mercenary, ownerless, owner's] = %v, want [1 0 1]", got)
	}
	merc := w.guards(gludioSwordMercenary)[0]
	if mx, my, _ := merc.Position(); int32(mx) != x || int32(my) != y {
		t.Fatalf("mercenary at (%d, %d), want the ticket's (%d, %d)", mx, my, x, y)
	}
	if !slices.Contains(deletedIDs(leader), ground) {
		t.Fatalf("leader at the start: opcodes %x, want the ticket deleted", opcodesOf(leader))
	}
	if _, ok := w.srv.State.Object(ground); ok {
		t.Fatal("ticket still in the world after the siege started")
	}
	if got := gludioDropped(t, w.srv); got != 0 {
		t.Fatalf("Gludio dropped tickets after the start = %d, want 0", got)
	}
	drainUntilQuiet(t, w.gm)
	if got := w.npcResidence(t, merc.ObjectID()); got != "Gludio Castle" {
		t.Fatalf("mercenary residence = %q, want Gludio Castle", got)
	}
	ownerGuard := w.guards(gludioOwnerGuard)[0].ObjectID()
	if got := w.npcResidence(t, ownerGuard); got != "Gludio Castle" {
		t.Fatalf("owner's guard residence = %q, want Gludio Castle", got)
	}
	if got := w.scriptCastle(t, merc); got != 1 {
		t.Fatalf("Script.Castle(mercenary) = %d, want Gludio (1)", got)
	}
	if got := w.scriptCastle(t, w.guards(gludioOwnerGuard)[0]); got != 1 {
		t.Fatalf("Script.Castle(owner's guard) = %d, want Gludio (1)", got)
	}

	gludio.MidVictory()
	if got := settledGuards(t, w); got != [3]int{0, 0, 1} {
		t.Fatalf("guards after mid-victory = %v, want [0 0 1]", got)
	}
	if again := w.guards(gludioOwnerGuard)[0].ObjectID(); again == ownerGuard {
		t.Fatal("the owner's guard was not spawned again at mid-victory")
	}

	gludio.End()
	if got := settledGuards(t, w); got != [3]int{} {
		t.Fatalf("guards after the end = %v, want none", got)
	}
}

// TestSiegeMercenariesDieWithOwner removes Gludio's owner while its siege
// runs: the mercenary dies where it stands, the owner's guard is deleted
// and the ownerless castle's guards spawn, Gludio's per //info 2; the
// siege's end deletes them.
func TestSiegeMercenariesDieWithOwner(t *testing.T) {
	t.Parallel()
	w, ticket := bootGuardWorld(t)
	w.placeTicket(t, ticket)
	gludio := gludioSiegeOf(t, w)
	gludio.Start()
	drainUntilQuiet(t, w.leader)
	drainUntilQuiet(t, w.gm)
	if got := w.guardCount(); got != [3]int{1, 0, 1} {
		t.Fatalf("guards at the start = %v, want [1 0 1]", got)
	}
	merc := w.guards(gludioSwordMercenary)[0]

	gm, _ := w.castleCommand(t, "remove gludio_castle")
	var died bool
	for _, f := range gm {
		if f[0] == serverpackets.OpcodeDie && wire.NewReader(f[1:]).ReadInt32() == merc.ObjectID() {
			died = true
		}
	}
	if !died || !merc.Dead() {
		t.Fatalf("mercenary after the owner's removal: dead %v, Die seen %v among opcodes %x", merc.Dead(), died, opcodesOf(gm))
	}
	if got := w.guardCount(); got[1] != 2 || got[2] != 0 {
		t.Fatalf("guards after the owner's removal = %v, want 2 ownerless and no owner's guard", got)
	}
	if got := w.npcResidence(t, w.guards(gludioGuard)[0].ObjectID()); got != "Gludio Castle" {
		t.Fatalf("ownerless guard residence = %q, want Gludio Castle", got)
	}
	if got := w.scriptCastle(t, w.guards(gludioGuard)[0]); got != 1 {
		t.Fatalf("Script.Castle(ownerless guard) = %d, want Gludio (1)", got)
	}

	gludio.End()
	if got := settledGuards(t, w); got[1] != 0 || got[2] != 0 {
		t.Fatalf("guards after the end = %v, want no maker guard", got)
	}
}

// settledGuards returns the guard counts once every queued task has run: a
// mercenary's delete runs on its own queue.
func settledGuards(t *testing.T, w *castleWorld) [3]int {
	t.Helper()
	w.srv.Settle(t)
	return w.guardCount()
}
