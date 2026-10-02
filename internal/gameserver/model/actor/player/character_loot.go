package player

import "github.com/fatal10110/acis_golang/internal/gameserver/party"

// IsLooterOrInLooterParty reports whether c may take loot reserved to
// ownerID: c is the owner, or the owner is in c's party, or in the command
// channel c's party belongs to. A player in no party can only be the owner.
func (c *Character) IsLooterOrInLooterParty(ownerID int32) bool {
	if ownerID == c.ID {
		return true
	}
	return c.social != nil && (c.social.SameParty(c.ID, ownerID) || c.social.SameChannel(c.ID, ownerID))
}

// PartyLoot shares the items a partied player auto-loots or sweeps.
type PartyLoot interface {
	// LootForParty gives count units of itemID, which playerID auto-looted
	// or (spoil) swept off origin, to the party member its party's loot
	// rule picks, or shares adena among the members. It reports false when
	// the player is in no party, leaving the item to the caller.
	LootForParty(playerID int32, itemID int32, count int, spoil bool, origin party.LootOrigin) bool
}

// LootForParty hands count units of itemID that c auto-looted or (spoil)
// swept off origin to its party's loot rule. It reports false when c is in
// no party, leaving the item to the caller.
func (c *Character) LootForParty(itemID int32, count int, spoil bool, origin party.LootOrigin) bool {
	return c.partyLoot != nil && c.partyLoot.LootForParty(c.ID, itemID, count, spoil, origin)
}
