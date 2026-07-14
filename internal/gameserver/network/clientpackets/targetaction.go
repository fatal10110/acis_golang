package clientpackets

import "fmt"

const targetActionSize = 4*4 + 1

// Action is the normal client click/action request against a world object.
type Action struct {
	ObjectID         int32
	OriginX, OriginY int32
	OriginZ          int32
	Shift            bool
}

// DecodeAction parses a raw Action payload (opcode byte included).
func DecodeAction(payload []byte) (Action, error) {
	req, err := decodeTargetAction(payload, "Action")
	return Action(req), err
}

// AttackRequest is the client physical-attack request against a world object.
type AttackRequest struct {
	ObjectID         int32
	OriginX, OriginY int32
	OriginZ          int32
	Shift            bool
}

// DecodeAttackRequest parses a raw AttackRequest payload (opcode byte included).
func DecodeAttackRequest(payload []byte) (AttackRequest, error) {
	req, err := decodeTargetAction(payload, "AttackRequest")
	return AttackRequest(req), err
}

type targetAction struct {
	ObjectID         int32
	OriginX, OriginY int32
	OriginZ          int32
	Shift            bool
}

func decodeTargetAction(payload []byte, name string) (targetAction, error) {
	r := newReader(payload)
	if r.Remaining() < targetActionSize {
		return targetAction{}, fmt.Errorf("clientpackets: %s: need %d bytes, got %d", name, targetActionSize, r.Remaining())
	}
	req := targetAction{
		ObjectID: r.ReadInt32(),
		OriginX:  r.ReadInt32(),
		OriginY:  r.ReadInt32(),
		OriginZ:  r.ReadInt32(),
		Shift:    r.ReadUint8() != 0,
	}
	if err := r.Err(); err != nil {
		return targetAction{}, fmt.Errorf("clientpackets: %s: %w", name, err)
	}
	return req, nil
}

// RequestTargetCancel asks the server to clear the current target.
type RequestTargetCancel struct {
	Unselect uint16
}

// DecodeRequestTargetCancel parses a raw RequestTargetCancel payload (opcode
// byte included).
func DecodeRequestTargetCancel(payload []byte) (RequestTargetCancel, error) {
	r := newReader(payload)
	if r.Remaining() < 2 {
		return RequestTargetCancel{}, fmt.Errorf("clientpackets: RequestTargetCancel: need 2 bytes, got %d", r.Remaining())
	}
	req := RequestTargetCancel{Unselect: r.ReadUint16()}
	if err := r.Err(); err != nil {
		return RequestTargetCancel{}, fmt.Errorf("clientpackets: RequestTargetCancel: %w", err)
	}
	return req, nil
}
