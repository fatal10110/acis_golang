package network

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Handle drives one game-client connection end to end. It matches Serve's
// handle signature, so a caller wires it in directly:
// network.Serve(ctx, ln, link.Handle, log).
func (l *GameClientLink) Handle(ctx context.Context, conn *Conn) {
	key, err := l.newCipherKey()
	if err != nil {
		l.log.Error().Err(err).Msg("generate game cipher key")
		return
	}
	cipher, err := NewCipher(key)
	if err != nil {
		l.log.Error().Err(err).Msg("build game cipher")
		return
	}
	session := NewSession(conn, cipher)
	client := NewClient(session)

	// chars and entering are read entirely by this goroutine: they resolve
	// the character-list slot indices RequestCharacterDelete,
	// CharacterRestore and RequestGameStart address, and the character
	// RequestGameStart selected for EnterWorld to finish spawning.
	var chars []*player.Character
	var entering *player.Character
	var live *livePlayer
	protocolReady := false
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
		if !protocolReady && opcode != clientpackets.OpcodeProtocolVersion {
			return
		}
		if !client.Accept(opcode) {
			l.log.Warn().Str("state", client.State().String()).Str("opcode", hex.EncodeToString(payload)).Msg("Accept opcode")
			return
		}

		switch opcode {
		case clientpackets.OpcodeProtocolVersion:
			req, err := clientpackets.DecodeProtocolVersion(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
			}
			if !validProtocolRevision(req.Revision) {
				return
			}
			if !session.SendFrame(serverpackets.FrameVersionCheck(key)) {
				return
			}
			protocolReady = true

		case clientpackets.OpcodeAuthLogin:
			req, err := clientpackets.DecodeAuthLogin(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
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
			req, err := clientpackets.DecodeRequestCharacterCreate(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
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
			req, err := clientpackets.DecodeRequestCharacterDelete(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
			}
			c, ok := slotCharacter(chars, req.Slot)
			if !ok {
				session.SendFrame(serverpackets.FrameCharDeleteFail(serverpackets.CharDeleteFailReasonDeletionFailed))
				continue
			}
			if err := l.roster.MarkForDeletion(ctx, c.ID); err != nil {
				l.log.Error().Err(err).Msg("mark character for deletion")
				session.SendFrame(serverpackets.FrameCharDeleteFail(serverpackets.CharDeleteFailReasonDeletionFailed))
				continue
			}
			session.SendFrame(serverpackets.FrameCharDeleteOk())
			list, err := l.sendCharSelectInfo(ctx, client)
			if err != nil {
				l.log.Error().Err(err).Msg("list characters")
				return
			}
			chars = list

		case clientpackets.OpcodeCharacterRestore:
			req, err := clientpackets.DecodeCharacterRestore(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
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

		case clientpackets.OpcodeRequestNewCharacter:
			frame, err := serverpackets.FrameNewCharacterSuccess(l.templates)
			if err != nil {
				l.log.Error().Err(err).Msg("build NewCharacterSuccess")
				return
			}
			session.SendFrame(frame)

		case clientpackets.OpcodeRequestGameStart:
			req, err := clientpackets.DecodeRequestGameStart(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
			}
			c, ok := slotCharacter(chars, req.Slot)
			if !ok {
				return
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
			switch second := r.ReadUint16(); {
			case r.Err() != nil:
				l.log.Warn().Str("state", client.State().String()).Msg("game client: extended opcode missing")
				continue
			case second == clientpackets.OpcodeRequestAutoSoulShot:
				req, err := clientpackets.DecodeRequestAutoSoulShot(payload)
				if err != nil {
					l.log.Warn().Err(err).Msg("game client")
					continue
				}
				if live != nil {
					l.handleAutoSoulShot(live, req)
				}
			case second == clientpackets.OpcodeRequestManorList:
				session.SendFrame(serverpackets.FrameExSendManorList())
			default:
				l.log.Info().
					Uint16("opcode2", second).
					Str("state", client.State().String()).
					Msg("game client: accepted extended opcode not implemented yet")
			}

		case clientpackets.OpcodeRequestSkillCoolTime:
			if live == nil {
				continue
			}
			now := time.Now()
			session.SendFrame(serverpackets.FrameSkillCoolTime(skillCoolTimeEntries(live.SkillReuseTimers(now), now)))

		case clientpackets.OpcodeRequestMagicSkillUse:
			req, err := clientpackets.DecodeRequestMagicSkillUse(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live != nil {
				l.handleMagicSkillUse(live, req)
			}

		case clientpackets.OpcodeAction:
			req, err := clientpackets.DecodeAction(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live == nil {
				continue
			}
			l.handleTargetAction(live, req.ObjectID, false)

		case clientpackets.OpcodeAttackRequest:
			req, err := clientpackets.DecodeAttackRequest(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live == nil {
				continue
			}
			selected := live.target != nil && live.target.ObjectID() == req.ObjectID
			l.handleTargetAction(live, req.ObjectID, selected)

		case clientpackets.OpcodeLogout:
			if live != nil {
				session.SendFrame(serverpackets.FrameLeaveWorld())
				return
			}
			return

		case clientpackets.OpcodeMoveBackwardToLocation:
			req, err := clientpackets.DecodeMoveBackwardToLocation(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
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
				location.Location{X: int(req.OriginX), Y: int(req.OriginY), Z: int(req.OriginZ)},
				location.Location{X: int(req.TargetX), Y: int(req.TargetY), Z: int(req.TargetZ)},
			)

		case clientpackets.OpcodeCannotMoveAnymore:
			req, err := clientpackets.DecodeCannotMoveAnymore(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live == nil {
				continue
			}
			l.stopLivePlayer(live, location.Location{X: int(req.X), Y: int(req.Y), Z: int(req.Z)}, int(req.Heading))

		case clientpackets.OpcodeValidatePosition:
			req, err := clientpackets.DecodeValidatePosition(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live != nil {
				l.updateLivePlayerPosition(live, location.Location{X: int(req.X), Y: int(req.Y), Z: int(req.Z)}, int(req.Heading))
			}

		case clientpackets.OpcodeRequestItemList:
			if live == nil {
				continue
			}
			frame, err := serverpackets.FrameItemList(live.inventoryItems(), l.itemTemplates, false)
			if err != nil {
				l.log.Error().Err(err).Msg("build ItemList")
				return
			}
			session.SendFrame(frame)

		case clientpackets.OpcodeUseItem:
			req, err := clientpackets.DecodeUseItem(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live == nil {
				continue
			}
			l.useItem(live, req.ObjectID)

		case clientpackets.OpcodeRequestUnEquipItem:
			req, err := clientpackets.DecodeUnequipItem(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live == nil {
				continue
			}
			l.unequipItem(live, req.BodySlot)

		case clientpackets.OpcodeRequestDropItem:
			req, err := clientpackets.DecodeRequestDropItem(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live == nil {
				continue
			}
			l.dropLiveItem(live, req)

		case clientpackets.OpcodeRequestDestroyItem:
			req, err := clientpackets.DecodeRequestDestroyItem(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live == nil {
				continue
			}
			l.destroyLiveItem(live, req.ObjectID, int(req.Count))

		case clientpackets.OpcodeRequestCrystallizeItem:
			req, err := clientpackets.DecodeRequestCrystallizeItem(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live == nil {
				continue
			}
			l.crystallizeLiveItem(live, req)

		case clientpackets.OpcodeRequestSkillList:
			if live == nil {
				continue
			}
			session.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))

		case clientpackets.OpcodeRequestAcquireSkillInfo:
			req, err := clientpackets.DecodeRequestAcquireSkillInfo(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live != nil {
				l.sendAcquireSkillInfo(live, req)
			}

		case clientpackets.OpcodeRequestAcquireSkill:
			req, err := clientpackets.DecodeRequestAcquireSkill(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live != nil {
				l.learnAcquireSkill(ctx, live, req)
			}

		case clientpackets.OpcodeRequestActionUse:
			req, err := clientpackets.DecodeRequestActionUse(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live != nil {
				l.handleSummonActionUse(live, req)
			}

		case clientpackets.OpcodeRequestSocialAction:
			req, err := clientpackets.DecodeRequestSocialAction(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live != nil {
				l.broadcastLiveSocialAction(live, req.ActionID)
			}

		case clientpackets.OpcodeRequestChangeMoveType:
			req, err := clientpackets.DecodeRequestChangeMoveType(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live != nil {
				l.changeLiveMoveType(live, req.Run)
			}

		case clientpackets.OpcodeRequestChangeWaitType:
			req, err := clientpackets.DecodeRequestChangeWaitType(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live != nil {
				l.changeLiveWaitType(live, req.Stand)
			}

		case clientpackets.OpcodeRequestTargetCancel:
			if _, err := clientpackets.DecodeRequestTargetCancel(payload); err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live != nil {
				l.clearLiveTarget(live)
			}

		case clientpackets.OpcodeStartRotating:
			req, err := clientpackets.DecodeStartRotating(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			if live == nil {
				continue
			}
			l.broadcastLiveFrame(live, func() wire.Frame {
				return serverpackets.FrameStartRotation(live.ObjectID(), int(req.Degree), int(req.Side), 0)
			})

		case clientpackets.OpcodeFinishRotating:
			req, err := clientpackets.DecodeFinishRotating(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
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
			if _, err := clientpackets.DecodeSendTimeCheck(payload); err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			continue

		case clientpackets.OpcodeRequestPackageItemList:
			req, err := clientpackets.DecodeRequestPackageSendableItemList(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				continue
			}
			l.sendPackageSendableItemList(live, req.ObjectID)

		case clientpackets.OpcodeTradeRequest,
			clientpackets.OpcodeAddTradeItem,
			clientpackets.OpcodeTradeDone,
			clientpackets.OpcodeDummy1A,
			clientpackets.OpcodeRequestSellItem,
			clientpackets.OpcodeRequestBuyItem,
			clientpackets.OpcodeDummy23,
			clientpackets.OpcodeDummy2E,
			clientpackets.OpcodeAppearing,
			clientpackets.OpcodeSendWarehouseDeposit,
			clientpackets.OpcodeSendWarehouseWithdraw,
			clientpackets.OpcodeRequestShortCutReg,
			clientpackets.OpcodeDummy34,
			clientpackets.OpcodeRequestShortCutDel,
			clientpackets.OpcodeDummy3E,
			clientpackets.OpcodeRequestGetOnVehicle,
			clientpackets.OpcodeRequestGetOffVehicle,
			clientpackets.OpcodeAnswerTradeRequest,
			clientpackets.OpcodeRequestEnchantItem,
			clientpackets.OpcodeRequestMoveInVehicle,
			clientpackets.OpcodeCannotMoveInVehicle,
			clientpackets.OpcodeRequestQuestListInGame,
			clientpackets.OpcodeRequestQuestAbort,
			clientpackets.OpcodeRequestRestartPoint,
			clientpackets.OpcodeRequestChangePetName,
			clientpackets.OpcodeRequestPetUseItem,
			clientpackets.OpcodeRequestGiveItemToPet,
			clientpackets.OpcodeRequestGetItemFromPet,
			clientpackets.OpcodeRequestPetGetItem,
			clientpackets.OpcodeRequestPackageSend,
			clientpackets.OpcodeDlgAnswer,
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

func normalReadFrameError(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed)
}

// authenticate validates req against the login server over the game
// server's current link, advancing client to StateAuthed on success.
// AuthLoginFail (and the connection close that follows) is the caller's
// job for every false/error result except the login-link-down case, which
// authenticate handles itself since there is no in-flight validation to
// fail.
