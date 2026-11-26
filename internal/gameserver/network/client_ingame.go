package network

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// handleAction selects the clicked object, or acts on it when it is
// already selected.
func (l *GameClientLink) handleAction(ctx context.Context, live *livePlayer, req clientpackets.Action) {
	// A plain click on the already-selected object acts on it
	// (attack, sit, pick up), exactly like an attack request —
	// the client sends this second click expecting the action
	// to resolve, and locks its own input until Attack or
	// ActionFailed answers it.
	if refuseObserverAction(live) {
		return
	}
	// A player holding a request it has not answered may click
	// nothing: ActionFailed alone, before the object is resolved.
	if l.holdsActiveRequester(live) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if l.refuseFrozenDuellist(live, req.ObjectID) {
		return
	}
	selected := live.Target() != nil && live.Target().ObjectID() == req.ObjectID
	l.handleTargetAction(ctx, live, req.ObjectID, selected, false, req.Shift)
}

// handleAttackRequest attacks the selected object, or selects another one.
func (l *GameClientLink) handleAttackRequest(ctx context.Context, live *livePlayer, req clientpackets.AttackRequest) {
	// A player out of control may send no attack request at all: the
	// whole packet is refused before the object is resolved, ahead of
	// the observer refusal, so it neither selects, picks up nor
	// interacts.
	if liveOutOfControl(live) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if refuseObserverAttack(live) {
		return
	}
	// A forced attack is an attack request on the object already
	// selected; on any other object it only selects it.
	selected := live.Target() != nil && live.Target().ObjectID() == req.ObjectID
	l.handleTargetAction(ctx, live, req.ObjectID, selected, selected, req.Shift)
}

// handleLogout takes the player out of the world and closes the connection,
// unless something blocks the exit.
func (l *GameClientLink) handleLogout(_ context.Context, cs *connState, _ struct{}) error {
	session := cs.client.Session
	if cs.live == nil {
		// Logout from character select has no character to take out
		// of the world: it is ignored silently and the connection
		// stays open. The client registers no pending action for
		// it, so there is nothing to release.
		return nil
	}
	refused := false
	onLive(cs.live, func() {
		if block := l.exitBlockReason(cs.live); block != exitAllowed {
			l.refuseExit(session, cs.live, block, false)
			refused = true
			return
		}
		l.revokeBossEntries(cs.live)
		// LeaveWorld is the last packet: what detach sends
		// afterwards never reaches the client.
		session.sendLast(serverpackets.FrameLeaveWorld())
	})
	if refused {
		return nil
	}
	return errDisconnect
}

// handleMoveBackwardToLocation moves the player towards a clicked point.
// With walker protection on, a request missing its movement mode, which
// only a bot client sends, ends the session with ServerClose.
func (l *GameClientLink) handleMoveBackwardToLocation(_ context.Context, cs *connState, req clientpackets.MoveBackwardToLocation) error {
	session := cs.client.Session
	if cs.live == nil {
		return nil
	}
	if req.MovementMissing && l.chat.WalkerProtection {
		cs.client.closeNow()
		return errDisconnect
	}
	if req.MoveMovement == 0 {
		session.SendFrame(serverpackets.FrameActionFailed())
		return nil
	}
	onLive(cs.live, func() {
		l.moveLivePlayer(cs.live,
			location.Location{X: int(req.TargetX), Y: int(req.TargetY), Z: int(req.TargetZ)},
			location.Location{X: int(req.OriginX), Y: int(req.OriginY), Z: int(req.OriginZ)},
		)
	})
	return nil
}

// handleCannotMoveAnymore stops the player's walk where the server-side
// walk stands.
func (l *GameClientLink) handleCannotMoveAnymore(live *livePlayer, _ clientpackets.CannotMoveAnymore) {
	l.stopLivePlayer(live)
}

// handleValidatePosition reconciles the client's idea of the player's
// position with the server's.
func (l *GameClientLink) handleValidatePosition(live *livePlayer, req clientpackets.ValidatePosition) {
	l.validateLivePlayerPosition(live, location.Location{X: int(req.X), Y: int(req.Y), Z: int(req.Z)}, req.BoatID)
}

