package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// assertVanished requires frames to dismiss the summon (PetDelete) and then
// tell its owner it vanished (YOUR_SERVITOR_HAS_VANISHED, once), and the
// owner's summon slot to be empty.
func assertVanished(t *testing.T, h *petWorld, frames [][]byte) {
	t.Helper()
	deleted, vanished := -1, -1
	for i, f := range frames {
		switch {
		case f[0] == serverpackets.OpcodePetDelete && deleted < 0:
			deleted = i
		case f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, f) == serverpackets.SystemMessageServitorHasVanished:
			if vanished >= 0 {
				t.Fatal("YOUR_SERVITOR_HAS_VANISHED sent twice")
			}
			vanished = i
		}
	}
	if deleted < 0 || vanished < 0 || vanished < deleted {
		t.Fatalf("PetDelete at %d, YOUR_SERVITOR_HAS_VANISHED at %d in opcodes %x, want both, PetDelete first",
			deleted, vanished, frameOpcodes(frames))
	}
	h.srv.AdvanceUntil(t, "summon slot empty", func() bool {
		_, ok := h.srv.State.Summon(h.ownerID)
		return !ok
	})
}

// assertStillSummoned requires frames to hold neither PetDelete nor
// YOUR_SERVITOR_HAS_VANISHED, and the owner's servitor to be summoned.
func assertStillSummoned(t *testing.T, h *petWorld, frames [][]byte) {
	t.Helper()
	for _, f := range frames {
		if f[0] == serverpackets.OpcodePetDelete ||
			f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, f) == serverpackets.SystemMessageServitorHasVanished {
			t.Fatalf("ordinary servitor got opcode %#x, want it left summoned", f[0])
		}
	}
	if _, ok := h.srv.State.Summon(h.ownerID); !ok {
		t.Fatal("ordinary servitor was dismissed")
	}
}

// TestSiegeSummonCalledOffBattlefieldVanishes pins SiegeSummon.onSpawn: a
// siege summon that appears outside an active siege battlefield is dismissed
// at once and its owner is told YOUR_SERVITOR_HAS_VANISHED (1521). It
// appears 40 east of its owner, so a battlefield ending between the two
// leaves it off; an inactive battlefield is no battlefield. An ordinary
// servitor stays wherever it appears.
func TestSiegeSummonCalledOffBattlefieldVanishes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		tmpl     func() *npc.Template
		skillID  int
		zones    func(*testing.T) *zone.Index
		vanishes bool
	}{
		{name: "no battlefield", tmpl: siegeGolemTemplate, skillID: summonGolemSkillID, vanishes: true},
		{
			name: "inactive battlefield", tmpl: siegeGolemTemplate, skillID: summonGolemSkillID, vanishes: true,
			zones: func(t *testing.T) *zone.Index { return battlefieldZones(t, -1_000, 1_000, false) },
		},
		{
			name: "battlefield edge between owner and summon", tmpl: siegeGolemTemplate, skillID: summonGolemSkillID, vanishes: true,
			zones: func(t *testing.T) *zone.Index { return battlefieldZones(t, -1_000, 30, true) },
		},
		{name: "ordinary servitor, no battlefield", tmpl: catTemplate, skillID: summonCatSkillID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var extra []gameservertest.Option
			if tc.zones != nil {
				extra = append(extra, gameservertest.WithZones(tc.zones(t)))
			}
			h := bootServitorCaster(t, tc.tmpl(), tc.skillID, extra...)
			drainUntilQuiet(t, h.client)
			h.client.Send(encodeRequestMagicSkillUse(int32(tc.skillID)))
			frames := readUntilOpcode(t, h.client, serverpackets.OpcodePetInfo, "servitor PetInfo")
			frames = append(frames, drainFrames(t, h.client)...)
			if tc.vanishes {
				assertVanished(t, h, frames)
				return
			}
			assertStillSummoned(t, h, frames)
		})
	}
}

// TestSiegeSummonVanishesOnTeleport pins the jump of a siege summon after its
// owner's teleport (Creature.teleportTo, SiegeSummon.onTeleported): leaving
// its zones for the jump takes it off its battlefield, so it is dismissed
// and its owner told YOUR_SERVITOR_HAS_VANISHED wherever it lands, on the
// same battlefield or off it. An ordinary servitor follows its owner.
func TestSiegeSummonVanishesOnTeleport(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		dx       int
		golem    bool
		vanishes bool
	}{
		{name: "golem landing on the battlefield", dx: 200, golem: true, vanishes: true},
		{name: "golem landing off the battlefield", dx: 5_000, golem: true, vanishes: true},
		{name: "ordinary servitor landing on the battlefield", dx: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tmpl, skillID := catTemplate, summonCatSkillID
			if tc.golem {
				tmpl, skillID = siegeGolemTemplate, summonGolemSkillID
			}
			h, _ := bootSiegeServitor(t, tmpl(), skillID)
			obj, ok := h.srv.State.Player(h.ownerID)
			if !ok {
				t.Fatal("owner missing from world")
			}
			owner, ok := network.OnlineCharacter(obj)
			if !ok {
				t.Fatalf("owner = %T, want online character", obj)
			}
			x, y, z := h.srv.PlayerPosition(t, h.ownerID)
			owner.TeleportTo(x+tc.dx, y, z, 0)
			readUntilOpcode(t, h.client, serverpackets.OpcodeTeleportToLocation, "owner teleport")
			h.client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
			frames := drainFrames(t, h.client)
			if tc.vanishes {
				assertVanished(t, h, frames)
				return
			}
			assertStillSummoned(t, h, frames)
		})
	}
}
