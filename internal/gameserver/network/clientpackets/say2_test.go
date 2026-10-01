package clientpackets

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// Say2 carries the text, then the channel, then a target name only on the
// whisper channel (2).
func TestDecodeSay2(t *testing.T) {
	hi := []byte{'h', 0x00, 'i', 0x00, 0x00, 0x00}
	bo := []byte{'B', 0x00, 'o', 0x00, 0x00, 0x00}

	shout, err := DecodeSay2(append(append([]byte{OpcodeSay2}, hi...), 0x01, 0x00, 0x00, 0x00))
	if err != nil || shout != (Say2{Text: "hi", Type: 1}) {
		t.Fatalf("DecodeSay2 shout = %+v, %v; want hi on 1", shout, err)
	}
	// A name after a line on any other channel is not read.
	all, err := DecodeSay2(append(append(append([]byte{OpcodeSay2}, hi...), 0x00, 0x00, 0x00, 0x00), bo...))
	if err != nil || all != (Say2{Text: "hi"}) {
		t.Fatalf("DecodeSay2 all = %+v, %v; want hi on 0 with no target", all, err)
	}
	tell, err := DecodeSay2(append(append(append([]byte{OpcodeSay2}, hi...), 0x02, 0x00, 0x00, 0x00), bo...))
	if err != nil || tell != (Say2{Text: "hi", Type: 2, Target: "Bo"}) {
		t.Fatalf("DecodeSay2 tell = %+v, %v; want hi to Bo", tell, err)
	}
}

// A line without its channel, or a whisper without its target, is a short
// packet.
func TestDecodeSay2Short(t *testing.T) {
	hi := []byte{'h', 0x00, 'i', 0x00, 0x00, 0x00}
	if _, err := DecodeSay2(append([]byte{OpcodeSay2}, hi...)); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("DecodeSay2 without a channel: err = %v, want ErrShortPacket", err)
	}
	if _, err := DecodeSay2(append(append([]byte{OpcodeSay2}, hi...), 0x02, 0x00, 0x00, 0x00)); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("DecodeSay2 whisper without a target: err = %v, want ErrShortPacket", err)
	}
}
