package network

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// itemStore is the persistence GameClientLink needs to list a character's
// items. Satisfied by *sql.ItemStore.
type itemStore interface {
	ListByOwner(ctx context.Context, ownerID int32) ([]*item.Instance, error)
}

type attackStanceTracker interface {
	Add(task.AttackStanceActor)
}

type idAllocator interface {
	NextID() (int32, error)
}

type groundItemDropper interface {
	Drop(*grounditem.Item, task.DropOptions)
}

const crystallizeSkillID = 248

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
	skills        *SkillPersistence
	world         *world.State
	ids           idAllocator
	groundItems   groundItemDropper
	attackStance  attackStanceTracker
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
	skills *SkillPersistence,
	worldState *world.State,
	ids idAllocator,
	groundItems groundItemDropper,
	attackStance attackStanceTracker,
	log zerolog.Logger,
) *GameClientLink {
	return &GameClientLink{
		validator:     validator,
		loginLink:     loginLink,
		roster:        roster,
		items:         items,
		templates:     templates,
		itemTemplates: itemTemplates,
		skills:        skills,
		world:         worldState,
		ids:           ids,
		groundItems:   groundItems,
		attackStance:  attackStance,
		log:           log,
		newCipherKey:  randomCipherKey,
	}
}

type livePlayer struct {
	*player.Character
	template *player.Template
	items    []*item.Instance
	target   world.Tracked

	stopAttack func(*livePlayer)
}

func (p *livePlayer) SendFrame(frame wire.Frame) bool {
	return p.Character.SendFrame(frame)
}

func (p *livePlayer) Stop() {
	if p.stopAttack != nil {
		p.stopAttack(p)
	}
}

func (p *livePlayer) Discover(obj world.Tracked) {
	switch o := obj.(type) {
	case *livePlayer:
		p.SendFrame(serverpackets.FrameCharInfo(serverpackets.CharInfoSnapshot{
			Character: o.Character,
			Template:  o.template,
			Items:     o.items,
		}))
	case *npc.Hostile:
		p.SendFrame(serverpackets.FrameNPCInfo(npcInfoSnapshot(o)))
	case groundItemObject:
		if dropped, ok := o.(interface{ DropperID() int32 }); ok {
			if dropperID := dropped.DropperID(); dropperID != 0 {
				p.SendFrame(serverpackets.FrameDropItem(o, dropperID))
				return
			}
		}
		p.SendFrame(serverpackets.FrameSpawnItem(o))
	case doorObject:
		p.SendFrame(serverpackets.FrameDoorInfo(o, false))
	case staticObject:
		p.SendFrame(serverpackets.FrameStaticObjectInfo(o))
	}
}

func (p *livePlayer) Forget(obj world.Tracked) {
	if !rendersObject(obj) {
		return
	}
	p.SendFrame(serverpackets.FrameDeleteObject(obj.ObjectID(), false))
}

type groundItemObject interface {
	ObjectID() int32
	ItemID() int32
	Count() int
	Stackable() bool
	Position() (int, int, int)
}

type doorObject interface {
	ObjectID() int32
	DoorID() int
	Opened() bool
	MaxHP() int
	HP() int
	Damage() int
}

type staticObject interface {
	ObjectID() int32
	StaticObjectID() int
}

func rendersObject(obj world.Tracked) bool {
	switch obj.(type) {
	case *livePlayer, *npc.Hostile, groundItemObject, doorObject, staticObject:
		return true
	default:
		return false
	}
}

