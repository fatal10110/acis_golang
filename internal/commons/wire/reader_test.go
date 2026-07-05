package wire

import "testing"

func TestReaderPrimitivesRoundTripWriter(t *testing.T) {
	var w Writer
	w.WriteUint8(0x7F)
	w.WriteInt16(0xBEEF)
	w.WriteInt32(-1)
	w.WriteInt64(-2)
	w.WriteBytes([]byte{1, 2, 3})
	w.WriteString("abc")

	r := NewReader(w.Bytes())
	if got := r.ReadUint8(); got != 0x7F {
		t.Fatalf("ReadUint8() = %#x, want 0x7F", got)
	}
	if got := r.ReadInt16(); got != 0xBEEF {
		t.Fatalf("ReadInt16() = %#x, want 0xBEEF", got)
	}
	if got := r.ReadInt32(); got != -1 {
		t.Fatalf("ReadInt32() = %d, want -1", got)
	}
	if got := r.ReadInt64(); got != -2 {
		t.Fatalf("ReadInt64() = %d, want -2", got)
	}
	if got := r.ReadBytes(3); string(got) != "\x01\x02\x03" {
		t.Fatalf("ReadBytes(3) = % X, want 01 02 03", got)
	}
	if got := r.ReadString(); got != "abc" {
		t.Fatalf("ReadString() = %q, want %q", got, "abc")
	}
	if r.Err() != nil {
		t.Fatalf("Err() = %v, want nil", r.Err())
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("Remaining() = %d, want 0", rem)
	}
}

func TestReaderShortPacketSetsErrInsteadOfPanicking(t *testing.T) {
	r := NewReader([]byte{0x01})

	_ = r.ReadUint8()
	if r.Err() != nil {
		t.Fatalf("Err() after in-bounds read = %v, want nil", r.Err())
	}

	if got := r.ReadInt32(); got != 0 {
		t.Fatalf("ReadInt32() past end = %d, want 0", got)
	}
	if r.Err() != ErrShortPacket {
		t.Fatalf("Err() = %v, want %v", r.Err(), ErrShortPacket)
	}

	// Once short, every further read stays zero instead of reading
	// out-of-bounds memory or panicking.
	if got := r.ReadUint8(); got != 0 {
		t.Fatalf("ReadUint8() after short read = %d, want 0", got)
	}
}

func TestReaderReadStringWithoutTerminatorIsShort(t *testing.T) {
	r := NewReader([]byte{'a', 0}) // one code unit, no null terminator

	if got := r.ReadString(); got != "a" {
		t.Fatalf("ReadString() = %q, want %q", got, "a")
	}
	if r.Err() != ErrShortPacket {
		t.Fatalf("Err() = %v, want %v", r.Err(), ErrShortPacket)
	}
}
