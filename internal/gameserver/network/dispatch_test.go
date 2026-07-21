package network

import (
	"bytes"
	"context"
	"net"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/link"
)

// --- fake character/item stores: Roster's own persistence seam, no DB needed ---

type fakeCharStore struct {
	mu             sync.Mutex
	byID           map[int32]*player.Character
	names          map[string]bool
	savedPositions map[int32]savedPosition
}

func newFakeCharStore() *fakeCharStore {
	return &fakeCharStore{byID: map[int32]*player.Character{}, names: map[string]bool{}, savedPositions: map[int32]savedPosition{}}
}

type savedPosition struct {
	location    location.Location
	heading     int
	ctxErr      error
	hasDeadline bool
	deadline    time.Time
}

func (s *fakeCharStore) Create(_ context.Context, c *player.Character) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[c.ID] = c
	s.names[c.Name] = true
	return nil
}

func (s *fakeCharStore) ListByAccount(_ context.Context, account string) ([]*player.Character, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*player.Character
	for _, c := range s.byID {
		if c.AccountName == account {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *fakeCharStore) CountByAccount(ctx context.Context, account string) (int, error) {
	list, _ := s.ListByAccount(ctx, account)
	return len(list), nil
}

func (s *fakeCharStore) NameTaken(_ context.Context, name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.names[name], nil
}

func (s *fakeCharStore) SetDeleteAt(_ context.Context, id int32, at int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.byID[id]; ok {
		c.DeleteAt = at
	}
	return nil
}

func (s *fakeCharStore) SetPosition(ctx context.Context, id int32, loc location.Location, heading int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.byID[id]; ok {
		c.Location = loc
		c.LastHeading = heading
	}
	deadline, hasDeadline := ctx.Deadline()
	s.savedPositions[id] = savedPosition{
		location:    loc,
		heading:     heading,
		ctxErr:      ctx.Err(),
		hasDeadline: hasDeadline,
		deadline:    deadline,
	}
	return ctx.Err()
}

func (s *fakeCharStore) Delete(_ context.Context, id int32) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.byID[id]
	delete(s.byID, id)
	return ok, nil
}

func (s *fakeCharStore) deleteAt(id int32) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.byID[id].DeleteAt
}

func (s *fakeCharStore) soleObjectID(t *testing.T) int32 {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.byID) != 1 {
		t.Fatalf("fakeCharStore has %d characters, want 1", len(s.byID))
	}
	for id := range s.byID {
		return id
	}
	return 0
}

func (s *fakeCharStore) savedPosition(t *testing.T, id int32) savedPosition {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	pos, ok := s.savedPositions[id]
	if !ok {
		t.Fatalf("character %d position was not saved", id)
	}
	return pos
}

func (s *fakeCharStore) updateCharacter(t *testing.T, id int32, update func(*player.Character)) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.byID[id]
	if !ok {
		t.Fatalf("character %d missing", id)
	}
	update(ch)
}

type fakeItemStore struct {
	mu    sync.Mutex
	items map[int32][]*item.Instance
}

func newFakeItemStore() *fakeItemStore {
	return &fakeItemStore{items: map[int32][]*item.Instance{}}
}

func (s *fakeItemStore) Create(_ context.Context, ownerID int32, inst item.Instance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := inst
	s.items[ownerID] = append(s.items[ownerID], &cp)
	return nil
}

func (s *fakeItemStore) DeleteByOwner(_ context.Context, ownerID int32) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := int64(len(s.items[ownerID]))
	delete(s.items, ownerID)
	return n, nil
}

func (s *fakeItemStore) ListByOwner(_ context.Context, ownerID int32) ([]*item.Instance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*item.Instance(nil), s.items[ownerID]...), nil
}

func (s *fakeItemStore) Save(_ context.Context, inst *item.Instance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ownerID, items := range s.items {
		for i, existing := range items {
			if existing.ObjectID == inst.ObjectID {
				cp := *inst
				s.items[ownerID][i] = &cp
				return nil
			}
		}
	}
	cp := *inst
	s.items[inst.OwnerID] = append(s.items[inst.OwnerID], &cp)
	return nil
}

func (s *fakeItemStore) Update(ctx context.Context, inst *item.Instance) error {
	return s.Save(ctx, inst)
}

func (s *fakeItemStore) Delete(_ context.Context, objectID int32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ownerID, items := range s.items {
		for i, existing := range items {
			if existing.ObjectID == objectID {
				s.items[ownerID] = append(items[:i], items[i+1:]...)
				return nil
			}
		}
	}
	return nil
}

type fakeShortcutStore struct {
	mu      sync.Mutex
	byOwner map[int32][]shortcut.Shortcut
}

func newFakeShortcutStore() *fakeShortcutStore {
	return &fakeShortcutStore{byOwner: map[int32][]shortcut.Shortcut{}}
}

func (s *fakeShortcutStore) ListByOwner(_ context.Context, ownerID int32) ([]shortcut.Shortcut, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]shortcut.Shortcut(nil), s.byOwner[ownerID]...), nil
}