func npcInfoSnapshot(n *npc.Hostile) serverpackets.NPCInfoSnapshot {
	tmpl := n.Instance.Template
	x, y, z := n.Position()
	name, title := "", ""
	if tmpl.UsingServerSideName {
		name = tmpl.Name
	}
	if tmpl.UsingServerSideTitle {
		title = tmpl.Title
	}
	return serverpackets.NPCInfoSnapshot{
		ObjectID:        n.ObjectID(),
		TemplateID:      tmpl.TemplateID,
		Attackable:      true,
		X:               x,
		Y:               y,
		Z:               z,
		Heading:         n.Heading(),
		MAtkSpd:         int(tmpl.AtkSpd),
		PAtkSpd:         n.AttackSpeed(),
		RunSpd:          int(tmpl.RunSpeed),
		WalkSpd:         int(tmpl.WalkSpeed),
		CollisionRadius: tmpl.CollisionRadius,
		CollisionHeight: tmpl.CollisionHeight,
		RightHand:       tmpl.RightHand,
		LeftHand:        tmpl.LeftHand,
		Running:         true,
		AlikeDead:       n.AlikeDead(),
		SummonAnimation: 2,
		Name:            name,
		Title:           title,
	}
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

const livePlayerDetachSaveTimeout = 2 * time.Second

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

		case clientpackets.OpcodeAction:
			req, err := clientpackets.DecodeAction(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
			}
			if live == nil {
				continue
			}
			l.handleTargetAction(live, req.ObjectID, false)

		case clientpackets.OpcodeAttackRequest:
			req, err := clientpackets.DecodeAttackRequest(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
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

		case clientpackets.OpcodeCannotMoveAnymore:
			req, err := clientpackets.DecodeCannotMoveAnymore(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
			}
			if live == nil {
				continue
			}
			l.stopLivePlayer(live, location.Location{X: int(req.X), Y: int(req.Y), Z: int(req.Z)}, int(req.Heading))

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

		case clientpackets.OpcodeUseItem:
			req, err := clientpackets.DecodeUseItem(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
			}
			if live == nil {
				continue
			}
			l.useItem(live, req.ObjectID)

		case clientpackets.OpcodeRequestUnEquipItem:
			req, err := clientpackets.DecodeUnequipItem(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
			}
			if live == nil {
				continue
			}
			l.unequipItem(live, req.BodySlot)

		case clientpackets.OpcodeRequestDropItem:
			req, err := clientpackets.DecodeRequestDropItem(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
			}
			if live == nil {
				continue
			}
			l.dropLiveItem(live, req)

		case clientpackets.OpcodeRequestDestroyItem:
			req, err := clientpackets.DecodeRequestDestroyItem(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
			}
			if live == nil {
				continue
			}
			l.destroyLiveItem(live, req.ObjectID, int(req.Count))

		case clientpackets.OpcodeRequestCrystallizeItem:
			req, err := clientpackets.DecodeRequestCrystallizeItem(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
			}
			if live == nil {
				continue
			}
			l.crystallizeLiveItem(live, req)

		case clientpackets.OpcodeRequestSkillList:
			session.SendFrame(serverpackets.FrameSkillList(nil))

		case clientpackets.OpcodeRequestActionUse:
			req, err := clientpackets.DecodeRequestActionUse(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
			}
			if live != nil {
				l.handleSummonActionUse(live, req)
			}

		case clientpackets.OpcodeRequestSocialAction:
			req, err := clientpackets.DecodeRequestSocialAction(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
			}
			if live != nil {
				l.broadcastLiveSocialAction(live, req.ActionID)
			}

		case clientpackets.OpcodeRequestChangeMoveType:
			req, err := clientpackets.DecodeRequestChangeMoveType(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
			}
			if live != nil {
				l.changeLiveMoveType(live, req.Run)
			}

		case clientpackets.OpcodeRequestChangeWaitType:
			req, err := clientpackets.DecodeRequestChangeWaitType(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
			}
			if live != nil {
				l.changeLiveWaitType(live, req.Stand)
			}

		case clientpackets.OpcodeRequestTargetCancel:
			if _, err := clientpackets.DecodeRequestTargetCancel(payload); err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
			}
			if live != nil {
				l.clearLiveTarget(live)
			}

		case clientpackets.OpcodeStartRotating:
			req, err := clientpackets.DecodeStartRotating(payload)
			if err != nil {
				l.log.Warn().Err(err).Msg("game client")
				return
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
				return
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
				return
			}
			continue

		case clientpackets.OpcodeTradeRequest,
			clientpackets.OpcodeAddTradeItem,
			clientpackets.OpcodeTradeDone,
			clientpackets.OpcodeDummy1A,
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
			clientpackets.OpcodeDummy3E,
			clientpackets.OpcodeRequestGetOnVehicle,
			clientpackets.OpcodeRequestGetOffVehicle,
			clientpackets.OpcodeAnswerTradeRequest,
			clientpackets.OpcodeRequestEnchantItem,
			clientpackets.OpcodeRequestMoveInVehicle,
			clientpackets.OpcodeCannotMoveInVehicle,
			clientpackets.OpcodeRequestQuestListInGame,
			clientpackets.OpcodeRequestQuestAbort,
			clientpackets.OpcodeRequestAcquireSkillInfo,
			clientpackets.OpcodeRequestAcquireSkill,
			clientpackets.OpcodeRequestRestartPoint,
			clientpackets.OpcodeRequestChangePetName,
			clientpackets.OpcodeRequestPetUseItem,
			clientpackets.OpcodeRequestGiveItemToPet,
			clientpackets.OpcodeRequestGetItemFromPet,
			clientpackets.OpcodeRequestPetGetItem,
			clientpackets.OpcodeRequestPackageItemList,
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
	if l.skills != nil {
		if err := l.skills.Restore(ctx, c); err != nil {
			l.log.Error().Err(err).Int32("object_id", c.ID).Msg("enter world: restore skill state")
			return nil, false
		}
	}
	if c.CurHP < 0.5 {
		c.MarkDead()
	}

	itemListFrame, err := serverpackets.FrameItemList(items, l.itemTemplates, false)
	if err != nil {
		l.log.Error().Err(err).Msg("enter world: build ItemList")
		return nil, false
	}
	now := time.Now()
	coolTimes := skillCoolTimeEntries(c.SkillReuseTimers(now), now)

	live := l.attachLivePlayer(client, c, tmpl, items)
	if l.world != nil {
		x, y, z := c.Position()
		l.world.Spawn(live, x, y, z, c.Heading)
		l.world.AddPlayer(live)
	}

	client.Session.SendFrame(serverpackets.FrameExStorageMaxCount(c))
	client.Session.SendFrame(serverpackets.FrameHennaInfo(c.ClassID))
	client.Session.SendFrame(serverpackets.FrameEtcStatusUpdate(serverpackets.EtcStatus{}))
	client.Session.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageWelcomeToLineage))
	client.Session.SendFrame(serverpackets.FrameQuestList(nil))
	client.Session.SendFrame(serverpackets.FrameSkillList(nil))
	client.Session.SendFrame(serverpackets.FrameFriendList(nil))
	client.Session.SendFrame(serverpackets.FrameUserInfo(serverpackets.UserInfoSnapshot{Character: c, Template: tmpl, Items: items}))
	client.Session.SendFrame(itemListFrame)
	client.Session.SendFrame(serverpackets.FrameShortCutInit(serverpackets.StarterShortcuts()))
	if c.Dead() {
		client.Session.SendFrame(serverpackets.FrameDie(c.ObjectID(), serverpackets.DieOptions{}))
	}
	client.Session.SendFrame(serverpackets.FrameSkillCoolTime(coolTimes))
	client.Session.SendFrame(serverpackets.FrameActionFailed())
	return live, true
}

