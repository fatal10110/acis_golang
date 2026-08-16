package player

import (
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// summonFriendCasterInfo is what ConfirmSummon and TeleportAnswer's accept
// path need from the untyped caster handed to TeleportRequest/ConfirmSummon:
// identity for the client-facing dialog and the requesterId anti-spoof
// check (Player.java:6917), and position for the eventual teleport
// (SummonFriend.teleportTo, Player.java:198).
type summonFriendCasterInfo interface {
	ObjectID() int32
	CharacterName() string
	Position() (int, int, int)
}

// summonFriendRequesterState is what TeleportAnswer's accept path needs to
// re-validate the pending requester, matching SummonFriend.teleportTo's own
// defensive checkSummoner/checkSummoned re-check at accept time
// (Player.java:6919 calling SummonFriend.java:183-199): teleportTo's
// `player` parameter (the one being teleported — the accepting character,
// `c` itself here) is re-checked via checkSummoner, and its `target`
// parameter (the original requester) via checkSummoned.
type summonFriendRequesterState interface {
	summonFriendCasterInfo
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

// SetSummonConfirmSender records the packet-layer hook that sends skill
// 1403's confirm-summon dialog (ConfirmDlg,
// S1_WISHES_TO_SUMMON_YOU_FROM_S2_DO_YOU_ACCEPT).
func (c *Character) SetSummonConfirmSender(send func(casterName string, casterID int32, x, y, z int, timeout time.Duration)) {
	c.summonFriendMu.Lock()
	defer c.summonFriendMu.Unlock()
	c.sendSummonConfirm = send
}

// SetTeleportHook records the packet-layer hook that relocates this
// character discontinuously (ground-snap, attack/cast stop,
// TeleportToLocation broadcast), used by the plain SUMMON_FRIEND/SUMMON_PARTY
// path and by TeleportAnswer's accept path.
func (c *Character) SetTeleportHook(hook func(x, y, z, radius int)) {
	c.summonFriendMu.Lock()
	defer c.summonFriendMu.Unlock()
	c.teleportHook = hook
}

// TeleportTo is summonFriendTraveler's entry point
// (handler/skill/summon.go), matching Player.teleportTo
// (Player.java:196-198).
func (c *Character) TeleportTo(x, y, z, radius int) {
	c.summonFriendMu.Lock()
	hook := c.teleportHook
	c.summonFriendMu.Unlock()
	if hook != nil {
		hook(x, y, z, radius)
	}
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
func (c *Character) TeleportRequest(caster any, skill modelskill.Definition) bool {
	c.summonFriendMu.Lock()
	defer c.summonFriendMu.Unlock()
	if c.summonRequester != nil && caster != nil {
		return false
	}
	c.summonRequester = caster
	c.summonSkill = skill
	if info, ok := caster.(summonFriendCasterInfo); ok {
		c.summonRequesterID = info.ObjectID()
	} else {
		c.summonRequesterID = 0
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
func (c *Character) ConfirmSummon(caster any, skill modelskill.Definition, timeout time.Duration) {
	info, ok := caster.(summonFriendCasterInfo)
	if !ok {
		return
	}
	c.summonFriendMu.Lock()
	send := c.sendSummonConfirm
	c.summonFriendMu.Unlock()
	if send == nil {
		return
	}
	x, y, z := info.Position()
	send(info.CharacterName(), info.ObjectID(), x, y, z, timeout)
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
	info, ok := requester.(summonFriendRequesterState)
	if !ok {
		return
	}
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
