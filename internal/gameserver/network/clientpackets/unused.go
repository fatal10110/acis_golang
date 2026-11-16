package clientpackets

import (
	"errors"
	"fmt"
)

// Opcodes of requests that ask the server for nothing. The client sends
// them, so they are accepted, but nothing a client can observe follows.
const (
	// OpcodeRequestDismissParty and OpcodeMessengerDialog build no request
	// at all: nothing reads their bodies and nothing runs for them.
	OpcodeRequestDismissParty = 0x2d
	OpcodeMessengerDialog     = 0xce
	// OpcodeSnoopQuit reads one int32 and runs nothing.
	OpcodeSnoopQuit = 0xab
	// OpcodeRequestPCCafeCouponUse reads one string and runs nothing.
	OpcodeRequestPCCafeCouponUse uint16 = 0x0020
)

// ErrDiscarded marks a request that was read in full but whose contents are
// refused before anything is done with them: the frame counts as read, so the
// player acted, yet the request is dropped without an answer.
var ErrDiscarded = errors.New("request discarded")

// discarded marks err, a refusal of a request's contents, as ErrDiscarded.
func discarded(err error) error {
	return fmt.Errorf("%w: %w", err, ErrDiscarded)
}

// DecodeSnoopQuit reads a raw SnoopQuit payload (opcode byte included): one
// int32 nothing uses.
func DecodeSnoopQuit(payload []byte) (struct{}, error) {
	r := newReader(payload)
	r.ReadInt32()
	if err := r.Err(); err != nil {
		return struct{}{}, fmt.Errorf("clientpackets: SnoopQuit: %w", err)
	}
	return struct{}{}, nil
}

// DecodeRequestPCCafeCouponUse reads a raw RequestPCCafeCouponUse payload
// (extended opcode included): one string nothing uses.
func DecodeRequestPCCafeCouponUse(payload []byte) (struct{}, error) {
	const name = "RequestPCCafeCouponUse"
	r, err := newExtendedReader(payload, name, OpcodeRequestPCCafeCouponUse, 2)
	if err != nil {
		return struct{}{}, err
	}
	r.ReadString()
	if err := r.Err(); err != nil {
		return struct{}{}, fmt.Errorf("clientpackets: %s: %w", name, err)
	}
	return struct{}{}, nil
}
