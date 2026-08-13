package clientpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

func TestDecodeRequestChangePetName(t *testing.T) {
	w := wire.NewPacketWriter(OpcodeRequestChangePetName)
	w.WriteString("Rex")

	got, err := DecodeRequestChangePetName(w.Bytes())
	if err != nil {
		t.Fatalf("DecodeRequestChangePetName: %v", err)
	}
	if got.Name != "Rex" {
		t.Fatalf("Name = %q, want Rex", got.Name)
	}
}

func TestDecodeRequestChangePetNameShort(t *testing.T) {
	if _, err := DecodeRequestChangePetName([]byte{OpcodeRequestChangePetName, 'x'}); err == nil {
		t.Fatal("DecodeRequestChangePetName: want error on unterminated string")
	}
}
