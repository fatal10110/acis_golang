package link

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"encoding/hex"
	"reflect"
	"testing"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
)

// ---- from authresponse_test.go ----
func TestEncodeAuthResponse(t *testing.T) {
	got := EncodeAuthResponse(3, "MyServer")
	want := appendString([]byte{OpcodeAuthResponse, 3}, "MyServer")
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeAuthResponse() = %x, want %x", got, want)
	}
}

func TestDecodeAuthResponse(t *testing.T) {
	id, name, err := DecodeAuthResponse(EncodeAuthResponse(3, "MyServer"))
	if err != nil {
		t.Fatalf("DecodeAuthResponse: %v", err)
	}
	if id != 3 || name != "MyServer" {
		t.Fatalf("DecodeAuthResponse() = %d, %q, want 3, MyServer", id, name)
	}
}

func TestDecodeAuthResponseShort(t *testing.T) {
	if _, _, err := DecodeAuthResponse([]byte{OpcodeAuthResponse}); err == nil {
		t.Error("DecodeAuthResponse: want error on short payload, got nil")
	}
}

// ---- from blowfishkey_test.go ----
func TestDecodeBlowFishKey(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	dynamicKey := []byte{0x03, 0x0a, 0x11, 0x18, 0x1f, 0x26, 0x2d, 0x34, 0x3b, 0x42, 0x49, 0x50, 0x57, 0x5e, 0x65, 0x6c}
	ciphertext := crypt.EncryptDynamicKey(&priv.PublicKey, dynamicKey)

	var payload []byte
	payload = append(payload, OpcodeBlowFishKey)
	payload = binary.LittleEndian.AppendUint32(payload, uint32(len(ciphertext)))
	payload = append(payload, ciphertext...)

	got, err := DecodeBlowFishKey(payload, priv)
	if err != nil {
		t.Fatalf("DecodeBlowFishKey: %v", err)
	}
	if !bytes.Equal(got, dynamicKey) {
		t.Fatalf("DecodeBlowFishKey() = %x, want %x", got, dynamicKey)
	}
}

func TestDecodeBlowFishKeyShort(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	payload := []byte{OpcodeBlowFishKey, 0xff, 0xff, 0xff, 0x7f} // claims ~2GB of ciphertext
	if _, err := DecodeBlowFishKey(payload, priv); err == nil {
		t.Error("DecodeBlowFishKey: want error on truncated payload, got nil")
	}
}

func TestEncodeBlowFishKeyRoundTrip(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	dynamicKey := []byte{0x03, 0x0a, 0x11, 0x18, 0x1f, 0x26, 0x2d, 0x34, 0x3b, 0x42, 0x49, 0x50, 0x57, 0x5e, 0x65, 0x6c}

	payload := EncodeBlowFishKey(&priv.PublicKey, dynamicKey)
	got, err := DecodeBlowFishKey(payload, priv)
	if err != nil {
		t.Fatalf("DecodeBlowFishKey(EncodeBlowFishKey()): %v", err)
	}
	if !bytes.Equal(got, dynamicKey) {
		t.Fatalf("round trip = %x, want %x", got, dynamicKey)
	}
}

// ---- from changeaccesslevel_test.go ----
func TestDecodeChangeAccessLevel(t *testing.T) {
	payload := binary.LittleEndian.AppendUint32([]byte{OpcodeChangeAccessLevel}, 100)
	payload = appendString(payload, "alice")

	got, err := DecodeChangeAccessLevel(payload)
	if err != nil {
		t.Fatalf("DecodeChangeAccessLevel: %v", err)
	}
	want := ChangeAccessLevel{Level: 100, Account: "alice"}
	if got != want {
		t.Fatalf("DecodeChangeAccessLevel() = %+v, want %+v", got, want)
	}
}

func TestDecodeChangeAccessLevelShort(t *testing.T) {
	if _, err := DecodeChangeAccessLevel([]byte{OpcodeChangeAccessLevel, 1, 2}); err == nil {
		t.Error("DecodeChangeAccessLevel: want error on short payload, got nil")
	}
}

