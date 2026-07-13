package clientpackets

import "fmt"

// OpcodeMoveBackwardToLocation is the wire opcode for a client click/keyboard
// movement request.
const OpcodeMoveBackwardToLocation = 0x01

const moveBackwardToLocationSize = 7 * 4

// MoveBackwardToLocation asks the server to move the active character toward
// Target from Origin. MoveMovement is 0 for keyboard movement and 1 for mouse
// movement.
type MoveBackwardToLocation struct {
	TargetX, TargetY, TargetZ int32
	OriginX, OriginY, OriginZ int32
	MoveMovement              int32
}

// DecodeMoveBackwardToLocation parses a raw movement request payload (opcode
// byte included).
func DecodeMoveBackwardToLocation(payload []byte) (MoveBackwardToLocation, error) {
	r := newReader(payload)
	if r.Remaining() < moveBackwardToLocationSize {
		return MoveBackwardToLocation{}, fmt.Errorf("clientpackets: MoveBackwardToLocation: need %d bytes, got %d", moveBackwardToLocationSize, r.Remaining())
	}
	req := MoveBackwardToLocation{
		TargetX:      r.ReadInt32(),
		TargetY:      r.ReadInt32(),
		TargetZ:      r.ReadInt32(),
		OriginX:      r.ReadInt32(),
		OriginY:      r.ReadInt32(),
		OriginZ:      r.ReadInt32(),
		MoveMovement: r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return MoveBackwardToLocation{}, fmt.Errorf("clientpackets: MoveBackwardToLocation: %w", err)
	}
	return req, nil
}
