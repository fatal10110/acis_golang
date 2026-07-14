package serverpackets

import (
	"fmt"

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
// sent as freight.
func FramePackageSendableList(objectID, playerAdena int32, items []*item.Instance, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodePackageSendableList)
	w.WriteInt32(objectID)
	w.WriteInt32(playerAdena)
	w.WriteInt32(int32(len(items)))
	for _, inst := range items {
		if err := writeWarehouseItem(w, inst, templates, false); err != nil {
			releaseFrameWriter(w)
			return wire.Frame{}, fmt.Errorf("serverpackets: PackageSendableList: %w", err)
		}
	}
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

func frameWarehouseList(opcode byte, whType WarehouseType, playerAdena int32, items []*item.Instance, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(opcode)
	w.WriteUint16(uint16(whType))
	w.WriteInt32(playerAdena)
	w.WriteUint16(uint16(len(items)))
	for _, inst := range items {
		if err := writeWarehouseItem(w, inst, templates, true); err != nil {
			releaseFrameWriter(w)
			return wire.Frame{}, fmt.Errorf("serverpackets: warehouse list: %w", err)
		}
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

func writeWarehouseItem(w *wire.Writer, inst *item.Instance, templates *item.Table, includeAugmentation bool) error {
	if inst == nil {
		return fmt.Errorf("nil item")
	}
	tmpl, ok := templates.Get(inst.TemplateID)
	if !ok {
		return fmt.Errorf("no template loaded for item template %d", inst.TemplateID)
	}
	category, subCategory := tmpl.Category()

	w.WriteUint16(uint16(category))
	w.WriteInt32(inst.ObjectID)
	w.WriteInt32(inst.TemplateID)
	w.WriteInt32(int32(inst.Count))
	w.WriteUint16(uint16(subCategory))
	w.WriteUint16(uint16(inst.CustomType1))
	w.WriteInt32(int32(tmpl.Slot))
	w.WriteUint16(uint16(inst.EnchantLevel))
	w.WriteUint16(uint16(inst.CustomType2))
	w.WriteUint16(0)
	w.WriteInt32(inst.ObjectID)

	if includeAugmentation {
		if inst.Augmentation == nil {
			w.WriteInt64(0)
		} else {
			w.WriteInt32(inst.Augmentation.Attributes & 0x0000ffff)
			w.WriteInt32(inst.Augmentation.Attributes >> 16)
		}
	}
	return nil
}
