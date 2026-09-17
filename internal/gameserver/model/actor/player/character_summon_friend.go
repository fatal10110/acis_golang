package player

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// SummonFriendRequester is the caster of a summon-friend request. Its
// identity feeds the client-facing dialog and the requesterId anti-spoof
// check (Player.java:6917), its position the eventual teleport
// (SummonFriend.teleportTo, Player.java:198), and its state the accept
// path's re-validation, matching SummonFriend.teleportTo's own defensive
// checkSummoner/checkSummoned re-check at accept time (Player.java:6919
// calling SummonFriend.java:183-199).
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

// OlympiadMode always reports false: Olympiad (#216) isn't ported yet, so
// no character can be a participant.
func (c *Character) OlympiadMode() bool { return false }

// ObserverMode always reports false: observer/spectate mode (#219) isn't
// ported yet.
func (c *Character) ObserverMode() bool { return false }

// FestivalParticipant always reports false: the Festival of Darkness (#223)
// isn't ported yet.
func (c *Character) FestivalParticipant() bool { return false }

// TeleportTo is summonFriendTraveler's entry point
// (handler/skill/summon.go), matching Player.teleportTo
// (Player.java:196-198).
func (c *Character) TeleportTo(x, y, z, radius int) {
	c.emit(event.TeleportRequested{X: x, Y: y, Z: z, Radius: radius})
}

// ItemCount is summonFriendItemConsumer's entry point, matching
// Player.getInventory().getItemByItemId(...).getCount() used by
// SummonFriend.teleportTo's item-requirement check (Player.java:190-191).
func (c *Character) ItemCount(itemID int) int {
	if c.Inventory() == nil {
		return 0
	}
	return c.Inventory().ItemCount(int32(itemID), -1, true)
}

// ConsumeItem is summonFriendItemConsumer's entry point, matching
// Player.destroyItemByItemId (Player.java:193).
func (c *Character) ConsumeItem(itemID, count int) bool {
	if c.Inventory() == nil {
		return false
	}
	return c.Inventory().DestroyByTemplateID(int32(itemID), count) != nil
}

// TeleportRequest is summonFriendRequester's entry point
// (handler/skill/summon.go), matching Player.teleportRequest
// (Player.java:6902-6910): a second concurrent request from a different
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

// ClearTeleportRequest resets pending summon-confirm state, matching
// Player.teleportRequest(null, null)'s use as a state-clearing call
// (SummonFriend.java:88) after the plain (non-1403) path's immediate,
// synchronous teleport.
func (c *Character) ClearTeleportRequest() {
	c.summonFriendMu.Lock()
	defer c.summonFriendMu.Unlock()
	c.summonRequester = nil
	c.summonRequesterID = 0
	c.summonSkill = modelskill.Definition{}
}

// ConfirmSummon is summonFriendRequester's entry point for skill 1403,
// matching SummonFriend.java:76-84: it sends the accept/decline dialog to
// this character. The pending request TeleportRequest already recorded
// stays in place until TeleportAnswer resolves it or another cast
// overwrites/clears it — Java enforces no server-side timeout either (the
// timeout argument is a client-UI-only countdown, ConfirmDlg.addTime).
func (c *Character) ConfirmSummon(caster SummonFriendRequester, skill modelskill.Definition, timeout time.Duration) {
	if caster == nil {
		return
	}
	x, y, z := caster.Position()
	c.emit(event.SummonConfirmRequested{CasterName: caster.CharacterName(), CasterID: caster.ObjectID(), X: x, Y: y, Z: z, Timeout: timeout})
}

// TeleportAnswer handles the client's DlgAnswer response to ConfirmSummon,
// matching Player.teleportAnswer (Player.java:6912-6922): pending state is
// cleared unconditionally, and the summon only completes on accept
// (answer == 1) with requesterID matching the stored requester's object id.
// The accept path re-validates the full summoner/summoned gate — not just
// the item-consume check — since there is no server-side timeout on the
// pending request and either side's eligibility may have changed while the
// dialog sat open, matching teleportTo's own defensive re-check
// (SummonFriend.java:183-186).
func (c *Character) TeleportAnswer(answer, requesterID int32) {
	c.summonFriendMu.Lock()
	requester := c.summonRequester
	storedID := c.summonRequesterID
	skill := c.summonSkill
	c.summonRequester = nil
	c.summonRequesterID = 0
	c.summonSkill = modelskill.Definition{}
	c.summonFriendMu.Unlock()

	if requester == nil || answer != 1 || storedID != requesterID {
		return
	}
	info := requester
	if c.Mounted() || c.OlympiadMode() || c.ObserverMode() || c.NoSummonFriendZone() {
		return
	}
	if info.AlikeDead() || info.Operating() || info.Rooted() || info.InCombat() {
		return
	}
	if info.OlympiadMode() || info.FestivalParticipant() || info.Mounted() {
		return
	}
	if info.ObserverMode() || info.NoSummonFriendZone() {
		return
	}
	if skill.TargetConsumeID > 0 && skill.TargetConsumeCount > 0 {
		if c.ItemCount(skill.TargetConsumeID) < skill.TargetConsumeCount || !c.ConsumeItem(skill.TargetConsumeID, skill.TargetConsumeCount) {
			return
		}
	}
	x, y, z := info.Position()
	c.TeleportTo(x, y, z, 20)
}
