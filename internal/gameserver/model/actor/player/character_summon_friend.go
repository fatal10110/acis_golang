package player

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// SummonFriendRequester is the caster of a summon-friend request. Its
// identity feeds the client-facing dialog and the requesterId anti-spoof
// check, its position the eventual teleport, and its state the accept
// path's re-validation: the teleport re-runs the summoner and summoned
// checks at accept time.
type SummonFriendRequester interface {
	ObjectID() int32
	CharacterName() string
	Position() (int, int, int)
	AlikeDead() bool
	Operating() bool
	Rooted() bool
	InCombat() bool
	OlympiadMode() bool
	FestivalParticipant() bool
	Mounted() bool
	ObserverMode() bool
	NoSummonFriendZone() bool
}

// TeleportTo moves the character to (x, y, z), scattered within radius. It
// may be called from any queue: the teleport runs on the character's own
// queue once the calling task is done.
func (c *Character) TeleportTo(x, y, z, radius int) {
	c.emit(event.TeleportRequested{X: x, Y: y, Z: z, Radius: radius})
}

// TeleportOnQueue is TeleportTo for a caller running on the character's own
// queue: the teleport runs inside the call.
func (c *Character) TeleportOnQueue(x, y, z, radius int) {
	c.emit(event.TeleportRequested{X: x, Y: y, Z: z, Radius: radius, OwnQueue: true})
}

// ForceTeleport is TeleportTo, or TeleportOnQueue when ownQueue, for a
// move that must happen: one refused while the character's own teleport is
// still landing is made once that landing completes.
func (c *Character) ForceTeleport(x, y, z, radius int, ownQueue bool) {
	c.emit(event.TeleportRequested{X: x, Y: y, Z: z, Radius: radius, OwnQueue: ownQueue, Forced: true})
}

// ItemCount is the held count of itemID, for the summon teleport's
// item-requirement check.
func (c *Character) ItemCount(itemID int) int {
	if c.Inventory() == nil {
		return 0
	}
	return c.Inventory().ItemCount(int32(itemID), -1, true)
}

// ConsumeItem destroys count units of itemID, the summon teleport's item
// cost.
func (c *Character) ConsumeItem(itemID, count int) bool {
	if c.Inventory() == nil {
		return false
	}
	return c.Inventory().DestroyByTemplateID(int32(itemID), count) != nil
}

// TeleportRequest is summonFriendRequester's entry point
// (handler/skill/summon.go): a second concurrent request from a different
// requester is refused (the caller sends S1_ALREADY_SUMMONED and skips this
// target), while every other call — including the plain
// SUMMON_FRIEND/SUMMON_PARTY path's post-teleport `nil` call — records the
// request unconditionally.
func (c *Character) TeleportRequest(caster SummonFriendRequester, skill modelskill.Definition) bool {
	c.summonFriendMu.Lock()
	defer c.summonFriendMu.Unlock()
	if c.summonRequester != nil && caster != nil {
		return false
	}
	c.summonRequester = caster
	c.summonSkill = skill
	c.summonRequesterID = 0
	if caster != nil {
		c.summonRequesterID = caster.ObjectID()
	}
	return true
}

// ClearTeleportRequest resets pending summon-confirm state, as a
// state-clearing call after the plain (non-1403) path's immediate,
// synchronous teleport.
func (c *Character) ClearTeleportRequest() {
	c.summonFriendMu.Lock()
	defer c.summonFriendMu.Unlock()
	c.summonRequester = nil
	c.summonRequesterID = 0
	c.summonSkill = modelskill.Definition{}
}

// ConfirmSummon is summonFriendRequester's entry point for skill 1403: it
// sends the accept/decline dialog to
// this character. The pending request TeleportRequest already recorded
// stays in place until TeleportAnswer resolves it or another cast
// overwrites/clears it — there is no server-side timeout (the timeout
// argument is a client-UI-only countdown, the dialog's time parameter).
func (c *Character) ConfirmSummon(caster SummonFriendRequester, skill modelskill.Definition, timeout time.Duration) {
	if caster == nil {
		return
	}
	x, y, z := caster.Position()
	c.emit(event.SummonConfirmRequested{CasterName: caster.CharacterName(), CasterID: caster.ObjectID(), X: x, Y: y, Z: z, Timeout: timeout})
}

// TeleportAnswer handles the client's DlgAnswer response to ConfirmSummon:
// pending state is cleared unconditionally, and the summon only completes on
// accept (answer == 1) with requesterID matching the stored requester's
// object id. The accept path runs SummonTo's full re-check, since there is
// no server-side timeout on the pending request and either side's
// eligibility may have changed while the dialog sat open. It returns the
// refusal to tell c, the answering player.
func (c *Character) TeleportAnswer(answer, requesterID int32) SummonRefusal {
	c.summonFriendMu.Lock()
	requester := c.summonRequester
	storedID := c.summonRequesterID
	skill := c.summonSkill
	c.summonRequester = nil
	c.summonRequesterID = 0
	c.summonSkill = modelskill.Definition{}
	c.summonFriendMu.Unlock()

	if requester == nil || answer != 1 || storedID != requesterID {
		return SummonRefusal{}
	}
	return c.summonTo(requester, skill, c.TeleportOnQueue)
}

