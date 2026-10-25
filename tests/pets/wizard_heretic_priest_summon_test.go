package pets

import (
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// whpPriestID is a WizardHereticPriestAggressive NPC.
const whpPriestID = int32(22143)

// whpPriestScripts are the behavior chain down to Wizard and
// WizardHereticPriestAggressive, in scripts.xml order.
func whpPriestScripts() ([]script.Listing, script.Catalog) {
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":                                              defaultnpc.New,
		"script.ai.individual.Monster.MonsterAI":                                       monster.New,
		"script.ai.individual.Monster.WizardBase.WizardBase":                           wizard.Base,
		"script.ai.individual.Monster.WizardBase.Wizard.Wizard":                        wizard.Wizard,
		"script.ai.individual.Monster.WizardBase.Wizard.WizardHereticPriestAggressive": wizard.HereticPriestAggressive,
	}
	list := make([]script.Listing, 0, len(catalog))
	for _, path := range []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.individual.Monster.MonsterAI",
		"script.ai.individual.Monster.WizardBase.WizardBase",
		"script.ai.individual.Monster.WizardBase.Wizard.Wizard",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardHereticPriestAggressive",
	} {
		list = append(list, script.Listing{Path: path})
	}
	return list, catalog
}

// TestWizardHereticPriestSendsTheSummonAfterItsOwner (Creature.teleportTo
// of the player, then of Player.getSummon) runs the wizard heretic priest's
// see-creature teleport, every draw 0, for an owner with its wolf out in a
// party of two, larger than the priest's party_members of 1. Seeing the
// owner, the priest sends it to its first teleport point and its wolf
// after it: the owner sees one ordinary TeleportToLocation of itself to
// the point, both stand there, and the wolf has landed and follows its
// owner again. Seeing the wolf sends nobody: it is no player.
func TestWizardHereticPriestSendsTheSummonAfterItsOwner(t *testing.T) {
	t.Parallel()
	list, catalog := whpPriestScripts()
	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.ID, tmpl.TemplateID = int(whpPriestID), int(whpPriestID)
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), treeTemplate(), tmpl})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{whpPriestID: script.KindHostile}, list, catalog),
	})
	wolf, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	mateID := h.srv.SeedCharacterFor(t, "player3", "Bystander", 1, 0).ID
	mate := h.srv.DialClient(t, "player3", 1)
	startInWorld(t, mate)
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, mate)
	h.client.Send(encodeJoinParty("Bystander"))
	drainUntilQuiet(t, h.client)
	mate.Send(encodeAnswerJoinParty(1))
	drainUntilQuiet(t, mate)
	drainUntilQuiet(t, h.client)

	// The party mate waits out of the priest's sight, so the owner alone
	// is sent.
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	mateObj, ok := h.srv.State.Player(mateID)
	if !ok {
		t.Fatal("the party mate is not in the world")
	}
	if err := h.srv.State.Move(mateObj, x-3000, y, z); err != nil {
		t.Fatalf("move the party mate: %v", err)
	}
	h.srv.Settle(t)
	drainUntilQuiet(t, mate)
	drainUntilQuiet(t, h.client)
	dest := location.Location{X: x + 2000, Y: y + 100, Z: z}
	// The points lie beside the owner, whose place the boot decides; the
	// priest spawns after they are set.
	tmpl.AIParams = npc.AIParams{"party_members": "1"}
	for i := 1; i <= 3; i++ {
		tmpl.AIParams[fmt.Sprintf("TelPosX%d", i)] = fmt.Sprint(dest.X)
		tmpl.AIParams[fmt.Sprintf("TelPosY%d", i)] = fmt.Sprint(dest.Y)
		tmpl.AIParams[fmt.Sprintf("TelPosZ%d", i)] = fmt.Sprint(dest.Z)
	}
	// 100 from the owner, the owner stands in the territory of the
	// priest's spawn (its drift range around the spawn point).
	if _, err := h.srv.NpcSpawns.AddSpawn(whpPriestID, x+100, y, z, 0, false, 0); err != nil {
		t.Fatalf("spawn the priest: %v", err)
	}
	h.srv.Settle(t)
	drainUntilQuiet(t, h.client)

	if err := h.srv.AI.Tick(); err != nil {
		t.Fatalf("AI.Tick() error: %v", err)
	}
	h.srv.Settle(t)
	frames := drainFrames(t, h.client)

	got, fast := teleports(frames, h.ownerID)
	if !slices.Equal(got, []location.Location{dest}) || !slices.Equal(fast, []bool{false}) {
		t.Fatalf("owner teleports = %v fast %v, want one ordinary to %v", got, fast, dest)
	}
	if ox, oy, _ := h.srv.PlayerPosition(t, h.ownerID); ox != dest.X || oy != dest.Y {
		t.Fatalf("owner at %d,%d, want %d,%d", ox, oy, dest.X, dest.Y)
	}
	if at := positionOf(wolf); at.X != dest.X || at.Y != dest.Y {
		t.Fatalf("wolf stands at %v, want %d,%d", at, dest.X, dest.Y)
	}
	if wolf.Teleporting() || !wolf.FollowActive() {
		t.Fatalf("wolf teleporting = %v following = %v, want it landed and following its owner", wolf.Teleporting(), wolf.FollowActive())
	}
	if got, _ := teleports(drainFrames(t, mate), h.ownerID); len(got) != 0 {
		t.Fatalf("party mate saw the owner teleport %v, want it out of sight", got)
	}
}
