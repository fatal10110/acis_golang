package clientpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// TestDecodeSendBypassBuildCmd reads the "//command" text as sent, its
// surrounding blanks kept for the handler to trim.
func TestDecodeSendBypassBuildCmd(t *testing.T) {
	w := wire.NewPacketWriter(OpcodeSendBypassBuildCmd)
	w.WriteString(" teleport 1 2 3 ")

	got, err := DecodeSendBypassBuildCmd(w.Bytes())
	if err != nil {
		t.Fatalf("DecodeSendBypassBuildCmd: %v", err)
	}
	if got.Command != " teleport 1 2 3 " {
		t.Fatalf("Command = %q, want %q", got.Command, " teleport 1 2 3 ")
	}
	if _, err := DecodeSendBypassBuildCmd([]byte{OpcodeSendBypassBuildCmd, 'x'}); err == nil {
		t.Fatal("DecodeSendBypassBuildCmd: want error on unterminated string")
	}
}