func (s *fakeShortcutStore) Save(_ context.Context, ownerID int32, sc shortcut.Shortcut) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := shortcut.NewList(s.byOwner[ownerID])
	list.Register(sc)
	s.byOwner[ownerID] = list.All()
	return nil
}

func (s *fakeShortcutStore) Delete(_ context.Context, ownerID int32, slot, page int32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := shortcut.NewList(s.byOwner[ownerID])
	list.Delete(slot, page)
	s.byOwner[ownerID] = list.All()
	return nil
}

func (s *fakeShortcutStore) DeleteByOwner(_ context.Context, ownerID int32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byOwner, ownerID)
	return nil
}

func (s *fakeShortcutStore) seed(ownerID int32, shortcuts ...shortcut.Shortcut) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byOwner[ownerID] = append([]shortcut.Shortcut(nil), shortcuts...)
}

func (s *fakeShortcutStore) shortcuts(ownerID int32) []shortcut.Shortcut {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]shortcut.Shortcut(nil), s.byOwner[ownerID]...)
}

type sequentialIDs struct{ next int32 }

func (s *sequentialIDs) NextID() (int32, error) {
	s.next++
	return s.next, nil
}

func testTemplates(t *testing.T) *player.TemplateTable {
	t.Helper()
	tmpl := &player.Template{
		ID:        0,
		BaseLevel: 1,
		HPTable:   []float64{80},
		MPTable:   []float64{30},
		CPTable:   []float64{32},
		Spawns:    []location.Location{{X: 10, Y: 20, Z: 30}},
		RunSpeed:  120,
		WalkSpeed: 60,
		SwimSpeed: 50,
		Skills:    []player.SkillGrant{{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50}},
	}
	table, err := player.NewTemplateTable(map[int]*player.Template{0: tmpl})
	if err != nil {
		t.Fatalf("build template table: %v", err)
	}
	return table
}

func testItemTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{
			ID:          item.AdenaID,
			Name:        "Adena",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			Depositable: true,
			EtcItem:     &item.EtcItemDetail{},
		},
		{
			ID:          20,
			Name:        "Potion",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			Depositable: true,
			EtcItem:     &item.EtcItemDetail{Type: item.EtcItemPotion},
		},
		{
			ID:            1463,
			Name:          "Soulshot: No Grade",
			Kind:          item.KindEtcItem,
			Duration:      -1,
			Stackable:     true,
			DefaultAction: item.ActionSoulshot,
			EtcItem:       &item.EtcItemDetail{Type: item.EtcItemShot},
		},
		{
			ID:           30,
			Name:         "Sword",
			Kind:         item.KindWeapon,
			Slot:         item.SlotRHand,
			Duration:     -1,
			Crystal:      item.CrystalD,
			CrystalCount: 10,
			Dropable:     true,
			Tradable:     true,
			Destroyable:  true,
			Depositable:  true,
			Weapon:       &item.WeaponDetail{Type: item.WeaponSword},
		},
		{
			ID:        item.CrystalD.ItemID(),
			Name:      "D-grade Crystal",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{},
		},
		{
			ID:        955,
			Name:      "Scroll: Enchant Weapon (D)",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{Type: item.EtcItemScrollEnchantWeapon, Handler: "EnchantScrolls"},
		},
		{
			ID:             1060,
			Name:           "Lesser Healing Potion",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			Depositable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemPotion, Handler: "ItemSkills", ReuseDelay: 10000, SharedReuseGroup: 8},
			AttachedSkills: []item.SkillRef{{ID: 2031, Level: 1}},
		},
		{
			ID:             728,
			Name:           "Mana Potion",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			Depositable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemPotion, Handler: "ItemSkills", ReuseDelay: 2000},
			AttachedSkills: []item.SkillRef{{ID: 2279, Level: 2}},
		},
		{
			ID:             736,
			Name:           "Scroll: Escape",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			Depositable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemScroll, Handler: "ItemSkills"},
			AttachedSkills: []item.SkillRef{{ID: 2013, Level: 1}},
		},
	})
}

// --- fake game client, driving the wire protocol from the other side ---
//
// A real game client speaks first: it sends ProtocolVersion cleartext,
// receives VersionCheck cleartext, then arms the rolling XOR cipher from
// VersionCheck's 8 random bytes plus the fixed static key half.

type fakeGameClient struct {
	t          *testing.T
	conn       net.Conn
	handshaken bool
	cipher     *Cipher
}

func dialGameClient(t *testing.T, addr string) *fakeGameClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	t.Cleanup(func() { conn.Close() })

	return &fakeGameClient{t: t, conn: conn}
}

