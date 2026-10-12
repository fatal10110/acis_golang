package clientpackets

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// Reference: RequestBuySeed.readImpl. Rows are 8 bytes (seed id, count); a
// count of 0, over the cap or not matching the bytes left, or a row with an
// id or count under 1, leaves the list unset while the request still runs.
func TestDecodeRequestBuySeed(t *testing.T) {
	got, err := DecodeRequestBuySeed(seedPacket(OpcodeRequestBuySeed, int32(1), int32(2), int32(5016), int32(3), int32(5012), int32(1)), 2)
	if err != nil {
		t.Fatal(err)
	}
	want := RequestBuySeed{ManorID: 1, Items: []SeedOrder{{5016, 3}, {5012, 1}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DecodeRequestBuySeed = %+v, want %+v", got, want)
	}

	for name, payload := range map[string][]byte{
		"no rows":        seedPacket(OpcodeRequestBuySeed, int32(1), int32(0)),
		"over the cap":   seedPacket(OpcodeRequestBuySeed, int32(1), int32(3), int32(5016), int32(1), int32(5016), int32(1), int32(5016), int32(1)),
		"trailing bytes": seedPacket(OpcodeRequestBuySeed, int32(1), int32(1), int32(5016), int32(1), byte(0)),
		"no seed":        seedPacket(OpcodeRequestBuySeed, int32(1), int32(1), int32(0), int32(1)),
		"no count":       seedPacket(OpcodeRequestBuySeed, int32(1), int32(2), int32(5016), int32(1), int32(5012), int32(0)),
	} {
		got, err := DecodeRequestBuySeed(payload, 2)
		if err != nil || got.ManorID != 1 || got.Items != nil {
			t.Errorf("%s: = %+v, %v; want manor 1 with no list", name, got, err)
		}
	}
	if _, err := DecodeRequestBuySeed(seedPacket(OpcodeRequestBuySeed, int32(1)), 2); !errors.Is(err, wire.ErrShortPacket) {
		t.Errorf("short header: err = %v, want ErrShortPacket", err)
	}
}

// Reference: RequestProcureCropList.readImpl. Rows are 16 bytes (object
// id, crop id, manor id, count); a count of 0 sells nothing yet passes, a
// negative one or manor id, or an id under 1, leaves the list unset.
func TestDecodeRequestProcureCropList(t *testing.T) {
	got, err := DecodeRequestProcureCropList(seedExtended(OpcodeRequestProcureCropList, int32(2),
		int32(268), int32(5073), int32(1), int32(5), int32(269), int32(5068), int32(0), int32(0)), 2)
	if err != nil {
		t.Fatal(err)
	}
	want := RequestProcureCropList{Items: []CropSale{{268, 5073, 1, 5}, {269, 5068, 0, 0}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DecodeRequestProcureCropList = %+v, want %+v", got, want)
	}

	for name, payload := range map[string][]byte{
		"no rows":        seedExtended(OpcodeRequestProcureCropList, int32(0)),
		"seed-sized row": seedExtended(OpcodeRequestProcureCropList, int32(1), int32(268), int32(5073)),
		"no object":      seedExtended(OpcodeRequestProcureCropList, int32(1), int32(0), int32(5073), int32(1), int32(1)),
		"no crop":        seedExtended(OpcodeRequestProcureCropList, int32(1), int32(268), int32(0), int32(1), int32(1)),
		"negative manor": seedExtended(OpcodeRequestProcureCropList, int32(1), int32(268), int32(5073), int32(-1), int32(1)),
		"negative count": seedExtended(OpcodeRequestProcureCropList, int32(1), int32(268), int32(5073), int32(1), int32(-1)),
		"over the cap":   seedExtended(OpcodeRequestProcureCropList, int32(3)),
	} {
		got, err := DecodeRequestProcureCropList(payload, 2)
		if err != nil || got.Items != nil {
			t.Errorf("%s: = %+v, %v; want no list", name, got, err)
		}
	}
	if _, err := DecodeRequestProcureCropList(seedExtended(OpcodeRequestProcureCropList), 2); !errors.Is(err, wire.ErrShortPacket) {
		t.Errorf("no count: err = %v, want ErrShortPacket", err)
	}
}

// Reference: RequestBuyProcure.readImpl checks the count against 8-byte
// rows, then reads 12 bytes a row: a single row always runs out of bytes,
// and two rows run out on the second unless the first is refused.
func TestDecodeRequestBuyProcure(t *testing.T) {
	for name, payload := range map[string][]byte{
		"one row":  seedPacket(OpcodeRequestBuyProcure, int32(1), int32(1), int32(0), int32(5073)),
		"two rows": seedPacket(OpcodeRequestBuyProcure, int32(1), int32(2), int32(0), int32(5073), int32(1), int32(0)),
	} {
		if _, err := DecodeRequestBuyProcure(payload, 2); !errors.Is(err, wire.ErrShortPacket) {
			t.Errorf("%s: err = %v, want ErrShortPacket", name, err)
		}
	}
	for name, payload := range map[string][]byte{
		"no rows":         seedPacket(OpcodeRequestBuyProcure, int32(1), int32(0)),
		"12-byte row":     seedPacket(OpcodeRequestBuyProcure, int32(1), int32(1), int32(0), int32(5073), int32(1)),
		"first row no id": seedPacket(OpcodeRequestBuyProcure, int32(1), int32(2), int32(0), int32(0), int32(1), int32(0)),
	} {
		if _, err := DecodeRequestBuyProcure(payload, 2); err == nil || errors.Is(err, wire.ErrShortPacket) {
			t.Errorf("%s: err = %v, want a refusal that is no underflow", name, err)
		}
	}
}
