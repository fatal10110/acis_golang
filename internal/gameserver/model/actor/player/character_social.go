package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// SocialGraph answers the party and clan standing between players, by
// object id and clan id. Any goroutine may call it.
type SocialGraph interface {
	InParty(objectID int32) bool
	SameParty(a, b int32) bool
	// SameChannel reports whether b is in the command channel a's party
	// belongs to.
	SameChannel(a, b int32) bool
	// AllyID is the alliance of the clan, 0 when it is in none.
	AllyID(clanID int32) int32
	// ClanLeaderID is the object id of the clan's leader, 0 for no clan.
	ClanLeaderID(clanID int32) int32
	// AtWar reports whether clanID declared war on targetClanID.
	AtWar(clanID, targetClanID int32) bool
}

// socialPeer is the player state the social rules read from the player
// acting through another playable.
type socialPeer interface {
	ObjectID() int32
	ClanID() int32
	Level() int
	Karma() int
	PvPFlagState() task.PvPFlagState
	ProtectionBlessing() bool
	CursedWeaponEquipped() bool
	InPvPZone() bool
}

// socialPeerOf returns the player acting through a: a itself for a player,
// its owner for a summon. Other actors have none.
func socialPeerOf(a target.Actor) (socialPeer, bool) {
	if a == nil {
		return nil, false
	}
	switch a.Kind() {
	case actor.KindPlayer:
		p, ok := a.(socialPeer)
		return p, ok
	case actor.KindSummon:
		owner, ok := a.Owner()
		if !ok || owner == nil {
			return nil, false
		}
		p, ok := owner.(socialPeer)
		return p, ok
	}
	return nil, false
}

// IsInParty reports whether c is in a party.
func (c *Character) IsInParty() bool {
	return c.social != nil && c.social.InParty(c.ID)
}

// PartyContains reports whether c's party holds the player acting through
// other.
func (c *Character) PartyContains(other target.Actor) bool { return c.IsInSameParty(other) }

// IsInSameParty reports whether the player acting through other is in c's
// party.
func (c *Character) IsInSameParty(other target.Actor) bool {
	p, ok := socialPeerOf(other)
	return ok && c.social != nil && c.social.SameParty(c.ID, p.ObjectID())
}

// IsInSameChannel reports whether the player acting through other is in
// the command channel c's party belongs to.
func (c *Character) IsInSameChannel(other target.Actor) bool {
	p, ok := socialPeerOf(other)
	return ok && c.social != nil && c.social.SameChannel(c.ID, p.ObjectID())
}

// HasClan reports whether c belongs to a clan.
func (c *Character) HasClan() bool { return c.ClanID() != 0 }

// IsInSameClan reports whether the player acting through other belongs to
// c's clan.
func (c *Character) IsInSameClan(other target.Actor) bool {
	p, ok := socialPeerOf(other)
	return ok && c.ClanID() > 0 && c.ClanID() == p.ClanID()
}

// AllyID is the alliance of c's clan, 0 when it is in none.
func (c *Character) AllyID() int32 { return c.allyOf(c.ClanID()) }

func (c *Character) allyOf(clanID int32) int32 {
	if clanID == 0 || c.social == nil {
		return 0
	}
	return c.social.AllyID(clanID)
}

// IsInSameAlly reports whether the player acting through other belongs to
// c's alliance.
func (c *Character) IsInSameAlly(other target.Actor) bool {
	p, ok := socialPeerOf(other)
	if !ok {
		return false
	}
	ally := c.AllyID()
	return ally > 0 && ally == c.allyOf(p.ClanID())
}

// IsClanLeader reports whether c leads its clan.
func (c *Character) IsClanLeader() bool {
	clanID := c.ClanID()
	return clanID != 0 && c.social != nil && c.social.ClanLeaderID(clanID) == c.ID
}

// AtWarWith reports whether c's clan and the clan of the player acting
// through other have each declared war on the other.
func (c *Character) AtWarWith(other target.Actor) bool {
	p, ok := socialPeerOf(other)
	if !ok || c.social == nil {
		return false
	}
	own, theirs := c.ClanID(), p.ClanID()
	return own != 0 && theirs != 0 && c.social.AtWar(own, theirs) && c.social.AtWar(theirs, own)
}

// MageClass reports whether c plays a mystic class.
func (c *Character) MageClass() bool { return ClassMage(c.ClassID()) }

