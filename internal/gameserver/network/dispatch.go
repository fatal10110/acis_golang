package network

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// itemStore is the persistence GameClientLink needs to list a character's
// items. Satisfied by *sql.ItemStore.
type itemStore interface {
	ListByOwner(ctx context.Context, ownerID int32) ([]*item.Instance, error)
}

// GameClientLink accepts and drives connections from Interlude game
// clients: the VersionCheck/cipher handshake, session-key validation
// against the login server, character list/create/delete/restore, and
// character select through to world entry.
type GameClientLink struct {
	validator     *SessionValidator
	loginLink     func() *LoginLink
	roster        *manager.Roster
	items         itemStore
	templates     *player.TemplateTable
	itemTemplates *item.Table
	world         *world.State
	log           zerolog.Logger

	// newCipherKey supplies each connection's XOR cipher key; overridden in
	// tests for a deterministic handshake.
	newCipherKey func() ([]byte, error)
}

// NewGameClientLink builds a GameClientLink from its collaborators.
// loginLink returns the game server's current link to the login server, or
// nil while disconnected/reconnecting: session validation fails clients
// gracefully (AuthLoginFail) rather than panicking while the link is down.
func NewGameClientLink(
	validator *SessionValidator,
	loginLink func() *LoginLink,
	roster *manager.Roster,
	items itemStore,
	templates *player.TemplateTable,
	itemTemplates *item.Table,
	worldState *world.State,
	log zerolog.Logger,
) *GameClientLink {
	return &GameClientLink{
		validator:     validator,
		loginLink:     loginLink,
		roster:        roster,
		items:         items,
		templates:     templates,
		itemTemplates: itemTemplates,
		world:         worldState,
		log:           log,
		newCipherKey:  randomCipherKey,
	}
}

type livePlayer struct {
	*player.Character
	template *player.Template
	items    []*item.Instance
}

func (p *livePlayer) SendFrame(frame wire.Frame) bool {
	return p.Character.SendFrame(frame)
}

func (p *livePlayer) Discover(obj world.Tracked) {
	other, ok := obj.(*livePlayer)
	if !ok {
		return
	}
	p.SendFrame(serverpackets.FrameCharInfo(serverpackets.CharInfoSnapshot{
		Character: other.Character,
		Template:  other.template,
		Items:     other.items,
	}))
}

func (p *livePlayer) Forget(obj world.Tracked) {
	p.SendFrame(serverpackets.FrameDeleteObject(obj.ObjectID(), false))
}

func randomCipherKey() ([]byte, error) {
	key := make([]byte, keySize)
	if _, err := rand.Read(key[:8]); err != nil {
		return nil, fmt.Errorf("generate game cipher key: %w", err)
	}
	copy(key[8:], gameCipherStaticKey[:])
	return key, nil
}