func (f *fakeGameClient) sendProtocolVersion(revision int32) {
	f.t.Helper()
	if err := wire.WriteFrame(f.conn, encodeProtocolVersion(revision)); err != nil {
		f.t.Fatalf("write ProtocolVersion: %v", err)
	}

	f.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	raw, err := wire.ReadFrame(f.conn)
	if err != nil {
		f.t.Fatalf("read VersionCheck: %v", err)
	}
	if raw[0] != serverpackets.OpcodeVersionCheck {
		f.t.Fatalf("first packet opcode = %#x, want VersionCheck (%#x)", raw[0], serverpackets.OpcodeVersionCheck)
	}
	if len(raw) != 18 {
		f.t.Fatalf("VersionCheck payload size = %d, want 18", len(raw))
	}
	if enabled := wire.NewReader(raw[10:14]).ReadInt32(); enabled != 0 {
		key := make([]byte, keySize)
		copy(key[:8], raw[2:10])
		copy(key[8:], gameCipherStaticKey[:])

		cipher, err := NewCipher(key)
		if err != nil {
			f.t.Fatalf("NewCipher: %v", err)
		}
		cipher.Encrypt(nil)
		f.cipher = cipher
	}
	f.handshaken = true
}

func (f *fakeGameClient) send(payload []byte) {
	f.t.Helper()
	if !f.handshaken {
		f.t.Fatal("send called before ProtocolVersion/VersionCheck handshake")
	}
	buf := append([]byte(nil), payload...)
	if f.cipher != nil {
		f.cipher.Encrypt(buf)
	}
	if err := wire.WriteFrame(f.conn, buf); err != nil {
		f.t.Fatalf("WriteFrame: %v", err)
	}
}

func (f *fakeGameClient) read() []byte {
	f.t.Helper()
	if !f.handshaken {
		f.t.Fatal("read called before ProtocolVersion/VersionCheck handshake")
	}
	f.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	payload, err := wire.ReadFrame(f.conn)
	if err != nil {
		f.t.Fatalf("ReadFrame: %v", err)
	}
	if f.cipher != nil {
		f.cipher.Decrypt(payload)
	}
	return payload
}

func (f *fakeGameClient) expectNoFrame() {
	f.t.Helper()
	f.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if payload, err := wire.ReadFrame(f.conn); err == nil {
		if f.cipher != nil {
			f.cipher.Decrypt(payload)
		}
		f.t.Fatalf("unexpected frame: %x", payload)
	} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		f.t.Fatalf("ReadFrame: %v", err)
	}
}

func readEnterWorldBurst(t *testing.T, c *fakeGameClient, wantDie bool) [][]byte {
	t.Helper()
	want := []byte{
		serverpackets.OpcodeExtended,
		serverpackets.OpcodeHennaInfo,
		serverpackets.OpcodeEtcStatusUpdate,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeQuestList,
		serverpackets.OpcodeSkillList,
		serverpackets.OpcodeFriendList,
		serverpackets.OpcodeUserInfo,
		serverpackets.OpcodeItemList,
		serverpackets.OpcodeShortCutInit,
	}
	if wantDie {
		want = append(want, serverpackets.OpcodeDie)
	}
	want = append(want, serverpackets.OpcodeSkillCoolTime, serverpackets.OpcodeActionFailed)

	frames := make([][]byte, 0, len(want))
	for i, opcode := range want {
		frame := c.read()
		if frame[0] != opcode {
			t.Fatalf("EnterWorld frame %d opcode = %#x, want %#x", i, frame[0], opcode)
		}
		if i == 0 {
			if second := wire.NewReader(frame[1:]).ReadUint16(); second != serverpackets.OpcodeExStorageMaxCount {
				t.Fatalf("EnterWorld first extended opcode = %#x, want ExStorageMaxCount (%#x)", second, serverpackets.OpcodeExStorageMaxCount)
			}
		}
		frames = append(frames, frame)
	}
	return frames
}

func (f *fakeGameClient) expectClosed() {
	f.t.Helper()
	f.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if n, err := f.conn.Read(buf); n != 0 || err == nil {
		f.t.Fatalf("expected connection to close, got n=%d err=%v", n, err)
	}
}

func encodeProtocolVersion(revision int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeProtocolVersion)
	w.WriteInt32(revision)
	return w.Bytes()
}

func encodeAuthLogin(name string, key link.SessionKey) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAuthLogin)
	w.WriteString(name)
	w.WriteInt32(key.PlayKey2)
	w.WriteInt32(key.PlayKey1)
	w.WriteInt32(key.LoginKey1)
	w.WriteInt32(key.LoginKey2)
	return w.Bytes()
}

func encodeRequestCharacterCreate(name string, race, sex, classID int32, hairStyle, hairColor, face byte) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCharacterCreate)
	w.WriteString(name)
	w.WriteInt32(race)
	w.WriteInt32(sex)
	w.WriteInt32(classID)
	for i := 0; i < 6; i++ {
		w.WriteInt32(0)
	}
	w.WriteInt32(int32(hairStyle))
	w.WriteInt32(int32(hairColor))
	w.WriteInt32(int32(face))
	return w.Bytes()
}

func encodeRequestCharacterDelete(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCharacterDelete)
	w.WriteInt32(slot)
	return w.Bytes()
}

func encodeCharacterRestore(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeCharacterRestore)
	w.WriteInt32(slot)
	return w.Bytes()
}

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

func encodeRequestManorList() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestManorList)
	return w.Bytes()
}