// handleRequestItemList sends the inventory window.
func (l *GameClientLink) handleRequestItemList(_ context.Context, cs *connState, _ struct{}) error {
	session := cs.client.Session
	if cs.live == nil {
		return nil
	}
	// Build and send on the player's queue. Handing the frame back
	// to the connection to send would let an inventory drain queued
	// behind this task overtake the snapshot it supersedes, leaving
	// the client on stale counts until that item changes again.
	var failed bool
	onLive(cs.live, func() {
		// A shop or warehouse window just opened keeps the request
		// unanswered, as specified: no list, no
		// ActionFailed. The inventory button leaves no client
		// action pending.
		if cs.live.inventoryDisabled.Load() {
			return
		}
		// Carried weight is recomputed on every item-list send, not
		// only at login.
		if inv := cs.live.Inventory(); inv != nil {
			inv.UpdateWeight()
		}
		// Building through the inventory drops the pending update
		// queue the snapshot supersedes, so no InventoryUpdate for
		// those same deltas follows the full list.
		//
		// showWindow is true here and false in the EnterWorld burst:
		// an on-demand request is the player asking for the inventory
		// window, while the login snapshot only seeds item state and
		// must not pop the window open.
		frame, err := cs.live.buildItemList(l.itemTemplates, true)
		if err != nil {
			l.log.Error().Err(err).Msg("build ItemList")
			failed = true
			return
		}
		session.SendFrame(frame)
	})
	if failed {
		return errDisconnect
	}
	return nil
}

// handleUseItem uses an inventory item.
func (l *GameClientLink) handleUseItem(_ context.Context, cs *connState, req clientpackets.UseItem) error {
	client := cs.client
	if cs.live == nil {
		return nil
	}
	// The dice gate belongs to this read loop, which stays parked
	// in onLive while useItem runs, so the throw consults it there.
	rollDice := func() bool {
		return client.performFloodProtected(floodProtectorRollDice, l.playerConfig().RollDiceDelay, time.Now())
	}
	onLive(cs.live, func() { l.useItem(cs.live, req.ObjectID, req.CtrlPressed, rollDice) })
	return nil
}

// handleRequestUnEquipItem takes off what a body slot holds.
func (l *GameClientLink) handleRequestUnEquipItem(live *livePlayer, req clientpackets.UnequipItem) {
	l.unequipItem(live, req.BodySlot)
}

// handleRequestDestroyItem destroys inventory items.
func (l *GameClientLink) handleRequestDestroyItem(live *livePlayer, req clientpackets.RequestDestroyItem) {
	l.destroyLiveItem(live, req.ObjectID, int(req.Count))
}

// handleRequestRecipeItemMakeSelf crafts a recipe for the player itself.
func (l *GameClientLink) handleRequestRecipeItemMakeSelf(_ context.Context, cs *connState, req clientpackets.RequestRecipeItemMakeSelf) error {
	client := cs.client
	// A craft inside the manufacture reuse window is dropped
	// silently, before anything else is looked at.
	if !client.performFloodProtected(floodProtectorManufacture, l.playerConfig().ManufactureDelay, time.Now()) {
		return nil
	}
	if cs.live != nil {
		onLive(cs.live, func() { l.makeRecipeSelf(cs.live, req) })
	}
	return nil
}

// handleRequestHennaItemList sends the dyes the player can draw.
func (l *GameClientLink) handleRequestHennaItemList(live *livePlayer, _ clientpackets.RequestHennaItemList) {
	l.sendHennaEquipList(live)
}

// handleRequestHennaUnequipList sends the dyes the player can remove.
func (l *GameClientLink) handleRequestHennaUnequipList(live *livePlayer, _ clientpackets.RequestHennaUnequipList) {
	sendHennaUnequipList(live)
}

// handleMultiSellChoose makes a multisell exchange.
func (l *GameClientLink) handleMultiSellChoose(_ context.Context, cs *connState, req clientpackets.MultiSellChoose) error {
	client := cs.client
	if cs.live != nil {
		// The reuse gate is consulted only with a player in the
		// world; a refused exchange still drops the open list.
		allowed := client.performFloodProtected(floodProtectorMultisell, l.playerConfig().MultisellDelay, time.Now())
		onLive(cs.live, func() { l.requestMultiSellChoose(cs.live, req, allowed) })
	}
	return nil
}