func validProtocolRevision(revision int32) bool {
	switch revision {
	case 737, 740, 744, 746:
		return true
	default:
		return false
	}
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
		l.detachLivePlayer(live)
		l.notifyPlayerLogout(client.AccountName())
	}()

	for {
		payload, err := session.ReadFrame()
		if err != nil {
			l.log.Error().Err(err).Msg("Read frame")
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
			case second == clientpackets.OpcodeRequestManorList:
				session.SendFrame(serverpackets.FrameExSendManorList())
			default:
				l.log.Info().
					Uint16("opcode2", second).
					Str("state", client.State().String()).
					Msg("game client: accepted extended opcode not implemented yet")
			}

		case clientpackets.OpcodeRequestSkillCoolTime:
			continue

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
				return
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

		case clientpackets.OpcodeValidatePosition:
			req, err := clientpackets.DecodeValidatePosition(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
			}
			if live != nil {
				l.updateLivePlayerPosition(live, location.Location{X: int(req.X), Y: int(req.Y), Z: int(req.Z)}, int(req.Heading))
			}

		case clientpackets.OpcodeRequestItemList:
			if live == nil {
				continue
			}
			frame, err := serverpackets.FrameItemList(live.items, l.itemTemplates, false)
			if err != nil {
				l.log.Error().Err(err).Msg("build ItemList")
				return
			}
			session.SendFrame(frame)

		case clientpackets.OpcodeRequestSkillList:
			session.SendFrame(serverpackets.FrameSkillList(nil))

		case clientpackets.OpcodeRequestRestart:
			if live == nil {
				continue
			}
			l.detachLivePlayer(live)
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

		case clientpackets.OpcodeAction,
			clientpackets.OpcodeAttackRequest,
			clientpackets.OpcodeRequestUnEquipItem,
			clientpackets.OpcodeRequestDropItem,
			clientpackets.OpcodeUseItem,
			clientpackets.OpcodeTradeRequest,
			clientpackets.OpcodeAddTradeItem,
			clientpackets.OpcodeTradeDone,
			clientpackets.OpcodeDummy1A,
			clientpackets.OpcodeRequestSocialAction,
			clientpackets.OpcodeRequestChangeMoveType,
			clientpackets.OpcodeRequestChangeWaitType,
			clientpackets.OpcodeRequestSellItem,
			clientpackets.OpcodeRequestBuyItem,
			clientpackets.OpcodeDummy23,
			clientpackets.OpcodeDummy2E,
			clientpackets.OpcodeRequestMagicSkillUse,
			clientpackets.OpcodeAppearing,
			clientpackets.OpcodeSendWarehouseDeposit,
			clientpackets.OpcodeSendWarehouseWithdraw,
			clientpackets.OpcodeRequestShortCutReg,
			clientpackets.OpcodeDummy34,
			clientpackets.OpcodeRequestShortCutDel,
			clientpackets.OpcodeCannotMoveAnymore,
			clientpackets.OpcodeRequestTargetCancel,
			clientpackets.OpcodeDummy3E,
			clientpackets.OpcodeRequestGetOnVehicle,
			clientpackets.OpcodeRequestGetOffVehicle,
			clientpackets.OpcodeAnswerTradeRequest,
			clientpackets.OpcodeRequestActionUse,
			clientpackets.OpcodeStartRotating,
			clientpackets.OpcodeFinishRotating,
			clientpackets.OpcodeRequestEnchantItem,
			clientpackets.OpcodeRequestDestroyItem,
			clientpackets.OpcodeRequestMoveInVehicle,
			clientpackets.OpcodeCannotMoveInVehicle,
			clientpackets.OpcodeRequestQuestListInGame,
			clientpackets.OpcodeRequestQuestAbort,
			clientpackets.OpcodeRequestAcquireSkillInfo,
			clientpackets.OpcodeRequestAcquireSkill,
			clientpackets.OpcodeRequestRestartPoint,
			clientpackets.OpcodeRequestCrystallizeItem,
			clientpackets.OpcodeRequestChangePetName,
			clientpackets.OpcodeRequestPetUseItem,
			clientpackets.OpcodeRequestGiveItemToPet,
			clientpackets.OpcodeRequestGetItemFromPet,
			clientpackets.OpcodeRequestPetGetItem,
			clientpackets.OpcodeSendTimeCheck,
			clientpackets.OpcodeRequestPackageItemList,
			clientpackets.OpcodeRequestPackageSend,
			clientpackets.OpcodeDlgAnswer,
			clientpackets.OpcodeGameGuardReply,
			clientpackets.OpcodeRequestShowMiniMap:
			continue

		default:
			l.log.Info().Str("opcode", fmt.Sprintf("%#x", opcode)).Str("state", client.State().String()).
				Msg("game client: accepted opcode not implemented yet")
		}
	}
}

// authenticate validates req against the login server over the game
// server's current link, advancing client to StateAuthed on success.
// AuthLoginFail (and the connection close that follows) is the caller's
// job for every false/error result except the login-link-down case, which
// authenticate handles itself since there is no in-flight validation to
// fail.
func (l *GameClientLink) authenticate(ctx context.Context, client *Client, req clientpackets.AuthLogin) (bool, error) {
	loginLink := l.loginLink()
	if loginLink == nil {
		client.Session.SendFrame(serverpackets.FrameAuthLoginFail(serverpackets.LoginFailSystemErrorTryLater))
		return false, nil
	}
	return l.validator.Validate(ctx, client, req, loginLink)
}

