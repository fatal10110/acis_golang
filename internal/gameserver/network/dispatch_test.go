package network

import (
	"context"
	"net"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/link"
)

// --- fake character/item stores: Roster's own persistence seam, no DB needed ---

type fakeCharStore struct {
	mu    sync.Mutex
	byID  map[int32]*player.Character
	names map[string]bool
}

func newFakeCharStore() *fakeCharStore {
	return &fakeCharStore{byID: map[int32]*player.Character{}, names: map[string]bool{}}
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
	}
	table, err := player.NewTemplateTable(map[int]*player.Template{0: tmpl})
	if err != nil {
		t.Fatalf("build template table: %v", err)
	}
	return table
}

func testItemTemplates() *item.Table {
	return item.NewTable(nil)
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

func encodeRequestSkillCoolTime() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeRequestSkillCoolTime).Bytes()
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

func encodeSingleOpcode(opcode byte) []byte {
	return wire.NewPacketWriter(opcode).Bytes()
}

// --- test server setup ---

func newTestGameClientLink(t *testing.T, loginLink func() *LoginLink, validator *SessionValidator) (addr string, chars *fakeCharStore, items *fakeItemStore, state *world.State) {
	t.Helper()

	chars = newFakeCharStore()
	items = newFakeItemStore()
	state = world.New()
	templates := testTemplates(t)
	itemTemplates := testItemTemplates()
	roster := gamemanager.NewRoster(chars, items, templates, itemTemplates, &sequentialIDs{next: 100}, gamemanager.DefaultDeleteAfter, time.Now)
	gcl := NewGameClientLink(validator, loginLink, roster, items, templates, itemTemplates, state, zerolog.Nop())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go Serve(ctx, ln, gcl.Handle, zerolog.Nop())

	return ln.Addr().String(), chars, items, state
}

// newLinkedGameClient wires a GameClientLink to a real login-server-side
// GS-LS link (the same infrastructure loginlink_test.go uses), dials a fake
// game client through VersionCheck and a successful AuthLogin, and returns
// it positioned right after the initial (empty) CharSelectInfo.
func newLinkedGameClient(t *testing.T) (c *fakeGameClient, chars *fakeCharStore, items *fakeItemStore, state *world.State) {
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

	addr, chars, items, state := newTestGameClientLink(t, func() *LoginLink { return loginLink }, validator)

	c = dialGameClient(t, addr)
	c.sendProtocolVersion(746)

	key := link.SessionKey{LoginKey1: 11, LoginKey2: 22, PlayKey1: 33, PlayKey2: 44}
	sessions.Put("player1", key)
	c.send(encodeAuthLogin("player1", key))

	reply := c.read()
	if reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("opcode = %#x, want CharSelectInfo (%#x)", reply[0], serverpackets.OpcodeCharSelectInfo)
	}
	if count := wire.NewReader(reply[1:]).ReadInt32(); count != 0 {
		t.Fatalf("initial char count = %d, want 0", count)
	}
	return c, chars, items, state
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

func newTestLivePlayer(t *testing.T, id int32, capture *frameCapture) *livePlayer {
	t.Helper()
	tmpl, ok := testTemplates(t).Get(0)
	if !ok {
		t.Fatal("missing test class template")
	}
	ch := &player.Character{
		ID: id, Name: "Player", ClassID: 0, BaseClassID: 0,
		Race: player.RaceHuman, Sex: player.SexMale,
		Level: 1, MaxHP: 80, CurHP: 80, MaxMP: 30, CurMP: 30,
		Location: location.Location{X: int(id) * 100, Y: 0, Z: 0},
	}
	ch.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventory(ch.ID, testItemTemplates(), nil))
	ch.SetFrameSender(capture.send)
	return &livePlayer{Character: ch, template: tmpl}
}

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

func TestGameClientLinkWaitsForProtocolVersion(t *testing.T) {
	addr, _, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	c := dialGameClient(t, addr)

	c.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, err := wire.ReadFrame(c.conn); err == nil {
		t.Fatal("server sent data before ProtocolVersion")
	} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("read before ProtocolVersion error = %v, want timeout", err)
	}
}

