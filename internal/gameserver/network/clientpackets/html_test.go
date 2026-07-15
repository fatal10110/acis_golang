package clientpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

func TestDecodeRequestLinkHTML(t *testing.T) {
	w := wire.NewPacketWriter(OpcodeRequestLinkHtml)
	w.WriteString("help/tutorial.htm")

	got, err := DecodeRequestLinkHTML(w.Bytes())
	if err != nil {
		t.Fatalf("DecodeRequestLinkHTML: %v", err)
	}
	if got.Link != "help/tutorial.htm" {
		t.Fatalf("Link = %q, want help/tutorial.htm", got.Link)
	}
}

func TestDecodeRequestLinkHTMLShort(t *testing.T) {
	if _, err := DecodeRequestLinkHTML([]byte{OpcodeRequestLinkHtml, 'x'}); err == nil {
		t.Fatal("DecodeRequestLinkHTML: want error on unterminated string")
	}
}

func TestDecodeRequestBypassToServer(t *testing.T) {
	w := wire.NewPacketWriter(OpcodeRequestBypassToServer)
	w.WriteString("player_help tutorial.htm")

	got, err := DecodeRequestBypassToServer(w.Bytes())
	if err != nil {
		t.Fatalf("DecodeRequestBypassToServer: %v", err)
	}
	if got.Command != "player_help tutorial.htm" {
		t.Fatalf("Command = %q, want player_help tutorial.htm", got.Command)
	}
}

func TestDecodeRequestBypassToServerShort(t *testing.T) {
	if _, err := DecodeRequestBypassToServer([]byte{OpcodeRequestBypassToServer, 'x'}); err == nil {
		t.Fatal("DecodeRequestBypassToServer: want error on unterminated string")
	}
}
