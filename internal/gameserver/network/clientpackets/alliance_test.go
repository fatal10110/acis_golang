package clientpackets

import (
	"bytes"
	"testing"
)

// Reference: RequestJoinAlly and RequestAnswerJoinAlly read one D,
// AllyDismiss one S, and RequestSetAllyCrest a D length then that many
// bytes unless it exceeds 192.
func TestDecodeAlliancePackets(t *testing.T) {
	if req, err := DecodeRequestJoinAlly([]byte{0x82, 0x01, 0x00, 0x00, 0x10}); err != nil || req.TargetID != 0x10000001 {
		t.Fatalf("RequestJoinAlly = %+v, %v", req, err)
	}
	if req, err := DecodeRequestAnswerJoinAlly([]byte{0x83, 0x01, 0x00, 0x00, 0x00}); err != nil || req.Answer != 1 {
		t.Fatalf("RequestAnswerJoinAlly = %+v, %v", req, err)
	}
	if _, err := DecodeRequestAnswerJoinAlly([]byte{0x83, 0x01}); err == nil {
		t.Fatal("short RequestAnswerJoinAlly decoded")
	}
	if req, err := DecodeAllyDismiss([]byte{0x85, 'A', 0, 'b', 0, 0, 0}); err != nil || req.Name != "Ab" {
		t.Fatalf("AllyDismiss = %+v, %v", req, err)
	}
	image := bytes.Repeat([]byte{0x5a}, AllyCrestMaxLength)
	u, err := DecodeRequestSetAllyCrest(append([]byte{0x87, 0xc0, 0x00, 0x00, 0x00}, image...))
	if err != nil || u.Length != 192 || !bytes.Equal(u.Data, image) {
		t.Fatalf("alliance crest upload = %+v, %v", u, err)
	}
	if u, err := DecodeRequestSetAllyCrest([]byte{0x87, 0xc1, 0x00, 0x00, 0x00}); err != nil || u.Length != 193 || u.Data != nil {
		t.Fatalf("over-limit alliance crest upload = %+v, %v", u, err)
	}
}
