package clientpackets

import "fmt"

// OpcodeSay2 is a chat line the player says.
const OpcodeSay2 = 0x38

// say2Tell is the whisper channel, the only one whose line names a target.
const say2Tell = 2

// Say2 is a chat line: its text, the channel it is said on and, for a
// whisper, the name it is addressed to.
type Say2 struct {
	Text   string
	Type   int32
	Target string
}

// DecodeSay2 parses a raw Say2 payload (opcode byte included).
func DecodeSay2(payload []byte) (Say2, error) {
	r := newReader(payload)
	req := Say2{Text: r.ReadString()}
	req.Type = r.ReadInt32()
	if req.Type == say2Tell {
		req.Target = r.ReadString()
	}
	if err := r.Err(); err != nil {
		return Say2{}, fmt.Errorf("clientpackets: Say2: %w", err)
	}
	return req, nil
}
