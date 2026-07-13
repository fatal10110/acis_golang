package serverpackets

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestEncodeServerList(t *testing.T) {
	servers := []ServerEntry{
		{
			ID:             1,
			IP:             [4]byte{127, 0, 0, 1},
			Port:           7777,
			AgeLimit:       15,
			PvP:            true,
			CurrentPlayers: 10,
			MaxPlayers:     100,
			Online:         true,
			TestServer:     true,
			ShowClock:      true,
			ShowBrackets:   true,
		},
		{
			ID:   2,
			IP:   [4]byte{10, 0, 0, 2},
			Port: 7778,
		},
	}

	// Known-good vector: 3-byte header, then one 21-byte block per server
	// (id 1, ip 4, port 4, age 1, pvp 1, current 2, max 2, online 1,
	// flag bits 4, brackets 1).
	want, err := hex.DecodeString(
		"040201" +
			"017f000001611e00000f010a006400010600000001" +
			"020a000002621e0000000000000000000000000000")
	if err != nil {
		t.Fatalf("decode vector: %v", err)
	}

	got := EncodeServerList(1, servers)

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeServerList = %x, want %x", got, want)
	}
}
