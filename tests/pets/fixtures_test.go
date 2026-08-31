package pets

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Fixture identifiers shared by every scenario: one wolf-pet collar, one
// wyvern collar, one decorative-summon kit, and wolf food, all mapped
// through the summon-item and NPC tables the boot wires.
const (
	wolfCollarID   = int32(9600)
	wyvernCollarID = int32(9601)
	treeKitID      = int32(9602)
	wolfFoodID     = int32(2515)
	wolfNPCID      = 12500
	wyvernNPCID    = 12621
	treeNPCID      = 13006

	// wolfLevelRow is the single pet-level stat row the wolf template
	// carries; every spawn starts (and every save round-trips) through it.
	wolfLevel        = 10
	wolfMaxHP        = 400
	wolfMaxMP        = 80
	wolfMaxMeal      = 3000
	wolfFeedSkill    = 2048
	wolfFeedAmount   = 500
	summonCreatureID = 2046

	// hostileX/Y/Z is the fixture monster spawn point (gameservertest's
	// hostile fixture) the pet-attack scenarios target.
	hostileX = 60
	hostileY = 20
	hostileZ = 30

	// petAttackAction/petReturnAction/petUnsummonAction are the pet
	// shortcut action ids.
	beastSoulshotID = int32(6645)

	petAttackAction   = int32(16)
	petReturnAction   = int32(19)
	petUnsummonAction = int32(52)
)

// wolfTemplate builds the pet NPC template every collar resolves to.
func wolfTemplate() *npc.Template {
	return &npc.Template{
		ID: wolfNPCID, Name: "Wolf", Level: wolfLevel,
		STR: 40, CON: 43, DEX: 30, INT: 22, WIT: 20, MEN: 20,
		BaseAttackRange: 40, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20,
		Pet: &npc.PetData{
			Food1:         int(wolfFoodID),
			AutoFeedLimit: 0.55, HungryLimit: 0.3, UnsummonLimit: 0.1,
			Levels: map[int]npc.PetLevelStats{
				wolfLevel: {MaxHP: wolfMaxHP, MaxMP: wolfMaxMP, PAtk: 100, PDef: 90, MAtk: 20, MDef: 40, MaxMeal: wolfMaxMeal, MealInNormal: 5, MealInBattle: 10, SSCount: 1},
			},
		},
	}
}

// treeTemplate builds the decorative Christmas-Tree NPC template.
func treeTemplate() *npc.Template {
	return &npc.Template{
		ID: treeNPCID, TemplateID: treeNPCID, Type: "ChristmasTree", Name: "Tree",
		AtkSpd: 300, WalkSpeed: 30, RunSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
}

// petSummonItems maps the fixture collar items to their summon kinds.
func petSummonItems() *item.SummonItemTable {
	items, err := item.NewSummonItemTable([]item.SummonItem{
		{ItemID: wolfCollarID, NPCID: wolfNPCID, SummonType: 1},
		{ItemID: wyvernCollarID, NPCID: wyvernNPCID, SummonType: 2},
		{ItemID: treeKitID, NPCID: treeNPCID, SummonType: 0},
	})
	if err != nil {
		panic(err)
	}
	return items
}

// petSkillTable defines the two skills the pet flows drive: the collar
// cast's SUMMON_CREATURE (zero hit time so the Hit-phase spawn lands on the
// first timer sweep) and wolf food's feed skill.
func petSkillTable(t testing.TB) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		{ID: wolfFeedSkill, Level: 1, Feed: wolfFeedAmount},
	}), gamesql.NewCharacterSkillStore(db))
}

// bootPets boots the stack with the pet fixture data wired and one
// selectable character, still before the character-select screen.
func bootPets(t *testing.T, extra ...gameservertest.Option) *gameservertest.Server {
	t.Helper()
	opts := []gameservertest.Option{
		gameservertest.WithCharacter("Owner", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), treeTemplate()})),
		gameservertest.WithSummonItems(petSummonItems()),
		gameservertest.WithSkills(petSkillTable(t)),
	}
	return gameservertest.Boot(t, append(opts, extra...)...)
}

// petWorld boots the stack and brings the owner in the world with a wolf
// collar plus any extra seeded inventory in place before the enter-world
// load, so every stack is live in the client's inventory.
type petWorld struct {
	srv      *gameservertest.Server
	client   *testsupport.ScriptedClient
	ownerID  int32
	collarID int32
	seeded   map[int32][]int32
}

// seedItem is one {templateID, count} pair to persist before the owner
// enters the world.
type seedItem struct {
	TemplateID int32
	Count      int32
}

func bootOwnerWithCollar(t *testing.T, seeds ...seedItem) *petWorld {
	t.Helper()
	srv := bootPets(t)
	ownerID := srv.SoleObjectID(t)
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	for _, s := range seeds {
		id := srv.GiveItem(t, ownerID, s.TemplateID, s.Count)
		h.seeded[s.TemplateID] = append(h.seeded[s.TemplateID], id)
	}
	startInWorld(t, h.client)
	return h
}

