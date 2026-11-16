package clientpackets

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

func rowFrame(head []byte, fields ...int32) []byte {
	out := append([]byte(nil), head...)
	for _, v := range fields {
		out = binary.LittleEndian.AppendUint32(out, uint32(v))
	}
	return out
}

// TestDecodeDiscardsReadRequests pins which refusals read the request in
// full (ErrDiscarded: the player acted) and which leave it unread: a short
// frame, and a seed or crop setting whose row count is refused.
func TestDecodeDiscardsReadRequests(t *testing.T) {
	const maxItems = 3
	capped := func(decode func([]byte, int) error) func([]byte) error {
		return func(p []byte) error { return decode(p, maxItems) }
	}
	buy := capped(func(p []byte, n int) error { _, err := DecodeRequestBuyItem(p, n); return err })
	sell := capped(func(p []byte, n int) error { _, err := DecodeRequestSellItem(p, n); return err })
	deposit := capped(func(p []byte, n int) error { _, err := DecodeSendWarehouseDepositList(p, n); return err })
	withdraw := capped(func(p []byte, n int) error { _, err := DecodeSendWarehouseWithdrawList(p, n); return err })
	pkg := capped(func(p []byte, n int) error { _, err := DecodeRequestPackageSend(p, n); return err })
	seed := capped(func(p []byte, n int) error { _, err := DecodeRequestSetSeed(p, n); return err })
	crop := capped(func(p []byte, n int) error { _, err := DecodeRequestSetCrop(p, n); return err })
	preview := func(p []byte) error { _, err := DecodeRequestPreviewItem(p); return err }
	seedHead := []byte{OpcodeExtended, byte(OpcodeRequestSetSeed), 0}
	cropHead := []byte{OpcodeExtended, byte(OpcodeRequestSetCrop), 0}

	for _, tc := range []struct {
		name      string
		decode    func([]byte) error
		payload   []byte
		discarded bool
	}{
		{"buy item count 0", buy, rowFrame([]byte{OpcodeRequestBuyItem}, 1, 0), true},
		{"buy item over cap", buy, rowFrame([]byte{OpcodeRequestBuyItem}, 1, maxItems+1), true},
		{"buy item without rows", buy, rowFrame([]byte{OpcodeRequestBuyItem}, 1, 1), true},
		{"buy item naming no item", buy, rowFrame([]byte{OpcodeRequestBuyItem}, 1, 1, 0, 1), true},
		{"buy item short", buy, []byte{OpcodeRequestBuyItem, 0, 0, 0}, false},
		{"sell item without rows", sell, rowFrame([]byte{OpcodeRequestSellItem}, 1, 1), true},
		{"sell item count 0 object", sell, rowFrame([]byte{OpcodeRequestSellItem}, 1, 1, 0, 57, 1), true},
		{"preview of 101", preview, rowFrame([]byte{OpcodeRequestPreviewItem}, 0, 1, 101), true},
		{"preview rows short", preview, rowFrame([]byte{OpcodeRequestPreviewItem}, 0, 1, 2, 57), false},
		{"deposit count 0", deposit, rowFrame([]byte{OpcodeSendWarehouseDeposit}, 0), true},
		{"deposit naming no object", deposit, rowFrame([]byte{OpcodeSendWarehouseDeposit}, 1, 0, 1), true},
		{"withdraw without rows", withdraw, rowFrame([]byte{OpcodeSendWarehouseWithdraw}, 1), true},
		{"withdraw short", withdraw, []byte{OpcodeSendWarehouseWithdraw, 1}, false},
		{"package send of -1", pkg, rowFrame([]byte{OpcodeRequestPackageSend}, 1, -1), true},
		{"package send over cap", pkg, rowFrame([]byte{OpcodeRequestPackageSend}, 1, maxItems+1), true},
		{"package send rows short", pkg, rowFrame([]byte{OpcodeRequestPackageSend}, 1, 1), false},
		{"seed naming no id", seed, rowFrame(seedHead, 1, 1, 0, 0, 0), true},
		{"seed named twice", seed, rowFrame(seedHead, 1, 2, 5016, 1, 1, 5016, 1, 1), true},
		{"seed count 0", seed, rowFrame(seedHead, 1, 0), false},
		{"seed length mismatch", seed, rowFrame(seedHead, 1, 1, 5016), false},
		{"crop naming no id", crop, append(rowFrame(cropHead, 1, 1, 0, 0, 0), 1), true},
		{"crop count 0", crop, rowFrame(cropHead, 1, 0), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.decode(tc.payload)
			if err == nil {
				t.Fatal("decode accepted the request")
			}
			if got := errors.Is(err, ErrDiscarded); got != tc.discarded {
				t.Errorf("discarded = %v (%v), want %v", got, err, tc.discarded)
			}
		})
	}
}

// TestDecodeIgnoredBodies pins the two requests whose single field is read
// and ignored: a short body is an underflow, a whole one decodes.
func TestDecodeIgnoredBodies(t *testing.T) {
	coupon := []byte{OpcodeExtended, byte(OpcodeRequestPCCafeCouponUse), 0}
	for _, tc := range []struct {
		name    string
		decode  func([]byte) (struct{}, error)
		payload []byte
		short   bool
	}{
		{"snoop quit", DecodeSnoopQuit, []byte{OpcodeSnoopQuit, 0, 0, 0, 0}, false},
		{"snoop quit short", DecodeSnoopQuit, []byte{OpcodeSnoopQuit, 0, 0, 0}, true},
		{"coupon empty string", DecodeRequestPCCafeCouponUse, append(coupon, 0, 0), false},
		{"coupon string", DecodeRequestPCCafeCouponUse, append(coupon, 'a', 0, 0, 0), false},
		{"coupon unterminated", DecodeRequestPCCafeCouponUse, append(coupon, 'a', 0), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.decode(tc.payload)
			if tc.short != errors.Is(err, wire.ErrShortPacket) || (!tc.short && err != nil) {
				t.Errorf("err = %v, want short %v", err, tc.short)
			}
		})
	}
}
