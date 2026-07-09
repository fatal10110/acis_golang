package wire

import (
	"bytes"
	"io"
	"testing"
)

func TestFrameReaderReadsSequentialFrames(t *testing.T) {
	var stream []byte
	stream = append(stream, FrameBytes([]byte{1, 2, 3})...)
	stream = append(stream, FrameBytes([]byte{4, 5})...)

	fr := NewFrameReader(bytes.NewReader(stream))

	first, err := fr.ReadFrame()
	if err != nil {
		t.Fatalf("first ReadFrame: %v", err)
	}
	if !bytes.Equal(first, []byte{1, 2, 3}) {
		t.Fatalf("first payload = % X, want 01 02 03", first)
	}

	second, err := fr.ReadFrame()
	if err != nil {
		t.Fatalf("second ReadFrame: %v", err)
	}
	if !bytes.Equal(second, []byte{4, 5}) {
		t.Fatalf("second payload = % X, want 04 05", second)
	}

	if _, err := fr.ReadFrame(); err != io.EOF {
		t.Fatalf("ReadFrame at end = %v, want io.EOF", err)
	}
}

func TestFrameReaderRejectsHeaderShorterThanItself(t *testing.T) {
	fr := NewFrameReader(bytes.NewReader([]byte{0x01, 0x00}))
	if _, err := fr.ReadFrame(); err == nil {
		t.Fatal("ReadFrame() err = nil, want an error for a length shorter than the header")
	}
}

func TestFrameReaderReusesItsBuffer(t *testing.T) {
	var stream []byte
	stream = append(stream, FrameBytes([]byte{1, 2, 3})...)
	stream = append(stream, FrameBytes([]byte{9, 9, 9})...)

	r := bytes.NewReader(stream)
	fr := NewFrameReader(r)

	// Warm the buffer past both frames, then measure a steady-state pass.
	for {
		if _, err := fr.ReadFrame(); err != nil {
			break
		}
	}

	allocs := testing.AllocsPerRun(100, func() {
		if _, err := r.Seek(0, io.SeekStart); err != nil {
			t.Fatalf("seek: %v", err)
		}
		for {
			if _, err := fr.ReadFrame(); err != nil {
				return
			}
		}
	})
	if allocs != 0 {
		t.Errorf("steady-state allocations per pass = %v, want 0", allocs)
	}
}

func BenchmarkReadFrame(b *testing.B) {
	stream := FrameBytes(make([]byte, 64))
	r := bytes.NewReader(stream)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := r.Seek(0, io.SeekStart); err != nil {
			b.Fatalf("seek: %v", err)
		}
		if _, err := ReadFrame(r); err != nil {
			b.Fatalf("ReadFrame: %v", err)
		}
	}
}

func BenchmarkFrameReader(b *testing.B) {
	stream := FrameBytes(make([]byte, 64))
	r := bytes.NewReader(stream)
	fr := NewFrameReader(r)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := r.Seek(0, io.SeekStart); err != nil {
			b.Fatalf("seek: %v", err)
		}
		if _, err := fr.ReadFrame(); err != nil {
			b.Fatalf("ReadFrame: %v", err)
		}
	}
}