// CanCastOnPlayable judges whether c may cast skill on target, a playable,
// as the offensive or the beneficial social policy decides.
func (c *Character) CanCastOnPlayable(t target.Actor, skill *modelskill.Definition, ctrl, offensive bool) bool {
	if offensive {
		return c.OffensiveCastAllowed(c, t, skill, ctrl)
	}
	return c.beneficialCastAllowed(t, ctrl)
}

// SocialWithoutForce judges the party, command channel, clan and alliance
// rules for attacking self, a playable acting through c, without force:
// decided is false when none of them settles it. Two playables in an arena
// fight freely unless they share a party or command channel; otherwise a
// shared party, command channel, clan or alliance needs force.
//
// Olympiad matches (#216), duels (#215) and siege sides (#234) are not
// modeled, so their rules never apply.
func (c *Character) SocialWithoutForce(self attackable.ArenaMember, attacker target.Actor) (allowed, decided bool) {
	sameParty := c.IsInSameParty(attacker)
	sameChannel := c.IsInSameChannel(attacker)
	if attackable.InArena(self) && attackable.InArena(attacker) && !sameParty && !sameChannel {
		return true, true
	}
	if sameParty || sameChannel || c.IsInSameClan(attacker) || c.IsInSameAlly(attacker) {
		return false, true
	}
	return false, false
}

// OffensiveCastAllowed judges an offensive skill cast on t, a playable, by
// caster, c itself or c's summon, whose own zones count. The cast is always
// judged on its final target, so a CTRL damage skill counts as aimed at its
// main target.
//
// Olympiad matches (#216), duels (#215) and siege sides (#234) are not
// modeled, so their rules never apply.
func (c *Character) OffensiveCastAllowed(caster attackable.ArenaMember, t target.Actor, skill *modelskill.Definition, ctrl bool) bool {
	targetPlayer, ok := socialPeerOf(t)
	if !ok || targetPlayer.ObjectID() == c.ID {
		return false
	}
	sameParty := c.IsInSameParty(t)
	sameChannel := c.IsInSameChannel(t)
	if attackable.InArena(caster) && attackable.InArena(t) && !sameParty && !sameChannel {
		return true
	}
	ctrlDamage := ctrl && skill != nil && skill.IsDamage()
	if sameParty || sameChannel || c.IsInSameClan(t) || c.IsInSameAlly(t) {
		return ctrlDamage
	}
	if caster.InPvPZone() && inPvPZone(t) {
		return true
	}
	if targetPlayer.ProtectionBlessing() && c.Level()-targetPlayer.Level() >= 10 && c.Karma() > 0 {
		return false
	}
	if c.ProtectionBlessing() && targetPlayer.Level()-c.Level() >= 10 && targetPlayer.Karma() > 0 {
		return false
	}
	if targetPlayer.CursedWeaponEquipped() && c.Level() <= 20 {
		return false
	}
	if c.CursedWeaponEquipped() && targetPlayer.Level() <= 20 {
		return false
	}
	if targetPlayer.PvPFlagState() != task.PvPFlagNone || targetPlayer.Karma() > 0 {
		return true
	}
	if c.AtWarWith(t) {
		return ctrl
	}
	if skill != nil && skill.Debuff {
		return false
	}
	return ctrlDamage
}

// beneficialCastAllowed judges a beneficial skill cast by c on t, a
// playable: c itself always; inside a PvP zone the target's player may be
// helped from one, or from a peace zone with CTRL; a party, command
// channel, clan or alliance mate always; any other flagged or karma player
// only with CTRL.
//
// Olympiad matches (#216) and duels (#215) are not modeled, so their rules
// never apply.
func (c *Character) beneficialCastAllowed(t target.Actor, ctrl bool) bool {
	if t.Kind() == actor.KindPlayer && t.ObjectID() == c.ID {
		return true
	}
	targetPlayer, ok := socialPeerOf(t)
	if !ok {
		return true
	}
	if targetPlayer.InPvPZone() {
		if c.InPvPZone() {
			return true
		}
		if c.InPeaceZone() {
			return ctrl
		}
	}
	if c.IsInSameParty(t) || c.IsInSameChannel(t) || c.IsInSameClan(t) || c.IsInSameAlly(t) {
		return true
	}
	if targetPlayer.PvPFlagState() != task.PvPFlagNone || targetPlayer.Karma() > 0 {
		return ctrl
	}
	return true
}