func TestEncodeChangeAccessLevelRoundTrip(t *testing.T) {
	want := ChangeAccessLevel{Level: -1, Account: "alice"}
	got, err := DecodeChangeAccessLevel(EncodeChangeAccessLevel(want))
	if err != nil {
		t.Fatalf("DecodeChangeAccessLevel(EncodeChangeAccessLevel()): %v", err)
	}
	if got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

// ---- from gameserverauth_test.go ----
func TestDecodeGameServerAuth(t *testing.T) {
	hexID := []byte{0xde, 0xad, 0xbe, 0xef}

	payload := []byte{OpcodeGameServerAuth, 3, 1, 0}
	payload = appendString(payload, "gs.example.com")
	payload = binary.LittleEndian.AppendUint16(payload, 7777)
	payload = binary.LittleEndian.AppendUint32(payload, 500)
	payload = binary.LittleEndian.AppendUint32(payload, uint32(len(hexID)))
	payload = append(payload, hexID...)

	got, err := DecodeGameServerAuth(payload)
	if err != nil {
		t.Fatalf("DecodeGameServerAuth: %v", err)
	}
	want := GameServerAuth{
		DesiredID:         3,
		AcceptAlternateID: true,
		HostReserved:      false,
		HostName:          "gs.example.com",
		Port:              7777,
		MaxPlayers:        500,
		HexID:             hexID,
	}
	if got.DesiredID != want.DesiredID || got.AcceptAlternateID != want.AcceptAlternateID ||
		got.HostReserved != want.HostReserved || got.HostName != want.HostName ||
		got.Port != want.Port || got.MaxPlayers != want.MaxPlayers || !bytes.Equal(got.HexID, want.HexID) {
		t.Fatalf("DecodeGameServerAuth() = %+v, want %+v", got, want)
	}
}

func TestDecodeGameServerAuthShort(t *testing.T) {
	if _, err := DecodeGameServerAuth([]byte{OpcodeGameServerAuth, 1}); err == nil {
		t.Error("DecodeGameServerAuth: want error on short payload, got nil")
	}
}

func TestEncodeGameServerAuthRoundTrip(t *testing.T) {
	want := GameServerAuth{
		DesiredID:         3,
		AcceptAlternateID: true,
		HostReserved:      false,
		HostName:          "gs.example.com",
		Port:              7777,
		MaxPlayers:        500,
		HexID:             []byte{0xde, 0xad, 0xbe, 0xef},
	}

	got, err := DecodeGameServerAuth(EncodeGameServerAuth(want))
	if err != nil {
		t.Fatalf("DecodeGameServerAuth(EncodeGameServerAuth()): %v", err)
	}
	if got.DesiredID != want.DesiredID || got.AcceptAlternateID != want.AcceptAlternateID ||
		got.HostReserved != want.HostReserved || got.HostName != want.HostName ||
		got.Port != want.Port || got.MaxPlayers != want.MaxPlayers || !bytes.Equal(got.HexID, want.HexID) {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

// ---- from helpers_test.go ----
// appendString appends s UTF-16LE-encoded with its 0x0000 terminator, for
// building test payloads.
func appendString(buf []byte, s string) []byte {
	for _, u := range utf16.Encode([]rune(s)) {
		buf = binary.LittleEndian.AppendUint16(buf, u)
	}
	return binary.LittleEndian.AppendUint16(buf, 0)
}

// ---- from initls_test.go ----
func TestEncodeInitLS(t *testing.T) {
	pubKey := bytes.Repeat([]byte{0xaa}, 128)

	got := EncodeInitLS(pubKey)

	var want []byte
	want = append(want, OpcodeInitLS)
	want = binary.LittleEndian.AppendUint32(want, ProtocolRevision)
	want = binary.LittleEndian.AppendUint32(want, uint32(len(pubKey)))
	want = append(want, pubKey...)

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeInitLS() = %x, want %x", got, want)
	}
}

func TestDecodeInitLS(t *testing.T) {
	pubKey := bytes.Repeat([]byte{0xaa}, 128)
	payload := EncodeInitLS(pubKey)

	revision, key, err := DecodeInitLS(payload)
	if err != nil {
		t.Fatalf("DecodeInitLS: %v", err)
	}
	if revision != ProtocolRevision {
		t.Errorf("revision = %#x, want %#x", revision, ProtocolRevision)
	}
	if !bytes.Equal(key, pubKey) {
		t.Errorf("publicKey = %x, want %x", key, pubKey)
	}
}

func TestDecodeInitLSShort(t *testing.T) {
	payload := []byte{OpcodeInitLS, 0x02, 0x01, 0x00, 0x00, 0xff, 0xff, 0xff, 0x7f} // claims ~2GB of key bytes
	if _, _, err := DecodeInitLS(payload); err == nil {
		t.Error("DecodeInitLS: want error on truncated payload, got nil")
	}
}

// ---- from kickplayer_test.go ----
func TestEncodeKickPlayer(t *testing.T) {
	got := EncodeKickPlayer("alice")
	want := appendString([]byte{OpcodeKickPlayer}, "alice")
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeKickPlayer() = %x, want %x", got, want)
	}
}

func TestDecodeKickPlayer(t *testing.T) {
	got, err := DecodeKickPlayer(EncodeKickPlayer("alice"))
	if err != nil {
		t.Fatalf("DecodeKickPlayer: %v", err)
	}
	if got != "alice" {
		t.Fatalf("DecodeKickPlayer() = %q, want %q", got, "alice")
	}
}

func TestDecodeKickPlayerShort(t *testing.T) {
	if _, err := DecodeKickPlayer([]byte{OpcodeKickPlayer, 'a'}); err == nil {
		t.Error("DecodeKickPlayer: want error on unterminated string, got nil")
	}
}

// ---- from loginserverfail_test.go ----
func TestEncodeLoginServerFail(t *testing.T) {
	got := EncodeLoginServerFail(ReasonAlreadyLoggedIn)
	want := []byte{OpcodeLoginServerFail, byte(ReasonAlreadyLoggedIn)}
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeLoginServerFail() = %x, want %x", got, want)
	}
}