// handleRequestEnchantItem enchants an item with the scroll in use.
func (l *GameClientLink) handleRequestEnchantItem(ctx context.Context, live *livePlayer, req clientpackets.RequestEnchantItem) {
	l.enchantLiveItem(ctx, live, req)
}

// handleRequestSkillList sends the player's skill list.
//
// While entering, 0x3f is the quest-list request the client
// sends during loading: it is answered with the selected
// character's quest list, or the quest panel stays empty.
func (l *GameClientLink) handleRequestSkillList(_ context.Context, cs *connState, _ struct{}) error {
	client, session := cs.client, cs.client.Session
	if client.State() == StateEntering {
		if cs.entering != nil {
			session.SendFrame(questListFrame(cs.entering.Character))
		}
		return nil
	}
	if cs.live == nil {
		return nil
	}
	onLive(cs.live, func() {
		session.SendFrame(serverpackets.FrameSkillList(skillListEntries(cs.live.Character, l.skills)))
	})
	return nil
}

// handleRequestActionUse runs an action-bar command.
func (l *GameClientLink) handleRequestActionUse(ctx context.Context, live *livePlayer, req clientpackets.RequestActionUse) {
	// A dead or out-of-control player is only released, whatever the
	// action; a fake-dead one too, unless it sits or stands.
	if (live.FakeDead() && req.ActionID != actionSitStand) || live.Dead() || liveOutOfControl(live) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if refuseObserverActionUse(live) {
		return
	}
	switch req.ActionID {
	case actionSitStand:
		l.requestChangeWaitType(live, !live.Standing())
	case actionWalkRun:
		// A rider keeps its stance. The specified answer is
		// nothing, and the toggle leaves no client action
		// pending, so silence is the matching answer.
		if live.Mounted() {
			return
		}
		l.changeLiveMoveType(live, !live.Running())
	case actionMountDismount:
		l.actionMountDismount(live)
	default:
		if l.storeActionUse(live, req.ActionID) {
			return
		}
		if !l.handleSummonActionUse(ctx, live, req) {
			// An action-bar command no handler claims must still
			// answer the client — it locks its input until the
			// action resolves. The log keeps the gap visible
			// instead of silently dropped.
			l.log.Warn().Int32("action_id", req.ActionID).Int32("object_id", live.ObjectID()).
				Msg("game client: action-bar command not implemented yet")
			live.SendFrame(serverpackets.FrameActionFailed())
		}
	}
}

// handleRequestSocialAction plays a social animation. Every request first
// spends the emote reuse gate; one it refuses answers nothing, like the
// other silent emote rejections (see broadcastLiveSocialAction).
func (l *GameClientLink) handleRequestSocialAction(_ context.Context, cs *connState, req clientpackets.RequestSocialAction) error {
	if cs.live == nil {
		return nil
	}
	client, live := cs.client, cs.live
	// The gate belongs to this read loop, which stays parked in onLive
	// while this runs; it is timed on the player's queue clock.
	onLive(live, func() {
		if client.performFloodProtected(floodProtectorSocial, l.playerConfig().SocialDelay, live.Now()) {
			l.broadcastLiveSocialAction(live, req.ActionID)
		}
	})
	return nil
}

// handleRequestChangeMoveType switches between walking and running.
func (l *GameClientLink) handleRequestChangeMoveType(live *livePlayer, req clientpackets.RequestChangeMoveType) {
	// A rider keeps its stance; silent for the same reasons
	// as the action-bar toggle above.
	if live.Mounted() {
		return
	}
	l.changeLiveMoveType(live, req.Run)
}

// handleRequestChangeWaitType sits the player down or stands it up.
func (l *GameClientLink) handleRequestChangeWaitType(live *livePlayer, req clientpackets.RequestChangeWaitType) {
	l.requestChangeWaitType(live, req.Stand)
}

// handleRequestLinkHTML opens a linked HTML page.
func (l *GameClientLink) handleRequestLinkHTML(live *livePlayer, req clientpackets.RequestLinkHTML) {
	l.requestLinkHTML(live, req)
}

