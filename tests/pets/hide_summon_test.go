package pets

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// petInfoAbnormal returns a PetInfo frame's abnormal effect mask, the
// int32 ahead of its 14-byte tail (mountable, move type, padding, team,
// soulshots and spiritshots per hit).
func petInfoAbnormal(t *testing.T, frame []byte) uint32 {
	t.Helper()
	if frame[0] != serverpackets.OpcodePetInfo {
		t.Fatalf("opcode = %#x, want PetInfo", frame[0])
	}
	n := len(frame)
	return binary.LittleEndian.Uint32(frame[n-18:])
}

func encodeBuildCmd(command string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
	w.WriteString(command)
	return w.Bytes()
}

// TestHiddenOwnersPetIsShownOnlyToItsOwner pins //hide's summon refresh
// (AdminEffects.java admin_hide) and what its owner's invisibility does to
// a summon: owner and pet are taken off the grid and put back, so a watcher
// forgets it and is not shown it again (SummonInfo writes nothing for an
// invisible owner's summon to anyone but the owner), while the owner's
// PetInfo draws it in stealth (PetInfo.java:103). Showing again shows the
// pet to the watcher and drops the stealth mask.
func TestHiddenOwnersPetIsShownOnlyToItsOwner(t *testing.T) {
	t.Parallel()
	adminData, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	srv := bootPets(t, gameservertest.WithAdmin(adminData))
	ownerID := srv.SoleObjectID(t)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = 7 WHERE obj_Id = ?", ownerID); err != nil {
		t.Fatalf("set access level: %v", err)
	}
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	startInWorld(t, srv.Client)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	srv.SeedCharacterFor(t, "player2", "Watcher", 1, 0)
	watcher := srv.DialClient(t, "player2", 1)
	startInWorld(t, watcher)
	drainUntilQuiet(t, h.client)
	pet, _ := h.spawnWolf(t)
	drainUntilQuiet(t, watcher)

	h.client.Send(encodeBuildCmd("hide"))
	var petInfos [][]byte
	for _, f := range drainFrames(t, h.client) {
		if f[0] == serverpackets.OpcodePetInfo {
			petInfos = append(petInfos, f)
		}
	}
	// The owner rediscovers its pet twice: once as it comes back on the
	// grid, once as the pet does.
	if len(petInfos) != 2 {
		t.Fatalf("hidden owner got %d PetInfo, want 2", len(petInfos))
	}
	for _, f := range petInfos {
		if petInfoAbnormal(t, f)&0x100000 == 0 {
			t.Fatal("hidden owner's PetInfo does not draw the pet in stealth")
		}
	}
	frames := drainFrames(t, watcher)
	if n := countNPCInfoFor(frames, pet.ObjectID()); n != 0 {
		t.Fatalf("watcher was shown the hidden owner's pet %d time(s)", n)
	}
	deleted := false
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeDeleteObject && int32(binary.LittleEndian.Uint32(f[1:])) == pet.ObjectID() {
			deleted = true
		}
	}
	if !deleted {
		t.Fatalf("watcher frames %x, want the pet forgotten", frameOpcodes(frames))
	}

	h.client.Send(encodeBuildCmd("hide"))
	petInfos = nil
	for _, f := range drainFrames(t, h.client) {
		if f[0] == serverpackets.OpcodePetInfo {
			petInfos = append(petInfos, f)
		}
	}
	if len(petInfos) != 1 || petInfoAbnormal(t, petInfos[0])&0x100000 != 0 {
		t.Fatalf("shown owner got %d PetInfo, want one without stealth", len(petInfos))
	}
	if n := countNPCInfoFor(drainFrames(t, watcher), pet.ObjectID()); n != 1 {
		t.Fatalf("watcher was shown the visible owner's pet %d time(s), want 1", n)
	}
}
