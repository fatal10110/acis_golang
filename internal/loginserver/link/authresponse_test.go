package link

import (
	"bytes"
	"testing"
)

func TestEncodeAuthResponse(t *testing.T) {
	got := EncodeAuthResponse(3, "MyServer")
	want := appendString([]byte{OpcodeAuthResponse, 3}, "MyServer")
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeAuthResponse() = %x, want %x", got, want)
	}
}
