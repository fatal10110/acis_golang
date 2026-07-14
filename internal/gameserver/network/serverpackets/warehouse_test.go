package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func TestFramePackageToList(t *testing.T) {
	got := framePayload(t, FramePackageToList([]PackageRecipient{
		{ObjectID: 100, Name: "Alpha"},
		{ObjectID: 200, Name: "Beta"},
	}))

	want := []byte{OpcodePackageToList}
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = appendUTF16Z(want, "Alpha")
	want = binary.LittleEndian.AppendUint32(want, 200)
	want = appendUTF16Z(want, "Beta")

	if !bytes.Equal(got, want) {
		t.Fatalf("FramePackageToList = %x, want %x", got, want)
	}
}

func TestFramePackageSendableList(t *testing.T) {
	templates := warehousePacketTemplates()
	items := []*item.Instance{
		{ObjectID: 500, TemplateID: 30, Count: 1, Location: item.LocationInventory, EnchantLevel: 2, CustomType1: 7, CustomType2: 8},
		{ObjectID: 501, TemplateID: item.AdenaID, Count: 100, Location: item.LocationInventory},
	}

	frame, err := FramePackageSendableList(200, 777, items, templates)
	if err != nil {
		t.Fatalf("FramePackageSendableList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodePackageSendableList}
	want = binary.LittleEndian.AppendUint32(want, 200)
	want = binary.LittleEndian.AppendUint32(want, 777)
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = appendWarehouseVisibleItem(want, items[0], templates, false)
	want = appendWarehouseVisibleItem(want, items[1], templates, false)

	if !bytes.Equal(got, want) {
		t.Fatalf("FramePackageSendableList = %x, want %x", got, want)
	}
}

func TestFrameWarehouseDepositList(t *testing.T) {
	templates := warehousePacketTemplates()
	items := []*item.Instance{
		{
			ObjectID: 500, TemplateID: 30, Count: 1, Location: item.LocationInventory,
			EnchantLevel: 2, CustomType1: 7, CustomType2: 8,
			Augmentation: &item.Augmentation{Attributes: 0x12345678},
		},
	}

	frame, err := FrameWarehouseDepositList(WarehousePrivate, 777, items, templates)
	if err != nil {
		t.Fatalf("FrameWarehouseDepositList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeWarehouseDepositList}
	want = binary.LittleEndian.AppendUint16(want, uint16(WarehousePrivate))
	want = binary.LittleEndian.AppendUint32(want, 777)
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = appendWarehouseVisibleItem(want, items[0], templates, true)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameWarehouseDepositList = %x, want %x", got, want)
	}
}

func TestFrameWarehouseWithdrawList(t *testing.T) {
	templates := warehousePacketTemplates()
	items := []*item.Instance{{ObjectID: 501, TemplateID: item.AdenaID, Count: 100, Location: item.LocationWarehouse}}

	frame, err := FrameWarehouseWithdrawList(WarehouseFreight, 777, items, templates)
	if err != nil {
		t.Fatalf("FrameWarehouseWithdrawList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeWarehouseWithdrawList}
	want = binary.LittleEndian.AppendUint16(want, uint16(WarehouseFreight))
	want = binary.LittleEndian.AppendUint32(want, 777)
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = appendWarehouseVisibleItem(want, items[0], templates, true)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameWarehouseWithdrawList = %x, want %x", got, want)
	}
}

func TestFramePackageSendableListMissingTemplate(t *testing.T) {
	_, err := FramePackageSendableList(200, 777, []*item.Instance{{ObjectID: 500, TemplateID: 999, Count: 1}}, item.NewTable(nil))
	if err == nil {
		t.Fatal("FramePackageSendableList: want error for missing template")
	}
}

func warehousePacketTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: item.AdenaID, Kind: item.KindEtcItem, Slot: item.SlotNone, Duration: -1, Stackable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: 30, Kind: item.KindWeapon, Slot: item.SlotRHand, Duration: -1, Weapon: &item.WeaponDetail{}},
	})
}

func appendWarehouseVisibleItem(out []byte, inst *item.Instance, templates *item.Table, includeAugmentation bool) []byte {
	tmpl, _ := templates.Get(inst.TemplateID)
	category, subCategory := tmpl.Category()

	out = binary.LittleEndian.AppendUint16(out, uint16(category))
	out = binary.LittleEndian.AppendUint32(out, uint32(inst.ObjectID))
	out = binary.LittleEndian.AppendUint32(out, uint32(inst.TemplateID))
	out = binary.LittleEndian.AppendUint32(out, uint32(inst.Count))
	out = binary.LittleEndian.AppendUint16(out, uint16(subCategory))
	out = binary.LittleEndian.AppendUint16(out, uint16(inst.CustomType1))
	out = binary.LittleEndian.AppendUint32(out, uint32(tmpl.Slot))
	out = binary.LittleEndian.AppendUint16(out, uint16(inst.EnchantLevel))
	out = binary.LittleEndian.AppendUint16(out, uint16(inst.CustomType2))
	out = binary.LittleEndian.AppendUint16(out, 0)
	out = binary.LittleEndian.AppendUint32(out, uint32(inst.ObjectID))
	if includeAugmentation {
		if inst.Augmentation != nil {
			out = binary.LittleEndian.AppendUint32(out, uint32(inst.Augmentation.Attributes&0x0000ffff))
			return binary.LittleEndian.AppendUint32(out, uint32(inst.Augmentation.Attributes>>16))
		}
		return binary.LittleEndian.AppendUint64(out, 0)
	}
	return out
}

func appendUTF16Z(out []byte, s string) []byte {
	for _, unit := range utf16.Encode([]rune(s)) {
		out = binary.LittleEndian.AppendUint16(out, unit)
	}
	return binary.LittleEndian.AppendUint16(out, 0)
}
