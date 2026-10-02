package clientpackets

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// The payloads follow the duel requests' readImpl in the reference:
// RequestDuelStart reads the name then the party flag; RequestDuelAnswerStart
// the party flag, an unused int and the answer.
func TestDecodeDuelRequests(t *testing.T) {
	start := []byte{OpcodeExtended, 0x27, 0x00, 'A', 0, 'b', 0, 0, 0, 1, 0, 0, 0}
	req, err := DecodeRequestDuelStart(start)
	if err != nil || req.Target != "Ab" || !req.Party {
		t.Fatalf("RequestDuelStart = %+v, %v; want Ab, party", req, err)
	}
	if _, err := DecodeRequestDuelStart(start[:len(start)-1]); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("short RequestDuelStart error = %v, want ErrShortPacket", err)
	}

	answer := []byte{OpcodeExtended, 0x28, 0x00, 0, 0, 0, 0, 9, 9, 9, 9, 1, 0, 0, 0}
	ans, err := DecodeRequestDuelAnswerStart(answer)
	if err != nil || ans.Party || !ans.Accepted {
		t.Fatalf("RequestDuelAnswerStart = %+v, %v; want one on one, accepted", ans, err)
	}
	answer[3], answer[11] = 1, 0
	if ans, err = DecodeRequestDuelAnswerStart(answer); err != nil || !ans.Party || ans.Accepted {
		t.Fatalf("RequestDuelAnswerStart = %+v, %v; want party, declined", ans, err)
	}
	if _, err := DecodeRequestDuelAnswerStart(answer[:len(answer)-1]); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("short RequestDuelAnswerStart error = %v, want ErrShortPacket", err)
	}
}