func skillCoolTimeEntries(timers []effect.ReuseTimer, now time.Time) []serverpackets.SkillCoolTimeEntry {
	if len(timers) == 0 {
		return nil
	}
	nowMillis := now.UnixMilli()
	entries := make([]serverpackets.SkillCoolTimeEntry, 0, len(timers))
	for _, timer := range timers {
		remaining := timer.ExpiresAt - nowMillis
		if remaining <= 0 {
			continue
		}
		entries = append(entries, serverpackets.SkillCoolTimeEntry{
			SkillID:          int32(timer.Skill.ID),
			Level:            int32(timer.Skill.Level),
			ReuseSeconds:     int32(timer.Delay / 1000),
			RemainingSeconds: int32(remaining / 1000),
		})
	}
	return entries
}

func (l *GameClientLink) attachLivePlayer(client *Client, c *player.Character, tmpl *player.Template, items []*item.Instance) *livePlayer {
	c.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventory(c.ID, l.itemTemplates, items))
	c.SetWorld(l.world)
	c.SetFrameSender(client.Session.SendFrame)
	live := &livePlayer{Character: c, template: tmpl, items: items, stopAttack: l.stopLiveAutoAttack}
	c.SetAttackBroadcaster(func(snapshot attack.Snapshot) {
		l.broadcastAttack(live, snapshot)
	})
	return live
}