// seededItem returns the first seeded stack's object id for a template.
func (h *petWorld) seededItem(t *testing.T, templateID int32) int32 {
	t.Helper()
	ids := h.seeded[templateID]
	if len(ids) == 0 {
		t.Fatalf("no seeded item for template %d", templateID)
	}
	return ids[0]
}

// returnPet sends the return command (action 19) and consumes the despawn
// frames, leaving the pets row persisted.
func (h *petWorld) returnPet(t *testing.T) {
	t.Helper()
	h.client.Send(encodeRequestActionUse(19, false))
	readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete")
	waitFor(t, "pets row saved", func() bool {
		_, ok, err := h.srv.Pets.Get(context.Background(), h.collarID)
		return err == nil && ok
	})
	drainUntilQuiet(t, h.client)
}

// spawnWolf uses the collar through the item window and waits for the pet
// to appear in world state, returning the actor plus every frame captured
// between the cast broadcast and a quiet stream (MagicSkillLaunched,
// PetInfo, PetItemList, relation updates).
func (h *petWorld) spawnWolf(t *testing.T) (*summon.Actor, [][]byte) {
	t.Helper()
	h.client.Send(encodeUseItem(h.collarID, false))
	frame := mustRead(t, h.client, "SUMMON_A_PET system message")
	assertStaticSystemMessage(t, frame, serverpackets.SystemMessageSummonAPet)
	frame = mustRead(t, h.client, "collar MagicSkillUse")
	assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, "collar MagicSkillUse")
	r := wire.NewReader(frame[1:])
	if caster, _, skill, level := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != h.ownerID || skill != summonCreatureID || level != 1 {
		t.Fatalf("collar cast = caster %d skill %d level %d, want %d/%d/1", caster, skill, level, h.ownerID, summonCreatureID)
	}
	var actor *summon.Actor
	waitFor(t, "pet in world state", func() bool {
		obj, ok := h.srv.State.Summon(h.ownerID)
		if !ok {
			return false
		}
		actor, ok = obj.(*summon.Actor)
		return ok
	})
	burst := drainFrames(t, h.client)
	return actor, burst
}

// savedPetState reads the persisted pets row for the collar.
func (h *petWorld) savedPetState(t *testing.T) pet.State {
	t.Helper()
	state, ok, err := h.srv.Pets.Get(context.Background(), h.collarID)
	if err != nil || !ok {
		t.Fatalf("pets row for collar %d: ok=%v err=%v", h.collarID, ok, err)
	}
	return state
}

// ownerItemCount flushes pending item mutations and sums the owner's
// persisted stacks of one template.
func (h *petWorld) ownerItemCount(t *testing.T, templateID int32) int {
	t.Helper()
	h.srv.FlushItems(t)
	rows, err := h.srv.Items.ListByOwner(petCtx(), h.ownerID)
	if err != nil {
		t.Fatalf("list owner items: %v", err)
	}
	count := 0
	for _, row := range rows {
		if row.TemplateID == templateID {
			count += row.Count
		}
	}
	return count
}

// giveToPet hands an owner inventory stack (or part of it) to the active
// pet through the give flow, syncing on a RequestItemList round-trip before
// the batching tick so the transfer has settled, then consuming both
// inventory-update frames in wire order.
func (h *petWorld) giveToPet(t *testing.T, objectID, count int32) {
	t.Helper()
	h.client.Send(encodeRequestGiveItemToPet(objectID, count))
	testsupport.SyncBarrier(t, h.client, func() {
		h.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}, serverpackets.OpcodeItemList)
	drainFrames(t, h.client)
	h.srv.InventoryUpdates.Tick()
	frames := drainFrames(t, h.client)
	if len(frames) < 2 {
		t.Fatalf("transfer frames = %d, want PetInventoryUpdate then InventoryUpdate", len(frames))
	}
	assertFrameOpcode(t, frames[0], serverpackets.OpcodePetInventoryUpdate, "pet-side update")
	assertFrameOpcode(t, frames[1], serverpackets.OpcodeInventoryUpdate, "owner-side update")
}

func petCtx() context.Context { return context.Background() }

func encodeRequestGameStart(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(slot)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeEnterWorld() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes()
}

func encodeSingleOpcode(opcode byte) []byte {
	return wire.NewPacketWriter(opcode).Bytes()
}

func encodeUseItem(objectID int32, ctrl bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	return w.Bytes()
}

func encodeRequestActionUse(actionID int32, ctrl bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestActionUse)
	w.WriteInt32(actionID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	w.WriteUint8(0)
	return w.Bytes()
}

func encodeAction(objectID int32, x, y, z int32, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeRequestGiveItemToPet(objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGiveItemToPet)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

func encodeRequestGetItemFromPet(objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGetItemFromPet)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	w.WriteInt32(0) // Unknown trailing field the reference packet carries
	return w.Bytes()
}

func encodeRequestPetGetItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPetGetItem)
	w.WriteInt32(objectID)
	return w.Bytes()
}

func encodeRequestPetUseItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPetUseItem)
	w.WriteInt32(objectID)
	return w.Bytes()
}

func encodeRequestChangePetName(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestChangePetName)
	w.WriteString(name)
	return w.Bytes()
}

