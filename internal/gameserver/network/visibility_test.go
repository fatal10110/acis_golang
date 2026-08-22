package network

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	gamecipher "github.com/fatal10110/acis_golang/internal/gameserver/network/cipher"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

type summonDamageAttacker struct{ name string }

func (a summonDamageAttacker) ObjectID() int32       { return 1 }
func (a summonDamageAttacker) CharacterName() string { return a.name }

// fullQueueConn builds a conn whose outbound queue is already full, for
// asserting that visibility sends never block on it.
func fullQueueConn(t *testing.T) *Conn {
	t.Helper()
	serverRaw, clientRaw := net.Pipe()
	t.Cleanup(func() { serverRaw.Close(); clientRaw.Close() })
	conn := &Conn{Conn: serverRaw, out: make(chan queuedWrite, outboundBuffer), stopping: make(chan struct{})}
	for range outboundBuffer {
		conn.out <- queuedWrite{frame: wire.BorrowedFrame(mustFrameBytes([]byte{0x02, 0x00}))}
	}
	return conn
}

func mustFrameBytes(payload []byte) []byte {
	frame, err := wire.FrameBytes(payload)
	if err != nil {
		panic(err)
	}
	return frame
}

func TestLivePlayerVisibilitySendsCharInfoAndDeleteObject(t *testing.T) {
	state := world.New()
	firstFrames := &testsupport.FrameCapture{}
	secondFrames := &testsupport.FrameCapture{}
	first := newTestLivePlayer(t, 1, firstFrames)
	second := newTestLivePlayer(t, 2, secondFrames)

	state.Spawn(first, 0, 0, 0, 0)
	state.Spawn(second, 100, 0, 0, 0)

	if len(firstFrames.Frames()) != 1 || firstFrames.Frames()[0][0] != serverpackets.OpcodeCharInfo {
		t.Fatalf("first player frames = %x, want one CharInfo", firstFrames.Frames())
	}
	if len(secondFrames.Frames()) != 1 || secondFrames.Frames()[0][0] != serverpackets.OpcodeCharInfo {
		t.Fatalf("second player frames = %x, want one CharInfo", secondFrames.Frames())
	}

	state.Despawn(second)
	if got := firstFrames.Frames()[len(firstFrames.Frames())-1][0]; got != serverpackets.OpcodeDeleteObject {
		t.Fatalf("last first-player frame opcode = %#x, want DeleteObject (%#x)", got, serverpackets.OpcodeDeleteObject)
	}
}