// handleRequestBypassToServer runs a bypass command.
func (l *GameClientLink) handleRequestBypassToServer(ctx context.Context, cs *connState, req clientpackets.RequestBypassToServer) error {
	client, live := cs.client, cs.live
	refused := false
	var lookups []func(context.Context)
	// An empty command returns before the bypass reuse gate is
	// consulted, so it never consumes the cooldown; a refused
	// command is dropped silently. The gate belongs to this read loop,
	// which stays parked in onLive while this runs; in the world it is
	// timed on the player's queue clock.
	onLive(live, func() {
		now := time.Now()
		if live != nil {
			now = live.Now()
		}
		if req.Command != "" && !client.performFloodProtected(floodProtectorServerBypass, l.playerConfig().ServerBypassDelay, now) {
			refused = true
			return
		}
		l.requestBypassToServer(live, req)
		if live != nil {
			lookups = live.takeLookups()
		}
	})
	if refused {
		return nil
	}
	// The lookups the command deferred run here, off the queue; see
	// deferLookup.
	if live != nil {
		live.runLookups(ctx, lookups)
	}
	l.finishPendingClassChange(live)
	return nil
}

// handleAppearing completes a teleport once the client has loaded the
// destination.
func (l *GameClientLink) handleAppearing(live *livePlayer, _ clientpackets.Appearing) {
	deferred := l.completeLivePlayerTeleport(live)
	live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	for _, teleport := range deferred {
		teleport()
	}
}

// handleObserverReturn ends observation mode.
func (l *GameClientLink) handleObserverReturn(live *livePlayer, _ clientpackets.ObserverReturn) {
	l.observerReturn(live)
}

// handleStartRotating shows the player turning around.
func (l *GameClientLink) handleStartRotating(live *livePlayer, req clientpackets.StartRotating) {
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameStartRotation(live.ObjectID(), int(req.Degree), int(req.Side), 0)
	})
}

// handleFinishRotating shows the end of the player's turn to everyone around.
// The degree is the client's claim and only echoed: the server-side heading,
// which every behind and front check reads, is left as it was.
func (l *GameClientLink) handleFinishRotating(live *livePlayer, req clientpackets.FinishRotating) {
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameStopRotation(live.ObjectID(), int(req.Degree), 0)
	})
}

// handleRequestRestart takes the player out of the world and back to
// character select, unless something blocks the exit.
func (l *GameClientLink) handleRequestRestart(ctx context.Context, cs *connState, _ struct{}) error {
	client, session := cs.client, cs.client.Session
	if cs.live == nil {
		return nil
	}
	// The exit check and detach run on the player's queue; waiting for
	// the saves detach queued stays on this goroutine.
	var owners []int32
	refused := false
	// This is the one dispatch site that clears live, so a panic
	// here must be acted on before that happens: past `cs.live = nil`
	// the guard at the top of the loop can never fire again, the
	// session would stay open with a half-detached character still
	// registered in the world, and Handle's deferred detach would
	// be skipped too. Leaving live in place and taking the guard
	// instead ends the session and finishes the teardown the
	// panicking task abandoned.
	if !onLive(cs.live, func() {
		if block := l.exitBlockReason(cs.live); block != exitAllowed {
			l.refuseExit(session, cs.live, block, true)
			refused = true
			return
		}
		l.revokeBossEntries(cs.live)
		cs.live.Character.DetachSession()
		owners = l.detachLivePlayer(cs.live)
	}) {
		return nil
	}
	if refused {
		return nil
	}
	_ = l.awaitPersistence(cs.conn, owners...)
	cs.live = nil
	cs.entering = nil
	client.SetState(StateAuthed)
	session.SendFrame(serverpackets.FrameRestartResponse(true))
	cs.chars = l.sendCharSelectInfo(ctx, client, highlightLastPlayed)
	return nil
}

// handleSendTimeCheck takes the client's time check, which answers nothing.
func (l *GameClientLink) handleSendTimeCheck(_ context.Context, cs *connState, _ clientpackets.SendTimeCheck) error {
	return nil
}

// handleRequestPackageItemList sends the items that can be sent as a
// package.
func (l *GameClientLink) handleRequestPackageItemList(live *livePlayer, req clientpackets.RequestPackageSendableItemList) {
	l.sendPackageSendableItemList(live, req.ObjectID)
}

