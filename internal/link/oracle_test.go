package link

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// These are Java GS-LS packet payloads before ServerBasePacket reserves its
// checksum and pads for Blowfish; LinkCrypt owns that common transport step.
func TestJavaOracleLinkPacketVectors(t *testing.T) {
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
		{"PlayerInGame", EncodePlayerInGame([]string{"alice", "bob"}), "02020061006c00690063006500000062006f0062000000"},
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
