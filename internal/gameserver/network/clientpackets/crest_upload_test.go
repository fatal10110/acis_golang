package clientpackets

import (
	"bytes"
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// TestDecodeCrestUploads reads the declared length then the image; a length
// over the limit reads no image, a negative one is refused without counting
// as a short packet, and a missing image is a short packet.
func TestDecodeCrestUploads(t *testing.T) {
	image := bytes.Repeat([]byte{0xab}, 4)
	small := append([]byte{0x53, 0x04, 0x00, 0x00, 0x00}, image...)
	u, err := DecodeRequestSetPledgeCrest(small)
	if err != nil || u.Length != 4 || !bytes.Equal(u.Data, image) {
		t.Fatalf("pledge upload = %+v, %v", u, err)
	}

	large := []byte{0xd0, 0x11, 0x00, 0x00, 0x00, 0x00, 0x00}
	u, err = DecodeRequestExSetPledgeCrestLarge(large)
	if err != nil || u.Length != 0 || len(u.Data) != 0 {
		t.Fatalf("empty large upload = %+v, %v", u, err)
	}

	over := []byte{0x53, 0x01, 0x01, 0x00, 0x00} // 257
	u, err = DecodeRequestSetPledgeCrest(over)
	if err != nil || u.Length != 257 || u.Data != nil {
		t.Fatalf("over-limit upload = %+v, %v", u, err)
	}
	overLarge := []byte{0xd0, 0x11, 0x00, 0x81, 0x08, 0x00, 0x00} // 2177
	if u, err = DecodeRequestExSetPledgeCrestLarge(overLarge); err != nil || u.Length != 2177 || u.Data != nil {
		t.Fatalf("over-limit large upload = %+v, %v", u, err)
	}

	if _, err := DecodeRequestSetPledgeCrest([]byte{0x53, 0xff, 0xff, 0xff, 0xff}); err == nil || errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("negative length error = %v, want a non-short refusal", err)
	}
	if _, err := DecodeRequestSetPledgeCrest([]byte{0x53, 0x04, 0x00, 0x00, 0x00, 0x01}); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("truncated image error = %v, want ErrShortPacket", err)
	}
	if _, err := DecodeRequestSetPledgeCrest([]byte{0x53, 0x04}); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("truncated length error = %v, want ErrShortPacket", err)
	}
	if _, err := DecodeRequestExSetPledgeCrestLarge([]byte{0xd0, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00}); err == nil {
		t.Fatal("large upload decoder accepted another sub-opcode")
	}
}
