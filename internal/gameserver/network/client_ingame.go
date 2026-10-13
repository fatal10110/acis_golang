package network

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// handleRequestSkillCoolTime takes the reuse-timer request.
//
// This opcode is accepted and answered with nothing: reuse
// timers reach the client unsolicited (e.g. in the EnterWorld
// burst), never as a request reply.
func (l *GameClientLink) handleRequestSkillCoolTime(_ context.Context, cs *connState, _ struct{}) error {
	return nil
}

// handleAction selects the clicked object, or acts on it when it is
// already selected.
func (l *GameClientLink) handleAction(ctx context.Context, cs *connState, req clientpackets.Action) error {
	if cs.live == nil {
		return nil
	}
	onLive(cs.live, func() {
		// A plain click on the already-selected object acts on it
		// (attack, sit, pick up), exactly like an attack request —
		// the client sends this second click expecting the action
		// to resolve, and locks its own input until Attack or
		// ActionFailed answers it.
		if refuseObserverAction(cs.live) {
			return
		}
		if l.refuseFrozenDuellist(cs.live, req.ObjectID) {
			return
		}
		selected := cs.live.Target() != nil && cs.live.Target().ObjectID() == req.ObjectID
		l.handleTargetAction(ctx, cs.live, req.ObjectID, selected, false, req.Shift)
	})
	return nil
}