func encodeRequestCursedWeaponList() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestCursedWeaponList)
	return w.Bytes()
}

func encodeRequestCursedWeaponLocation() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestCursedWeaponLocation)
	return w.Bytes()
}

func encodeRequestAutoSoulShot(itemID, typ int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestAutoSoulShot)
	w.WriteInt32(itemID)
	w.WriteInt32(typ)
	return w.Bytes()
}

func encodeUseItem(objectID int32, ctrl bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	return w.Bytes()
}

func encodeRequestEnchantItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestEnchantItem)
	w.WriteInt32(objectID)
	return w.Bytes()
}

func encodeRequestAcquireSkillInfo(skillID, level, skillType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAcquireSkillInfo)
	w.WriteInt32(skillID)
	w.WriteInt32(level)
	w.WriteInt32(skillType)
	return w.Bytes()
}

func encodeRequestAcquireSkill(skillID, level, skillType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAcquireSkill)
	w.WriteInt32(skillID)
	w.WriteInt32(level)
	w.WriteInt32(skillType)
	return w.Bytes()
}

func encodeRequestPackageSendableItemList(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPackageItemList)
	w.WriteInt32(objectID)
	return w.Bytes()
}

func encodeRequestMagicSkillUse(skillID int32, ctrl, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(skillID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeRequestSkillCoolTime() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeRequestSkillCoolTime).Bytes()
}

func encodeRequestActionUse(actionID int32, ctrl, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestActionUse)
	w.WriteInt32(actionID)
	if ctrl {
		w.WriteInt32(1)
	} else {
		w.WriteInt32(0)
	}
	if shift {
		w.WriteUint8(1)
	} else {
		w.WriteUint8(0)
	}
	return w.Bytes()
}

func encodeMoveBackwardToLocation(target, origin location.Location, moveMovement int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	w.WriteInt32(int32(target.X))
	w.WriteInt32(int32(target.Y))
	w.WriteInt32(int32(target.Z))
	w.WriteInt32(int32(origin.X))
	w.WriteInt32(int32(origin.Y))
	w.WriteInt32(int32(origin.Z))
	w.WriteInt32(moveMovement)
	return w.Bytes()
}

func encodeValidatePosition(at location.Location, heading int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeValidatePosition)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteInt32(heading)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeCannotMoveAnymore(at location.Location, heading int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeCannotMoveAnymore)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteInt32(heading)
	return w.Bytes()
}

func encodeStartRotating(degree, side int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeStartRotating)
	w.WriteInt32(degree)
	w.WriteInt32(side)
	return w.Bytes()
}

func encodeFinishRotating(degree, side int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeFinishRotating)
	w.WriteInt32(degree)
	w.WriteInt32(side)
	return w.Bytes()
}

func encodeAction(objectID int32, origin location.Location, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(origin.X))
	w.WriteInt32(int32(origin.Y))
	w.WriteInt32(int32(origin.Z))
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeAttackRequest(objectID int32, origin location.Location, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAttackRequest)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(origin.X))
	w.WriteInt32(int32(origin.Y))
	w.WriteInt32(int32(origin.Z))
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeRequestTargetCancel(unselect uint16) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestTargetCancel)
	w.WriteUint16(unselect)
	return w.Bytes()
}

func encodeRequestChangeMoveType(run bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestChangeMoveType)
	w.WriteInt32(wire.BoolInt32(run))
	return w.Bytes()
}

func encodeRequestChangeWaitType(stand bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestChangeWaitType)
	w.WriteInt32(wire.BoolInt32(stand))
	return w.Bytes()
}

func encodeRequestSocialAction(actionID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestSocialAction)
	w.WriteInt32(actionID)
	return w.Bytes()
}

func encodeRequestDropItem(objectID, count int32, at location.Location) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestDropItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	return w.Bytes()
}

func encodeRequestDestroyItem(objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestDestroyItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

func encodeRequestCrystallizeItem(objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCrystallizeItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

func encodeRequestShortCutReg(typ, slot, id, characterType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestShortCutReg)
	w.WriteInt32(typ)
	w.WriteInt32(slot)
	w.WriteInt32(id)
	w.WriteInt32(characterType)
	return w.Bytes()
}

func encodeRequestShortCutDel(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestShortCutDel)
	w.WriteInt32(slot)
	return w.Bytes()
}

func encodeSendTimeCheck(requestID, responseID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSendTimeCheck)
	w.WriteInt32(requestID)
	w.WriteInt32(responseID)
	return w.Bytes()
}

func encodeSingleOpcode(opcode byte) []byte {
	return wire.NewPacketWriter(opcode).Bytes()
}

// --- test server setup ---

func newTestGameClientLink(t *testing.T, loginLink func() *LoginLink, validator *SessionValidator) (addr string, chars *fakeCharStore, items *fakeItemStore, state *world.State) {
	t.Helper()
	return newTestGameClientLinkWithLog(t, loginLink, validator, zerolog.Nop())
}

