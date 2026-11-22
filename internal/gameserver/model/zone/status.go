package zone

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"

// A player character reads its zones from its flag ledger.
var _ player.ZoneStatus = (*Flags)(nil)

func (f *Flags) InPvPZone() bool          { return f.Has(FlagPvP) }
func (f *Flags) InPeaceZone() bool        { return f.Has(FlagPeace) }
func (f *Flags) InSiegeZone() bool        { return f.Has(FlagSiege) }
func (f *Flags) NoSummonFriendZone() bool { return f.Has(FlagNoSummonFriend) }
func (f *Flags) InDangerArea() bool       { return f.Has(FlagDanger) }
func (f *Flags) InWater() bool            { return f.Has(FlagWater) }
func (f *Flags) InBossZone() bool         { return f.Has(FlagBoss) }
func (f *Flags) InClanHallZone() bool     { return f.Has(FlagClanHall) }
func (f *Flags) InCastleZone() bool       { return f.Has(FlagCastle) }
func (f *Flags) InMotherTreeZone() bool   { return f.Has(FlagMotherTree) }