// SummonTo moves c to caster for caster's summon-friend skill. The summon
// re-runs both gates with the roles swapped, c as the summoner and caster as
// the summoned, then takes the skill's consume item from c, naming it to c
// ahead of the teleport; the returned
// refusal is told to c. It may be called from any queue: the teleport runs
// on c's own queue.
func (c *Character) SummonTo(caster SummonFriendRequester, skill modelskill.Definition) SummonRefusal {
	return c.summonTo(caster, skill, c.TeleportTo)
}

func (c *Character) summonTo(caster SummonFriendRequester, skill modelskill.Definition, teleport func(x, y, z, radius int)) SummonRefusal {
	if r := SummonerRefusal(c); r.Refused() {
		return r
	}
	if r := SummonedRefusal(c, caster); r.Refused() {
		return r
	}
	if skill.Extra().TargetConsumeID > 0 && skill.Extra().TargetConsumeCount > 0 {
		if c.ItemCount(skill.Extra().TargetConsumeID) < skill.Extra().TargetConsumeCount {
			return SummonRefusal{Reason: SummonRefusedItem, ItemID: skill.Extra().TargetConsumeID}
		}
		if !c.ConsumeItem(skill.Extra().TargetConsumeID, skill.Extra().TargetConsumeCount) {
			// The count check just passed, so only a concurrent removal
			// lands here: the summon stops rather than going on unpaid.
			return SummonRefusal{Reason: SummonRefusedSilently}
		}
		c.emit(event.ItemsTaken{ItemID: int32(skill.Extra().TargetConsumeID), Count: skill.Extra().TargetConsumeCount})
	}
	x, y, z := caster.Position()
	teleport(x, y, z, 20)
	return SummonRefusal{}
}

// SummonRefusalReason is why a summon-friend gate refused.
type SummonRefusalReason uint8

const (
	// SummonAllowed lets the summon go on.
	SummonAllowed SummonRefusalReason = iota
	// SummonRefusedSilently stops the summon with nothing told: a mounted
	// summoner or a target that is no player.
	SummonRefusedSilently
	// SummonRefusedLocation: the summoner is in the olympiad, observing or
	// inside a no-summon-friend zone.
	SummonRefusedLocation
	// SummonRefusedSelf: the target is the summoner.
	SummonRefusedSelf
	// SummonRefusedDead names a target that is dead or feigning death.
	SummonRefusedDead
	// SummonRefusedOperating names a target trading or running a private
	// store.
	SummonRefusedOperating
	// SummonRefusedCombat names a target that is rooted or in combat.
	SummonRefusedCombat
	// SummonRefusedOlympiad: the target is in the olympiad.
	SummonRefusedOlympiad
	// SummonRefusedBlocked: the target takes part in the festival or is
	// mounted.
	SummonRefusedBlocked
	// SummonRefusedBlockingArea names a target observing or inside a
	// no-summon-friend zone.
	SummonRefusedBlockingArea
	// SummonRefusedPending names a target that already has a summon
	// request pending.
	SummonRefusedPending
	// SummonRefusedItem: the summoned player lacks the skill's consume item.
	SummonRefusedItem
)

// SummonRefusal is a summon-friend gate's answer: Name is the player the
// reason names, ItemID the missing consume item.
type SummonRefusal struct {
	Reason SummonRefusalReason
	Name   string
	ItemID int
}

// Refused reports whether the summon stops.
func (r SummonRefusal) Refused() bool { return r.Reason != SummonAllowed }

// SummonerRefusal checks whether summoner may summon from where it stands.
func SummonerRefusal(summoner SummonFriendRequester) SummonRefusal {
	switch {
	case summoner.Mounted():
		return SummonRefusal{Reason: SummonRefusedSilently}
	case summoner.OlympiadMode() || summoner.ObserverMode() || summoner.NoSummonFriendZone():
		return SummonRefusal{Reason: SummonRefusedLocation}
	}
	return SummonRefusal{}
}

// SummonedRefusal checks whether summoner may summon the player target,
// reasons taken in the order the first one found is told.
func SummonedRefusal(summoner, target SummonFriendRequester) SummonRefusal {
	var reason SummonRefusalReason
	switch {
	case target.ObjectID() == summoner.ObjectID():
		reason = SummonRefusedSelf
	case target.AlikeDead():
		reason = SummonRefusedDead
	case target.Operating():
		reason = SummonRefusedOperating
	case target.Rooted() || target.InCombat():
		reason = SummonRefusedCombat
	case target.OlympiadMode():
		reason = SummonRefusedOlympiad
	case target.FestivalParticipant() || target.Mounted():
		reason = SummonRefusedBlocked
	case target.ObserverMode() || target.NoSummonFriendZone():
		reason = SummonRefusedBlockingArea
	default:
		return SummonRefusal{}
	}
	return SummonRefusal{Reason: reason, Name: target.CharacterName()}
}