func newTestGameClientLinkWithLog(t *testing.T, loginLink func() *LoginLink, validator *SessionValidator, log zerolog.Logger) (addr string, chars *fakeCharStore, items *fakeItemStore, state *world.State) {
	t.Helper()
	return newTestGameClientLinkWithSkillsAndLog(t, loginLink, validator, nil, log)
}

func newTestGameClientLinkWithSkillsAndLog(t *testing.T, loginLink func() *LoginLink, validator *SessionValidator, skills *skillstate.Persistence, log zerolog.Logger) (addr string, chars *fakeCharStore, items *fakeItemStore, state *world.State) {
	t.Helper()
	addr, chars, items, _, state = newTestGameClientLinkWithSkillsShortcutsAndLog(t, loginLink, validator, skills, log)
	return addr, chars, items, state
}

func newTestGameClientLinkWithSkillsShortcutsAndLog(t *testing.T, loginLink func() *LoginLink, validator *SessionValidator, skills *skillstate.Persistence, log zerolog.Logger) (addr string, chars *fakeCharStore, items *fakeItemStore, shortcuts *fakeShortcutStore, state *world.State) {
	t.Helper()
	return newTestGameClientLinkWithSkillsShortcutsCrestsAndLog(t, loginLink, validator, skills, nil, modelskill.BookPolicy{}, nil, log)
}

func newTestGameClientLinkWithSkillsShortcutsCrestsAndLog(t *testing.T, loginLink func() *LoginLink, validator *SessionValidator, skills *skillstate.Persistence, crests *datacache.Crests, spellbooks modelskill.BookPolicy, trees *modelskill.Trees, log zerolog.Logger, cursedWeapons ...*entity.CursedWeaponTable) (addr string, chars *fakeCharStore, items *fakeItemStore, shortcuts *fakeShortcutStore, state *world.State) {
	t.Helper()
	chars = newFakeCharStore()
	items = newFakeItemStore()
	shortcuts = newFakeShortcutStore()
	state = world.New()
	templates := testTemplates(t)
	itemTemplates := testItemTemplates()
	ids := &sequentialIDs{next: 100}
	groundItems := task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Hour, PlayerDroppedMultiplier: 1}, time.Now)
	roster := gamemanager.NewRoster(chars, items, shortcuts, templates, itemTemplates, npc.NewTable(nil), ids, gamemanager.DefaultDeleteAfter, time.Now)
	html := testHTMLCache(t, map[string]string{"help/tutorial.htm": "<html><body>tutorial</body></html>"})
	if crests == nil {
		crests = datacache.NewCrests()
	}
	var cursed *entity.CursedWeaponTable
	if len(cursedWeapons) > 0 {
		cursed = cursedWeapons[0]
	}
	gcl := NewGameClientLink(validator, loginLink, roster, items, shortcuts, templates, itemTemplates, html, crests, skills, spellbooks, trees, cursed, state, testGeo{}, ids, groundItems, nil, task.NewPositionUpdates(state), nil, 0.7, nil, true, log)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go Serve(ctx, ln, gcl.Handle, zerolog.Nop())

	return ln.Addr().String(), chars, items, shortcuts, state
}

// newLinkedGameClient wires a GameClientLink to a real login-server-side
// GS-LS link (the same infrastructure loginlink_test.go uses), dials a fake
// game client through VersionCheck and a successful AuthLogin, and returns
// it positioned right after the initial (empty) CharSelectInfo.
func newLinkedGameClient(t *testing.T) (c *fakeGameClient, chars *fakeCharStore, items *fakeItemStore, state *world.State) {
	t.Helper()
	return newLinkedGameClientWithSkills(t, nil)
}

func newLinkedGameClientWithSkills(t *testing.T, skills *skillstate.Persistence) (c *fakeGameClient, chars *fakeCharStore, items *fakeItemStore, state *world.State) {
	t.Helper()
	return newLinkedGameClientWithSkillsSeed(t, skills, nil, 0)
}

func newLinkedGameClientWithSkillsSeed(t *testing.T, skills *skillstate.Persistence, seed func(*fakeCharStore, *fakeItemStore), wantChars int) (c *fakeGameClient, chars *fakeCharStore, items *fakeItemStore, state *world.State) {
	t.Helper()
	c, chars, items, _, state = newLinkedGameClientWithSkillsShortcutsSeed(t, skills, nil, seed, wantChars)
	return c, chars, items, state
}

func newLinkedGameClientWithShortcuts(t *testing.T) (c *fakeGameClient, chars *fakeCharStore, items *fakeItemStore, shortcuts *fakeShortcutStore, state *world.State) {
	t.Helper()
	return newLinkedGameClientWithSkillsShortcutsSeed(t, nil, nil, nil, 0)
}

func newLinkedGameClientWithSkillsShortcutsSeed(t *testing.T, skills *skillstate.Persistence, shortcutSeed func(*fakeShortcutStore), seed func(*fakeCharStore, *fakeItemStore), wantChars int) (c *fakeGameClient, chars *fakeCharStore, items *fakeItemStore, shortcuts *fakeShortcutStore, state *world.State) {
	t.Helper()
	return newLinkedGameClientWithSkillsShortcutsCrestsSeed(t, skills, shortcutSeed, nil, modelskill.BookPolicy{}, nil, seed, wantChars)
}

