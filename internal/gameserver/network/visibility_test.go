package network

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestLivePlayerVisibilitySendsCharInfoAndDeleteObject(t *testing.T) {
	state := world.New()
	firstFrames := &frameCapture{}
	secondFrames := &frameCapture{}
	first := newTestLivePlayer(t, 1, firstFrames)
	second := newTestLivePlayer(t, 2, secondFrames)

	state.Spawn(first, 0, 0, 0, 0)
	state.Spawn(second, 100, 0, 0, 0)

	if len(firstFrames.frames) != 1 || firstFrames.frames[0][0] != serverpackets.OpcodeCharInfo {
		t.Fatalf("first player frames = %x, want one CharInfo", firstFrames.frames)
	}
	if len(secondFrames.frames) != 1 || secondFrames.frames[0][0] != serverpackets.OpcodeCharInfo {
		t.Fatalf("second player frames = %x, want one CharInfo", secondFrames.frames)
	}

	state.Despawn(second)
	if got := firstFrames.frames[len(firstFrames.frames)-1][0]; got != serverpackets.OpcodeDeleteObject {
		t.Fatalf("last first-player frame opcode = %#x, want DeleteObject (%#x)", got, serverpackets.OpcodeDeleteObject)
	}
}

func TestPetInfoSnapshotMirrorsOwnerPvPFlag(t *testing.T) {
	owner := newTestLivePlayer(t, 1, &frameCapture{})
	owner.UpdatePvPFlag(task.PvPFlagBlinking)
	npcs := npc.NewTable([]*npc.Template{{ID: 12077, TemplateID: 12077}})
	pet := summon.NewPet(summon.PetConfig{ObjectID: 20, Owner: owner, NPCID: 12077})

	snapshot, ok := petInfoSnapshot(pet, owner, npcs)
	if !ok {
		t.Fatal("petInfoSnapshot() returned no snapshot")
	}
	if snapshot.PvpFlag != int(task.PvPFlagBlinking) {
		t.Fatalf("PvpFlag = %d, want %d", snapshot.PvpFlag, task.PvPFlagBlinking)
	}
}

func TestLivePlayerVisibilityRendersSupportedWorldObjectsSymmetrically(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	viewer := newTestLivePlayer(t, 1, frames)
	state.Spawn(viewer, 0, 0, 0, 0)

	ground := &visibleGroundItem{id: 10, itemID: 57, count: 3, stackable: true}
	door := &visibleDoor{id: 11, doorID: 100}
	static := &visibleStaticObject{id: 12, staticID: 200}
	invisible := &invisibleTracked{id: 13}

	state.Spawn(ground, 100, 0, 0, 0)
	state.Spawn(door, 200, 0, 0, 0)
	state.Spawn(static, 300, 0, 0, 0)
	state.Spawn(invisible, 400, 0, 0, 0)

	want := []byte{
		serverpackets.OpcodeSpawnItem,
		serverpackets.OpcodeDoorInfo,
		serverpackets.OpcodeStaticObjectInfo,
	}
	if got := frameOpcodes(frames.frames); string(got) != string(want) {
		t.Fatalf("spawn opcodes = %x, want %x", got, want)
	}

	state.Despawn(invisible)
	if got := frameOpcodes(frames.frames); string(got) != string(want) {
		t.Fatalf("opcodes after despawning unsupported object = %x, want still %x", got, want)
	}

	state.Despawn(static)
	state.Despawn(door)
	state.Despawn(ground)
	want = append(want,
		serverpackets.OpcodeDeleteObject,
		serverpackets.OpcodeDeleteObject,
		serverpackets.OpcodeDeleteObject,
	)
	if got := frameOpcodes(frames.frames); string(got) != string(want) {
		t.Fatalf("despawn opcodes = %x, want %x", got, want)
	}
}

