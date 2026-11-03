package botclient

import (
	"crypto/rsa"
	"reflect"
	"testing"
	"time"

	commoncrypt "github.com/fatal10110/acis_golang/internal/commons/crypt"
	gameclient "github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	logincrypt "github.com/fatal10110/acis_golang/internal/loginserver/crypt"
	loginclient "github.com/fatal10110/acis_golang/internal/loginserver/network/clientpackets"
	loginserver "github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

// Every packet the bot encodes is read back through the server's own decoder.

func TestLoginEncodersRoundTrip(t *testing.T) {
	gg, err := loginclient.DecodeAuthGameGuard(encodeAuthGameGuard(0x5a5a5a5a))
	if err != nil || gg != (loginclient.AuthGameGuard{SessionID: 0x5a5a5a5a}) {
		t.Fatalf("AuthGameGuard = %+v, %v", gg, err)
	}
	list, err := loginclient.DecodeRequestServerList(encodeRequestServerList(7, -9))
	if err != nil || list != (loginclient.RequestServerList{SessionKey1: 7, SessionKey2: -9}) {
		t.Fatalf("RequestServerList = %+v, %v", list, err)
	}
	sl, err := loginclient.DecodeRequestServerLogin(encodeRequestServerLogin(7, -9, 3))
	if err != nil || sl != (loginclient.RequestServerLogin{SessionKey1: 7, SessionKey2: -9, ServerID: 3}) {
		t.Fatalf("RequestServerLogin = %+v, %v", sl, err)
	}

	pair, err := commoncrypt.NewLoginKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	auth, err := loginclient.DecodeRequestAuthLogin(encodeRequestAuthLogin(&pair.Private.PublicKey, "bot12345678901", "pass456789012345"), pair.Private)
	if err != nil || auth != (loginclient.RequestAuthLogin{Username: "bot12345678901", Password: "pass456789012345"}) {
		t.Fatalf("RequestAuthLogin = %+v, %v", auth, err)
	}
}

func TestLoginRepliesDecode(t *testing.T) {
	pair, err := commoncrypt.NewLoginKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("fedcba9876543210")
	server, err := logincrypt.NewLoginCrypt(key)
	if err != nil {
		t.Fatal(err)
	}
	init, err := decodeInit(server.Encrypt(loginserver.EncodeInit(0x1234, pair.ScrambledModulus, key)))
	if err != nil {
		t.Fatal(err)
	}
	want := initPacket{sessionID: 0x1234, publicKey: &rsa.PublicKey{N: pair.Private.N, E: pair.Private.E}, blowfishKey: key}
	if !reflect.DeepEqual(init, want) {
		t.Fatalf("Init = %+v, want %+v", init, want)
	}

	if k1, k2 := decodeLoginOk(loginserver.EncodeLoginOk(11, -22)); k1 != 11 || k2 != -22 {
		t.Fatalf("LoginOk keys = %d, %d", k1, k2)
	}
	if k1, k2 := decodePlayOk(loginserver.EncodePlayOk(33, -44)); k1 != 33 || k2 != -44 {
		t.Fatalf("PlayOk keys = %d, %d", k1, k2)
	}
	servers, err := decodeServerList(loginserver.EncodeServerList(1, []loginserver.ServerEntry{
		{ID: 1, IP: [4]byte{127, 0, 0, 1}, Port: 7777, Online: true, MaxPlayers: 300},
		{ID: 2, IP: [4]byte{10, 0, 0, 2}, Port: 7778},
	}))
	wantServers := []ServerEntry{{ID: 1, Addr: "127.0.0.1:7777", Online: true}, {ID: 2, Addr: "10.0.0.2:7778"}}
	if err != nil || !reflect.DeepEqual(servers, wantServers) {
		t.Fatalf("ServerList = %+v, %v", servers, err)
	}

	var le *LoginError
	if err := expectLoginReply(loginserver.EncodePlayFail(loginserver.PlayFailReason(3)), loginserver.OpcodePlayOk, "PlayOk"); !asLoginError(err, &le) || le.Packet != "PlayFail" || le.Reason != 3 {
		t.Fatalf("PlayFail reply = %v", err)
	}
	if err := expectLoginReply(loginserver.EncodeLoginFail(loginserver.LoginFailReason(2)), loginserver.OpcodeLoginOk, "LoginOk"); !asLoginError(err, &le) || le.Packet != "LoginFail" || le.Reason != 2 {
		t.Fatalf("LoginFail reply = %v", err)
	}
}

func asLoginError(err error, out **LoginError) bool {
	le, ok := err.(*LoginError)
	*out = le
	return ok
}

func TestGameEncodersRoundTrip(t *testing.T) {
	check := func(name string, got, want any, err error) {
		t.Helper()
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %+v, %v; want %+v", name, got, err, want)
		}
	}
	pv, err := gameclient.DecodeProtocolVersion(encodeProtocolVersion(746))
	check("ProtocolVersion", pv, gameclient.ProtocolVersion{Revision: 746}, err)

	keys := SessionKeys{LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4}
	al, err := gameclient.DecodeAuthLogin(encodeAuthLogin("bot7", keys))
	check("AuthLogin", al, gameclient.AuthLogin{LoginName: "bot7", LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4}, err)

	cc, err := gameclient.DecodeRequestCharacterCreate(encodeRequestCharacterCreate("bot7"))
	check("RequestCharacterCreate", cc, gameclient.RequestCharacterCreate{Name: "bot7", HairStyle: 1}, err)

	gs, err := gameclient.DecodeRequestGameStart(encodeRequestGameStart(2))
	check("RequestGameStart", gs, gameclient.RequestGameStart{Slot: 2}, err)

	mv, err := gameclient.DecodeMoveBackwardToLocation(encodeMove(1, 2, 3, -4, -5, -6))
	check("MoveBackwardToLocation", mv, gameclient.MoveBackwardToLocation{TargetX: 1, TargetY: 2, TargetZ: 3, OriginX: -4, OriginY: -5, OriginZ: -6, MoveMovement: 1}, err)

	vp, err := gameclient.DecodeValidatePosition(encodeValidatePosition(10, 20, 30, 40))
	check("ValidatePosition", vp, gameclient.ValidatePosition{X: 10, Y: 20, Z: 30, Heading: 40}, err)

	say, err := gameclient.DecodeSay2(encodeSay("hello"))
	check("Say2", say, gameclient.Say2{Text: "hello", Type: chatAll}, err)

	act, err := gameclient.DecodeAction(encodeTargetAction(gameclient.OpcodeAction, 99, 1, 2, 3))
	check("Action", act, gameclient.Action{ObjectID: 99, OriginX: 1, OriginY: 2, OriginZ: 3}, err)

	atk, err := gameclient.DecodeAttackRequest(encodeTargetAction(gameclient.OpcodeAttackRequest, 99, 1, 2, 3))
	check("AttackRequest", atk, gameclient.AttackRequest{ObjectID: 99, OriginX: 1, OriginY: 2, OriginZ: 3}, err)

	rp, err := gameclient.DecodeRequestRestartPoint(encodeRequestRestartPoint())
	check("RequestRestartPoint", rp, gameclient.RequestRestartPoint{}, err)

	for name, p := range map[string][]byte{"EnterWorld": encodeEnterWorld(), "Logout": encodeLogout()} {
		if len(p) != 1 {
			t.Errorf("%s payload = %x, want the opcode alone", name, p)
		}
	}
	if encodeEnterWorld()[0] != gameclient.OpcodeEnterWorld || encodeLogout()[0] != gameclient.OpcodeLogout {
		t.Error("EnterWorld/Logout opcode mismatch")
	}
}

func TestSummarizeNearestRank(t *testing.T) {
	var ds []time.Duration
	for i := 100; i >= 1; i-- {
		ds = append(ds, time.Duration(i)*time.Millisecond)
	}
	got := summarize(ds)
	if got != (Latency{Count: 100, P50ms: 50, P99ms: 99, Maxms: 100}) {
		t.Fatalf("summarize = %+v", got)
	}
	if summarize(nil) != (Latency{}) {
		t.Fatal("summarize(nil) is not zero")
	}
}

func TestConfigValidate(t *testing.T) {
	ok := DefaultConfig()
	if err := ok.Validate(); err != nil {
		t.Fatalf("default config: %v", err)
	}
	for name, mutate := range map[string]func(*Config){
		"long account":  func(c *Config) { c.Prefix = "abcdefghijklm"; c.Bots = 11 },
		"upper prefix":  func(c *Config) { c.Prefix = "Bot" },
		"no budget":     func(c *Config) { c.Duration, c.Actions = 0, 0 },
		"long password": func(c *Config) { c.Password = "12345678901234567" },
		"no bots":       func(c *Config) { c.Bots = 0 },
	} {
		c := DefaultConfig()
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: Validate accepted %+v", name, c)
		}
	}
}
