package clientpackets

import "fmt"

// OpcodeRequestServerList is the wire opcode for RequestServerList, valid
// once the client has authenticated its login/password.
const OpcodeRequestServerList = 0x05

const requestServerListSize = 8

// RequestServerList asks for the game server list, presenting back the
// session key halves the client received in LoginOk.
type RequestServerList struct {
	SessionKey1 int32
	SessionKey2 int32
}

// DecodeRequestServerList parses a raw RequestServerList payload (opcode
// byte included).
func DecodeRequestServerList(payload []byte) (RequestServerList, error) {
	r := newReader(payload)
	if r.Remaining() < requestServerListSize {
		return RequestServerList{}, fmt.Errorf("clientpackets: RequestServerList: need %d bytes, got %d", requestServerListSize, r.Remaining())
	}
	return RequestServerList{
		SessionKey1: r.ReadInt32(),
		SessionKey2: r.ReadInt32(),
	}, nil
}