func (l *GameClientLink) broadcastAttack(attacker *livePlayer, snapshot attack.Snapshot) {
	if attacker == nil {
		return
	}

	frame := serverpackets.FrameAttack(snapshot)
	encoded := append([]byte(nil), frame.Bytes()...)
	frame.Release()

	send := func(receiver interface{ SendFrame(wire.Frame) bool }) {
		receiver.SendFrame(wire.BorrowedFrame(append([]byte(nil), encoded...)))
	}
	send(attacker)

	if l.world == nil {
		return
	}
	l.world.ForEachKnown(attacker, func(o world.Tracked) {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			return
		}
		send(receiver)
	})
}

// useItem toggles the equip state of the inventory item objectID: equips
// it if unworn, unequips it if worn. A missing or non-equipable item is a
// silent no-op, matching how a stale or invalid client request is ignored
// rather than disconnecting the session.
func (l *GameClientLink) useItem(live *livePlayer, objectID int32) {
	inv := live.Inventory()
	if inv == nil {
		return
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || tmpl.Slot == item.SlotNone {
		return
	}

	var altered []*item.Instance
	if inst.Equipped() {
		if old := inv.UnequipSlot(inst.LocationData); old != nil {
			altered = []*item.Instance{old}
		}
	} else {
		altered = inv.EquipItem(inst, tmpl)
	}
	if len(altered) == 0 {
		return
	}
	l.sendInventoryUpdate(live, inv)
	l.broadcastEquipmentChange(live)
}

// unequipItem clears whatever item occupies the paperdoll position that
// bodySlot (a Slot bitmask value from the item's own template) resolves
// to. An empty or unresolvable slot is a silent no-op.
func (l *GameClientLink) unequipItem(live *livePlayer, bodySlot int32) {
	inv := live.Inventory()
	if inv == nil {
		return
	}
	paperdollSlot, ok := item.Slot(bodySlot).PaperdollIndex()
	if !ok {
		return
	}
	if inv.UnequipSlot(paperdollSlot) == nil {
		return
	}
	l.sendInventoryUpdate(live, inv)
	l.broadcastEquipmentChange(live)
}

func (l *GameClientLink) dropLiveItem(live *livePlayer, req clientpackets.RequestDropItem) {
	if live.AlikeDead() || l.groundItems == nil || req.Count <= 0 {
		return
	}
	inv := live.Inventory()
	if inv == nil {
		return
	}
	inst := inv.ItemByObjectID(req.ObjectID)
	if inst == nil {
		return
	}
	count := int(req.Count)
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || !inst.Dropable(tmpl) || inst.QuestItem(tmpl) || inst.Count < count {
		return
	}
	if !tmpl.Stackable && count > 1 {
		return
	}

	newObjectID := int32(0)
	if inst.Count > count {
		if l.ids == nil {
			return
		}
		var err error
		newObjectID, err = l.ids.NextID()
		if err != nil {
			l.log.Error().Err(err).Msg("allocate dropped item id")
			return
		}
	}
	wasEquipped := inst.Equipped() && inst.Count <= count
	dropped := inv.DropItem(req.ObjectID, count, newObjectID)
	if dropped == nil {
		return
	}
	ground, err := grounditem.New(*dropped, tmpl)
	if err != nil {
		l.log.Error().Err(err).Msg("build dropped ground item")
		return
	}

	l.sendInventoryUpdate(live, inv)
	if wasEquipped {
		l.broadcastEquipmentChange(live)
	}

	ground.SetDropperID(live.ObjectID())
	l.groundItems.Drop(ground, task.DropOptions{
		X:             int(req.X),
		Y:             int(req.Y),
		Z:             int(req.Z),
		Heading:       live.CurrentHeading(),
		PlayerDropped: true,
	})
	ground.SetDropperID(0)
}

func (l *GameClientLink) destroyLiveItem(live *livePlayer, objectID int32, count int) {
	if count <= 0 {
		return
	}
	inv := live.Inventory()
	if inv == nil {
		return
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || !inst.Destroyable(tmpl) || tmpl.HeroItem() || inst.Count < count {
		return
	}
	if !tmpl.Stackable && count > 1 {
		return
	}

	wasEquipped := inst.Equipped() && inst.Count <= count
	if inv.DestroyItem(inst, count) == nil {
		return
	}
	l.sendInventoryUpdate(live, inv)
	if wasEquipped {
		l.broadcastEquipmentChange(live)
	}
}

func (l *GameClientLink) crystallizeLiveItem(live *livePlayer, req clientpackets.RequestCrystallizeItem) {
	if req.Count <= 0 {
		return
	}
	skillLevel := live.SkillLevel(crystallizeSkillID)
	if skillLevel <= 0 {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCrystallizeLevelTooLow))
		return
	}

	inv := live.Inventory()
	if inv == nil {
		return
	}
	inst := inv.ItemByObjectID(req.ObjectID)
	if inst == nil {
		return
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || tmpl.HeroItem() || inst.ShadowItem(tmpl) {
		return
	}
	crystalItemID, crystalCount, ok := tmpl.CrystalReward(inst.EnchantLevel)
	if !ok {
		return
	}
	if !item.CanCrystallize(tmpl.Crystal, skillLevel) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCrystallizeLevelTooLow))
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if _, ok := inv.Templates().Get(crystalItemID); !ok || l.ids == nil {
		return
	}
	crystalObjectID, err := l.ids.NextID()
	if err != nil {
		l.log.Error().Err(err).Msg("allocate crystal item id")
		return
	}

	count := int(req.Count)
	if count > inst.Count {
		count = inst.Count
	}
	wasEquipped := inst.Equipped() && inst.Count <= count
	sourceItemID := inst.TemplateID
	if inv.DestroyItem(inst, count) == nil {
		return
	}
	if inv.AddNew(crystalItemID, int(crystalCount), crystalObjectID) == nil {
		return
	}

	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageItemCrystallized, sourceItemID))
	l.sendInventoryUpdate(live, inv)
	if wasEquipped {
		l.broadcastEquipmentChange(live)
	}
}