func encodeRequestDropItem(objectID, count, x, y, z int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestDropItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	return w.Bytes()
}

// startInWorld selects slot 0 and enters the world, consuming the fixed
// EnterWorld reply burst plus every trailing frame.
func startInWorld(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo (%#x)", reply[0], serverpackets.OpcodeSSQInfo)
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected (%#x)", reply[0], serverpackets.OpcodeCharSelected)
	}
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	drainUntilQuiet(t, c)
}

func readEnterWorldBurst(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	want := []byte{
		serverpackets.OpcodeSendMacroList,
		serverpackets.OpcodeExtended,
		serverpackets.OpcodeHennaInfo,
		serverpackets.OpcodeEtcStatusUpdate,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeQuestList,
		serverpackets.OpcodeSkillList,
		serverpackets.OpcodeFriendList,
		serverpackets.OpcodeUserInfo,
		serverpackets.OpcodeItemList,
		serverpackets.OpcodeShortCutInit,
		serverpackets.OpcodeSkillCoolTime,
		serverpackets.OpcodeActionFailed,
	}
	for i, opcode := range want {
		var frame []byte
		for {
			frame = c.Read()
			if frame == nil {
				t.Fatalf("EnterWorld frame %d (want %#x) never arrived", i, opcode)
			}
			// Skip nearby CharInfo/NPCInfo (owner, pets, NPCs) interleaved ahead
			// of this client's own EnterWorld burst.
			if frame[0] != serverpackets.OpcodeCharInfo && frame[0] != serverpackets.OpcodeNPCInfo {
				break
			}
		}
		if frame[0] != opcode {
			t.Fatalf("EnterWorld frame %d opcode = %#x, want %#x", i, frame[0], opcode)
		}
	}
}

func mustRead(t *testing.T, c *testsupport.ScriptedClient, what string) []byte {
	t.Helper()
	frame := c.Read()
	if frame == nil {
		t.Fatalf("%s never arrived", what)
	}
	return frame
}

func drainUntilQuiet(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if c.ReadWithTimeout(300*time.Millisecond) == nil {
			return
		}
	}
	t.Fatal("client kept receiving frames after 100 drains")
}

// drainFrames collects every frame the client receives until the server
// goes quiet, returning them in arrival order.
func drainFrames(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	frames := make([][]byte, 0, 8)
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 100 drains")
	return nil
}

// readUntilOpcode collects frames until one carries want, returning every
// frame read including the match.
func readUntilOpcode(t *testing.T, c *testsupport.ScriptedClient, want byte, what string) [][]byte {
	t.Helper()
	frames := make([][]byte, 0, 4)
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			t.Fatalf("%s never arrived", what)
		}
		frames = append(frames, frame)
		if frame[0] == want {
			return frames
		}
	}
	t.Fatalf("%s not found within 100 frames", what)
	return nil
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s not observed within 5s", what)
}

func assertFrameOpcode(t *testing.T, frame []byte, want byte, what string) {
	t.Helper()
	if frame[0] != want {
		t.Fatalf("%s opcode = %#x, want %#x", what, frame[0], want)
	}
}

// assertStaticSystemMessage asserts a parameterless SystemMessage id.
func assertStaticSystemMessage(t *testing.T, frame []byte, messageID int) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != int32(messageID) {
		t.Fatalf("system message id = %d, want %d", id, messageID)
	}
	if params := r.ReadInt32(); params != 0 {
		t.Fatalf("system message params = %d, want 0", params)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}

// assertSystemMessageID asserts only a SystemMessage's id, for messages
// whose parameter shape the scenario doesn't depend on.
func assertSystemMessageID(t *testing.T, frame []byte, messageID int) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	if id := wire.NewReader(frame[1:]).ReadInt32(); id != int32(messageID) {
		t.Fatalf("system message id = %d, want %d", id, messageID)
	}
}

// assertSystemMessageText asserts a SystemMessage with one text param.
func assertSystemMessageText(t *testing.T, frame []byte, messageID int, text string) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != int32(messageID) {
		t.Fatalf("system message id = %d, want %d", id, messageID)
	}
	if params := r.ReadInt32(); params != 1 {
		t.Fatalf("param count = %d, want 1", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText {
		t.Fatalf("param type = %d, want text", typ)
	}
	if got := r.ReadString(); got != text {
		t.Fatalf("system message text = %q, want %q", got, text)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}

// readPetInfoName parses a PetInfo frame's summon-type and name fields,
// walking the fixed field sequence the writer emits ahead of the name.
func readPetInfoName(t *testing.T, frame []byte) (int32, string) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodePetInfo, "PetInfo")
	r := wire.NewReader(frame[1:])
	summonType := r.ReadInt32()
	for i := 1; i < 19; i++ {
		r.ReadInt32()
	}
	for i := 0; i < 4; i++ {
		r.ReadFloat64()
	}
	for i := 0; i < 3; i++ {
		r.ReadInt32()
	}
	for i := 0; i < 5; i++ {
		r.ReadUint8()
	}
	name := r.ReadString()
	if err := r.Err(); err != nil {
		t.Fatalf("read PetInfo: %v", err)
	}
	return summonType, name
}