func TestPetInfoSnapshotMirrorsOwnerPvPFlag(t *testing.T) {
	owner := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
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
	frames := &testsupport.FrameCapture{}
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
	if got := testsupport.FrameOpcodes(frames.Frames()); string(got) != string(want) {
		t.Fatalf("spawn opcodes = %x, want %x", got, want)
	}

	state.Despawn(invisible)
	if got := testsupport.FrameOpcodes(frames.Frames()); string(got) != string(want) {
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
	if got := testsupport.FrameOpcodes(frames.Frames()); string(got) != string(want) {
		t.Fatalf("despawn opcodes = %x, want %x", got, want)
	}
}

func TestLivePlayerVisibilityRendersHostileNPC(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	viewer := newTestLivePlayer(t, 1, frames)
	state.Spawn(viewer, 0, 0, 0, 0)

	hostile := newTestHostileNPC(t, 20)
	state.Spawn(hostile, 100, 0, -50, 123)

	const opcodeNPCInfo = 0x16
	if len(frames.Frames()) != 1 {
		t.Fatalf("frames = %x, want one NPCInfo frame", frames.Frames())
	}
	got := frames.Frames()[0]
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
	if len(frames.Frames()) != 2 || frames.Frames()[1][0] != serverpackets.OpcodeDeleteObject {
		t.Fatalf("frames after NPC despawn = %x, want DeleteObject after NPCInfo", frames.Frames())
	}
}

func TestHostileAbnormalEffectRefreshResendsLiveNPCInfo(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	viewer := newTestLivePlayer(t, 1, frames)
	state.Spawn(viewer, 0, 0, 0, 0)

	hostile := newTestHostileNPC(t, 20)
	hostile.SetWorld(state)
	state.Spawn(hostile, 100, 0, -50, 123)
	testsupport.ResetCapture(frames)

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

	if len(frames.Frames()) != 1 {
		t.Fatalf("frames = %x, want one refreshed NPCInfo", frames.Frames())
	}
	got := frames.Frames()[0]
	if got[0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("opcode = %#x, want NPCInfo (%#x)", got[0], serverpackets.OpcodeNPCInfo)
	}
}

func TestSummonAbnormalEffectRefreshesOwnerPetInfoThenBystanderSummonInfo(t *testing.T) {
	state := world.New()
	ownerFrames := &testsupport.FrameCapture{}
	bystanderFrames := &testsupport.FrameCapture{}
	owner := newTestLivePlayer(t, 1, ownerFrames)
	bystander := newTestLivePlayer(t, 2, bystanderFrames)
	owner.npcs = npc.NewTable([]*npc.Template{{ID: 12077, TemplateID: 12077, Name: "Wolf", AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60}})
	bystander.npcs = owner.npcs
	state.Spawn(owner, 0, 0, 0, 0)
	state.Spawn(bystander, 500, 0, 0, 0)

	pet := summon.NewPet(summon.PetConfig{ObjectID: 20, Owner: owner, NPCID: 12077, Name: "Wolf", Level: 5, Stats: summon.CombatStats{MaxHP: 100, MaxMP: 30}})
	summon.SpawnBesideOwner(state, pet, owner, location.Location{X: 10})
	testsupport.ResetCapture(ownerFrames)
	testsupport.ResetCapture(bystanderFrames)

	pet.StartAbnormalEffect(0x010000)
	pet.UpdateAbnormalEffect()

	snapshot, ok := petInfoSnapshot(pet, owner, owner.npcs)
	if !ok || snapshot.AbnormalEffect != 0x010000 {
		t.Fatalf("PetInfo abnormal effect = %#x, %v; want %#x, true", snapshot.AbnormalEffect, ok, 0x010000)
	}
	if len(ownerFrames.Frames()) != 1 || ownerFrames.Frames()[0][0] != serverpackets.OpcodePetInfo {
		t.Fatalf("owner refresh frames = %x, want one PetInfo (%#x)", ownerFrames.Frames(), serverpackets.OpcodePetInfo)
	}
	if len(bystanderFrames.Frames()) != 1 || bystanderFrames.Frames()[0][0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("bystander refresh frames = %x, want one SummonInfo (%#x)", bystanderFrames.Frames(), serverpackets.OpcodeNPCInfo)
	}
	snap, ok := summonInfoSnapshot(pet, bystander.npcs)
	if !ok || snap.AbnormalEffect != 0x010000 {
		t.Fatalf("SummonInfo abnormal effect = %#x, %v; want %#x, true", snap.AbnormalEffect, ok, 0x010000)
	}
}

func TestLivePlayerVisibilitySendsOwnerPetInfoAndBystanderSummonInfo(t *testing.T) {
	state := world.New()
	ownerFrames := &testsupport.FrameCapture{}
	bystanderFrames := &testsupport.FrameCapture{}
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

	if n := len(ownerFrames.Frames()); n < 2 || ownerFrames.Frames()[n-2][0] != serverpackets.OpcodePetInfo || ownerFrames.Frames()[n-1][0] != serverpackets.OpcodePetItemList {
		t.Fatalf("owner last frames = %x, want PetInfo then PetItemList", ownerFrames.Frames())
	}
	if updates := petInventory.DrainUpdates(); len(updates) != 0 {
		t.Fatalf("pet inventory updates after full snapshot = %v, want cleared", updates)
	}
	if n := len(bystanderFrames.Frames()); n == 0 || bystanderFrames.Frames()[n-1][0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("bystander last frame = %x, want SummonInfo (%#x)", bystanderFrames.Frames(), serverpackets.OpcodeNPCInfo)
	}

	state.Despawn(pet)
	if n := len(ownerFrames.Frames()); n == 0 || ownerFrames.Frames()[n-1][0] != serverpackets.OpcodePetDelete {
		t.Fatalf("owner last frame after pet despawn = %x, want PetDelete (%#x) last", ownerFrames.Frames(), serverpackets.OpcodePetDelete)
	}
	if n := len(bystanderFrames.Frames()); n == 0 || bystanderFrames.Frames()[n-1][0] != serverpackets.OpcodeDeleteObject {
		t.Fatalf("bystander last frame after pet despawn = %x, want DeleteObject", bystanderFrames.Frames())
	}
}

func TestSummonDamagePublishesPetStatusToOwnerAndSummonInfoToObservers(t *testing.T) {
	state := world.New()
	ownerFrames := &testsupport.FrameCapture{}
	bystanderFrames := &testsupport.FrameCapture{}
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
	pet := summon.NewPet(summon.PetConfig{
		ObjectID: 20, Owner: owner, NPCID: 12077, Level: 5,
		Stats: summon.CombatStats{MaxHP: 100, MaxMP: 30},
	})
	gcl := &GameClientLink{world: state}
	gcl.wireSummonAI(pet)
	summon.SpawnBesideOwner(state, pet, owner, location.Location{X: 10})
	testsupport.ResetCapture(ownerFrames)
	testsupport.ResetCapture(bystanderFrames)

	for _, damage := range []struct {
		name  string
		apply func()
	}{
		{"direct", func() { pet.ReduceHP(10, nil, modelskill.Definition{}) }},
		{"dot", func() { pet.ReduceHPByDOT(10, nil, true) }},
	} {
		t.Run(damage.name, func(t *testing.T) {
			testsupport.ResetCapture(ownerFrames)
			testsupport.ResetCapture(bystanderFrames)
			damage.apply()

			if got := testsupport.FrameOpcodes(ownerFrames.Frames()); string(got) != string([]byte{serverpackets.OpcodePetStatusUpdate}) {
				t.Fatalf("owner opcodes = %x, want PetStatusUpdate", got)
			}
			if got := testsupport.FrameOpcodes(bystanderFrames.Frames()); string(got) != string([]byte{serverpackets.OpcodeNPCInfo}) {
				t.Fatalf("bystander opcodes = %x, want SummonInfo", got)
			}
		})
	}
}

func TestSummonDamageSendsOwnerSystemMessageForPetAndServitor(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message int
		new     func(*livePlayer) *summon.Actor
	}{
		{"pet", serverpackets.SystemMessagePetReceivedS2DamageByS1, func(owner *livePlayer) *summon.Actor {
			return summon.NewPet(summon.PetConfig{ObjectID: 20, Owner: owner, NPCID: 12077, Stats: summon.CombatStats{MaxHP: 100}})
		}},
		{"servitor", serverpackets.SystemMessageSummonReceivedS2ByS1, func(owner *livePlayer) *summon.Actor {
			return summon.NewServitor(summon.ServitorConfig{ObjectID: 21, Owner: owner, NPCID: 12077, Stats: summon.CombatStats{MaxHP: 100}})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := world.New()
			ownerFrames := &testsupport.FrameCapture{}
			owner := newTestLivePlayer(t, 1, ownerFrames)
			attacker := newTestLivePlayer(t, 2, &testsupport.FrameCapture{})
			owner.npcs = npc.NewTable([]*npc.Template{{ID: 12077, TemplateID: 12077, Name: "Wolf", AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20}})
			state.AddPlayer(owner)
			state.AddPlayer(attacker)
			state.Spawn(owner, 0, 0, 0, 0)
			state.Spawn(attacker, 100, 0, 0, 0)
			actor := tc.new(owner)
			link := &GameClientLink{world: state}
			link.wireSummonAI(actor)
			testsupport.ResetCapture(ownerFrames)

			actor.ReduceHP(12.9, summonDamageAttacker{name: "Attacker"}, modelskill.Definition{})

			if len(ownerFrames.Frames()) != 2 {
				t.Fatalf("owner frames = %d, want status update and damage message", len(ownerFrames.Frames()))
			}
			assertSystemMessageStringNumberFrame(t, ownerFrames.Frames()[1], tc.message, "Attacker", 12)
		})
	}
}

