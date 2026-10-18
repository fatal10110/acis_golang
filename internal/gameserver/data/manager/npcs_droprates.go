package manager

import "github.com/fatal10110/acis_golang/internal/gameserver/model/item"

// DropRates returns the drop-rate multipliers in force, which the
// population's kills roll their drop categories with.
func (n *Npcs) DropRates() item.Rates { return n.rewards.Settings.Settings().Rates }
