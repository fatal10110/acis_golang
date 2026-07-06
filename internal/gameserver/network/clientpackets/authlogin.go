package clientpackets

import (
	"fmt"
	"strings"
)

// OpcodeAuthLogin is the wire opcode for AuthLogin, presenting the account
// name and session keys a client received from the login server while
// selecting this game server.
const OpcodeAuthLogin = 0x08

// AuthLogin asks the game server to confirm, with the login server, the
// session keys this client was issued: one pair delivered at login, the
// other delivered for this specific game server.
type AuthLogin struct {
	LoginName string
	PlayKey1  int32
	PlayKey2  int32
	LoginKey1 int32
	LoginKey2 int32
}

// DecodeAuthLogin parses a raw AuthLogin payload (opcode byte included).
// The account name is lower-cased, matching how account names are stored
// and looked up everywhere else in this server. The two play-key fields
// are read in reverse order (second half before first) — a fixed quirk of
// how the client assembles this packet, not a decoding choice.
func DecodeAuthLogin(payload []byte) (AuthLogin, error) {
	r := newReader(payload)

	name := strings.ToLower(r.ReadString())
	playKey2 := r.ReadInt32()
	playKey1 := r.ReadInt32()
	loginKey1 := r.ReadInt32()
	loginKey2 := r.ReadInt32()

	if r.Err() != nil {
		return AuthLogin{}, fmt.Errorf("clientpackets: AuthLogin: %w", r.Err())
	}

	return AuthLogin{
		LoginName: name,
		PlayKey1:  playKey1,
		PlayKey2:  playKey2,
		LoginKey1: loginKey1,
		LoginKey2: loginKey2,
	}, nil
}
