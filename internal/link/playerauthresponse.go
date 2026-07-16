package link

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// OpcodePlayerAuthResponse is the wire opcode for PlayerAuthResponse,
// answering a game server's PlayerAuthRequest.
const OpcodePlayerAuthResponse = 0x03

// EncodePlayerAuthResponse builds the PlayerAuthResponse packet, telling
// the game server whether account's presented session keys were valid.
func EncodePlayerAuthResponse(account string, ok bool) []byte {
	w := newWriter(OpcodePlayerAuthResponse)
	w.WriteString(account)
	w.WriteUint8(wire.BoolByte(ok))
	return w.Bytes()
}

// DecodePlayerAuthResponse parses a raw PlayerAuthResponse payload (opcode
// byte included) into the account it answers for and whether its session
// keys were valid.
func DecodePlayerAuthResponse(payload []byte) (account string, ok bool, err error) {
	r := newReader(payload)
	account = r.ReadString()
	ok = r.ReadUint8() != 0
	if r.Err() != nil {
		return "", false, fmt.Errorf("link: PlayerAuthResponse: %w", r.Err())
	}
	return account, ok, nil
}