func TestDecodeLoginServerFail(t *testing.T) {
	got, err := DecodeLoginServerFail(EncodeLoginServerFail(ReasonWrongHexID))
	if err != nil {
		t.Fatalf("DecodeLoginServerFail: %v", err)
	}
	if got != ReasonWrongHexID {
		t.Fatalf("DecodeLoginServerFail() = %v, want %v", got, ReasonWrongHexID)
	}
}

func TestDecodeLoginServerFailShort(t *testing.T) {
	if _, err := DecodeLoginServerFail([]byte{OpcodeLoginServerFail}); err == nil {
		t.Error("DecodeLoginServerFail: want error on short payload, got nil")
	}
}

func TestLoginServerFailReasonString(t *testing.T) {
	tests := map[LoginServerFailReason]string{
		ReasonIPBanned:        "ip banned",
		ReasonIPReserved:      "ip reserved",
		ReasonWrongHexID:      "wrong hexid",
		ReasonIDReserved:      "id reserved",
		ReasonNoFreeID:        "no free ID",
		ReasonNotAuthed:       "not authed",
		ReasonAlreadyLoggedIn: "already logged in",
	}
	for reason, want := range tests {
		if got := reason.String(); got != want {
			t.Errorf("%v.String() = %q, want %q", reason, got, want)
		}
	}
	if got := LoginServerFailReason(0).String(); got == "" {
		t.Error("LoginServerFailReason(0).String() = empty, want a fallback description")
	}
}