func (l *GameClientLink) sendInventoryUpdate(live *livePlayer, inv *itemcontainer.Inventory) {
	updates := inv.DrainUpdates()
	if len(updates) == 0 {
		return
	}
	items := inv.Items()
	live.items = items
	frame, err := serverpackets.FrameInventoryUpdate(updates, items, inv.Templates())
	if err != nil {
		l.log.Error().Err(err).Msg("build InventoryUpdate")
		return
	}
	live.SendFrame(frame)
}

// broadcastEquipmentChange resends UserInfo to live (refreshing its own
// paperdoll/stats) and CharInfo to every client that already knows about
// it (refreshing the worn-item visuals on their screen).
func (l *GameClientLink) broadcastEquipmentChange(live *livePlayer) {
	live.SendFrame(serverpackets.FrameUserInfo(serverpackets.UserInfoSnapshot{
		Character: live.Character, Template: live.template, Items: live.items,
	}))
	if l.world == nil {
		return
	}
	l.world.ForEachKnown(live, func(o world.Tracked) {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			return
		}
		receiver.SendFrame(serverpackets.FrameCharInfo(serverpackets.CharInfoSnapshot{
			Character: live.Character, Template: live.template, Items: live.items,
		}))
	})
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

func (l *GameClientLink) stopLivePlayer(live *livePlayer, at location.Location, heading int) {
	l.updateLivePlayerPosition(live, at, heading)
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameStopMove(live.ObjectID(), at, heading)
	})
}

func (l *GameClientLink) changeLiveMoveType(live *livePlayer, run bool) {
	if !live.SetRunning(run) {
		return
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameChangeMoveType(live.ObjectID(), live.Running(), false)
	})
}

