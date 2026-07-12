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
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
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
	s.byID[c.ObjectID] = c
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
	sort.Slice(out, func(i, j int) bool { return out[i].ObjectID < out[j].ObjectID })
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
// VersionCheck carries its key in cleartext, so unlike the login server's
// Init packet, no static-key puzzle needs solving here: the fake client
// just reads the key and arms its own Cipher instance the same way the
// server's armed at the moment it sent VersionCheck (one throwaway Encrypt
// call, whose only effect is flipping the cipher's internal "enabled" gate)
// so both sides' XOR streams start rolling in lockstep from packet zero.

type fakeGameClient struct {
	t      *testing.T
	conn   net.Conn
	cipher *Cipher
}

func dialGameClient(t *testing.T, addr string) *fakeGameClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	t.Cleanup(func() { conn.Close() })

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	raw, err := wire.ReadFrame(conn)
	if err != nil {
		t.Fatalf("read VersionCheck: %v", err)
	}
	if raw[0] != serverpackets.OpcodeVersionCheck {
		t.Fatalf("first packet opcode = %#x, want VersionCheck (%#x)", raw[0], serverpackets.OpcodeVersionCheck)
	}
	key := append([]byte(nil), raw[2:2+keySize]...)

	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	cipher.Encrypt(nil)

	return &fakeGameClient{t: t, conn: conn, cipher: cipher}
}

func (f *fakeGameClient) send(payload []byte) {
	f.t.Helper()
	buf := append([]byte(nil), payload...)
	f.cipher.Encrypt(buf)
	if err := wire.WriteFrame(f.conn, buf); err != nil {
		f.t.Fatalf("WriteFrame: %v", err)
	}
}

func (f *fakeGameClient) read() []byte {
	f.t.Helper()
	f.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	payload, err := wire.ReadFrame(f.conn)
	if err != nil {
		f.t.Fatalf("ReadFrame: %v", err)
	}
	f.cipher.Decrypt(payload)
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

// --- test server setup ---

func newTestGameClientLink(t *testing.T, loginLink func() *LoginLink, validator *SessionValidator) (addr string, chars *fakeCharStore, items *fakeItemStore) {
	t.Helper()

	chars = newFakeCharStore()
	items = newFakeItemStore()
	templates := testTemplates(t)
	itemTemplates := testItemTemplates()
	roster := gamemanager.NewRoster(chars, items, templates, itemTemplates, &sequentialIDs{next: 100}, gamemanager.DefaultDeleteAfter, time.Now)
	gcl := NewGameClientLink(validator, loginLink, roster, items, templates, itemTemplates, zerolog.Nop())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go Serve(ctx, ln, gcl.Handle, zerolog.Nop())

	return ln.Addr().String(), chars, items
}

// newLinkedGameClient wires a GameClientLink to a real login-server-side
// GS-LS link (the same infrastructure loginlink_test.go uses), dials a fake
// game client through VersionCheck and a successful AuthLogin, and returns
// it positioned right after the initial (empty) CharSelectInfo.
func newLinkedGameClient(t *testing.T) (c *fakeGameClient, chars *fakeCharStore, items *fakeItemStore) {
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

	addr, chars, items := newTestGameClientLink(t, func() *LoginLink { return loginLink }, validator)

	c = dialGameClient(t, addr)
	c.send(encodeProtocolVersion(0xc621))

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
	return c, chars, items
}

func TestGameClientLinkSendsVersionCheckOnConnect(t *testing.T) {
	addr, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	dialGameClient(t, addr) // dial fails the test itself if VersionCheck never arrives
}

func TestGameClientLinkOpcodeBeforeAuthCloses(t *testing.T) {
	addr, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	c := dialGameClient(t, addr)

	c.send(encodeEnterWorld())
	c.expectClosed()
}

func TestGameClientLinkAuthLoginServerDownFails(t *testing.T) {
	addr, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	c := dialGameClient(t, addr)

	c.send(encodeAuthLogin("player1", link.SessionKey{}))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeAuthLoginFail {
		t.Fatalf("opcode = %#x, want AuthLoginFail (%#x)", reply[0], serverpackets.OpcodeAuthLoginFail)
	}
	c.expectClosed()
}

func TestGameClientLinkFullFlow(t *testing.T) {
	c, _, _ := newLinkedGameClient(t)

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

	c.send(encodeRequestGameStart(0))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo (%#x)", reply[0], serverpackets.OpcodeSSQInfo)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected (%#x)", reply[0], serverpackets.OpcodeCharSelected)
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
}

func TestGameClientLinkCreateInvalidNameKeepsConnectionOpen(t *testing.T) {
	c, _, _ := newLinkedGameClient(t)

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
	c, chars, _ := newLinkedGameClient(t)

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