func TestLivePlayerVisibilityRendersHostileNPC(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	viewer := newTestLivePlayer(t, 1, frames)
	state.Spawn(viewer, 0, 0, 0, 0)

	hostile := newTestHostileNPC(t, 20)
	state.Spawn(hostile, 100, 0, -50, 123)

	const opcodeNPCInfo = 0x16
	if len(frames.frames) != 1 {
		t.Fatalf("frames = %x, want one NPCInfo frame", frames.frames)
	}
	got := frames.frames[0]
	appendInt32 := func(b []byte, v int32) []byte {
		return binary.LittleEndian.AppendUint32(b, uint32(v))
	}
	wantPrefix := []byte{opcodeNPCInfo}
	wantPrefix = appendInt32(wantPrefix, 20)
	wantPrefix = appendInt32(wantPrefix, 1000100)
	wantPrefix = appendInt32(wantPrefix, 1)
	wantPrefix = appendInt32(wantPrefix, 100)
	wantPrefix = appendInt32(wantPrefix, 0)
	wantPrefix = appendInt32(wantPrefix, -50)
	wantPrefix = appendInt32(wantPrefix, 123)
	if len(got) < len(wantPrefix) || string(got[:len(wantPrefix)]) != string(wantPrefix) {
		t.Fatalf("NPCInfo prefix = % x, want % x", got[:min(len(got), len(wantPrefix))], wantPrefix)
	}

	state.Despawn(hostile)
	if len(frames.frames) != 2 || frames.frames[1][0] != serverpackets.OpcodeDeleteObject {
		t.Fatalf("frames after NPC despawn = %x, want DeleteObject after NPCInfo", frames.frames)
	}
}

func TestHostileAbnormalEffectRefreshResendsLiveNPCInfo(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	viewer := newTestLivePlayer(t, 1, frames)
	state.Spawn(viewer, 0, 0, 0, 0)

	hostile := newTestHostileNPC(t, 20)
	hostile.SetWorld(state)
	state.Spawn(hostile, 100, 0, -50, 123)
	frames.frames = nil

	hostile.SetCollisionRadius(9 * 1.19)
	hostile.StartAbnormalEffect(0x010000)
	snapshot := hostile.NPCInfoSnapshot()
	if snapshot.CollisionRadius != 9*1.19 {
		t.Fatalf("collision radius = %v, want %v", snapshot.CollisionRadius, 9*1.19)
	}
	if snapshot.AbnormalEffect != 0x010000 {
		t.Fatalf("abnormal effect = %#x, want %#x", snapshot.AbnormalEffect, 0x010000)
	}
	hostile.UpdateAbnormalEffect()

	if len(frames.frames) != 1 {
		t.Fatalf("frames = %x, want one refreshed NPCInfo", frames.frames)
	}
	got := frames.frames[0]
	if got[0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("opcode = %#x, want NPCInfo (%#x)", got[0], serverpackets.OpcodeNPCInfo)
	}
}

func TestLivePlayerVisibilitySendsOwnerPetInfoAndBystanderSummonInfo(t *testing.T) {
	state := world.New()
	ownerFrames := &frameCapture{}
	bystanderFrames := &frameCapture{}
	owner := newTestLivePlayer(t, 1, ownerFrames)
	bystander := newTestLivePlayer(t, 2, bystanderFrames)
	owner.npcs = npc.NewTable([]*npc.Template{{
		ID: 12077, TemplateID: 12077, Name: "Wolf",
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20,
	}})
	bystander.npcs = owner.npcs

	state.Spawn(owner, 0, 0, 0, 0)
	state.Spawn(bystander, 500, 0, 0, 0)

	petInventory := itemcontainer.NewPetInventory(20, petTestTemplates())
	if petInventory.AddNew(57, 1, 21) == nil {
		t.Fatal("add pet inventory item")
	}
	pet := summon.NewPet(summon.PetConfig{
		ObjectID: 20, Owner: owner, NPCID: 12077, Name: "Wolf", Level: 5,
		Inventory: petInventory,
		Stats:     summon.CombatStats{MaxHP: 100, MaxMP: 30},
	})
	summon.SpawnBesideOwner(state, pet, owner, location.Location{X: 10})

	if n := len(ownerFrames.frames); n < 2 || ownerFrames.frames[n-2][0] != serverpackets.OpcodePetInfo || ownerFrames.frames[n-1][0] != serverpackets.OpcodePetItemList {
		t.Fatalf("owner last frames = %x, want PetInfo then PetItemList", ownerFrames.frames)
	}
	if updates := petInventory.DrainUpdates(); len(updates) != 0 {
		t.Fatalf("pet inventory updates after full snapshot = %v, want cleared", updates)
	}
	if n := len(bystanderFrames.frames); n == 0 || bystanderFrames.frames[n-1][0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("bystander last frame = %x, want SummonInfo (%#x)", bystanderFrames.frames, serverpackets.OpcodeNPCInfo)
	}

	state.Despawn(pet)
	if n := len(ownerFrames.frames); n == 0 || ownerFrames.frames[n-1][0] != serverpackets.OpcodePetDelete {
		t.Fatalf("owner last frame after pet despawn = %x, want PetDelete (%#x) last", ownerFrames.frames, serverpackets.OpcodePetDelete)
	}
	if n := len(bystanderFrames.frames); n == 0 || bystanderFrames.frames[n-1][0] != serverpackets.OpcodeDeleteObject {
		t.Fatalf("bystander last frame after pet despawn = %x, want DeleteObject", bystanderFrames.frames)
	}
}

