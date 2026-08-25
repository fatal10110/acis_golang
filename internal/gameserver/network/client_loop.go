package network

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	gamecipher "github.com/fatal10110/acis_golang/internal/gameserver/network/cipher"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Action-bar command ids carried by an action-use request that toggle the
// player's own stance rather than command a summon.
const (
	actionSitStand int32 = 0
	actionWalkRun  int32 = 1
)

var errMalformedPacketDisconnect = errors.New("malformed packet requires disconnect")

// decodeClientPacket disconnects buffer-underflow-equivalent pre-auth
// packets (insufficient bytes, wire.ErrShortPacket) immediately. Once
// authenticated, that same error class (char-select or in-game) is
// tolerated up to maxUnderflowsPerMin within a 60s sliding window, then
// disconnect, mirroring GameClient.onBufferUnderflow. Other decode
// validation errors (e.g. an out-of-range field) are logged and the packet
// dropped without counting or disconnecting, matching
// L2GameClientPacket.read()'s catch-all branch.
func decodeClientPacket[T any](l *GameClientLink, client *Client, payload []byte, decode func([]byte) (T, error)) (T, error) {
	req, err := decode(payload)
	if err != nil {
		l.log.Warn().Err(err).Msg("game client")
		if errors.Is(err, wire.ErrShortPacket) && (client.State() == StateConnected || client.countUnderflow()) {
			return req, errMalformedPacketDisconnect
		}
	}
	return req, err
}

