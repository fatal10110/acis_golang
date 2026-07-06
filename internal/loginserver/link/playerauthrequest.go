package link

import "fmt"

// OpcodePlayerAuthRequest is the wire opcode for PlayerAuthRequest, a game
// server asking the login server to validate a client's session keys at
// enter-world time.
const OpcodePlayerAuthRequest = 0x05

// PlayerAuthRequest asks the login server to confirm that account is
// currently authenticated with the given session key halves: the pair the
// login server issued at login (LoginKey1/2) and the pair it issued for
// this game server via PlayOk (PlayKey1/2).
type PlayerAuthRequest struct {
	Account   string
	PlayKey1  int32
	PlayKey2  int32
	LoginKey1 int32
	LoginKey2 int32
}

// DecodePlayerAuthRequest parses a raw PlayerAuthRequest payload (opcode
// byte included).
func DecodePlayerAuthRequest(payload []byte) (PlayerAuthRequest, error) {
	r := newReader(payload)
	req := PlayerAuthRequest{
		Account:   r.ReadString(),
		PlayKey1:  r.ReadInt32(),
		PlayKey2:  r.ReadInt32(),
		LoginKey1: r.ReadInt32(),
		LoginKey2: r.ReadInt32(),
	}
	if r.Err() != nil {
		return PlayerAuthRequest{}, fmt.Errorf("link: PlayerAuthRequest: %w", r.Err())
	}
	return req, nil
}

// EncodePlayerAuthRequest builds the PlayerAuthRequest packet asking the
// login server to validate req.Account's presented session keys.
func EncodePlayerAuthRequest(req PlayerAuthRequest) []byte {
	w := newWriter(OpcodePlayerAuthRequest)
	w.WriteString(req.Account)
	w.WriteInt32(req.PlayKey1)
	w.WriteInt32(req.PlayKey2)
	w.WriteInt32(req.LoginKey1)
	w.WriteInt32(req.LoginKey2)
	return w.Bytes()
}