func TestGameClientLinkSendsVersionCheckAfterProtocolVersion(t *testing.T) {
	addr, _, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	c := dialGameClient(t, addr)
	c.sendProtocolVersion(746)
}

func TestGameClientLinkBadProtocolVersionClosesSilently(t *testing.T) {
	addr, _, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	c := dialGameClient(t, addr)
	if err := wire.WriteFrame(c.conn, encodeProtocolVersion(1)); err != nil {
		t.Fatalf("write ProtocolVersion: %v", err)
	}
	c.expectClosed()
}

func TestGameClientLinkOpcodeBeforeAuthCloses(t *testing.T) {
	addr, _, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	c := dialGameClient(t, addr)
	c.sendProtocolVersion(746)

	c.send(encodeEnterWorld())
	c.expectClosed()
}

func TestGameClientLinkAuthLoginServerDownFails(t *testing.T) {
	addr, _, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	c := dialGameClient(t, addr)
	c.sendProtocolVersion(746)

	c.send(encodeAuthLogin("player1", link.SessionKey{}))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeAuthLoginFail {
		t.Fatalf("opcode = %#x, want AuthLoginFail (%#x)", reply[0], serverpackets.OpcodeAuthLoginFail)
	}
	c.expectClosed()
}

func TestGameClientLinkFullFlow(t *testing.T) {
	c, chars, _, state := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeCharCreateOk {
		t.Fatalf("opcode = %#x, want CharCreateOk (%#x)", reply[0], serverpackets.OpcodeCharCreateOk)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("opcode = %#x, want CharSelectInfo (%#x)", reply[0], serverpackets.OpcodeCharSelectInfo)
	}
	if count := wire.NewReader(reply[1:]).ReadInt32(); count != 1 {
		t.Fatalf("char count = %d, want 1", count)
	}
	objID := chars.soleObjectID(t)

	c.send(encodeRequestGameStart(0))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo (%#x)", reply[0], serverpackets.OpcodeSSQInfo)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected (%#x)", reply[0], serverpackets.OpcodeCharSelected)
	}

	c.send(encodeRequestManorList())
	reply = c.read()
	if reply[0] != serverpackets.OpcodeExtended {
		t.Fatalf("opcode = %#x, want extended packet (%#x)", reply[0], serverpackets.OpcodeExtended)
	}
	if second := wire.NewReader(reply[1:]).ReadUint16(); second != serverpackets.OpcodeExSendManorList {
		t.Fatalf("extended opcode = %#x, want ExSendManorList (%#x)", second, serverpackets.OpcodeExSendManorList)
	}

	c.send(encodeEnterWorld())
	reply = c.read()
	if reply[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("opcode = %#x, want UserInfo (%#x)", reply[0], serverpackets.OpcodeUserInfo)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeItemList {
		t.Fatalf("opcode = %#x, want ItemList (%#x)", reply[0], serverpackets.OpcodeItemList)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeSkillList {
		t.Fatalf("opcode = %#x, want SkillList (%#x)", reply[0], serverpackets.OpcodeSkillList)
	}
	if _, ok := state.Player(objID); !ok {
		t.Fatalf("world.Player(%d) missing after EnterWorld", objID)
	}
	if _, ok := state.Object(objID); !ok {
		t.Fatalf("world.Object(%d) missing after EnterWorld", objID)
	}
}

func TestGameClientLinkIgnoresRequestSkillCoolTimeInGame(t *testing.T) {
	c, _, _, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	c.read() // UserInfo
	c.read() // ItemList
	c.read() // SkillList

	c.send(encodeRequestSkillCoolTime())
	c.send(encodeRequestManorList())
	reply := c.read()
	if reply[0] != serverpackets.OpcodeExtended {
		t.Fatalf("opcode = %#x, want extended packet (%#x)", reply[0], serverpackets.OpcodeExtended)
	}
	if second := wire.NewReader(reply[1:]).ReadUint16(); second != serverpackets.OpcodeExSendManorList {
		t.Fatalf("extended opcode = %#x, want ExSendManorList (%#x)", second, serverpackets.OpcodeExSendManorList)
	}
}

func TestGameClientLinkWireSafeMovementAndRefreshPacketsInGame(t *testing.T) {
	c, chars, _, state := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	c.read() // UserInfo
	c.read() // ItemList
	c.read() // SkillList

	target := location.Location{X: 46160, Y: 41237, Z: -3534}
	origin := location.Location{X: 10, Y: 20, Z: 30}
	c.send(encodeMoveBackwardToLocation(target, origin, 1))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("move reply opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}
	r := wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("MoveToLocation object id = %d, want %d", got, objID)
	}
	gotTarget := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	if gotTarget != target {
		t.Fatalf("MoveToLocation target = %+v, want %+v", gotTarget, target)
	}
	obj, ok := state.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	positioned, ok := obj.(interface{ Position() (int, int, int) })
	if !ok {
		t.Fatalf("world.Player(%d) has no Position method", objID)
	}
	x, y, z := positioned.Position()
	if x != target.X || y != target.Y || z != target.Z {
		t.Fatalf("player position = (%d,%d,%d), want (%d,%d,%d)", x, y, z, target.X, target.Y, target.Z)
	}

	c.send(encodeValidatePosition(target, 32768))
	c.send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeItemList {
		t.Fatalf("item refresh opcode = %#x, want ItemList (%#x)", reply[0], serverpackets.OpcodeItemList)
	}

	c.send(encodeSingleOpcode(clientpackets.OpcodeRequestSkillList))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeSkillList {
		t.Fatalf("skill refresh opcode = %#x, want SkillList (%#x)", reply[0], serverpackets.OpcodeSkillList)
	}

	for _, opcode := range []byte{
		clientpackets.OpcodeUseItem,
		clientpackets.OpcodeTradeRequest,
		clientpackets.OpcodeSendWarehouseDeposit,
		clientpackets.OpcodeRequestQuestListInGame,
		clientpackets.OpcodeRequestPackageItemList,
		clientpackets.OpcodeDlgAnswer,
		clientpackets.OpcodeGameGuardReply,
		clientpackets.OpcodeRequestShowMiniMap,
	} {
		c.send(encodeSingleOpcode(opcode))
	}
	c.send(encodeRequestManorList())
	reply = c.read()
	if reply[0] != serverpackets.OpcodeExtended {
		t.Fatalf("post-stub opcode = %#x, want extended packet (%#x)", reply[0], serverpackets.OpcodeExtended)
	}
}

func TestGameClientLinkCreateInvalidNameKeepsConnectionOpen(t *testing.T) {
	c, _, _, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("bad name!", 0, 0, 0, 1, 0, 0))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeCharCreateFail {
		t.Fatalf("opcode = %#x, want CharCreateFail (%#x)", reply[0], serverpackets.OpcodeCharCreateFail)
	}

	// The connection must still be usable: a valid create now succeeds.
	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeCharCreateOk {
		t.Fatalf("opcode = %#x, want CharCreateOk (%#x)", reply[0], serverpackets.OpcodeCharCreateOk)
	}
}

func TestGameClientLinkDeleteAndRestore(t *testing.T) {
	c, chars, _, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo

	objID := chars.soleObjectID(t)

	c.send(encodeRequestCharacterDelete(0))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeCharDeleteOk {
		t.Fatalf("opcode = %#x, want CharDeleteOk (%#x)", reply[0], serverpackets.OpcodeCharDeleteOk)
	}
	c.read() // CharSelectInfo refresh

	if chars.deleteAt(objID) == 0 {
		t.Fatal("expected character to be scheduled for deletion")
	}

	c.send(encodeCharacterRestore(0))
	c.read() // CharSelectInfo refresh

	if chars.deleteAt(objID) != 0 {
		t.Fatal("expected character's scheduled deletion to be cleared")
	}
}
