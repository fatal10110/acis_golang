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
	if r.remaining() < requestServerLoginSize {
		return RequestServerLogin{}, fmt.Errorf("clientpackets: RequestServerLogin: need %d bytes, got %d", requestServerLoginSize, r.remaining())
	}
	return RequestServerLogin{
		SessionKey1: r.readInt32(),
		SessionKey2: r.readInt32(),
		ServerID:    r.readByte(),
	}, nil
}
