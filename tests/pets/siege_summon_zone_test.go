package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// siegeServitorCases are the servitors a siege battlefield judges: a siege
// summon, which the battlefield dismisses, and an ordinary one, which it
// leaves alone.
var siegeServitorCases = []struct {
	name      string
	tmpl      func() *npc.Template
	skillID   int
	dismissed bool
}{
	{name: "siege golem", tmpl: siegeGolemTemplate, skillID: summonGolemSkillID, dismissed: true},
	{name: "ordinary servitor", tmpl: catTemplate, skillID: summonCatSkillID, dismissed: false},
}

// bootSiegeServitor summons tmpl inside an active siege battlefield that
// covers the owner and the servitor but not the ground 5000 east of them.
func bootSiegeServitor(t *testing.T, tmpl *npc.Template, skillID int) (*petWorld, *zone.Siege) {
	t.Helper()
	siege, err := zone.NewSiege(1, summonZoneBox(t, -1_000, 1_000), commons.NewStatSet())
	if err != nil {
		t.Fatalf("build siege zone: %v", err)
	}
	siege.SetActive(true)
	zones := zone.NewIndex()
	zones.Add(siege)
	h, servitor, _ := bootFearServitor(t, tmpl, skillID, gameservertest.WithZones(zones))
	if !servitor.InsideZone(zone.FlagSiege) {
		t.Fatal("servitor summoned on an active battlefield is not in it")
	}
	return h, siege
}

// assertSiegeDismissal checks the servitor's fate: a siege summon is
// dismissed with PetDelete and leaves its owner's summon slot empty; an
// ordinary servitor stays summoned.
func assertSiegeDismissal(t *testing.T, h *petWorld, dismissed bool) {
	t.Helper()
	if dismissed {
		h.srv.AdvanceUntil(t, "siege summon dismissed", func() bool {
			_, ok := h.srv.State.Summon(h.ownerID)
			return !ok
		})
		readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete")
		return
	}
	h.srv.Settle(t)
	if _, ok := h.srv.State.Summon(h.ownerID); !ok {
		t.Fatal("an ordinary servitor was dismissed by the siege battlefield")
	}
	for _, frame := range drainFrames(t, h.client) {
		if len(frame) > 0 && frame[0] == serverpackets.OpcodePetDelete {
			t.Fatal("an ordinary servitor was sent PetDelete by the siege battlefield")
		}
	}
}

// TestSiegeEndDismissesSiegeSummon pins SiegeZone.setActive(false)
// (SiegeZone.java:105-118): a siege summon still on the battlefield when the
// siege ends is dismissed; an ordinary servitor is not.
func TestSiegeEndDismissesSiegeSummon(t *testing.T) {
	t.Parallel()
	for _, tc := range siegeServitorCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, siege := bootSiegeServitor(t, tc.tmpl(), tc.skillID)
			siege.SetActive(false)
			assertSiegeDismissal(t, h, tc.dismissed)
		})
	}
}

// TestSiegeSummonLeavingBattlefieldIsDismissed pins SiegeZone.onExit
// (SiegeZone.java:77-78): a siege summon that leaves the battlefield, here
// following its owner's teleport, is dismissed; an ordinary servitor is not.
func TestSiegeSummonLeavingBattlefieldIsDismissed(t *testing.T) {
	t.Parallel()
	for _, tc := range siegeServitorCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, _ := bootSiegeServitor(t, tc.tmpl(), tc.skillID)
			obj, ok := h.srv.State.Player(h.ownerID)
			if !ok {
				t.Fatal("owner missing from world")
			}
			owner, ok := network.OnlineCharacter(obj)
			if !ok {
				t.Fatalf("owner = %T, want online character", obj)
			}
			x, y, z := h.srv.PlayerPosition(t, h.ownerID)
			owner.TeleportTo(x+5_000, y, z, 0)
			readUntilOpcode(t, h.client, serverpackets.OpcodeTeleportToLocation, "owner teleport")
			h.client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
			assertSiegeDismissal(t, h, tc.dismissed)
			if tc.dismissed {
				return
			}
			s, ok := h.srv.State.Summon(h.ownerID)
			if !ok {
				t.Fatal("ordinary servitor missing after the teleport")
			}
			if s.(interface{ InsideZone(zone.Flag) bool }).InsideZone(zone.FlagSiege) {
				t.Fatal("servitor teleported off the battlefield is still in it")
			}
		})
	}
}
