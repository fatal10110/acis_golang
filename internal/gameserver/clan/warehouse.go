package clan

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"

// WarehouseRefusal is why a warehouse keeper does not open a player's clan
// warehouse.
type WarehouseRefusal int

// The clan warehouse refusals.
const (
	WarehouseOpen WarehouseRefusal = iota
	// WarehouseNoRight refuses a withdrawal window to a player without the
	// warehouse-search privilege, a clanless one included.
	WarehouseNoRight
	// WarehouseLevelTooLow refuses a player whose clan is level 0, or a
	// clanless one asking to deposit.
	WarehouseLevelTooLow
)

// OpenWarehouse returns the clan whose warehouse c may open, for a
// withdrawal when withdraw is set and a deposit otherwise. A withdrawal
// needs the warehouse-search privilege first; both need a clan of level 1
// or higher.
func (s *Service) OpenWarehouse(c *player.Character, withdraw bool) (*Clan, WarehouseRefusal) {
	cl, ok := s.ClanOf(c)
	if withdraw && (!ok || !cl.HasPrivilege(c.ID, PrivWarehouseSearch)) {
		return nil, WarehouseNoRight
	}
	if !ok || cl.Level() == 0 {
		return nil, WarehouseLevelTooLow
	}
	return cl, WarehouseOpen
}

// WithdrawRefusal is why a member's withdrawal from its clan's warehouse
// is refused.
type WithdrawRefusal int

// The clan warehouse withdrawal refusals. WithdrawSilent is not answered.
const (
	WithdrawAllowed WithdrawRefusal = iota
	WithdrawSilent
	WithdrawLeaderOnly
)

// CanWithdraw reports whether c may take items out of the warehouse of the
// clan clanID. A player no longer in that clan is refused silently. With
// MembersCanWithdrawFromClanWH a member needs the warehouse-search
// privilege; without it only the leader may withdraw.
func (s *Service) CanWithdraw(c *player.Character, clanID int32) WithdrawRefusal {
	cl, ok := s.ClanOf(c)
	switch {
	case !ok || cl.ID() != clanID:
		return WithdrawSilent
	case s.cfg.MembersCanWithdrawFromWarehouse:
		if !cl.HasPrivilege(c.ID, PrivWarehouseSearch) {
			return WithdrawSilent
		}
	case !cl.IsLeader(c.ID):
		return WithdrawLeaderOnly
	}
	return WithdrawAllowed
}

// InClan reports whether c is still a member of the clan clanID.
func (s *Service) InClan(c *player.Character, clanID int32) bool {
	cl, ok := s.ClanOf(c)
	return ok && cl.ID() == clanID
}
