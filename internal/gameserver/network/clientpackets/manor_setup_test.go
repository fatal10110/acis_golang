package clientpackets

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// Reference: RequestSetSeed.readImpl / RequestSetCrop.readImpl. The rows
// are 12 bytes (seed id, sales, price), 13 for a crop (and a reward byte);
// a count of 0, over the cap or not matching the bytes left, or a row with
// an id under 1 or a negative sales or price, refuses the whole list; a
// crop with no sales is left out.
func TestDecodeRequestSetSeed(t *testing.T) {
	got, err := DecodeRequestSetSeed(seedExtended(OpcodeRequestSetSeed, int32(3), int32(2),
		int32(5016), int32(10), int32(5), int32(5017), int32(0), int32(0)), 2)
	if err != nil {
		t.Fatal(err)
	}
	want := RequestSetSeed{ManorID: 3, Items: []ManorSeedSetting{{5016, 10, 5}, {5017, 0, 0}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DecodeRequestSetSeed = %+v, want %+v", got, want)
	}

	for name, payload := range map[string][]byte{
		"no rows":        seedExtended(OpcodeRequestSetSeed, int32(1), int32(0)),
		"over the cap":   seedExtended(OpcodeRequestSetSeed, int32(1), int32(3), int32(5016), int32(1), int32(1), int32(5016), int32(1), int32(1), int32(5016), int32(1), int32(1)),
		"trailing bytes": seedExtended(OpcodeRequestSetSeed, int32(1), int32(1), int32(5016), int32(1), int32(1), byte(0)),
		"no item":        seedExtended(OpcodeRequestSetSeed, int32(1), int32(1), int32(0), int32(1), int32(1)),
		"negative sales": seedExtended(OpcodeRequestSetSeed, int32(1), int32(1), int32(5016), int32(-1), int32(1)),
		"negative price": seedExtended(OpcodeRequestSetSeed, int32(1), int32(1), int32(5016), int32(1), int32(-1)),
	} {
		if _, err := DecodeRequestSetSeed(payload, 2); err == nil || errors.Is(err, wire.ErrShortPacket) {
			t.Errorf("%s: err = %v, want a refusal that is no underflow", name, err)
		}
	}
	if _, err := DecodeRequestSetSeed(seedExtended(OpcodeRequestSetSeed, int32(1)), 2); !errors.Is(err, wire.ErrShortPacket) {
		t.Errorf("short header: err = %v, want ErrShortPacket", err)
	}
}

func TestDecodeRequestSetCrop(t *testing.T) {
	got, err := DecodeRequestSetCrop(seedExtended(OpcodeRequestSetCrop, int32(1), int32(2),
		int32(5073), int32(0), int32(50), byte(1), int32(5068), int32(100), int32(40), byte(2)), 2)
	if err != nil {
		t.Fatal(err)
	}
	want := RequestSetCrop{ManorID: 1, Items: []ManorCropSetting{{CropID: 5068, Sales: 100, Price: 40, Reward: 2}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DecodeRequestSetCrop = %+v, want %+v", got, want)
	}

	for name, payload := range map[string][]byte{
		"seed-sized row": seedExtended(OpcodeRequestSetCrop, int32(1), int32(1), int32(5073), int32(1), int32(1)),
		"no item":        seedExtended(OpcodeRequestSetCrop, int32(1), int32(1), int32(0), int32(1), int32(1), byte(1)),
		"negative price after a crop": seedExtended(OpcodeRequestSetCrop, int32(1), int32(2),
			int32(5073), int32(1), int32(1), byte(1), int32(5068), int32(1), int32(-1), byte(1)),
	} {
		if _, err := DecodeRequestSetCrop(payload, 2); err == nil {
			t.Errorf("%s: decoded, want a refusal", name)
		}
	}
}
