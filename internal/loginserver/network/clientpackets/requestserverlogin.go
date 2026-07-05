package clientpackets

import "fmt"

// OpcodeRequestServerLogin is the wire opcode for RequestServerLogin, valid
// once the client has authenticated its login/password.
const OpcodeRequestServerLogin = 0x02

const requestServerLoginSize = 9

// RequestServerLogin asks to play on a chosen game server, presenting back
// the session key halves the client received in LoginOk.
type RequestServerLogin struct {
	SessionKey1 int32
	SessionKey2 int32
	ServerID    byte
}

// DecodeRequestServerLogin parses a raw RequestServerLogin payload (opcode
// byte included).
func DecodeRequestServerLogin(payload []byte) (RequestServerLogin, error) {
	r := newReader(payload)
	if r.Remaining() < requestServerLoginSize {
		return RequestServerLogin{}, fmt.Errorf("clientpackets: RequestServerLogin: need %d bytes, got %d", requestServerLoginSize, r.Remaining())
	}
	return RequestServerLogin{
		SessionKey1: r.ReadInt32(),
		SessionKey2: r.ReadInt32(),
		ServerID:    r.ReadUint8(),
	}, nil
}
