package clientpackets

import "fmt"

// Appearing tells the server the client finished the post-teleport appearing
// sequence. The Interlude packet carries no fields beyond the opcode.
type Appearing struct{}

// DecodeAppearing parses a raw Appearing payload (opcode byte included).
func DecodeAppearing(payload []byte) (Appearing, error) {
	r := newReader(payload)
	if err := r.Err(); err != nil {
		return Appearing{}, fmt.Errorf("clientpackets: Appearing: %w", err)
	}
	return Appearing{}, nil
}
