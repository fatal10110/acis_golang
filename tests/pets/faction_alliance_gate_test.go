package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// factionTemplate is a parked hostile NPC template of kind tagged with
// clans.
func factionTemplate(id int, kind string, clans ...string) *npc.Template {
	return &npc.Template{
		ID:              id,
		TemplateID:      id,
		Type:            kind,
		Level:           1,
		HPMax:           1000,
		AtkSpd:          300,
		RunSpeed:        120,
		WalkSpeed:       60,
		CollisionRadius: 8,
		CollisionHeight: 20,
		AggroRange:      1000,
		Clans:           clans,
	}
}

// TestFactionAllyIsNoAutoAttackTarget pins the faction exclusion of the
// automatic targeting rule (Npc.canAutoAttack): an NPC tagged
// varka_silenos_clan never auto-attacks a player allied with Varka Silenos,
// nor that player's summon, and one tagged ketra_orc_clan a player allied
// with the Ketra Orcs, whatever the alliance level — even a Guard facing a
// karma player. The other faction's ally, a neutral player, and an NPC with
// no faction tag are unaffected.
func TestFactionAllyIsNoAutoAttackTarget(t *testing.T) {
	t.Parallel()
	srv := bootPets(t)
	ownerID := srv.SoleObjectID(t)
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	startInWorld(t, srv.Client)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	drainUntilQuiet(t, h.client)
	pet, _ := h.spawnWolf(t)

	owner, ownerChar := onlinePlayer(t, srv, ownerID), onlineCharacterOf(t, srv, ownerID)
	x, y, z := ownerChar.Position()
	at := func(dx int) location.Location { return location.Location{X: x + dx, Y: y, Z: z} }
	ketra := srv.SpawnHostileNPCTemplateAt(t, factionTemplate(21324, "Monster", "ketra_orc_clan"), at(40))
	varka := srv.SpawnHostileNPCTemplateAt(t, factionTemplate(21350, "Monster", "varka_silenos_clan"), at(60))
	untagged := srv.SpawnHostileNPCTemplateAt(t, factionTemplate(20001, "Monster"), at(80))
	ketraGuard := srv.SpawnHostileNPCTemplateAt(t, factionTemplate(31000, "Guard", "ketra_orc_clan"), at(100))

	ownerChar.SetKarma(500)
	t.Cleanup(func() { ownerChar.SetKarma(0) })

	for _, standing := range []int{0, 1, 5, -1, -5} {
		ownerChar.SetVarkaKetraAlliance(standing)
		ketraAlly, varkaAlly := standing > 0, standing < 0
		for _, tc := range []struct {
			name string
			npc  *npc.Hostile
			want bool
		}{
			{"Ketra-tagged monster", ketra, !ketraAlly},
			{"Varka-tagged monster", varka, !varkaAlly},
			{"untagged monster", untagged, true},
			{"Ketra-tagged Guard", ketraGuard, !ketraAlly},
		} {
			if got := tc.npc.AutoAttackTargetValid(owner, 1000, true); got != tc.want {
				t.Errorf("standing %d: %s auto-attacks the player = %v, want %v", standing, tc.name, got, tc.want)
			}
			if got := tc.npc.AutoAttackTargetValid(pet, 1000, true); got != tc.want {
				t.Errorf("standing %d: %s auto-attacks the summon = %v, want %v", standing, tc.name, got, tc.want)
			}
		}
	}
}
