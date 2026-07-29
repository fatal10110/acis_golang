package serverpackets

import (
	"bytes"
	"encoding/hex"
	"testing"
)

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