func newLinkedGameClientWithCrests(t *testing.T, crests *datacache.Crests) (c *fakeGameClient, chars *fakeCharStore, items *fakeItemStore, state *world.State) {
	t.Helper()
	c, chars, items, _, state = newLinkedGameClientWithSkillsShortcutsCrestsSeed(t, nil, nil, crests, modelskill.BookPolicy{}, nil, nil, 0)
	return c, chars, items, state
}

func newLinkedGameClientWithSkillsShortcutsCrestsSeed(t *testing.T, skills *skillstate.Persistence, shortcutSeed func(*fakeShortcutStore), crests *datacache.Crests, spellbooks modelskill.BookPolicy, trees *modelskill.Trees, seed func(*fakeCharStore, *fakeItemStore), wantChars int, cursedWeapons ...*entity.CursedWeaponTable) (c *fakeGameClient, chars *fakeCharStore, items *fakeItemStore, shortcuts *fakeShortcutStore, state *world.State) {
	t.Helper()

	loginAddr, servers, sessions := newTestLoginServer(t, false)
	servers.Register(1, testHexID)

	validator := NewSessionValidator()
	auth := LoginServerAuth{ServerID: 1, HexID: testHexID, HostName: "*", Port: 7777, MaxPlayers: 300}
	loginLink, err := DialLoginLink(context.Background(), loginAddr, auth, LoginLinkHandlers{PlayerAuthResponse: validator.Resolve}, zerolog.Nop())
	if err != nil {
		t.Fatalf("DialLoginLink: %v", err)
	}
	t.Cleanup(func() { loginLink.Close() })

	addr, chars, items, shortcuts, state := newTestGameClientLinkWithSkillsShortcutsCrestsAndLog(t, func() *LoginLink { return loginLink }, validator, skills, crests, spellbooks, trees, zerolog.Nop(), cursedWeapons...)
	if seed != nil {
		seed(chars, items)
	}
	if shortcutSeed != nil {
		shortcutSeed(shortcuts)
	}

	c = dialGameClient(t, addr)
	c.sendProtocolVersion(746)

	key := link.SessionKey{LoginKey1: 11, LoginKey2: 22, PlayKey1: 33, PlayKey2: 44}
	sessions.Put("player1", key)
	c.send(encodeAuthLogin("player1", key))

	reply := c.read()
	if reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("opcode = %#x, want CharSelectInfo (%#x)", reply[0], serverpackets.OpcodeCharSelectInfo)
	}
	if count := wire.NewReader(reply[1:]).ReadInt32(); count != int32(wantChars) {
		t.Fatalf("initial char count = %d, want %d", count, wantChars)
	}
	return c, chars, items, shortcuts, state
}

func seedSelectableCharacter(t *testing.T, chars *fakeCharStore, account, name string, level, sp int) int32 {
	t.Helper()
	tmpl, ok := testTemplates(t).Get(0)
	if !ok {
		t.Fatal("missing test class template")
	}
	ch, err := player.NewCharacter(100, tmpl, account, name, 1, 0, 0, player.SexMale)
	if err != nil {
		t.Fatalf("seed character: %v", err)
	}
	ch.CharLevel = level
	ch.SP = sp
	if err := chars.Create(context.Background(), ch); err != nil {
		t.Fatalf("seed character store: %v", err)
	}
	return ch.ID
}

func TestGameClientLinkEnchantItemInGame(t *testing.T) {
	c, chars, items, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	if err := items.Create(context.Background(), objID, item.Instance{
		ObjectID:   504,
		TemplateID: 30,
		OwnerID:    objID,
		Count:      1,
		Location:   item.LocationInventory,
	}); err != nil {
		t.Fatalf("seed weapon: %v", err)
	}
	if err := items.Create(context.Background(), objID, item.Instance{
		ObjectID:   505,
		TemplateID: 955,
		OwnerID:    objID,
		Count:      1,
		Location:   item.LocationInventory,
	}); err != nil {
		t.Fatalf("seed scroll: %v", err)
	}

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeUseItem(505, false))
	assertStaticSystemMessageFrame(t, c.read(), serverpackets.SystemMessageSelectItemToEnchant)
	assertChooseInventoryItemFrame(t, c.read(), 955)

	c.send(encodeRequestEnchantItem(504))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("enchant success message opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("enchant inventory opcode = %#x, want InventoryUpdate (%#x)", reply[0], serverpackets.OpcodeInventoryUpdate)
	}
	assertEnchantResultFrame(t, c.read(), serverpackets.EnchantResultSuccess)
	if reply := c.read(); reply[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("enchant userinfo opcode = %#x, want UserInfo (%#x)", reply[0], serverpackets.OpcodeUserInfo)
	}
}

type frameCapture struct {
	frames [][]byte
}