// handleAttackRequest attacks the selected object, or selects another one.
func (l *GameClientLink) handleAttackRequest(ctx context.Context, cs *connState, req clientpackets.AttackRequest) error {
	if cs.live == nil {
		return nil
	}
	onLive(cs.live, func() {
		if refuseObserverAttack(cs.live) {
			return
		}
		// A forced attack is an attack request on the object already
		// selected; on any other object it only selects it.
		selected := cs.live.Target() != nil && cs.live.Target().ObjectID() == req.ObjectID
		l.handleTargetAction(ctx, cs.live, req.ObjectID, selected, selected, req.Shift)
	})
	return nil
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
func (l *GameClientLink) handleMoveBackwardToLocation(_ context.Context, cs *connState, req clientpackets.MoveBackwardToLocation) error {
	session := cs.client.Session
	if cs.live == nil {
		return nil
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
func (l *GameClientLink) handleCannotMoveAnymore(_ context.Context, cs *connState, _ clientpackets.CannotMoveAnymore) error {
	if cs.live == nil {
		return nil
	}
	onLive(cs.live, func() { l.stopLivePlayer(cs.live) })
	return nil
}

// handleValidatePosition reconciles the client's idea of the player's
// position with the server's.
func (l *GameClientLink) handleValidatePosition(_ context.Context, cs *connState, req clientpackets.ValidatePosition) error {
	if cs.live != nil {
		onLive(cs.live, func() {
			l.validateLivePlayerPosition(cs.live, location.Location{X: int(req.X), Y: int(req.Y), Z: int(req.Z)}, req.BoatID)
		})
	}
	return nil
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
func (l *GameClientLink) handleRequestUnEquipItem(_ context.Context, cs *connState, req clientpackets.UnequipItem) error {
	if cs.live == nil {
		return nil
	}
	onLive(cs.live, func() { l.unequipItem(cs.live, req.BodySlot) })
	return nil
}

// handleRequestDestroyItem destroys inventory items.
func (l *GameClientLink) handleRequestDestroyItem(_ context.Context, cs *connState, req clientpackets.RequestDestroyItem) error {
	if cs.live == nil {
		return nil
	}
	onLive(cs.live, func() { l.destroyLiveItem(cs.live, req.ObjectID, int(req.Count)) })
	return nil
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
func (l *GameClientLink) handleRequestHennaItemList(_ context.Context, cs *connState, _ clientpackets.RequestHennaItemList) error {
	if cs.live != nil {
		onLive(cs.live, func() { l.sendHennaEquipList(cs.live) })
	}
	return nil
}

// handleRequestHennaUnequipList sends the dyes the player can remove.
func (l *GameClientLink) handleRequestHennaUnequipList(_ context.Context, cs *connState, _ clientpackets.RequestHennaUnequipList) error {
	if cs.live != nil {
		onLive(cs.live, func() { sendHennaUnequipList(cs.live) })
	}
	return nil
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
func (l *GameClientLink) handleRequestEnchantItem(ctx context.Context, cs *connState, req clientpackets.RequestEnchantItem) error {
	if cs.live == nil {
		return nil
	}
	onLive(cs.live, func() { l.enchantLiveItem(ctx, cs.live, req) })
	return nil
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
func (l *GameClientLink) handleRequestActionUse(ctx context.Context, cs *connState, req clientpackets.RequestActionUse) error {
	if cs.live == nil {
		return nil
	}
	onLive(cs.live, func() {
		if refuseObserverActionUse(cs.live) {
			return
		}
		switch req.ActionID {
		case actionSitStand:
			l.requestChangeWaitType(cs.live, !cs.live.Standing())
		case actionWalkRun:
			// A rider keeps its stance. The specified answer is
			// nothing, and the toggle leaves no client action
			// pending, so silence is the matching answer.
			if cs.live.Mounted() {
				return
			}
			l.changeLiveMoveType(cs.live, !cs.live.Running())
		case actionMountDismount:
			l.actionMountDismount(cs.live)
		default:
			if l.storeActionUse(cs.live, req.ActionID) {
				return
			}
			if !l.handleSummonActionUse(ctx, cs.live, req) {
				// An action-bar command no handler claims must still
				// answer the client — it locks its input until the
				// action resolves. The log keeps the gap visible
				// instead of silently dropped.
				l.log.Warn().Int32("action_id", req.ActionID).Int32("object_id", cs.live.ObjectID()).
					Msg("game client: action-bar command not implemented yet")
				cs.live.SendFrame(serverpackets.FrameActionFailed())
			}
		}
	})
	return nil
}

// handleRequestSocialAction plays a social animation.
func (l *GameClientLink) handleRequestSocialAction(_ context.Context, cs *connState, req clientpackets.RequestSocialAction) error {
	if cs.live != nil {
		onLive(cs.live, func() { l.broadcastLiveSocialAction(cs.live, req.ActionID) })
	}
	return nil
}

// handleRequestChangeMoveType switches between walking and running.
func (l *GameClientLink) handleRequestChangeMoveType(_ context.Context, cs *connState, req clientpackets.RequestChangeMoveType) error {
	if cs.live != nil {
		onLive(cs.live, func() {
			// A rider keeps its stance; silent for the same reasons
			// as the action-bar toggle above.
			if cs.live.Mounted() {
				return
			}
			l.changeLiveMoveType(cs.live, req.Run)
		})
	}
	return nil
}

// handleRequestChangeWaitType sits the player down or stands it up.
func (l *GameClientLink) handleRequestChangeWaitType(_ context.Context, cs *connState, req clientpackets.RequestChangeWaitType) error {
	if cs.live != nil {
		onLive(cs.live, func() { l.requestChangeWaitType(cs.live, req.Stand) })
	}
	return nil
}

// handleRequestLinkHTML opens a linked HTML page.
func (l *GameClientLink) handleRequestLinkHTML(_ context.Context, cs *connState, req clientpackets.RequestLinkHTML) error {
	onLive(cs.live, func() { l.requestLinkHTML(cs.live, req) })
	return nil
}

// handleRequestBypassToServer runs a bypass command.
func (l *GameClientLink) handleRequestBypassToServer(_ context.Context, cs *connState, req clientpackets.RequestBypassToServer) error {
	client := cs.client
	// An empty command returns before the bypass reuse gate is
	// consulted, so it never consumes the cooldown; a refused
	// command is dropped silently.
	if req.Command != "" && !client.performFloodProtected(floodProtectorServerBypass, l.playerConfig().ServerBypassDelay, time.Now()) {
		return nil
	}
	onLive(cs.live, func() { l.requestBypassToServer(cs.live, req) })
	l.finishPendingClassChange(cs.live)
	return nil
}

// handleAppearing completes a teleport once the client has loaded the
// destination.
func (l *GameClientLink) handleAppearing(_ context.Context, cs *connState, _ clientpackets.Appearing) error {
	if cs.live != nil {
		onLive(cs.live, func() {
			l.completeLivePlayerTeleport(cs.live)
			cs.live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(cs.live)))
		})
	}
	return nil
}

// handleObserverReturn ends observation mode.
func (l *GameClientLink) handleObserverReturn(_ context.Context, cs *connState, _ clientpackets.ObserverReturn) error {
	if cs.live != nil {
		onLive(cs.live, func() { l.observerReturn(cs.live) })
	}
	return nil
}

// handleStartRotating shows the player turning around.
func (l *GameClientLink) handleStartRotating(_ context.Context, cs *connState, req clientpackets.StartRotating) error {
	if cs.live == nil {
		return nil
	}
	onLive(cs.live, func() {
		l.broadcastLiveFrame(cs.live, func() wire.Frame {
			return serverpackets.FrameStartRotation(cs.live.ObjectID(), int(req.Degree), int(req.Side), 0)
		})
	})
	return nil
}

// handleFinishRotating sets the heading the player turned to.
func (l *GameClientLink) handleFinishRotating(_ context.Context, cs *connState, req clientpackets.FinishRotating) error {
	if cs.live == nil {
		return nil
	}
	onLive(cs.live, func() {
		cs.live.SetHeading(int(req.Degree))
		l.broadcastLiveFrame(cs.live, func() wire.Frame {
			return serverpackets.FrameStopRotation(cs.live.ObjectID(), int(req.Degree), 0)
		})
	})
	return nil
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
	list, err := l.sendCharSelectInfo(ctx, client)
	if err != nil {
		l.log.Error().Err(err).Msg("list characters")
		return errDisconnect
	}
	cs.chars = list
	return nil
}

// handleSendTimeCheck takes the client's time check, which answers nothing.
func (l *GameClientLink) handleSendTimeCheck(_ context.Context, cs *connState, _ clientpackets.SendTimeCheck) error {
	return nil
}

// handleRequestPackageItemList sends the items that can be sent as a
// package.
func (l *GameClientLink) handleRequestPackageItemList(_ context.Context, cs *connState, req clientpackets.RequestPackageSendableItemList) error {
	onLive(cs.live, func() { l.sendPackageSendableItemList(cs.live, req.ObjectID) })
	return nil
}

// handleRequestPetUseItem uses an item of the pet's inventory.
func (l *GameClientLink) handleRequestPetUseItem(ctx context.Context, cs *connState, req clientpackets.RequestPetUseItem) error {
	onLive(cs.live, func() { l.petUseItem(ctx, cs.live, req) })
	return nil
}

// handleRequestGiveItemToPet moves items to the pet's inventory.
func (l *GameClientLink) handleRequestGiveItemToPet(ctx context.Context, cs *connState, req clientpackets.RequestGiveItemToPet) error {
	onLive(cs.live, func() { l.giveItemToPet(ctx, cs.live, req) })
	return nil
}

// handleRequestGetItemFromPet moves items out of the pet's inventory.
func (l *GameClientLink) handleRequestGetItemFromPet(ctx context.Context, cs *connState, req clientpackets.RequestGetItemFromPet) error {
	onLive(cs.live, func() { l.getItemFromPet(ctx, cs.live, req) })
	return nil
}

// handleRequestPetGetItem has the pet pick up a ground item.
func (l *GameClientLink) handleRequestPetGetItem(ctx context.Context, cs *connState, req clientpackets.RequestPetGetItem) error {
	onLive(cs.live, func() { l.petGetItem(ctx, cs.live, req) })
	return nil
}

// serveTradeDone confirms or cancels the open trade.
func (l *GameClientLink) serveTradeDone(ctx context.Context, cs *connState, req clientpackets.TradeDone) error {
	if cs.live != nil {
		onLive(cs.live, func() { l.handleTradeDone(ctx, cs.live, req) })
	}
	return nil
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
func (l *GameClientLink) handleRequestShowMiniMap(_ context.Context, cs *connState, _ struct{}) error {
	if cs.live != nil {
		onLive(cs.live, func() { l.showMiniMap(cs.live, serverpackets.RegularMapID) })
	}
	return nil
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
func (l *GameClientLink) handleRequestUserCommand(_ context.Context, cs *connState, req clientpackets.RequestUserCommand) error {
	if cs.live != nil {
		onLive(cs.live, func() { l.requestUserCommand(cs.live, req.CommandID) })
	}
	return nil
}

// handleRequestQuestListInGame sends the player's quest list.
func (l *GameClientLink) handleRequestQuestListInGame(_ context.Context, cs *connState, _ struct{}) error {
	if cs.live != nil {
		onLive(cs.live, func() { cs.live.SendFrame(questListFrame(cs.live.Character)) })
	}
	return nil
}

// handleRequestQuestAbort abandons a quest.
func (l *GameClientLink) handleRequestQuestAbort(_ context.Context, cs *connState, req clientpackets.RequestQuestAbort) error {
	if cs.live != nil {
		onLive(cs.live, func() { l.abortQuest(cs.live, req.QuestID) })
	}
	return nil
}

// handleTutorialEvent passes a tutorial window event to the tutorial
// script by name.
func (l *GameClientLink) handleTutorialEvent(_ context.Context, cs *connState, req clientpackets.TutorialEvent) error {
	if cs.live != nil {
		onLive(cs.live, func() { l.tutorialRequest(cs.live, req.Name) })
	}
	return nil
}
