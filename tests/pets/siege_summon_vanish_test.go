package pets

import (
	"encoding/binary"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
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
// before it lands and its owner told YOUR_SERVITOR_HAS_VANISHED, on the same
// battlefield or off it. After its TeleportToLocation the owner is never
// shown it again, and a watcher parked at the landing point never sees it
// there. An ordinary servitor follows its owner and is shown at the landing
// point.
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
			servitor, ok := h.srv.State.Summon(h.ownerID)
			if !ok {
				t.Fatal("servitor missing from world")
			}
			summonID := servitor.ObjectID()
			x, y, z := h.srv.PlayerPosition(t, h.ownerID)
			watcher := h.joinSecondPlayer(t, "Watcher")
			teleportAndAppear(t, h, watcher.id, watcher.client, x+tc.dx, y, z)
			drainUntilQuiet(t, watcher.client)
			drainUntilQuiet(t, h.client)

			teleportAndAppear(t, h, h.ownerID, h.client, x+tc.dx, y, z)
			frames := drainFrames(t, h.client)
			watched := drainFrames(t, watcher.client)
			if !tc.vanishes {
				assertStillSummoned(t, h, frames)
				if !showsObject(watched, summonID) {
					t.Fatalf("watcher opcodes %x, want the servitor shown at the landing point", frameOpcodes(watched))
				}
				return
			}
			assertVanished(t, h, frames)
			// An owner landing within sight of the summon's old position
			// sees it there, then its jump; one landing out of sight is told
			// of neither. Either way, once it jumped, the owner gets only
			// the pet window its dismissal sends as its effects stop, just
			// before PetDelete, and is never shown it again after that.
			after := frames
			if jumped := slices.IndexFunc(frames, func(f []byte) bool {
				return f[0] == serverpackets.OpcodeTeleportToLocation && frameObjectID(f, 1) == summonID
			}); jumped >= 0 {
				after = frames[jumped:]
			}
			deleted := opcodeIndex(after, serverpackets.OpcodePetDelete)
			if deleted < 1 || after[deleted-1][0] != serverpackets.OpcodePetInfo || showsObject(after[:deleted-1], summonID) || showsObject(after[deleted:], summonID) {
				t.Fatalf("owner opcodes %x after the summon's jump, want only its departing PetInfo right before PetDelete", frameOpcodes(after))
			}
			if showsObject(watched, summonID) {
				t.Fatalf("watcher at the landing point opcodes %x, want the summon never shown", frameOpcodes(watched))
			}
		})
	}
}

// teleportAndAppear teleports player id to (x, y, z) and has its client c
// report it appeared.
func teleportAndAppear(t *testing.T, h *petWorld, id int32, c *testsupport.ScriptedClient, x, y, z int) {
	t.Helper()
	obj, ok := h.srv.State.Player(id)
	if !ok {
		t.Fatalf("player %d missing from world", id)
	}
	live, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %d = %T, want online character", id, obj)
	}
	live.TeleportTo(x, y, z, 0)
	readUntilOpcode(t, c, serverpackets.OpcodeTeleportToLocation, "teleport")
	c.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
}

// showsObject reports whether frames show objectID to their client: a
// PetInfo to its owner or an NpcInfo to anyone else.
func showsObject(frames [][]byte, objectID int32) bool {
	return slices.ContainsFunc(frames, func(f []byte) bool {
		switch f[0] {
		case serverpackets.OpcodePetInfo:
			return frameObjectID(f, 5) == objectID
		case serverpackets.OpcodeNPCInfo:
			return frameObjectID(f, 1) == objectID
		}
		return false
	})
}

// frameObjectID reads the int32 object id at offset at of frame.
func frameObjectID(frame []byte, at int) int32 {
	if len(frame) < at+4 {
		return 0
	}
	return int32(binary.LittleEndian.Uint32(frame[at:]))
}

// TestDeadSiegeSummonHidesOnTeleport pins the jump of a siege summon's
// corpse after its owner's teleport (Creature.teleportTo, Summon.unSummon,
// SiegeSummon.onTeleported): a corpse cannot be dismissed, so its owner gets
// no PetDelete, but it leaves the grid all the same, so a watcher by its old
// spot forgets it and one at the landing point never sees it, and its owner
// is still told YOUR_SERVITOR_HAS_VANISHED. The corpse keeps its owner's
// summon slot.
func TestDeadSiegeSummonHidesOnTeleport(t *testing.T) {
	t.Parallel()
	h, _ := bootSiegeServitor(t, siegeGolemTemplate(), summonGolemSkillID)
	obj, ok := h.srv.State.Summon(h.ownerID)
	if !ok {
		t.Fatal("servitor missing from world")
	}
	golem := obj.(*summon.Actor)
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	const dx = 5_000
	nearby := h.joinSecondPlayer(t, "Nearby")
	landing := joinPlayer(t, h, "player3", "Landing")
	teleportAndAppear(t, h, landing.id, landing.client, x+dx, y, z)
	drainUntilQuiet(t, landing.client)
	killPet(t, h, golem)
	drainUntilQuiet(t, nearby.client)
	drainUntilQuiet(t, landing.client)
	if !golem.InsideZone(zone.FlagSiege) {
		t.Fatal("dead golem is not on the battlefield before the jump")
	}

	teleportAndAppear(t, h, h.ownerID, h.client, x+dx, y, z)
	frames := drainFrames(t, h.client)
	vanished := 0
	for _, f := range frames {
		switch {
		case f[0] == serverpackets.OpcodePetDelete:
			t.Fatalf("owner opcodes %x, want no PetDelete for a corpse", frameOpcodes(frames))
		case f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, f) == serverpackets.SystemMessageServitorHasVanished:
			vanished++
		}
	}
	if vanished != 1 {
		t.Fatalf("owner got YOUR_SERVITOR_HAS_VANISHED %d times in %x, want once", vanished, frameOpcodes(frames))
	}
	if near := drainFrames(t, nearby.client); !sawDeleteObject(near, golem.ObjectID()) {
		t.Fatalf("watcher by the old spot opcodes %x, want DeleteObject for the corpse", frameOpcodes(near))
	}
	if watched := drainFrames(t, landing.client); showsObject(watched, golem.ObjectID()) {
		t.Fatalf("watcher at the landing point opcodes %x, want the corpse never shown", frameOpcodes(watched))
	}
	h.srv.Settle(t)
	if got, ok := h.srv.State.Summon(h.ownerID); !ok || got.ObjectID() != golem.ObjectID() {
		t.Fatal("the corpse gave up its owner's summon slot")
	}
}

// joinPlayer brings character name of account into the world.
func joinPlayer(t *testing.T, h *petWorld, account, name string) secondPlayer {
	t.Helper()
	id := h.srv.SeedCharacterFor(t, account, name, 1, 0).ID
	c := h.srv.DialClient(t, account, 1)
	startInWorld(t, c)
	if _, ok := h.srv.State.Player(id); !ok {
		t.Fatalf("%s not in world", name)
	}
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, c)
	return secondPlayer{client: c, id: id}
}
