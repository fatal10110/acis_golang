package clientpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// RequestGetOnVehicle asks to board boat BoatID, standing at deck point X,
// Y, Z.
type RequestGetOnVehicle struct {
	BoatID  int32
	X, Y, Z int32
}

// RequestGetOffVehicle asks to leave boat BoatID toward world point X, Y, Z.
type RequestGetOffVehicle struct {
	BoatID  int32
	X, Y, Z int32
}

// RequestMoveToLocationInVehicle asks to walk across boat BoatID's deck from
// OriginX, OriginY, OriginZ to TargetX, TargetY, TargetZ, both in the
// boat's coordinates.
type RequestMoveToLocationInVehicle struct {
	BoatID                    int32
	TargetX, TargetY, TargetZ int32
	OriginX, OriginY, OriginZ int32
}

// CannotMoveAnymoreInVehicle reports where on boat BoatID's deck the
// client's walk stopped, and the heading it faces.
type CannotMoveAnymoreInVehicle struct {
	BoatID  int32
	X, Y, Z int32
	Heading int32
}

// DecodeRequestGetOnVehicle parses a raw RequestGetOnVehicle payload
// (opcode byte included).
func DecodeRequestGetOnVehicle(payload []byte) (RequestGetOnVehicle, error) {
	v, err := decodeInt32s(payload, "RequestGetOnVehicle", 4)
	if err != nil {
		return RequestGetOnVehicle{}, err
	}
	return RequestGetOnVehicle{BoatID: v[0], X: v[1], Y: v[2], Z: v[3]}, nil
}

// DecodeRequestGetOffVehicle parses a raw RequestGetOffVehicle payload
// (opcode byte included).
func DecodeRequestGetOffVehicle(payload []byte) (RequestGetOffVehicle, error) {
	v, err := decodeInt32s(payload, "RequestGetOffVehicle", 4)
	if err != nil {
		return RequestGetOffVehicle{}, err
	}
	return RequestGetOffVehicle{BoatID: v[0], X: v[1], Y: v[2], Z: v[3]}, nil
}

// DecodeRequestMoveToLocationInVehicle parses a raw
// RequestMoveToLocationInVehicle payload (opcode byte included).
func DecodeRequestMoveToLocationInVehicle(payload []byte) (RequestMoveToLocationInVehicle, error) {
	v, err := decodeInt32s(payload, "RequestMoveToLocationInVehicle", 7)
	if err != nil {
		return RequestMoveToLocationInVehicle{}, err
	}
	return RequestMoveToLocationInVehicle{
		BoatID:  v[0],
		TargetX: v[1], TargetY: v[2], TargetZ: v[3],
		OriginX: v[4], OriginY: v[5], OriginZ: v[6],
	}, nil
}

// DecodeCannotMoveAnymoreInVehicle parses a raw CannotMoveAnymoreInVehicle
// payload (opcode byte included).
func DecodeCannotMoveAnymoreInVehicle(payload []byte) (CannotMoveAnymoreInVehicle, error) {
	v, err := decodeInt32s(payload, "CannotMoveAnymoreInVehicle", 5)
	if err != nil {
		return CannotMoveAnymoreInVehicle{}, err
	}
	return CannotMoveAnymoreInVehicle{BoatID: v[0], X: v[1], Y: v[2], Z: v[3], Heading: v[4]}, nil
}

// decodeInt32s reads n int32 fields after the opcode byte of payload.
func decodeInt32s(payload []byte, name string, n int) ([]int32, error) {
	r := newReader(payload)
	if r.Remaining() < 4*n {
		return nil, fmt.Errorf("clientpackets: %s: need %d bytes, got %d: %w", name, 4*n, r.Remaining(), wire.ErrShortPacket)
	}
	out := make([]int32, n)
	for i := range out {
		out[i] = r.ReadInt32()
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("clientpackets: %s: %w", name, err)
	}
	return out, nil
}
