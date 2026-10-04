package clientpackets

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// The payloads below follow the reference readImpl field order
// (RequestGetOnVehicle.java, RequestGetOffVehicle.java,
// RequestMoveToLocationInVehicle.java, CannotMoveAnymoreInVehicle.java):
// every field a little-endian d.

func TestDecodeRequestGetOnVehicle(t *testing.T) {
	got, err := DecodeRequestGetOnVehicle(vehiclePacket(OpcodeRequestGetOnVehicle, 7, 230, -260, -40))
	if err != nil || got != (RequestGetOnVehicle{BoatID: 7, X: 230, Y: -260, Z: -40}) {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := DecodeRequestGetOnVehicle([]byte{OpcodeRequestGetOnVehicle, 1, 2}); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("short payload: %v", err)
	}
}

func TestDecodeRequestGetOffVehicle(t *testing.T) {
	got, err := DecodeRequestGetOffVehicle(vehiclePacket(OpcodeRequestGetOffVehicle, 7, 34600, -38100, -3610))
	if err != nil || got != (RequestGetOffVehicle{BoatID: 7, X: 34600, Y: -38100, Z: -3610}) {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := DecodeRequestGetOffVehicle([]byte{OpcodeRequestGetOffVehicle}); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("short payload: %v", err)
	}
}

func TestDecodeRequestMoveToLocationInVehicle(t *testing.T) {
	got, err := DecodeRequestMoveToLocationInVehicle(vehiclePacket(OpcodeRequestMoveInVehicle, 7, 100, -200, -40, 0, -100, -38))
	want := RequestMoveToLocationInVehicle{BoatID: 7, TargetX: 100, TargetY: -200, TargetZ: -40, OriginX: 0, OriginY: -100, OriginZ: -38}
	if err != nil || got != want {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := DecodeRequestMoveToLocationInVehicle(vehiclePacket(OpcodeRequestMoveInVehicle, 7, 100, -200, -40, 0, -100)); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("short payload: %v", err)
	}
}

func TestDecodeCannotMoveAnymoreInVehicle(t *testing.T) {
	got, err := DecodeCannotMoveAnymoreInVehicle(vehiclePacket(OpcodeCannotMoveInVehicle, 7, 50, -150, -40, 16384))
	if err != nil || got != (CannotMoveAnymoreInVehicle{BoatID: 7, X: 50, Y: -150, Z: -40, Heading: 16384}) {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := DecodeCannotMoveAnymoreInVehicle(vehiclePacket(OpcodeCannotMoveInVehicle, 7, 50, -150, -40)); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("short payload: %v", err)
	}
}

func vehiclePacket(opcode byte, fields ...int32) []byte {
	w := wire.NewPacketWriter(opcode)
	for _, v := range fields {
		w.WriteInt32(v)
	}
	return w.Bytes()
}
