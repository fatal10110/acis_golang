package serverpackets

import (
	"bytes"
	"testing"
)

// Golden bytes follow the record writer's festival page field by field:
// opcode 0xf5, page 2, period, h 1, c festival count, then per festival c
// number (from 1), d worth, and for dusk then dawn d score, c member
// count, S each member (UTF-16LE, NUL-terminated). A blank score still
// names one empty member.
func TestFrameSSQStatusFestival(t *testing.T) {
	got := framePayload(t, FrameSSQStatusFestival(1, []SSQFestival{
		{MaxScore: 60, Dusk: SSQFestivalBest{Score: 0, Members: []string{""}}, Dawn: SSQFestivalBest{Score: 7, Members: []string{"Ab", "C"}}},
		{MaxScore: 150, Dusk: SSQFestivalBest{Score: 300, Members: nil}, Dawn: SSQFestivalBest{Score: 0, Members: []string{""}}},
	}))
	want := []byte{
		0xf5, 0x02, 0x01,
		0x01, 0x00, // constant 1
		0x02,                   // two festivals
		0x01,                   // festival 1
		0x3c, 0x00, 0x00, 0x00, // worth 60
		0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, // dusk 0, one empty name
		0x07, 0x00, 0x00, 0x00, 0x02, 'A', 0x00, 'b', 0x00, 0x00, 0x00, 'C', 0x00, 0x00, 0x00, // dawn 7, Ab and C
		0x02,                   // festival 2
		0x96, 0x00, 0x00, 0x00, // worth 150
		0x2c, 0x01, 0x00, 0x00, 0x00, // dusk 300, no names
		0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, // dawn 0, one empty name
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("festival page = % x, want % x", got, want)
	}
}