// ---- from oracle_test.go ----
// These are Java GS-LS packet payloads before ServerBasePacket reserves its
// checksum and pads for Blowfish; LinkCrypt owns that common transport step.
func TestJavaOracleLinkPacketVectors(t *testing.T) {
	playerInGame, err := EncodePlayerInGame([]string{"alice", "bob"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		got  []byte
		want string
	}{
		{"InitLS", EncodeInitLS([]byte{0xaa, 0xbb, 0xcc}), "000201000003000000aabbcc"},
		{"AuthResponse", EncodeAuthResponse(7, "Giran"), "020747006900720061006e000000"},
		{"LoginServerFail", EncodeLoginServerFail(ReasonAlreadyLoggedIn), "0107"},
		{"KickPlayer", EncodeKickPlayer("alice"), "0461006c006900630065000000"},
		{"PlayerAuthResponse", EncodePlayerAuthResponse("alice", true), "0361006c00690063006500000001"},
		{"GameServerAuth", EncodeGameServerAuth(GameServerAuth{DesiredID: 7, AcceptAlternateID: true, HostName: "127.0.0.1", Port: 7777, MaxPlayers: 100, HexID: []byte{0xaa, 0xbb}}), "010701003100320037002e0030002e0030002e0031000000611e6400000002000000aabb"},
		{"PlayerAuthRequest", EncodePlayerAuthRequest(PlayerAuthRequest{Account: "alice", SessionKey: SessionKey{PlayKey1: 1, PlayKey2: 2, LoginKey1: 3, LoginKey2: 4}}), "0561006c00690063006500000001000000020000000300000004000000"},
		{"PlayerInGame", playerInGame, "02020061006c00690063006500000062006f0062000000"},
		{"PlayerLogout", EncodePlayerLogout("alice"), "0361006c006900630065000000"},
		{"ChangeAccessLevel", EncodeChangeAccessLevel(ChangeAccessLevel{Level: -1, Account: "alice"}), "04ffffffff61006c006900630065000000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, err := hex.DecodeString(tt.want)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(tt.got, want) {
				t.Errorf("packet = %x, want %x", tt.got, want)
			}
		})
	}
}

// ---- from playerauthrequest_test.go ----
func TestDecodePlayerAuthRequest(t *testing.T) {
	payload := appendString([]byte{OpcodePlayerAuthRequest}, "alice")
	payload = binary.LittleEndian.AppendUint32(payload, uint32(int32(11)))
	payload = binary.LittleEndian.AppendUint32(payload, uint32(int32(22)))
	payload = binary.LittleEndian.AppendUint32(payload, uint32(int32(33)))
	payload = binary.LittleEndian.AppendUint32(payload, uint32(int32(44)))

	got, err := DecodePlayerAuthRequest(payload)
	if err != nil {
		t.Fatalf("DecodePlayerAuthRequest: %v", err)
	}
	want := PlayerAuthRequest{Account: "alice", SessionKey: SessionKey{PlayKey1: 11, PlayKey2: 22, LoginKey1: 33, LoginKey2: 44}}
	if got != want {
		t.Fatalf("DecodePlayerAuthRequest() = %+v, want %+v", got, want)
	}
}

func TestDecodePlayerAuthRequestShort(t *testing.T) {
	payload := appendString([]byte{OpcodePlayerAuthRequest}, "alice")
	if _, err := DecodePlayerAuthRequest(payload); err == nil {
		t.Error("DecodePlayerAuthRequest: want error on short payload, got nil")
	}
}

