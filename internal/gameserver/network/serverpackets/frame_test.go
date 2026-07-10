package serverpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func frameBytes(t *testing.T, frame wire.Frame) []byte {
	t.Helper()
	t.Cleanup(frame.Release)
	return frame.Bytes()
}

func framePayload(t *testing.T, frame wire.Frame) []byte {
	t.Helper()
	bytes := frameBytes(t, frame)
	if len(bytes) < 2 {
		t.Fatalf("frame length = %d, want header", len(bytes))
	}
	return bytes[2:]
}

func TestFrameItemListErrorReturnsNoFrame(t *testing.T) {
	items := []*item.Instance{{ObjectID: 1, TemplateID: 999, Count: 1, Location: item.LocationInventory}}

	frame, err := FrameItemList(items, item.NewTable(nil), true)
	if err == nil {
		t.Fatal("FrameItemList err = nil, want an error for a missing template")
	}
	frame.Release() // must be a no-op on the zero frame
	if frame.Bytes() != nil {
		t.Errorf("frame.Bytes() = % X, want nil", frame.Bytes())
	}
}

func TestFrameNewCharacterSuccessErrorReturnsNoFrame(t *testing.T) {
	table, err := player.NewTemplateTable(map[int]*player.Template{0: rootTemplate(0, 1, 2, 3, 4, 5, 6)})
	if err != nil {
		t.Fatalf("build template table: %v", err)
	}

	frame, err := FrameNewCharacterSuccess(table)
	if err == nil {
		t.Fatal("FrameNewCharacterSuccess err = nil, want an error for a missing profession")
	}
	frame.Release()
	if frame.Bytes() != nil {
		t.Errorf("frame.Bytes() = % X, want nil", frame.Bytes())
	}
}