// sendCharSelectInfo lists client's characters, sends the resulting
// CharSelectInfo, and returns the list so the caller can cache it for
// subsequent slot-addressed requests.
func (l *GameClientLink) sendCharSelectInfo(ctx context.Context, client *Client) ([]*player.Character, error) {
	chars, err := l.roster.List(ctx, client.AccountName())
	if err != nil {
		return nil, err
	}

	slots := make([]serverpackets.CharacterSlot, len(chars))
	now := time.Now()
	for i, c := range chars {
		items, err := l.items.ListByOwner(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		slots[i] = serverpackets.NewCharacterSlot(c, items, now)
	}

	client.Session.SendFrame(serverpackets.FrameCharSelectInfo(client.AccountName(), client.SessionKey().PlayKey1, slots, -1))
	return chars, nil
}

// enterWorld sends the EnterWorld packet burst for c and registers it in the
// live world state.
func (l *GameClientLink) enterWorld(ctx context.Context, client *Client, c *player.Character) (*livePlayer, bool) {
	tmpl, ok := l.templates.Get(c.ClassID)
	if !ok {
		l.log.Error().Int("class_id", c.ClassID).Msg("enter world: no template loaded")
		return nil, false
	}
	items, err := l.items.ListByOwner(ctx, c.ID)
	if err != nil {
		l.log.Error().Err(err).Msg("enter world: list items")
		return nil, false
	}

	client.Session.SendFrame(serverpackets.FrameUserInfo(serverpackets.UserInfoSnapshot{Character: c, Template: tmpl, Items: items}))

	itemListFrame, err := serverpackets.FrameItemList(items, l.itemTemplates, false)
	if err != nil {
		l.log.Error().Err(err).Msg("enter world: build ItemList")
		return nil, false
	}
	client.Session.SendFrame(itemListFrame)

	client.Session.SendFrame(serverpackets.FrameSkillList(nil))
	live := l.attachLivePlayer(client, c, tmpl, items)
	if l.world != nil {
		x, y, z := c.Position()
		l.world.Spawn(live, x, y, z, c.Heading)
		l.world.AddPlayer(live)
	}
	return live, true
}

func (l *GameClientLink) attachLivePlayer(client *Client, c *player.Character, tmpl *player.Template, items []*item.Instance) *livePlayer {
	c.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventory(c.ID, l.itemTemplates, items))
	c.SetWorld(l.world)
	c.SetFrameSender(client.Session.SendFrame)
	c.SetAttackBroadcaster(func(snapshot attack.Snapshot) {
		if l.world == nil {
			return
		}
		l.world.ForEachKnown(c, func(o world.Tracked) {
			receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
			if !ok {
				return
			}
			receiver.SendFrame(serverpackets.FrameAttack(snapshot))
		})
	})
	return &livePlayer{Character: c, template: tmpl, items: items}
}

func (l *GameClientLink) moveLivePlayer(live *livePlayer, origin, target location.Location) {
	heading := origin.HeadingTo(target)
	l.updateLivePlayerPosition(live, origin, heading)
	live.SendFrame(serverpackets.FrameMoveToLocation(live.ObjectID(), target, origin))

	if l.world == nil {
		return
	}
	l.world.ForEachKnown(live, func(o world.Tracked) {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			return
		}
		receiver.SendFrame(serverpackets.FrameMoveToLocation(live.ObjectID(), target, origin))
	})
}

func (l *GameClientLink) detachLivePlayer(live *livePlayer) {
	if live == nil {
		return
	}
	if l.world != nil {
		l.world.Despawn(live)
		l.world.RemovePlayer(live.ObjectID())
	}
	live.Character.SetFrameSender(nil)
	live.Character.SetAttackBroadcaster(nil)
}

func (l *GameClientLink) notifyPlayerLogout(account string) {
	loginLink := l.loginLink()
	if account == "" || loginLink == nil {
		return
	}
	if err := loginLink.SendPlayerLogout(account); err != nil {
		l.log.Debug().Err(err).Str("account", account).Msg("notify player logout")
	}
}

func (l *GameClientLink) updateLivePlayerPosition(live *livePlayer, position location.Location, heading int) {
	live.Character.Location = position
	live.Character.Heading = heading
	live.Character.SetHeading(heading)
	if l.world == nil {
		return
	}
	if err := l.world.Move(live, position.X, position.Y, position.Z); err != nil {
		l.log.Debug().Err(err).Int32("object_id", live.ObjectID()).Msg("move player")
	}
}

func slotCharacter(chars []*player.Character, slot int32) (*player.Character, bool) {
	if slot < 0 || int(slot) >= len(chars) {
		return nil, false
	}
	return chars[slot], true
}

func createFailReason(outcome manager.CreateOutcome) serverpackets.CharCreateFailReason {
	switch outcome {
	case manager.CreateTooManyCharacters:
		return serverpackets.CharCreateFailReasonTooManyCharacters
	case manager.CreateNameTaken:
		return serverpackets.CharCreateFailReasonNameAlreadyExists
	case manager.CreateInvalidName:
		return serverpackets.CharCreateFailReasonIncorrectName
	default:
		return serverpackets.CharCreateFailReasonCreationFailed
	}
}
