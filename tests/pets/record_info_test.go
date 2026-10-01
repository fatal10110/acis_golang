package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestRecordInfoResendsWalkingPetInPlace pins RequestRecordInfo for an
// owned pet (Player.refreshInfos, Summon.sendInfo): after UserInfo, the
// owner gets the pet's PetInfo, its PetItemList and its MoveToLocation back
// to back, before the next known object's info. A monster spawned after the
// pet in its region is known after it, so a list deferred past the refresh
// would land after the monster's NpcInfo.
func TestRecordInfoResendsWalkingPetInPlace(t *testing.T) {
	t.Parallel()
	h, pet, queue, home := bootStillPetOutOfView(t)
	target := location.Location{X: home.X + discoverWalk, Y: home.Y, Z: home.Z}
	walkSummonIntoView(t, h, pet, queue, home, target, nil)
	monster := h.srv.SpawnHostileNPCAt(t, location.Location{X: home.X, Y: home.Y + 40, Z: home.Z})
	drainFrames(t, h.client)

	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRecordInfo))
	frames := drainFrames(t, h.client)
	if len(frames) == 0 || frames[0][0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("RequestRecordInfo frames = opcodes %x, want UserInfo first", frameOpcodes(frames))
	}
	info := framePetInfo(frames, pet.ObjectID())
	if info < 0 || info+2 >= len(frames) || frames[info+1][0] != serverpackets.OpcodePetItemList {
		t.Fatalf("RequestRecordInfo frames = opcodes %x, want PetInfo, PetItemList then the pet's MoveToLocation", frameOpcodes(frames))
	}
	if npcInfo := frameIndex(frames, serverpackets.OpcodeNPCInfo, monster.ObjectID()); npcInfo < info {
		t.Fatalf("RequestRecordInfo frames = opcodes %x, want the pet (known first) before the monster's NpcInfo", frameOpcodes(frames))
	}
	h.requireDescribedWalk(t, frames[info+2], pet.ObjectID(), home, target, "owner")
}