func (c *frameCapture) send(frame wire.Frame) bool {
	defer frame.Release()
	raw := frame.Bytes()
	payload := make([]byte, len(raw)-2)
	copy(payload, raw[2:])
	c.frames = append(c.frames, payload)
	return true
}

func frameOpcodes(frames [][]byte) []byte {
	out := make([]byte, 0, len(frames))
	for _, frame := range frames {
		if len(frame) > 0 {
			out = append(out, frame[0])
		}
	}
	return out
}

func skipInventoryRemainder(r *wire.Reader) {
	r.ReadUint16()
	r.ReadUint16()
	r.ReadUint16()
	r.ReadInt32()
	r.ReadUint16()
	r.ReadUint16()
	r.ReadInt32()
	r.ReadInt32()
}

func skipPackageSendableRemainder(r *wire.Reader) {
	r.ReadUint16()
	r.ReadUint16()
	r.ReadInt32()
	r.ReadUint16()
	r.ReadUint16()
	r.ReadUint16()
	r.ReadInt32()
}

// testGeo is an always-passable move.Geo double for tests that don't
// exercise geodata behavior.
type testGeo struct{}

func (testGeo) CanMove(int, int, int, int, int, int) bool { return true }
func (testGeo) Height(_, _, z int) int16                  { return int16(z) }

func newTestLivePlayer(t *testing.T, id int32, capture *frameCapture) *livePlayer {
	t.Helper()
	tmpl, ok := testTemplates(t).Get(0)
	if !ok {
		t.Fatal("missing test class template")
	}
	ch := &player.Character{
		ID: id, Name: "Player", ClassID: 0, BaseClassID: 0,
		Race: player.RaceHuman, Sex: player.SexMale,
		CharLevel: 1,
		Location:  location.Location{X: int(id) * 100, Y: 0, Z: 0},
	}
	ch.SetResourceValues(player.Resources{MaxHP: 80, CurrentHP: 80, MaxMP: 30, CurrentMP: 30})
	ch.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventory(ch.ID, testItemTemplates(), nil))
	ch.SetFrameSender(capture.send)

	x, y, z := ch.Position()
	cm, err := move.NewCreatureMove(location.Location{X: x, Y: y, Z: z}, tmpl.RunSpeed, testGeo{})
	if err != nil {
		t.Fatal(err)
	}
	moveCtl, err := move.NewController(cm, ch)
	if err != nil {
		t.Fatal(err)
	}
	attackCtl := attack.NewPlayer(ch)
	combat := ai.NewPlayerAttack(ch, moveCtl, attackCtl)
	moveCtl.SetArrived(combat.Think)
	attackCtl.SetFinished(combat.Think)

	return &livePlayer{Character: ch, template: tmpl, attack: attackCtl, move: moveCtl, combat: combat}
}

