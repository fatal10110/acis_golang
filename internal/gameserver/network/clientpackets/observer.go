package clientpackets

import "fmt"

// OpcodeObserverReturn asks to leave observer mode.
const OpcodeObserverReturn = 0xb8

// ObserverReturn asks to stop watching from a viewpoint and return to the
// position left. It carries no fields beyond the opcode.
type ObserverReturn struct{}

// DecodeObserverReturn parses a raw ObserverReturn payload (opcode byte
// included).
func DecodeObserverReturn(payload []byte) (ObserverReturn, error) {
	r := newReader(payload)
	if err := r.Err(); err != nil {
		return ObserverReturn{}, fmt.Errorf("clientpackets: ObserverReturn: %w", err)
	}
	return ObserverReturn{}, nil
}