func (l *GameClientLink) changeLiveWaitType(live *livePlayer, stand bool) {
	if live.AlikeDead() || !live.SetStanding(stand) {
		return
	}
	x, y, z := live.Position()
	waitType := serverpackets.WaitSitting
	if stand {
		waitType = serverpackets.WaitStanding
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameChangeWaitType(live.ObjectID(), waitType, location.Location{X: x, Y: y, Z: z})
	})
}

func (l *GameClientLink) broadcastLiveSocialAction(live *livePlayer, actionID int32) {
	if actionID < 2 || actionID > 13 || live.AlikeDead() || !live.Standing() || live.InCombat() {
		return
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameSocialAction(live.ObjectID(), actionID)
	})
}

func (l *GameClientLink) broadcastLiveFrame(live *livePlayer, frame func() wire.Frame) {
	live.SendFrame(frame())
	if l.world == nil {
		return
	}
	l.world.ForEachKnown(live, func(o world.Tracked) {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			return
		}
		receiver.SendFrame(frame())
	})
}

func (l *GameClientLink) handleTargetAction(live *livePlayer, objectID int32, selected bool) {
	target := l.resolveTarget(objectID)
	if target == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if live.target == nil || live.target.ObjectID() != target.ObjectID() {
		l.selectLiveTarget(live, target)
		return
	}
	if selected {
		l.attackLiveTarget(live, target)
	}
}

func (l *GameClientLink) resolveTarget(objectID int32) world.Tracked {
	if l.world == nil {
		return nil
	}
	obj, ok := l.world.Object(objectID)
	if !ok {
		return nil
	}
	target, ok := obj.(world.Tracked)
	if !ok {
		return nil
	}
	return target
}

func (l *GameClientLink) selectLiveTarget(live *livePlayer, target world.Tracked) bool {
	if live == nil || target == nil {
		return false
	}
	if live.target != nil && live.target.ObjectID() == target.ObjectID() {
		return true
	}
	live.target = target
	live.SendFrame(serverpackets.FrameMyTargetSelected(target.ObjectID(), targetColor(live.Character, target)))
	if attrs, ok := targetHPAttributes(target); ok {
		live.SendFrame(serverpackets.FrameStatusUpdate(target.ObjectID(), attrs))
	}
	l.broadcastTargetSelected(live, target)
	return true
}

func (l *GameClientLink) clearLiveTarget(live *livePlayer) {
	if live == nil {
		return
	}
	old := live.target
	live.target = nil
	live.SendFrame(serverpackets.FrameActionFailed())
	if old != nil {
		l.broadcastTargetUnselected(live)
	}
}

func (l *GameClientLink) attackLiveTarget(live *livePlayer, target world.Tracked) bool {
	combatant, ok := target.(attackable.Combatant)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return false
	}
	controller := attack.NewPlayer(live.Character)
	if !controller.CanAttack(combatant) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return false
	}
	l.startLiveAutoAttack(live)
	controller.DoAttack(combatant)
	return true
}

func (l *GameClientLink) startLiveAutoAttack(live *livePlayer) {
	if live == nil {
		return
	}
	if l.attackStance != nil {
		l.attackStance.Add(live)
	}
	if !live.SetInCombat(true) {
		return
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameAutoAttackStart(live.ObjectID())
	})
}

func (l *GameClientLink) stopLiveAutoAttack(live *livePlayer) {
	if live == nil || !live.SetInCombat(false) {
		return
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameAutoAttackStop(live.ObjectID())
	})
}

func (l *GameClientLink) broadcastTargetSelected(live *livePlayer, target world.Tracked) {
	if l.world == nil {
		return
	}
	x, y, z := live.Position()
	at := location.Location{X: x, Y: y, Z: z}
	l.world.ForEachKnown(live, func(o world.Tracked) {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			return
		}
		receiver.SendFrame(serverpackets.FrameTargetSelected(live.ObjectID(), target.ObjectID(), at))
	})
}

