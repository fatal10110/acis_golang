package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

const (
	// OpcodeWarehouseDepositList is the wire opcode for WarehouseDepositList.
	OpcodeWarehouseDepositList = 0x41
	// OpcodeWarehouseWithdrawList is the wire opcode for WarehouseWithdrawList.
	OpcodeWarehouseWithdrawList = 0x42
	// OpcodePackageToList is the wire opcode for PackageToList.
	OpcodePackageToList = 0xc2
	// OpcodePackageSendableList is the wire opcode for PackageSendableList.
	OpcodePackageSendableList = 0xc3
)

// WarehouseType identifies which warehouse tab the client displays.
type WarehouseType uint16

const (
	// WarehousePrivate is a player's private warehouse.
	WarehousePrivate WarehouseType = 1
	// WarehouseClan is a clan warehouse.
	WarehouseClan WarehouseType = 2
	// WarehouseCastle is a castle warehouse.
	WarehouseCastle WarehouseType = 3
	// WarehouseFreight is freight storage.
	WarehouseFreight WarehouseType = 4
)

// PackageRecipient is one same-account character that can receive freight.
type PackageRecipient struct {
	ObjectID int32
	Name     string
}

// FramePackageToList builds the freight recipient list.
func FramePackageToList(recipients []PackageRecipient) wire.Frame {
	w := newFrameWriter(OpcodePackageToList)
	w.WriteInt32(int32(len(recipients)))
	for _, r := range recipients {
		w.WriteInt32(r.ObjectID)
		w.WriteString(r.Name)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePackageSendableList builds the list of inventory items that can be
// sent as freight. An item whose template templates no longer holds (one a
// reload removed) is left out.
func FramePackageSendableList(objectID, playerAdena int32, items []*item.Instance, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodePackageSendableList)
	w.WriteInt32(objectID)
	w.WriteInt32(playerAdena)
	countOffset := w.Len()
	w.WriteInt32(0) // backfilled with the written count once the loop below finishes
	var written int32
	for _, inst := range items {
		if writeWarehouseItem(w, inst, templates, false) {
			written++
		}
	}
	w.PatchInt32(countOffset, written)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

// FrameWarehouseDepositList builds the list of inventory items that can be
// deposited into a warehouse.
func FrameWarehouseDepositList(whType WarehouseType, playerAdena int32, items []*item.Instance, templates *item.Table) (wire.Frame, error) {
	return frameWarehouseList(OpcodeWarehouseDepositList, whType, playerAdena, items, templates)
}

// FrameWarehouseWithdrawList builds the active warehouse contents that can
// be withdrawn.
func FrameWarehouseWithdrawList(whType WarehouseType, playerAdena int32, items []*item.Instance, templates *item.Table) (wire.Frame, error) {
	return frameWarehouseList(OpcodeWarehouseWithdrawList, whType, playerAdena, items, templates)
}

// frameWarehouseList builds a deposit or withdraw list. An item whose
// template templates no longer holds (one a reload removed) is left out.
func frameWarehouseList(opcode byte, whType WarehouseType, playerAdena int32, items []*item.Instance, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(opcode)
	w.WriteUint16(uint16(whType))
	w.WriteInt32(playerAdena)
	countOffset, err := reserveUint16Count(w, len(items))
	if err != nil {
		releaseFrameWriter(w)
		return wire.Frame{}, err
	}
	var written int
	for _, inst := range items {
		if writeWarehouseItem(w, inst, templates, true) {
			written++
		}
	}
	w.PatchUint16(countOffset, uint16(written))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

// writeWarehouseItem writes inst's row and reports whether it did: a nil
// item, or one whose template templates no longer holds, writes nothing.
func writeWarehouseItem(w *wire.Writer, inst *item.Instance, templates *item.Table, includeAugmentation bool) bool {
	if inst == nil {
		return false
	}
	st := inst.Snapshot()
	tmpl, ok := templates.Get(st.TemplateID)
	if !ok {
		return false
	}
	category, subCategory := tmpl.Category()

	w.WriteUint16(uint16(category))
	w.WriteInt32(st.ObjectID)
	w.WriteInt32(st.TemplateID)
	w.WriteInt32(int32(st.Count))
	w.WriteUint16(uint16(subCategory))
	w.WriteUint16(uint16(st.CustomType1))
	w.WriteInt32(int32(tmpl.Slot))
	w.WriteUint16(uint16(st.EnchantLevel))
	w.WriteUint16(uint16(st.CustomType2))
	w.WriteUint16(0)
	w.WriteInt32(st.ObjectID)

	if includeAugmentation {
		if st.Augmentation == nil {
			w.WriteInt64(0)
		} else {
			w.WriteInt32(st.Augmentation.Attributes & 0x0000ffff)
			w.WriteInt32(st.Augmentation.Attributes >> 16)
		}
	}
	return true
}
