package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

func rootTemplate(id, str, dex, con, intl, wit, men int) *player.Template {
	return &player.Template{
		ID: id, BaseLevel: 1,
		STR: str, DEX: dex, CON: con, INT: intl, WIT: wit, MEN: men,
	}
}

func allRootTemplates(t *testing.T) *player.TemplateTable {
	t.Helper()
	templates := map[int]*player.Template{
		0:  rootTemplate(0, 40, 30, 43, 21, 11, 25),
		10: rootTemplate(10, 21, 22, 23, 24, 25, 26),
		18: rootTemplate(18, 1, 2, 3, 4, 5, 6),
		25: rootTemplate(25, 1, 2, 3, 4, 5, 6),
		31: rootTemplate(31, 1, 2, 3, 4, 5, 6),
		38: rootTemplate(38, 1, 2, 3, 4, 5, 6),
		44: rootTemplate(44, 1, 2, 3, 4, 5, 6),
		49: rootTemplate(49, 1, 2, 3, 4, 5, 6),
		53: rootTemplate(53, 1, 2, 3, 4, 5, 6),
	}
	table, err := player.NewTemplateTable(templates)
	if err != nil {
		t.Fatalf("build template table: %v", err)
	}
	return table
}

func TestFrameNewCharacterSuccess(t *testing.T) {
	frame, err := FrameNewCharacterSuccess(allRootTemplates(t))
	if err != nil {
		t.Fatalf("FrameNewCharacterSuccess: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeNewCharacterSuccess}
	want = binary.LittleEndian.AppendUint32(want, uint32(len(creationScreenClassIDs)))
	for _, id := range creationScreenClassIDs {
		race, _ := player.ClassRace(id)
		want = binary.LittleEndian.AppendUint32(want, uint32(race))
		want = binary.LittleEndian.AppendUint32(want, uint32(id))

		tmpl := map[int][6]int{
			0:  {40, 30, 43, 21, 11, 25},
			10: {21, 22, 23, 24, 25, 26},
			18: {1, 2, 3, 4, 5, 6},
			25: {1, 2, 3, 4, 5, 6},
			31: {1, 2, 3, 4, 5, 6},
			38: {1, 2, 3, 4, 5, 6},
			44: {1, 2, 3, 4, 5, 6},
			49: {1, 2, 3, 4, 5, 6},
			53: {1, 2, 3, 4, 5, 6},
		}[id]
		for _, v := range tmpl {
			want = binary.LittleEndian.AppendUint32(want, 0x46)
			want = binary.LittleEndian.AppendUint32(want, uint32(v))
			want = binary.LittleEndian.AppendUint32(want, 0x0a)
		}
	}

	if !bytes.Equal(got, want) {
		t.Errorf("FrameNewCharacterSuccess mismatch:\n got  %x\n want %x", got, want)
	}
}

func TestFrameNewCharacterSuccess_MissingTemplate(t *testing.T) {
	table, err := player.NewTemplateTable(map[int]*player.Template{0: rootTemplate(0, 1, 1, 1, 1, 1, 1)})
	if err != nil {
		t.Fatalf("build template table: %v", err)
	}
	frame, err := FrameNewCharacterSuccess(table)
	frame.Release()
	if err == nil {
		t.Error("FrameNewCharacterSuccess: want error for missing profession, got nil")
	}
}