func (l *GameClientLink) broadcastTargetUnselected(live *livePlayer) {
	if l.world == nil {
		return
	}
	x, y, z := live.Position()
	at := location.Location{X: x, Y: y, Z: z}
	l.world.ForEachKnown(live, func(o world.Tracked) {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			return
		}
		receiver.SendFrame(serverpackets.FrameTargetUnselected(live.ObjectID(), at))
	})
}

func targetColor(attacker *player.Character, target world.Tracked) int {
	if attacker == nil {
		return 0
	}
	attackableTarget, ok := target.(interface {
		AttackableBy(attack.CreatureActor) bool
	})
	if !ok || !attackableTarget.AttackableBy(attacker) {
		return 0
	}
	return attacker.Level - targetLevel(target)
}

func targetLevel(target world.Tracked) int {
	switch t := target.(type) {
	case *livePlayer:
		return t.Level
	case *npc.Hostile:
		if t.Instance != nil && t.Instance.Template != nil {
			return t.Instance.Template.Level
		}
	}
	return 0
}

func targetHPAttributes(target world.Tracked) ([]serverpackets.StatusAttribute, bool) {
	switch t := target.(type) {
	case *livePlayer:
		return []serverpackets.StatusAttribute{
			{Type: serverpackets.StatusMaxHP, Value: int(t.MaxHP)},
			{Type: serverpackets.StatusCurrentHP, Value: int(t.CurHP)},
		}, true
	case interface {
		MaxHP() int
		CurrentHP() int
	}:
		return []serverpackets.StatusAttribute{
			{Type: serverpackets.StatusMaxHP, Value: t.MaxHP()},
			{Type: serverpackets.StatusCurrentHP, Value: t.CurrentHP()},
		}, true
	default:
		return nil, false
	}
}

func (l *GameClientLink) handleSummonActionUse(live *livePlayer, req clientpackets.RequestActionUse) bool {
	command, ok := summonCommandForActionID(req.ActionID)
	if !ok || l.world == nil {
		return false
	}
	obj, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return true
	}
	actor, ok := obj.(*summon.Actor)
	if !ok {
		return true
	}
	result := actor.ApplyCommand(summon.CommandContext{Command: command, World: l.world})
	if id, ok := systemMessageForSummonFeedback(result.Feedback); ok {
		live.SendFrame(serverpackets.FrameSystemMessage(id))
	}
	return true
}

func summonCommandForActionID(actionID int32) (summon.Command, bool) {
	switch actionID {
	case 15, 21:
		return summon.CommandToggleFollow, true
	case 16, 22:
		return summon.CommandAttack, true
	case 17, 23:
		return summon.CommandStop, true
	case 19:
		return summon.CommandReturnPet, true
	case 52:
		return summon.CommandUnsummonServitor, true
	case 53, 54:
		return summon.CommandMoveToTarget, true
	default:
		return 0, false
	}
}

func systemMessageForSummonFeedback(feedback summon.Feedback) (int, bool) {
	switch feedback {
	case summon.FeedbackPetRefusingOrder:
		return serverpackets.SystemMessagePetRefusingOrder, true
	case summon.FeedbackDeadPetCannotBeReturned:
		return serverpackets.SystemMessageDeadPetCannotBeReturned, true
	case summon.FeedbackPetCannotBeSentBackDuringBattle:
		return serverpackets.SystemMessagePetCannotSentBackDuringBattle, true
	case summon.FeedbackCannotRestoreHungryPet:
		return serverpackets.SystemMessageYouCannotRestoreHungryPets, true
	case summon.FeedbackPetTooHighToControl:
		return serverpackets.SystemMessagePetTooHighToControl, true
	default:
		return 0, false
	}
}

func (l *GameClientLink) detachLivePlayer(ctx context.Context, live *livePlayer) {
	if live == nil {
		return
	}
	if l.roster != nil || l.skills != nil {
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), livePlayerDetachSaveTimeout)
		defer cancel()
		if l.roster != nil {
			if err := l.roster.SavePosition(saveCtx, live.Character); err != nil {
				l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("save player position")
			}
		}
		if l.skills != nil {
			if err := l.skills.Save(saveCtx, live.Character, true); err != nil {
				l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("save player skill state")
			}
		}
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