func newTestHostileNPC(t *testing.T, id int32) *npc.Hostile {
	t.Helper()
	tmpl := &npc.Template{
		ID:              100,
		TemplateID:      100,
		Type:            "Monster",
		Level:           1,
		HPMax:           100,
		AtkSpd:          300,
		RunSpeed:        120,
		WalkSpeed:       60,
		CollisionRadius: 8,
		CollisionHeight: 20,
	}
	inst, err := npc.NewInstance(id, tmpl)
	if err != nil {
		t.Fatal(err)
	}
	live, err := creature.NewLive(location.Location{}, 100, testHostileGeo{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	hostile, err := npc.NewHostile(inst, live, testHostileMove{}, testHostileAttack{})
	if err != nil {
		t.Fatal(err)
	}
	return hostile
}

type testHostileGeo struct{}

func (testHostileGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (testHostileGeo) Height(_, _, _ int) int16          { return 0 }

type testHostileMove struct{}

func (testHostileMove) MaybeStartOffensiveFollow(attackable.Combatant, int) bool { return false }
func (testHostileMove) MoveHome(location.Location)                               {}
func (testHostileMove) Stop()                                                    {}

type testHostileAttack struct{}

func (testHostileAttack) BowCoolingDown() bool                { return false }
func (testHostileAttack) AttackingNow() bool                  { return false }
func (testHostileAttack) CanAttack(attackable.Combatant) bool { return false }
func (testHostileAttack) DoAttack(attackable.Combatant)       {}

type attackStanceRecorder struct {
	actors []task.AttackStanceActor
}

func (r *attackStanceRecorder) Add(actor task.AttackStanceActor) {
	r.actors = append(r.actors, actor)
}

type recordedGroundDrop struct {
	ground *grounditem.Item
	opts   task.DropOptions
}

type recordingGroundDropper struct {
	drops []recordedGroundDrop
}

func (r *recordingGroundDropper) Drop(ground *grounditem.Item, opts task.DropOptions) {
	r.drops = append(r.drops, recordedGroundDrop{ground: ground, opts: opts})
}

func (r *recordingGroundDropper) Remove(*grounditem.Item) {}

type visibleGroundItem struct {
	world.Presence
	id        int32
	itemID    int32
	count     int
	stackable bool
}

func (g *visibleGroundItem) ObjectID() int32 { return g.id }
func (g *visibleGroundItem) ItemID() int32   { return g.itemID }
func (g *visibleGroundItem) Count() int      { return g.count }
func (g *visibleGroundItem) Stackable() bool { return g.stackable }

type visibleDoor struct {
	world.Presence
	id     int32
	doorID int
}

func (d *visibleDoor) ObjectID() int32 { return d.id }
func (d *visibleDoor) DoorID() int     { return d.doorID }
func (d *visibleDoor) Opened() bool    { return false }
func (d *visibleDoor) MaxHP() int      { return 100 }
func (d *visibleDoor) HP() int         { return 100 }
func (d *visibleDoor) Damage() int     { return 0 }

type visibleStaticObject struct {
	world.Presence
	id       int32
	staticID int
}

func (o *visibleStaticObject) ObjectID() int32     { return o.id }
func (o *visibleStaticObject) StaticObjectID() int { return o.staticID }

type invisibleTracked struct {
	world.Presence
	id int32
}

func (o *invisibleTracked) ObjectID() int32 { return o.id }

type safeLogBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *safeLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}

func (b *safeLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}

func waitForLog(t *testing.T, logs *safeLogBuffer, needle string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got := logs.String()
		if strings.Contains(got, needle) {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for log containing %q; logs=%s", needle, logs.String())
	return ""
}

func assertTargetHPStatus(t *testing.T, frame []byte, objectID int32, maxHP, curHP int) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeStatusUpdate {
		t.Fatalf("status opcode = %#x, want StatusUpdate (%#x)", frame[0], serverpackets.OpcodeStatusUpdate)
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != objectID {
		t.Fatalf("StatusUpdate object id = %d, want %d", got, objectID)
	}
	if count := r.ReadInt32(); count != 2 {
		t.Fatalf("StatusUpdate attribute count = %d, want 2", count)
	}
	if typ, val := r.ReadInt32(), r.ReadInt32(); typ != int32(serverpackets.StatusMaxHP) || val != int32(maxHP) {
		t.Fatalf("StatusUpdate first attr = (%d,%d), want MAX_HP=%d", typ, val, maxHP)
	}
	if typ, val := r.ReadInt32(), r.ReadInt32(); typ != int32(serverpackets.StatusCurrentHP) || val != int32(curHP) {
		t.Fatalf("StatusUpdate second attr = (%d,%d), want CUR_HP=%d", typ, val, curHP)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("StatusUpdate read: %v", err)
	}
}

func assertSystemMessageSkillFrame(t *testing.T, frame []byte, messageID int, skillID, level int32) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("SystemMessage opcode = %#x, want %#x", frame[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != int32(messageID) {
		t.Fatalf("SystemMessage id = %d, want %d", id, messageID)
	}
	if params := r.ReadInt32(); params != 1 {
		t.Fatalf("SystemMessage params = %d, want 1", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamSkillName {
		t.Fatalf("SystemMessage param type = %d, want skill name", typ)
	}
	if id := r.ReadInt32(); id != skillID {
		t.Fatalf("SystemMessage skill id = %d, want %d", id, skillID)
	}
	if got := r.ReadInt32(); got != level {
		t.Fatalf("SystemMessage skill level = %d, want %d", got, level)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}

func assertSPStatus(t *testing.T, frame []byte, objectID int32, sp int) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeStatusUpdate {
		t.Fatalf("StatusUpdate opcode = %#x, want %#x", frame[0], serverpackets.OpcodeStatusUpdate)
	}
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != objectID {
		t.Fatalf("StatusUpdate object id = %d, want %d", id, objectID)
	}
	if count := r.ReadInt32(); count != 1 {
		t.Fatalf("StatusUpdate count = %d, want 1", count)
	}
	if typ := r.ReadInt32(); typ != int32(serverpackets.StatusSP) {
		t.Fatalf("StatusUpdate type = %d, want SP", typ)
	}
	if got := r.ReadInt32(); got != int32(sp) {
		t.Fatalf("StatusUpdate SP = %d, want %d", got, sp)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read StatusUpdate: %v", err)
	}
}

func assertStatusAttrs(t *testing.T, frame []byte, objectID int32, attrs []serverpackets.StatusAttribute) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeStatusUpdate {
		t.Fatalf("StatusUpdate opcode = %#x, want %#x", frame[0], serverpackets.OpcodeStatusUpdate)
	}
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != objectID {
		t.Fatalf("StatusUpdate object id = %d, want %d", id, objectID)
	}
	if count := r.ReadInt32(); count != int32(len(attrs)) {
		t.Fatalf("StatusUpdate count = %d, want %d", count, len(attrs))
	}
	for _, attr := range attrs {
		if typ := r.ReadInt32(); typ != int32(attr.Type) {
			t.Fatalf("StatusUpdate type = %d, want %d", typ, attr.Type)
		}
		if got := r.ReadInt32(); got != int32(attr.Value) {
			t.Fatalf("StatusUpdate value = %d, want %d", got, attr.Value)
		}
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read StatusUpdate: %v", err)
	}
}