// handleRequestPetUseItem uses an item of the pet's inventory.
func (l *GameClientLink) handleRequestPetUseItem(ctx context.Context, live *livePlayer, req clientpackets.RequestPetUseItem) {
	l.petUseItem(ctx, live, req)
}

// handleRequestGiveItemToPet moves items to the pet's inventory.
func (l *GameClientLink) handleRequestGiveItemToPet(ctx context.Context, live *livePlayer, req clientpackets.RequestGiveItemToPet) {
	l.giveItemToPet(ctx, live, req)
}

// handleRequestGetItemFromPet moves items out of the pet's inventory.
func (l *GameClientLink) handleRequestGetItemFromPet(ctx context.Context, live *livePlayer, req clientpackets.RequestGetItemFromPet) {
	l.getItemFromPet(ctx, live, req)
}

// handleRequestPetGetItem has the pet pick up a ground item.
func (l *GameClientLink) handleRequestPetGetItem(ctx context.Context, live *livePlayer, req clientpackets.RequestPetGetItem) {
	l.petGetItem(ctx, live, req)
}

// serveTradeDone confirms or cancels the open trade.
func (l *GameClientLink) serveTradeDone(ctx context.Context, live *livePlayer, req clientpackets.TradeDone) {
	l.handleTradeDone(ctx, live, req)
}

// serveRequestChangePetName names the player's pet.
func (l *GameClientLink) serveRequestChangePetName(ctx context.Context, cs *connState, req clientpackets.RequestChangePetName) error {
	if cs.live != nil {
		// Runs its own queue hops: the pets-table read in the middle
		// stays here, off the queue (see handleRequestChangePetName).
		l.handleRequestChangePetName(ctx, cs.live, req)
	}
	return nil
}

// handleRequestBuySeed buys seeds from the manor.
func (l *GameClientLink) handleRequestBuySeed(_ context.Context, cs *connState, req clientpackets.RequestBuySeed) error {
	client := cs.client
	// The reuse gate is spent before anything else, with or without
	// a player in the world. A gated purchase answers nothing, as
	// specified: the seed window's submit leaves no client action
	// pending.
	if !client.performFloodProtected(floodProtectorManor, l.playerConfig().ManorDelay, time.Now()) {
		return nil
	}
	if cs.live != nil {
		onLive(cs.live, func() { l.requestBuySeed(cs.live, req) })
	}
	return nil
}

// handleRequestBuyProcure takes a crop sale request.
//
// No window the server opens sends it, and no request decodes:
// the refusal answers nothing, as specified, and leaves no
// client action pending.
func (l *GameClientLink) handleRequestBuyProcure(_ context.Context, cs *connState, req clientpackets.RequestBuyProcure) error {
	l.log.Warn().Int("rows", len(req.Items)).Int32("manor", req.ManorID).Msg("RequestBuyProcure: normally unused")
	return nil
}

// handleRequestShowMiniMap opens the map.
//
// The request carries no body; with no player in the world
// nothing answers, as the specified handler does.
func (l *GameClientLink) handleRequestShowMiniMap(live *livePlayer) {
	l.showMiniMap(live, serverpackets.RegularMapID)
}

// serveSay2 sends a chat message.
func (l *GameClientLink) serveSay2(_ context.Context, cs *connState, req clientpackets.Say2) error {
	client := cs.client
	if cs.live != nil {
		onLive(cs.live, func() { l.handleSay2(client, cs.live, req) })
	}
	return nil
}

// handleRequestUserCommand runs a numbered user command.
func (l *GameClientLink) handleRequestUserCommand(live *livePlayer, req clientpackets.RequestUserCommand) {
	l.requestUserCommand(live, req.CommandID)
}

// handleRequestQuestListInGame sends the player's quest list.
func (l *GameClientLink) handleRequestQuestListInGame(live *livePlayer) {
	live.SendFrame(questListFrame(live.Character))
}

// handleRequestQuestAbort abandons a quest.
func (l *GameClientLink) handleRequestQuestAbort(live *livePlayer, req clientpackets.RequestQuestAbort) {
	l.abortQuest(live, req.QuestID)
}

// handleTutorialEvent passes a tutorial window event to the tutorial
// script by name.
func (l *GameClientLink) handleTutorialEvent(live *livePlayer, req clientpackets.TutorialEvent) {
	l.tutorialRequest(live, req.Name)
}
