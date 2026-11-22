package player

// zoneStub stands in for the zone flag ledger in pure unit tests: the
// character stands in the zones its fields name.
type zoneStub struct {
	pvp, peace, siege, noSummonFriend, danger, water bool
	boss, clanHall, castle, motherTree                bool
}

func (s *zoneStub) InPvPZone() bool          { return s.pvp }
func (s *zoneStub) InPeaceZone() bool        { return s.peace }
func (s *zoneStub) InSiegeZone() bool        { return s.siege }
func (s *zoneStub) NoSummonFriendZone() bool { return s.noSummonFriend }
func (s *zoneStub) InDangerArea() bool       { return s.danger }
func (s *zoneStub) InWater() bool            { return s.water }
func (s *zoneStub) InBossZone() bool         { return s.boss }
func (s *zoneStub) InClanHallZone() bool     { return s.clanHall }
func (s *zoneStub) InCastleZone() bool       { return s.castle }
func (s *zoneStub) InMotherTreeZone() bool   { return s.motherTree }

// inZones puts c in the zones s names, and in no other.
func inZones(c *Character, s zoneStub) { c.SetZoneStatus(&s) }