func TestEncodePlayerAuthRequestRoundTrip(t *testing.T) {
	want := PlayerAuthRequest{Account: "alice", SessionKey: SessionKey{PlayKey1: 11, PlayKey2: 22, LoginKey1: 33, LoginKey2: 44}}
	got, err := DecodePlayerAuthRequest(EncodePlayerAuthRequest(want))
	if err != nil {
		t.Fatalf("DecodePlayerAuthRequest(EncodePlayerAuthRequest()): %v", err)
	}
	if got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

// ---- from playerauthresponse_test.go ----
func TestEncodePlayerAuthResponse(t *testing.T) {
	tests := []struct {
		ok   bool
		want byte
	}{
		{true, 1},
		{false, 0},
	}
	for _, tt := range tests {
		got := EncodePlayerAuthResponse("alice", tt.ok)
		want := appendString([]byte{OpcodePlayerAuthResponse}, "alice")
		want = append(want, tt.want)
		if !bytes.Equal(got, want) {
			t.Errorf("EncodePlayerAuthResponse(%v) = %x, want %x", tt.ok, got, want)
		}
	}
}

func TestDecodePlayerAuthResponse(t *testing.T) {
	for _, ok := range []bool{true, false} {
		account, got, err := DecodePlayerAuthResponse(EncodePlayerAuthResponse("alice", ok))
		if err != nil {
			t.Fatalf("DecodePlayerAuthResponse: %v", err)
		}
		if account != "alice" || got != ok {
			t.Fatalf("DecodePlayerAuthResponse() = %q, %v, want alice, %v", account, got, ok)
		}
	}
}

func TestDecodePlayerAuthResponseShort(t *testing.T) {
	if _, _, err := DecodePlayerAuthResponse([]byte{OpcodePlayerAuthResponse, 'a'}); err == nil {
		t.Error("DecodePlayerAuthResponse: want error on unterminated string, got nil")
	}
}

// ---- from playeringame_test.go ----
func TestDecodePlayerInGame(t *testing.T) {
	payload := binary.LittleEndian.AppendUint16([]byte{OpcodePlayerInGame}, 2)
	payload = appendString(payload, "alice")
	payload = appendString(payload, "bob")

	got, err := DecodePlayerInGame(payload)
	if err != nil {
		t.Fatalf("DecodePlayerInGame: %v", err)
	}
	want := []string{"alice", "bob"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DecodePlayerInGame() = %v, want %v", got, want)
	}
}

func TestDecodePlayerInGameEmpty(t *testing.T) {
	payload := binary.LittleEndian.AppendUint16([]byte{OpcodePlayerInGame}, 0)
	got, err := DecodePlayerInGame(payload)
	if err != nil {
		t.Fatalf("DecodePlayerInGame: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("DecodePlayerInGame() = %v, want empty", got)
	}
}

func TestDecodePlayerInGameShort(t *testing.T) {
	payload := binary.LittleEndian.AppendUint16([]byte{OpcodePlayerInGame}, 5)
	if _, err := DecodePlayerInGame(payload); err == nil {
		t.Error("DecodePlayerInGame: want error on truncated payload, got nil")
	}
}

func TestEncodePlayerInGameRoundTrip(t *testing.T) {
	want := []string{"alice", "bob"}
	payload, err := EncodePlayerInGame(want)
	if err != nil {
		t.Fatalf("EncodePlayerInGame: %v", err)
	}
	got, err := DecodePlayerInGame(payload)
	if err != nil {
		t.Fatalf("DecodePlayerInGame(EncodePlayerInGame()): %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %v, want %v", got, want)
	}
}

func TestEncodePlayerInGameSingle(t *testing.T) {
	want := []string{"alice"}
	payload, err := EncodePlayerInGame(want)
	if err != nil {
		t.Fatalf("EncodePlayerInGame: %v", err)
	}
	got, err := DecodePlayerInGame(payload)
	if err != nil {
		t.Fatalf("DecodePlayerInGame(EncodePlayerInGame()): %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %v, want %v", got, want)
	}
}

func TestEncodePlayerInGameRejectsOversizedCount(t *testing.T) {
	if _, err := EncodePlayerInGame(make([]string, 1<<16)); err == nil {
		t.Fatal("EncodePlayerInGame oversized count error = nil, want error")
	}
}

// ---- from playerlogout_test.go ----
func TestDecodePlayerLogout(t *testing.T) {
	payload := appendString([]byte{OpcodePlayerLogout}, "alice")

	got, err := DecodePlayerLogout(payload)
	if err != nil {
		t.Fatalf("DecodePlayerLogout: %v", err)
	}
	if got != "alice" {
		t.Fatalf("DecodePlayerLogout() = %q, want %q", got, "alice")
	}
}

func TestDecodePlayerLogoutShort(t *testing.T) {
	if _, err := DecodePlayerLogout([]byte{OpcodePlayerLogout, 'a'}); err == nil {
		t.Error("DecodePlayerLogout: want error on unterminated string, got nil")
	}
}

func TestEncodePlayerLogoutRoundTrip(t *testing.T) {
	got, err := DecodePlayerLogout(EncodePlayerLogout("alice"))
	if err != nil {
		t.Fatalf("DecodePlayerLogout(EncodePlayerLogout()): %v", err)
	}
	if got != "alice" {
		t.Fatalf("round trip = %q, want %q", got, "alice")
	}
}

// ---- from serverstatus_test.go ----
func appendAttr(buf []byte, attr, value int32) []byte {
	buf = binary.LittleEndian.AppendUint32(buf, uint32(attr))
	return binary.LittleEndian.AppendUint32(buf, uint32(value))
}

func TestDecodeServerStatus(t *testing.T) {
	payload := binary.LittleEndian.AppendUint32([]byte{OpcodeServerStatus}, 4)
	payload = appendAttr(payload, 1, int32(ServerTypeNormal))
	payload = appendAttr(payload, 2, serverStatusOn)
	payload = appendAttr(payload, 4, 18)
	payload = appendAttr(payload, 7, 300)

	got, err := DecodeServerStatus(payload)
	if err != nil {
		t.Fatalf("DecodeServerStatus: %v", err)
	}
	if got.Status == nil || *got.Status != ServerTypeNormal {
		t.Errorf("Status = %v, want %v", got.Status, ServerTypeNormal)
	}
	if got.ShowClock == nil || *got.ShowClock != true {
		t.Errorf("ShowClock = %v, want true", got.ShowClock)
	}
	if got.ShowBrackets != nil {
		t.Errorf("ShowBrackets = %v, want nil (not sent)", got.ShowBrackets)
	}
	if got.AgeLimit == nil || *got.AgeLimit != 18 {
		t.Errorf("AgeLimit = %v, want 18", got.AgeLimit)
	}
	if got.MaxPlayers == nil || *got.MaxPlayers != 300 {
		t.Errorf("MaxPlayers = %v, want 300", got.MaxPlayers)
	}
}

func TestDecodeServerStatusEmpty(t *testing.T) {
	payload := binary.LittleEndian.AppendUint32([]byte{OpcodeServerStatus}, 0)
	got, err := DecodeServerStatus(payload)
	if err != nil {
		t.Fatalf("DecodeServerStatus: %v", err)
	}
	if got != (ServerStatus{}) {
		t.Fatalf("DecodeServerStatus() = %+v, want zero value", got)
	}
}

func TestDecodeServerStatusShort(t *testing.T) {
	payload := binary.LittleEndian.AppendUint32([]byte{OpcodeServerStatus}, 5)
	if _, err := DecodeServerStatus(payload); err == nil {
		t.Error("DecodeServerStatus: want error on truncated payload, got nil")
	}
}

func TestServerTypeString(t *testing.T) {
	tests := map[ServerType]string{
		ServerTypeAuto:   "Auto",
		ServerTypeGood:   "Good",
		ServerTypeNormal: "Normal",
		ServerTypeFull:   "Full",
		ServerTypeDown:   "Down",
		ServerTypeGMOnly: "Gm Only",
	}
	for st, want := range tests {
		if got := st.String(); got != want {
			t.Errorf("ServerType(%d).String() = %q, want %q", st, got, want)
		}
	}
}

func TestEncodeServerStatusRoundTrip(t *testing.T) {
	normal := ServerTypeNormal
	trueVal := true
	age := int32(18)
	maxPlayers := int32(300)
	want := ServerStatus{
		Status:     &normal,
		ShowClock:  &trueVal,
		AgeLimit:   &age,
		MaxPlayers: &maxPlayers,
	}

	got, err := DecodeServerStatus(EncodeServerStatus(want))
	if err != nil {
		t.Fatalf("DecodeServerStatus(EncodeServerStatus()): %v", err)
	}
	if got.Status == nil || *got.Status != *want.Status ||
		got.ShowClock == nil || *got.ShowClock != *want.ShowClock ||
		got.ShowBrackets != nil ||
		got.AgeLimit == nil || *got.AgeLimit != *want.AgeLimit ||
		got.TestServer != nil || got.Pvp != nil ||
		got.MaxPlayers == nil || *got.MaxPlayers != *want.MaxPlayers {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func TestEncodeServerStatusEmpty(t *testing.T) {
	want := []byte{OpcodeServerStatus, 0, 0, 0, 0}
	got := EncodeServerStatus(ServerStatus{})
	if len(got) != len(want) {
		t.Fatalf("EncodeServerStatus(zero) = %x, want %x", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("EncodeServerStatus(zero) = %x, want %x", got, want)
		}
	}
}