func TestPetInfoSnapshotPetUsesPerLevelShotCounts(t *testing.T) {
	owner := newTestLivePlayer(t, 1, &frameCapture{})
	npcs := npc.NewTable([]*npc.Template{{
		ID: 12077, TemplateID: 12077, Name: "Wolf",
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		SSCount: 2, SPSCount: 2, // template (servitor) shot counts
	}})
	pet := summon.NewPet(summon.PetConfig{
		ObjectID: 20, Owner: owner, NPCID: 12077, Level: 5,
		Stats: summon.CombatStats{MaxHP: 100, MaxMP: 30, SSCount: 1, SPSCount: 1}, // per-level pet-row shot counts
	})

	snap, ok := petInfoSnapshot(pet, owner, npcs)
	if !ok {
		t.Fatalf("petInfoSnapshot() ok = false")
	}
	if snap.SoulShotsPerHit != 1 || snap.SpiritShotsPerHit != 1 {
		t.Fatalf("SoulShotsPerHit/SpiritShotsPerHit = %d/%d, want the per-level pet-row values 1/1 (not the template's 2/2)",
			snap.SoulShotsPerHit, snap.SpiritShotsPerHit)
	}
}

func TestPetInfoSnapshotServitorUsesTemplateShotCountsAndLifetimeFed(t *testing.T) {
	owner := newTestLivePlayer(t, 1, &frameCapture{})
	npcs := npc.NewTable([]*npc.Template{{
		ID: 14, TemplateID: 14, Name: "Servitor",
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		SSCount: 3, SPSCount: 4,
	}})
	servitor := summon.NewServitor(summon.ServitorConfig{
		ObjectID: 21, Owner: owner, NPCID: 14, Level: 40,
		Lifetime: summon.LifetimeState{TimeRemaining: 900, TotalLifeTime: 1200},
		Stats:    summon.CombatStats{MaxHP: 500, MaxMP: 200},
	})

	snap, ok := petInfoSnapshot(servitor, owner, npcs)
	if !ok {
		t.Fatalf("petInfoSnapshot() ok = false")
	}
	if snap.SoulShotsPerHit != 3 || snap.SpiritShotsPerHit != 4 {
		t.Fatalf("SoulShotsPerHit/SpiritShotsPerHit = %d/%d, want the template values 3/4 (servitors have no per-level row)",
			snap.SoulShotsPerHit, snap.SpiritShotsPerHit)
	}
	if snap.CurFed != 900 || snap.MaxFed != 1200 {
		t.Fatalf("CurFed/MaxFed = %d/%d, want the servitor's TimeRemaining/TotalLifeTime 900/1200", snap.CurFed, snap.MaxFed)
	}
}

func TestLivePlayerForgetSkipsObjectsItWouldNotDiscover(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	player := newTestLivePlayer(t, 1, frames)
	obj := &invisibleTracked{id: 2}

	state.Spawn(player, 0, 0, 0, 0)
	state.Spawn(obj, 100, 0, 0, 0)
	state.Despawn(obj)

	if len(frames.frames) != 0 {
		t.Fatalf("frames for non-live tracked object = %x, want none", frames.frames)
	}
}

func TestLivePlayerForgetDoesNotBlockOnFullVisibilityQueue(t *testing.T) {
	cipher, err := NewCipher(bytes.Repeat([]byte{0x11}, keySize))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	conn := fullQueueConn(t)

	player := newTestLivePlayer(t, 1, &frameCapture{})
	player.visibilitySend = NewSession(conn, cipher).trySendFrame
	done := make(chan struct{})
	go func() {
		player.Forget(player)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Forget blocked on a full visibility queue")
	}
}

func TestLivePlayerDiscoverDroppedItemDoesNotBlockOnFullVisibilityQueue(t *testing.T) {
	cipher, err := NewCipher(bytes.Repeat([]byte{0x11}, keySize))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	conn := fullQueueConn(t)
	s := NewSession(conn, cipher)

	player := newTestLivePlayer(t, 1, &frameCapture{})
	player.Character.SetFrameSender(s.SendFrame)
	player.visibilitySend = s.trySendFrame
	item := &visibleGroundItem{id: 2, itemID: 57, count: 1, dropperID: 1}
	done := make(chan struct{})
	go func() {
		player.Discover(item)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Discover blocked on a full visibility queue for a dropped item")
	}
}