func assertSystemMessageStringNumberFrame(t *testing.T, frame []byte, messageID int, text string, number int32) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("SystemMessage opcode = %#x, want %#x", frame[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(messageID) {
		t.Fatalf("SystemMessage id = %d, want %d", got, messageID)
	}
	if got := r.ReadInt32(); got != 2 {
		t.Fatalf("SystemMessage params = %d, want 2", got)
	}
	if got := r.ReadInt32(); got != serverpackets.SystemMessageParamText {
		t.Fatalf("SystemMessage first param type = %d, want text", got)
	}
	if got := r.ReadString(); got != text {
		t.Fatalf("SystemMessage text = %q, want %q", got, text)
	}
	if got := r.ReadInt32(); got != serverpackets.SystemMessageParamNumber {
		t.Fatalf("SystemMessage second param type = %d, want number", got)
	}
	if got := r.ReadInt32(); got != number {
		t.Fatalf("SystemMessage number = %d, want %d", got, number)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}

func TestSummonStatusObserverFramesAreIndependent(t *testing.T) {
	state := world.New()
	owner := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
	first := newTestLivePlayer(t, 2, &testsupport.FrameCapture{})
	second := newTestLivePlayer(t, 3, &testsupport.FrameCapture{})
	owner.npcs = npc.NewTable([]*npc.Template{{ID: 12077, TemplateID: 12077, AtkSpd: 300}})
	state.Spawn(owner, 0, 0, 0, 0)
	state.Spawn(first, 100, 0, 0, 0)
	state.Spawn(second, 200, 0, 0, 0)
	pet := summon.NewPet(summon.PetConfig{ObjectID: 20, Owner: owner, NPCID: 12077, Stats: summon.CombatStats{MaxHP: 100}})
	gcl := &GameClientLink{world: state}
	gcl.wireSummonAI(pet)
	summon.SpawnBesideOwner(state, pet, owner, location.Location{X: 10})

	var firstFrame, secondFrame wire.Frame
	first.Character.SetFrameSender(func(frame wire.Frame) bool { firstFrame = frame; return true })
	second.Character.SetFrameSender(func(frame wire.Frame) bool { secondFrame = frame; return true })
	first.Character.SetBroadcastFrameSender(func(frame wire.Frame) bool { firstFrame = frame; return true })
	second.Character.SetBroadcastFrameSender(func(frame wire.Frame) bool { secondFrame = frame; return true })
	pet.ReduceHP(10, nil, modelskill.Definition{})
	defer firstFrame.Release()
	defer secondFrame.Release()

	if len(firstFrame.Bytes()) <= wire.FrameHeaderSize || len(secondFrame.Bytes()) <= wire.FrameHeaderSize {
		t.Fatal("observers did not receive status frames")
	}
	secondPayload := secondFrame.Bytes()[wire.FrameHeaderSize]
	firstFrame.Bytes()[wire.FrameHeaderSize] ^= 0xff
	if secondFrame.Bytes()[wire.FrameHeaderSize] != secondPayload {
		t.Fatal("mutating one observer frame changed another")
	}
}

func TestPetInfoSnapshotPetUsesPerLevelShotCounts(t *testing.T) {
	owner := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
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
	owner := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
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
	frames := &testsupport.FrameCapture{}
	player := newTestLivePlayer(t, 1, frames)
	obj := &invisibleTracked{id: 2}

	state.Spawn(player, 0, 0, 0, 0)
	state.Spawn(obj, 100, 0, 0, 0)
	state.Despawn(obj)

	if len(frames.Frames()) != 0 {
		t.Fatalf("frames for non-live tracked object = %x, want none", frames.Frames())
	}
}

func TestLivePlayerForgetDoesNotBlockOnFullVisibilityQueue(t *testing.T) {
	cipher, err := gamecipher.NewCipher(bytes.Repeat([]byte{0x11}, gamecipher.KeySize))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	conn := fullQueueConn(t)

	player := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
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
	cipher, err := gamecipher.NewCipher(bytes.Repeat([]byte{0x11}, gamecipher.KeySize))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	conn := fullQueueConn(t)
	s := NewSession(conn, cipher)

	player := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
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
