package pets

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// onlineCharacterOf returns objID's character in the world.
func onlineCharacterOf(t *testing.T, srv *gameservertest.Server, objID int32) *player.Character {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("player %d not in the world", objID)
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %d is %T, not an online character", objID, obj)
	}
	return c
}

// TestInvisiblePlayerGates pins every NPC and summon gate on an invisible
// player (Appearance.isVisible), each against a visible control:
//
//   - an invisible player, or its summon, is never an automatic target
//     (Npc.canAutoAttack), whatever its karma: a Guard skips an invisible
//     karma player, a monster the summon of an invisible owner, and a siege
//     guard both (SiegeGuard.canAutoAttack);
//   - only a game master knows an invisible player or its summon
//     (Creature.knows): a Folk and a monster never do, and a summon does
//     only when its owner is a game master (Summon.isGM is its owner's).
func TestInvisiblePlayerGates(t *testing.T) {
	t.Parallel()
	srv := bootPets(t)
	ownerID := srv.SoleObjectID(t)
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	startInWorld(t, srv.Client)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}

	carrierSeed := srv.SeedCharacterFor(t, "player2", "Carrier", 1, 0)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET karma = 500 WHERE obj_Id = ?", carrierSeed.ID); err != nil {
		t.Fatalf("set karma: %v", err)
	}
	carrierClient := srv.DialClient(t, "player2", 1)
	startInWorld(t, carrierClient)
	drainUntilQuiet(t, h.client)
	pet, _ := h.spawnWolf(t)
	drainUntilQuiet(t, carrierClient)

	// The gates see each player as the world holds it; the flags live on its
	// character.
	carrier := onlinePlayer(t, srv, carrierSeed.ID)
	ownerChar, carrierChar := onlineCharacterOf(t, srv, ownerID), onlineCharacterOf(t, srv, carrierSeed.ID)
	if carrier.Karma() <= 0 {
		t.Fatal("control: the carrier holds no karma")
	}
	x, y, z := ownerChar.Position()
	at := func(dx int) location.Location { return location.Location{X: x + dx, Y: y, Z: z} }
	monster := srv.SpawnHostileNPCKindAt(t, "Monster", at(40))
	guard := srv.SpawnHostileNPCKindAt(t, "Guard", at(60))
	folk := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 30100), at(80))
	siegeGuard := srv.SpawnHostileNPCKindAt(t, "SiegeGuard", at(100))

	cases := []struct {
		name     string
		hide     *player.Character
		gmOwner  bool
		check    func() bool
		visible  bool
		whenHide bool
	}{
		{"Guard auto-attacks a karma player", carrierChar, false, func() bool { return guard.AutoAttackTargetValid(carrier, 1000, true) }, true, false},
		{"monster auto-attacks a summon", ownerChar, false, func() bool { return monster.AutoAttackTargetValid(pet, 1000, true) }, true, false},
		{"monster knows a summon", ownerChar, false, func() bool { return monster.Knows(pet) }, true, false},
		{"SiegeGuard auto-attacks a player", carrierChar, false, func() bool { return siegeGuard.AutoAttackTargetValid(carrier, 1000, true) }, true, false},
		{"SiegeGuard auto-attacks a summon", ownerChar, false, func() bool { return siegeGuard.AutoAttackTargetValid(pet, 1000, true) }, true, false},
		{"Folk knows a player", carrierChar, false, func() bool { return folk.Knows(carrier) }, true, false},
		{"non-GM owner's summon knows a player", carrierChar, false, func() bool { return pet.Knows(carrier) }, true, false},
		{"GM owner's summon knows a player", carrierChar, true, func() bool { return pet.Knows(carrier) }, true, true},
	}
	for _, tc := range cases {
		ownerChar.SetSeesInvisible(tc.gmOwner)
		if got := tc.check(); got != tc.visible {
			t.Errorf("%s, visible: got %v, want %v", tc.name, got, tc.visible)
		}
		tc.hide.SetInvisible(true)
		if got := tc.check(); got != tc.whenHide {
			t.Errorf("%s, invisible: got %v, want %v", tc.name, got, tc.whenHide)
		}
		tc.hide.SetInvisible(false)
		ownerChar.SetSeesInvisible(false)
	}
}
