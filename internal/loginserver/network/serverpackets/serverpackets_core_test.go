package serverpackets

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"
)

// ---- from accountkicked_test.go ----
func TestEncodeAccountKicked(t *testing.T) {
	got := EncodeAccountKicked(AccountKickedPermanentlyBanned)

	var want []byte
	want = append(want, OpcodeAccountKicked)
	want = binary.LittleEndian.AppendUint32(want, uint32(AccountKickedPermanentlyBanned))

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeAccountKicked = %x, want %x", got, want)
	}
}

// ---- from ggauth_test.go ----
func TestEncodeGGAuth(t *testing.T) {
	got := EncodeGGAuth(GGAuthSkipRequest)

	var want []byte
	want = append(want, OpcodeGGAuth)
	want = binary.LittleEndian.AppendUint32(want, uint32(GGAuthSkipRequest))
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeGGAuth = %x, want %x", got, want)
	}
}

// ---- from init_test.go ----
func TestEncodeInit(t *testing.T) {
	modulus := bytes.Repeat([]byte{0xaa}, 128)
	blowfishKey := bytes.Repeat([]byte{0xbb}, 16)

	got := EncodeInit(0x11223344, modulus, blowfishKey)

	var want []byte
	want = append(want, OpcodeInit)
	want = binary.LittleEndian.AppendUint32(want, 0x11223344)
	want = binary.LittleEndian.AppendUint32(want, protocolVersion)
	want = append(want, modulus...)
	want = append(want, make([]byte, 16)...)
	want = append(want, blowfishKey...)
	want = append(want, 0x00)

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeInit = %x, want %x", got, want)
	}
}

// ---- from loginfail_test.go ----
func TestEncodeLoginFail(t *testing.T) {
	tests := []struct {
		name   string
		reason LoginFailReason
	}{
		{"system error", LoginFailSystemError},
		{"dual box", LoginFailDualBox},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeLoginFail(tt.reason)

			var want []byte
			want = append(want, OpcodeLoginFail)
			want = binary.LittleEndian.AppendUint32(want, uint32(tt.reason))

			if !bytes.Equal(got, want) {
				t.Errorf("EncodeLoginFail(%v) = %x, want %x", tt.reason, got, want)
			}
		})
	}
}

// ---- from loginok_test.go ----
func TestEncodeLoginOk(t *testing.T) {
	got := EncodeLoginOk(111, 222)

	var want []byte
	want = append(want, OpcodeLoginOk)
	want = binary.LittleEndian.AppendUint32(want, 111)
	want = binary.LittleEndian.AppendUint32(want, 222)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0x000003ea)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = append(want, make([]byte, 16)...)

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeLoginOk = %x, want %x", got, want)
	}
}

// ---- from oracle_test.go ----
// Fixed vectors from the Java writers in
// aCis_gameserver/java/net/sf/l2j/loginserver/network/serverpackets.
func TestJavaOracleServerPacketVectors(t *testing.T) {
	modulus, _ := hex.DecodeString("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f404142434445464748494a4b4c4d4e4f505152535455565758595a5b5c5d5e5f606162636465666768696a6b6c6d6e6f707172737475767778797a7b7c7d7e7f")
	key := []byte{0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f}

	tests := []struct {
		name string
		got  []byte
		want string
	}{
		{"AccountKicked", EncodeAccountKicked(AccountKickedPermanentlyBanned), "0220000000"},
		{"GGAuth", EncodeGGAuth(GGAuthSkipRequest), "0b0b00000000000000000000000000000000000000"},
		{"Init", EncodeInit(0x11223344, modulus, key), "004433221121c60000" + "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f404142434445464748494a4b4c4d4e4f505152535455565758595a5b5c5d5e5f606162636465666768696a6b6c6d6e6f707172737475767778797a7b7c7d7e7f" + "00000000000000000000000000000000" + "101112131415161718191a1b1c1d1e1f00"},
		{"LoginFail", EncodeLoginFail(LoginFailUserOrPassWrong), "0103000000"},
		{"LoginOk", EncodeLoginOk(0x11223344, -0x66554434), "0344332211ccbbaa990000000000000000ea03000000000000000000000000000000000000000000000000000000000000"},
		{"PlayFail", EncodePlayFail(PlayFailTooManyPlayers), "060f"},
		{"PlayOk", EncodePlayOk(0x11223344, -0x66554434), "0744332211ccbbaa99"},
		{"ServerList", EncodeServerList(7, []ServerEntry{{ID: 7, IP: [4]byte{192, 0, 2, 1}, Port: 7777, AgeLimit: 18, PvP: true, CurrentPlayers: 42, MaxPlayers: 100, Online: true, TestServer: true, ShowClock: true, ShowBrackets: true}}), "04010707c0000201611e000012012a006400010600000001"},
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

// ---- from playfail_test.go ----
func TestEncodePlayFail(t *testing.T) {
	got := EncodePlayFail(PlayFailTooManyPlayers)
	want := []byte{OpcodePlayFail, byte(PlayFailTooManyPlayers)}

	if !bytes.Equal(got, want) {
		t.Errorf("EncodePlayFail = %x, want %x", got, want)
	}
}

// ---- from playok_test.go ----
func TestEncodePlayOk(t *testing.T) {
	got := EncodePlayOk(333, 444)

	var want []byte
	want = append(want, OpcodePlayOk)
	want = binary.LittleEndian.AppendUint32(want, 333)
	want = binary.LittleEndian.AppendUint32(want, 444)

	if !bytes.Equal(got, want) {
		t.Errorf("EncodePlayOk = %x, want %x", got, want)
	}
}

// ---- from serverlist_test.go ----
func TestEncodeServerList(t *testing.T) {
	servers := []ServerEntry{
		{
			ID:             1,
			IP:             [4]byte{127, 0, 0, 1},
			Port:           7777,
			AgeLimit:       15,
			PvP:            true,
			CurrentPlayers: 10,
			MaxPlayers:     100,
			Online:         true,
			TestServer:     true,
			ShowClock:      true,
			ShowBrackets:   true,
		},
		{
			ID:   2,
			IP:   [4]byte{10, 0, 0, 2},
			Port: 7778,
		},
	}

	// Known-good vector: 3-byte header, then one 21-byte block per server
	// (id 1, ip 4, port 4, age 1, pvp 1, current 2, max 2, online 1,
	// flag bits 4, brackets 1).
	want, err := hex.DecodeString(
		"040201" +
			"017f000001611e00000f010a006400010600000001" +
			"020a000002621e0000000000000000000000000000")
	if err != nil {
		t.Fatalf("decode vector: %v", err)
	}

	got := EncodeServerList(1, servers)

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeServerList = %x, want %x", got, want)
	}
}