// Handle drives one game-client connection end to end. It matches Serve's
// handle signature, so a caller wires it in directly:
// network.Serve(ctx, ln, link.Handle, log).
func (l *GameClientLink) Handle(ctx context.Context, conn *Conn) {
	key, err := l.newCipherKey()
	if err != nil {
		l.log.Error().Err(err).Msg("generate game cipher key")
		return
	}
	gameCipher, err := gamecipher.NewCipher(key)
	if err != nil {
		l.log.Error().Err(err).Msg("build game cipher")
		return
	}
	session := NewSession(conn, gameCipher)
	client := NewClient(session)

	// chars and entering are read entirely by this goroutine: they resolve
	// the character-list slot indices RequestCharacterDelete,
	// CharacterRestore and RequestGameStart address, and the character
	// RequestGameStart selected for EnterWorld to finish spawning.
	var chars []*player.Character
	var entering *player.Character
	var live *livePlayer
	defer func() {
		l.detachLivePlayer(ctx, live)
		l.notifyPlayerLogout(client.AccountName())
	}()

	for {
		payload, err := session.ReadFrame()
		if err != nil {
			if normalReadFrameError(err) {
				l.log.Debug().Err(err).Msg("Read frame")
			} else {
				l.log.Error().Err(err).Msg("Read frame")
			}
			return
		}
		if len(payload) == 0 {
			return
		}
		opcode := payload[0]
		if !client.Accept(opcode) {
			l.log.Warn().Str("state", client.State().String()).Str("opcode", hex.EncodeToString(payload)).Msg("Accept opcode")
			// Pre-auth, any rejected opcode disconnects immediately; once
			// authenticated, tolerate up to maxUnknownPerMin within a 60s
			// sliding window, mirroring GameClient.onUnknownPacket.
			if client.State() == StateConnected || client.countUnknownPacket() {
				return
			}
			continue
		}

		if clearsSpawnProtection(opcode) {
			l.clearSpawnProtectionOnAction(live)
		}
		switch opcode {
		case clientpackets.OpcodeProtocolVersion:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeProtocolVersion)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if !validProtocolRevision(req.Revision) {
				return
			}
			if !session.SendFrame(serverpackets.FrameVersionCheck(key)) {
				return
			}
			// The key is out; every later frame crosses encrypted, matching
			// the reference where crypt starts only after VersionCheck.
			session.EnableCrypt()

		case clientpackets.OpcodeAuthLogin:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeAuthLogin)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			ok, err := l.authenticate(ctx, client, req)
			if err != nil || !ok {
				return
			}
			list, err := l.sendCharSelectInfo(ctx, client)
			if err != nil {
				l.log.Error().Err(err).Str("account", client.AccountName()).Msg("list characters")
				return
			}
			chars = list

		case clientpackets.OpcodeRequestCharacterCreate:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestCharacterCreate)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			sex, err := player.ParseSex(req.Sex)
			if err != nil {
				session.SendFrame(serverpackets.FrameCharCreateFail(serverpackets.CharCreateFailReasonCreationFailed))
				continue
			}
			_, outcome, err := l.roster.Create(ctx, client.AccountName(), manager.CreateRequest{
				Name: req.Name, ClassID: int(req.ClassID), Race: int(req.Race), Sex: sex,
				HairStyle: req.HairStyle, HairColor: req.HairColor, Face: req.Face,
			})
			if err != nil {
				l.log.Error().Err(err).Str("account", client.AccountName()).Msg("create character")
				return
			}
			if outcome != manager.CreateOK {
				session.SendFrame(serverpackets.FrameCharCreateFail(createFailReason(outcome)))
				continue
			}
			session.SendFrame(serverpackets.FrameCharCreateOk())
			list, err := l.sendCharSelectInfo(ctx, client)
			if err != nil {
				l.log.Error().Err(err).Msg("list characters")
				return
			}
			chars = list

		case clientpackets.OpcodeRequestCharacterDelete:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestCharacterDelete)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			c, ok := slotCharacter(chars, req.Slot)
			if !ok {
				// An unknown slot sends no failure reply — only the
				// refreshed character list every delete attempt ends with.
				list, err := l.sendCharSelectInfo(ctx, client)
				if err != nil {
					l.log.Error().Err(err).Msg("list characters")
					return
				}
				chars = list
				continue
			}
			if err := l.roster.MarkForDeletion(ctx, c.ID); err != nil {
				l.log.Error().Err(err).Msg("mark character for deletion")
				session.SendFrame(serverpackets.FrameCharDeleteFail(serverpackets.CharDeleteFailReasonDeletionFailed))
			} else {
				session.SendFrame(serverpackets.FrameCharDeleteOk())
			}
			list, err := l.sendCharSelectInfo(ctx, client)
			if err != nil {
				l.log.Error().Err(err).Msg("list characters")
				return
			}
			chars = list

		case clientpackets.OpcodeCharacterRestore:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeCharacterRestore)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if c, ok := slotCharacter(chars, req.Slot); ok {
				if err := l.roster.Restore(ctx, c.ID); err != nil {
					l.log.Error().Err(err).Msg("restore character")
				}
			}
			list, err := l.sendCharSelectInfo(ctx, client)
			if err != nil {
				l.log.Error().Err(err).Msg("list characters")
				return
			}
			chars = list

		case clientpackets.OpcodeRequestPledgeCrest:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPledgeCrest)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			session.SendFrame(l.framePledgeCrest(req))

		case clientpackets.OpcodeRequestAllyCrest:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestAllyCrest)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live == nil {
				continue
			}
			if frame, ok := l.frameAllyCrest(req); ok {
				session.SendFrame(frame)
			}

		case clientpackets.OpcodeRequestNewCharacter:
			frame, err := serverpackets.FrameNewCharacterSuccess(l.templates)
			if err != nil {
				l.log.Error().Err(err).Msg("build NewCharacterSuccess")
				return
			}
			session.SendFrame(frame)

		case clientpackets.OpcodeRequestGameStart:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestGameStart)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			c, ok := slotCharacter(chars, req.Slot)
			// An unknown slot or a banned character (AccessLevel < 0) aborts
			// the selection silently; the connection stays open.
			if !ok || c.AccessLevel < 0 {
				continue
			}
			// A character already in the world belongs to another live
			// session: that session is closed with ServerClose and this
			// selection aborts silently, matching loadCharFromDisk's
			// existing-player branch.
			if l.world != nil {
				if obj, exists := l.world.Player(c.ObjectID()); exists {
					if prev, isLive := obj.(*livePlayer); isLive {
						l.log.Info().Int32("object_id", c.ObjectID()).
							Msg("game client: duplicate character login, closing previous session")
						prev.kickClient()
					}
					continue
				}
			}
			tmpl, ok := l.templates.Get(c.ClassID)
			if !ok {
				l.log.Error().Int("class_id", c.ClassID).Msg("select character: no template loaded")
				return
			}
			session.SendFrame(serverpackets.FrameSSQInfo())
			client.SetState(StateEntering)
			session.SendFrame(serverpackets.FrameCharSelected(serverpackets.CharSelectedSnapshot{
				Character: c, Template: tmpl, SessionID: client.SessionKey().PlayKey1,
				GameTime: l.gameTime(),
			}))
			entering = c

		case clientpackets.OpcodeEnterWorld:
			if entering == nil {
				return
			}
			entered, ok := l.enterWorld(ctx, client, entering)
			if !ok {
				return
			}
			live = entered
			client.SetState(StateInGame)

		case clientpackets.OpcodeExtended:
			r := wire.NewReader(payload[1:])
			second := r.ReadUint16()
			if r.Err() != nil {
				l.log.Warn().Str("state", client.State().String()).Msg("game client: extended opcode missing")
				continue
			}
			// While entering, only the manor-list sub-opcode is dispatched;
			// every other one counts toward the unknown-packet disconnect
			// threshold instead of being absorbed by an in-game no-op.
			if client.State() == StateEntering && second != clientpackets.OpcodeRequestManorList {
				l.log.Info().
					Uint16("opcode2", second).
					Str("state", client.State().String()).
					Msg("game client: accepted extended opcode not handled while entering")
				if client.countUnknownPacket() {
					return
				}
				continue
			}
			switch second {
			case clientpackets.OpcodeRequestAutoSoulShot:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestAutoSoulShot)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if live != nil {
					l.handleAutoSoulShot(live, req)
				}
			case clientpackets.OpcodeRequestExEnchantSkillInfo:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestExEnchantSkillInfo)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if live != nil {
					l.sendEnchantSkillInfo(live, req)
				}
			case clientpackets.OpcodeRequestExEnchantSkill:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestExEnchantSkill)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if live != nil {
					l.applyEnchantSkill(ctx, live, req)
				}
			case clientpackets.OpcodeRequestManorList:
				session.SendFrame(serverpackets.FrameExSendManorList())
			case clientpackets.OpcodeRequestExPledgeCrestLarge:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestExPledgeCrestLarge)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if live == nil {
					continue
				}
				if frame, ok := l.frameExPledgeCrestLarge(req); ok {
					session.SendFrame(frame)
				}
			case clientpackets.OpcodeRequestCursedWeaponList:
				if live == nil {
					continue
				}
				session.SendFrame(serverpackets.FrameExCursedWeaponList(l.cursedWeapons.IDs()))
			case clientpackets.OpcodeRequestExMagicSkillUseGround:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestExMagicSkillUseGround)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if live != nil {
					l.handleMagicSkillUseGround(live, req)
				}
			case clientpackets.OpcodeRequestCursedWeaponLocation:
				if live == nil {
					continue
				}
				// The location-list reply is not implemented yet; log so
				// the accepted request is never a silent no-op.
				l.log.Warn().Int32("object_id", live.ObjectID()).
					Msg("game client: cursed-weapon location request not implemented yet")
			default:
				l.log.Info().
					Uint16("opcode2", second).
					Str("state", client.State().String()).
					Msg("game client: accepted extended opcode not implemented yet")
				// OpcodeExtended is only ever allowed in StateEntering/StateInGame
				// (see allowedOpcodes), so unlike the top-level Accept gate there is
				// no pre-auth immediate-disconnect case here; just count toward the
				// same sliding-60s threshold as GameClient.onUnknownPacket.
				if client.countUnknownPacket() {
					return
				}
			}

		case clientpackets.OpcodeRequestSkillCoolTime:
			// The reference registers an empty case: reuse timers reach the
			// client unsolicited (e.g. in the EnterWorld burst), never as a
			// request reply.
			continue

		case clientpackets.OpcodeRequestMagicSkillUse:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestMagicSkillUse)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.handleMagicSkillUse(live, req)
			}

		case clientpackets.OpcodeAction:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeAction)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live == nil {
				continue
			}
			// A plain click on the already-selected object acts on it
			// (attack, sit, pick up), exactly like an attack request —
			// the client sends this second click expecting the action to
			// resolve, and locks its own input until Attack or
			// ActionFailed answers it.
			selected := live.Target() != nil && live.Target().ObjectID() == req.ObjectID
			l.handleTargetAction(ctx, live, req.ObjectID, selected, req.Shift)

		case clientpackets.OpcodeAttackRequest:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeAttackRequest)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live == nil {
				continue
			}
			selected := live.Target() != nil && live.Target().ObjectID() == req.ObjectID
			l.handleTargetAction(ctx, live, req.ObjectID, selected, req.Shift)

		case clientpackets.OpcodeLogout:
			if live != nil {
				session.SendFrame(serverpackets.FrameLeaveWorld())
				return
			}
			return

		case clientpackets.OpcodeMoveBackwardToLocation:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeMoveBackwardToLocation)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live == nil {
				continue
			}
			if req.MoveMovement == 0 {
				session.SendFrame(serverpackets.FrameActionFailed())
				continue
			}
			l.moveLivePlayer(live,
				location.Location{X: int(req.TargetX), Y: int(req.TargetY), Z: int(req.TargetZ)},
			)

		case clientpackets.OpcodeCannotMoveAnymore:
			if _, err := decodeClientPacket(l, client, payload, clientpackets.DecodeCannotMoveAnymore); err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live == nil {
				continue
			}
			l.stopLivePlayer(live)

		case clientpackets.OpcodeValidatePosition:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeValidatePosition)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.validateLivePlayerPosition(live, location.Location{X: int(req.X), Y: int(req.Y), Z: int(req.Z)})
			}

		case clientpackets.OpcodeRequestItemList:
			if live == nil {
				continue
			}
			// The reference's ItemList constructor recomputes carried weight
			// on every send, not only at login.
			if inv := live.Inventory(); inv != nil {
				inv.UpdateWeight()
			}
			frame, err := serverpackets.FrameItemList(live.inventoryItems(), l.itemTemplates, false)
			if err != nil {
				l.log.Error().Err(err).Msg("build ItemList")
				return
			}
			session.SendFrame(frame)

		case clientpackets.OpcodeUseItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeUseItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live == nil {
				continue
			}
			l.useItem(live, req.ObjectID)

		case clientpackets.OpcodeRequestUnEquipItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeUnequipItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live == nil {
				continue
			}
			l.unequipItem(live, req.BodySlot)

		case clientpackets.OpcodeRequestDropItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestDropItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live == nil {
				continue
			}
			l.dropLiveItem(live, req)

		case clientpackets.OpcodeRequestDestroyItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestDestroyItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live == nil {
				continue
			}
			l.destroyLiveItem(live, req.ObjectID, int(req.Count))

		case clientpackets.OpcodeRequestCrystallizeItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestCrystallizeItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live == nil {
				continue
			}
			l.crystallizeLiveItem(live, req)

		case clientpackets.OpcodeRequestEnchantItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestEnchantItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live == nil {
				continue
			}
			l.enchantLiveItem(ctx, live, req)

		case clientpackets.OpcodeRequestSkillList:
			// While entering, 0x3f is the quest-list probe the client sends
			// during loading: it must be answered or the quest panel stays
			// empty. No quests are modeled yet, so the list is empty — the
			// same frame the EnterWorld burst sends.
			if client.State() == StateEntering {
				session.SendFrame(serverpackets.FrameQuestList(nil))
				continue
			}
			if live == nil {
				continue
			}
			session.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))

		case clientpackets.OpcodeRequestAcquireSkillInfo:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestAcquireSkillInfo)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.sendAcquireSkillInfo(live, req)
			}

		case clientpackets.OpcodeRequestAcquireSkill:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestAcquireSkill)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.learnAcquireSkill(ctx, live, req)
			}

		case clientpackets.OpcodeRequestActionUse:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestActionUse)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live == nil {
				continue
			}
			switch req.ActionID {
			case actionSitStand:
				l.requestChangeWaitType(live, !live.Standing())
			case actionWalkRun:
				l.changeLiveMoveType(live, !live.Running())
			default:
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

		case clientpackets.OpcodeRequestRestartPoint:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestRestartPoint)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.restartLivePlayer(live, req)
			}

		case clientpackets.OpcodeRequestSocialAction:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestSocialAction)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.broadcastLiveSocialAction(live, req.ActionID)
			}

		case clientpackets.OpcodeRequestChangeMoveType:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestChangeMoveType)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.changeLiveMoveType(live, req.Run)
			}

		case clientpackets.OpcodeRequestChangeWaitType:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestChangeWaitType)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.requestChangeWaitType(live, req.Stand)
			}

		case clientpackets.OpcodeRequestLinkHtml:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestLinkHTML)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			l.requestLinkHTML(live, req)

		case clientpackets.OpcodeRequestBypassToServer:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestBypassToServer)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			l.requestBypassToServer(live, req)

		case clientpackets.OpcodeRequestTargetCancel:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestTargetCancel)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.requestTargetCancel(live, req)
			}

		case clientpackets.OpcodeAppearing:
			if _, err := decodeClientPacket(l, client, payload, clientpackets.DecodeAppearing); err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.completeLivePlayerTeleport(live)
				live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
			}

		case clientpackets.OpcodeStartRotating:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeStartRotating)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live == nil {
				continue
			}
			l.broadcastLiveFrame(live, func() wire.Frame {
				return serverpackets.FrameStartRotation(live.ObjectID(), int(req.Degree), int(req.Side), 0)
			})

		case clientpackets.OpcodeFinishRotating:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeFinishRotating)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live == nil {
				continue
			}
			live.SetHeading(int(req.Degree))
			l.broadcastLiveFrame(live, func() wire.Frame {
				return serverpackets.FrameStopRotation(live.ObjectID(), int(req.Degree), 0)
			})

		case clientpackets.OpcodeRequestRestart:
			if live == nil {
				continue
			}
			l.detachLivePlayer(ctx, live)
			live = nil
			entering = nil
			client.SetState(StateAuthed)
			session.SendFrame(serverpackets.FrameRestartResponse(true))
			list, err := l.sendCharSelectInfo(ctx, client)
			if err != nil {
				l.log.Error().Err(err).Msg("list characters")
				return
			}
			chars = list

		case clientpackets.OpcodeSendTimeCheck:
			if _, err := decodeClientPacket(l, client, payload, clientpackets.DecodeSendTimeCheck); err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			continue

		case clientpackets.OpcodeRequestPackageItemList:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPackageSendableItemList)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			l.sendPackageSendableItemList(live, req.ObjectID)

		case clientpackets.OpcodeRequestPetUseItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPetUseItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			l.petUseItem(ctx, live, req)

		case clientpackets.OpcodeRequestGiveItemToPet:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestGiveItemToPet)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			l.giveItemToPet(ctx, live, req)

		case clientpackets.OpcodeRequestGetItemFromPet:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestGetItemFromPet)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			l.getItemFromPet(ctx, live, req)

		case clientpackets.OpcodeRequestPetGetItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPetGetItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			l.petGetItem(ctx, live, req)

		case clientpackets.OpcodeTradeRequest:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeTradeRequest)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.handleTradeRequest(live, req)
			}

		case clientpackets.OpcodeAnswerTradeRequest:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeAnswerTradeRequest)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.handleAnswerTradeRequest(live, req)
			}

		case clientpackets.OpcodeAddTradeItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeAddTradeItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.handleAddTradeItem(live, req)
			}

		case clientpackets.OpcodeTradeDone:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeTradeDone)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.handleTradeDone(ctx, live, req)
			}

		case clientpackets.OpcodeRequestShortCutReg:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestShortCutReg)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.registerShortcut(ctx, live, req)
			}

		case clientpackets.OpcodeRequestShortCutDel:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestShortCutDel)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.deleteShortcut(ctx, live, req)
			}

		case clientpackets.OpcodeRequestChangePetName:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestChangePetName)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.handleRequestChangePetName(ctx, live, req)
			}

		case clientpackets.OpcodeDlgAnswer:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeDlgAnswer)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				l.handleDlgAnswer(live, req)
			}

		case clientpackets.OpcodeDummy1A,
			clientpackets.OpcodeRequestSellItem,
			clientpackets.OpcodeRequestBuyItem,
			clientpackets.OpcodeDummy23,
			clientpackets.OpcodeDummy2E,
			clientpackets.OpcodeSendWarehouseDeposit,
			clientpackets.OpcodeSendWarehouseWithdraw,
			clientpackets.OpcodeDummy34,
			clientpackets.OpcodeDummy3E,
			clientpackets.OpcodeRequestGetOnVehicle,
			clientpackets.OpcodeRequestGetOffVehicle,
			clientpackets.OpcodeRequestMoveInVehicle,
			clientpackets.OpcodeCannotMoveInVehicle,
			clientpackets.OpcodeRequestQuestListInGame,
			clientpackets.OpcodeRequestQuestAbort,
			clientpackets.OpcodeRequestPackageSend,
			clientpackets.OpcodeGameGuardReply,
			clientpackets.OpcodeRequestShowMiniMap:
			l.log.Warn().Str("opcode", fmt.Sprintf("%#x", opcode)).Msg("Opcode not wired")
			continue

		default:
			l.log.Info().Str("opcode", fmt.Sprintf("%#x", opcode)).Str("state", client.State().String()).
				Msg("game client: accepted opcode not implemented yet")
		}
	}
}

func clearsSpawnProtection(opcode byte) bool {
	switch opcode {
	case clientpackets.OpcodeEnterWorld, clientpackets.OpcodeAction,
		clientpackets.OpcodeRequestPledgeCrest, clientpackets.OpcodeAppearing:
		return false
	default:
		return true
	}
}

func normalReadFrameError(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed)
}

// authenticate validates req against the login server over the game
// server's current link, advancing client to StateAuthed on success.
// AuthLoginFail (and the connection close that follows) is the caller's
// job for every false/error result except the login-link-down case, which
// authenticate handles itself since there is no in-flight validation to
// fail.
