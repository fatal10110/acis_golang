package serverpackets

import (
	"bytes"
	"encoding/binary"
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

	got := EncodeServerList(1, servers)

	var want []byte
	want = append(want, OpcodeServerList)
	want = append(want, byte(len(servers)), 1)
	for i, s := range servers {
		want = append(want, s.ID)
		want = append(want, s.IP[:]...)
		want = binary.LittleEndian.AppendUint32(want, uint32(s.Port))
		want = append(want, s.AgeLimit)
		if s.PvP {
			want = append(want, 1)
		} else {
			want = append(want, 0)
		}
		want = binary.LittleEndian.AppendUint16(want, s.CurrentPlayers)
		want = binary.LittleEndian.AppendUint16(want, s.MaxPlayers)
		if s.Online {
			want = append(want, 1)
		} else {
			want = append(want, 0)
		}

		var bits uint32
		if i == 0 {
			bits = 0x04 | 0x02
		}
		want = binary.LittleEndian.AppendUint32(want, bits)

		if s.ShowBrackets {
			want = append(want, 1)
		} else {
			want = append(want, 0)
		}
	}

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeServerList = %x, want %x", got, want)
	}
}
