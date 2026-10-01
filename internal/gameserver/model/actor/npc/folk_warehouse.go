package npc

import "strings"

// warehouseKinds are the NPCs a warehouse deposit or withdrawal may be made
// at.
var warehouseKinds = map[InstanceKind]struct{}{
	"WarehouseKeeper":       {},
	"CastleWarehouseKeeper": {},
	"ClanHallManagerNpc":    {},
}

// Warehouse reports whether f keeps warehouses: a deposit or withdrawal
// needs one selected and in reach.
func (f *Folk) Warehouse() bool {
	_, ok := warehouseKinds[hostileKind(f.Instance)]
	return ok
}

// WarehouseCommand is a warehouse keeper's storage command.
type WarehouseCommand int

const (
	// WarehouseNoCommand is not a storage command.
	WarehouseNoCommand WarehouseCommand = iota
	// WithdrawPrivate opens the private warehouse for withdrawal.
	WithdrawPrivate
	// DepositPrivate opens the private warehouse for deposit.
	DepositPrivate
	// WithdrawClan opens the clan warehouse for withdrawal.
	WithdrawClan
	// DepositClan opens the clan warehouse for deposit.
	DepositClan
	// WithdrawFreight opens the talker's own freight for withdrawal.
	WithdrawFreight
	// DepositFreight lists the talker's other characters a package may be
	// sent to.
	DepositFreight
	// FreightCharacter opens another character's freight for deposit; the
	// reply's FreightTarget names it.
	FreightCharacter
)

// warehouseCommand reads a warehouse keeper's storage command: the private
// and freight ones by prefix, the clan ones and DepositP only exactly.
func warehouseCommand(command string) WarehouseCommand {
	switch {
	case strings.HasPrefix(command, "WithdrawP"):
		return WithdrawPrivate
	case command == "DepositP":
		return DepositPrivate
	case command == "WithdrawC":
		return WithdrawClan
	case command == "DepositC":
		return DepositClan
	case strings.HasPrefix(command, "WithdrawF"):
		return WithdrawFreight
	case strings.HasPrefix(command, "DepositF"):
		return DepositFreight
	case strings.HasPrefix(command, "FreightChar"):
		return FreightCharacter
	}
	return WarehouseNoCommand
}
