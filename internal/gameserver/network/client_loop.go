package network

import (
	"cmp"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

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
			client.closeNow()
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

	// chars, entering and live are read entirely by this goroutine: chars
	// resolves the character-list slot indices RequestCharacterDelete,
	// CharacterRestore and RequestGameStart address; entering is the player
	// RequestGameStart restored and registered, for EnterWorld to spawn; live
	// is that player once EnterWorld took it.
	var chars []*player.Character
	var entering, live *livePlayer
	defer func() {
		// A connection lost between selection and EnterWorld takes the
		// selected player out of the world as a logout would.
		if leaving := cmp.Or(live, entering); leaving != nil {
			var owners []int32
			onLive(leaving, func() { owners = l.detachLivePlayer(leaving) })
			_ = l.awaitPersistence(conn, owners...)
		}
		if l.clients != nil {
			l.clients.Release(client.AccountName(), client)
		}
		l.notifyPlayerLogout(client.AccountName())
	}()

	for {
		// A task this goroutine waited on panicked. sim recovered it, but the
		// handler stopped part-way through its mutation and the client was
		// told nothing, so the session ends exactly as a fatal decode error
		// ends it: the deferred detach above saves and detaches the
		// character. Timers and ticks keep the pool's own recovery — nothing
		// waits on them, and dropping a session over a tick would be a
		// regression in the other direction.
		if live != nil && live.handlerPanicked {
			l.log.Warn().Str("account", client.AccountName()).Msg("game client disconnected: panic in queued packet handler")
			return
		}
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
		// Packet-protection gate, ahead of the state gate: every received
		// frame is counted, and while a flood is active frames are dropped,
		// answering ActionFailed at most once per second of ongoing flood.
		// After counting (so dropped frames never reach a handler), an
		// excess of detected floods per minute disconnects, as does a hard
		// cap on packets processed pre-auth. The queue-size accounting and
		// burst cap of a queued packet reader have no counterpart here: this
		// read loop processes each frame inline with its read, so there is
		// no inbound queue to overflow or drain in batches.
		now := l.now
		if now == nil {
			now = time.Now
		}
		// A dropped frame never reaches a handler: that includes the
		// flood-onset packet itself, which answers ActionFailed and is
		// discarded like every other frame of the flood.
		if actionFailed, drop := client.stats.countIncomingPacket(now()); actionFailed || drop {
			if actionFailed {
				session.SendFrame(serverpackets.FrameActionFailed())
			}
			continue
		} else if client.stats.floodsExceeded() {
			l.log.Warn().Str("state", client.State().String()).Msg("game client disconnected: too many packet floods")
			client.closeNow()
			return
		} else if client.State() == StateConnected && client.stats.processedPackets > preAuthMaxProcessedPackets {
			l.log.Warn().Str("state", client.State().String()).Msg("game client disconnected: too many packets in non-authed state")
			client.closeNow()
			return
		}
		if !client.Accept(opcode) {
			l.log.Warn().Str("state", client.State().String()).Str("opcode", hex.EncodeToString(payload)).Msg("Accept opcode")
			// Pre-auth, any rejected opcode disconnects immediately; once
			// authenticated, tolerate up to maxUnknownPerMin within a 60s
			// sliding window, mirroring GameClient.onUnknownPacket.
			if client.State() == StateConnected || client.countUnknownPacket() {
				client.closeNow()
				return
			}
			continue
		}

		// Once in the world, everything a frame does to the player runs as a
		// task on its queue (onLive), serialized with its timers and ticks.
		// The loop waits for each task before reading on, so frames are
		// still handled one at a time in read order; decoding and the
		// protection gates above stay on this goroutine.
		if clearsSpawnProtection(opcode) {
			// A panic here must not let this frame's own handler run behind
			// it; the check at the top of the loop ends the session.
			if !onLive(live, func() { l.clearSpawnProtectionOnAction(live) }) {
				continue
			}
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
			if !session.SendFrame(serverpackets.FrameVersionCheck(key, !l.noCipher)) {
				return
			}
			// The key is out; every later frame crosses encrypted: crypt
			// starts only after VersionCheck.
			if !l.noCipher {
				session.EnableCrypt()
			}

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
				// A rejection already answered AuthLoginFail; it also closes
				// with ServerClose. A canceled wait or a lost login link
				// closes silently.
				if err == nil {
					client.closeNow()
				}
				return
			}
			session.CompleteHandshake()
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
			// The character-select reuse gate answers a refused delete
			// with the deletion-failed reply; nothing else runs.
			if !client.performFloodProtected(floodProtectorCharacterSelect, l.playerConfig.CharacterSelectDelay, time.Now()) {
				session.SendFrame(serverpackets.FrameCharDeleteFail(serverpackets.CharDeleteFailReasonDeletionFailed))
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
			// A clan's members and leader may not be deleted. The roster
			// held in memory is the stored one plus every change since,
			// fresher than the character list read above.
			if cl, member := l.clanService().Table().MemberClan(c.ID); member {
				reason := serverpackets.CharDeleteFailReasonClanMemberMayNotDelete
				if cl.IsLeader(c.ID) {
					reason = serverpackets.CharDeleteFailReasonClanLeaderMayNotDelete
				}
				session.SendFrame(serverpackets.FrameCharDeleteFail(reason))
			} else if err := l.roster.MarkForDeletion(ctx, c.ID); err != nil {
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
			// The character-select reuse gate refuses a restore silently,
			// leaving the client's character list untouched.
			if !client.performFloodProtected(floodProtectorCharacterSelect, l.playerConfig.CharacterSelectDelay, time.Now()) {
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

		case clientpackets.OpcodeRequestSetPledgeCrest:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestSetPledgeCrest)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestSetPledgeCrest(live, req) })
			}

		case clientpackets.OpcodeRequestJoinAlly:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestJoinAlly)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestJoinAlly(live, req) })
			}

		case clientpackets.OpcodeRequestAnswerJoinAlly:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestAnswerJoinAlly)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestAnswerJoinAlly(live, req) })
			}

		case clientpackets.OpcodeAllyLeave:
			if live != nil {
				onLive(live, func() { l.allyLeave(live) })
			}

		case clientpackets.OpcodeAllyDismiss:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeAllyDismiss)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.allyDismiss(live, req) })
			}

		case clientpackets.OpcodeRequestDismissAlly:
			if live != nil {
				onLive(live, func() { l.requestDismissAlly(live) })
			}

		case clientpackets.OpcodeRequestSetAllyCrest:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestSetAllyCrest)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestSetAllyCrest(live, req) })
			}

		case clientpackets.OpcodeRequestAllyInfo:
			if live != nil {
				onLive(live, func() { l.requestAllyInfo(live) })
			}

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
			// The character-select reuse gate refuses a selection silently.
			if !client.performFloodProtected(floodProtectorCharacterSelect, l.playerConfig.CharacterSelectDelay, time.Now()) {
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
				// Another character of the account still in the world is a
				// previous session of the account that has not finished
				// leaving, and it may hold this character's freight, which it
				// writes on its way out. The selection aborts silently until
				// that session is gone; its writes are then on this
				// character's lane, which the wait below covers.
				if prev, ok := l.accountPlayerInWorld(chars); ok {
					l.log.Info().Int32("object_id", prev.ObjectID()).
						Msg("game client: account character still in the world, closing previous session")
					prev.kickClient()
					continue
				}
			}
			// A previous session of this character has left the world, so
			// every save it queued is on the lane. Wait for them, then read
			// the row fresh: the list above may predate those saves, and
			// selection restores the character from its saved row.
			// A wait that gave up refuses the selection silently, as the
			// other early exits here do, rather than load unwritten rows.
			if l.awaitPersistence(conn, c.ObjectID()) != nil {
				continue
			}
			fresh, err := l.roster.Load(ctx, c.ObjectID())
			if err != nil {
				l.log.Error().Err(err).Int32("object_id", c.ObjectID()).Msg("select character: reload row")
				continue
			}
			// The list may predate a ban stored since; the row is what
			// counts.
			if fresh.AccessLevel < 0 {
				continue
			}
			c = fresh
			chars[req.Slot] = fresh
			l.clanService().RestoreMembership(c, time.Now())
			l.applyLoadedAccessLevel(c)
			tmpl, ok := l.templates.Get(c.ClassID())
			if !ok {
				l.log.Error().Int("class_id", c.ClassID()).Msg("select character: no template loaded")
				return
			}
			// The selection restores the character in full. A restore that
			// fails attaches nothing, and closes the connection.
			selected, ok := l.restoreSelected(ctx, client, c)
			if !ok {
				client.closeNow()
				return
			}
			entering = selected
			session.SendFrame(serverpackets.FrameSSQInfo())
			client.SetState(StateEntering)
			session.SendFrame(serverpackets.FrameCharSelected(serverpackets.CharSelectedSnapshot{
				Character: c, Template: tmpl, SessionID: client.SessionKey().PlayKey1,
				GameTime: l.gameTime(),
			}))
			// From here on lookups by name and id find the character, as
			// the world checks above do for a later selection; it is spawned
			// only once EnterWorld arrives. Registered after CharSelected,
			// so nothing sent to it can reach its client ahead of the
			// selection's answer.
			if l.world != nil {
				l.world.AddPlayer(selected)
			}

		case clientpackets.OpcodeEnterWorld:
			// Unreachable while the state gate admits EnterWorld only in
			// StateEntering, which character selection enters together with
			// entering; closed like any other failed entry if that changes.
			if entering == nil {
				client.closeNow()
				return
			}
			// A failed entry leaves live attached and registered; the
			// deferred detachLivePlayer above is what releases it.
			live, entering = entering, nil
			if !l.enterWorld(client, live) {
				client.closeNow()
				return
			}
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
					client.closeNow()
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
					onLive(live, func() { l.handleAutoSoulShot(live, req) })
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
					onLive(live, func() { l.sendEnchantSkillInfo(live, req) })
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
					onLive(live, func() { l.applyEnchantSkill(live, req) })
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
			case clientpackets.OpcodeRequestExSetPledgeCrestLarge:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestExSetPledgeCrestLarge)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if live != nil {
					onLive(live, func() { l.requestSetLargePledgeCrest(live, req) })
				}
			case clientpackets.OpcodeRequestPledgePowerGrades:
				if live != nil {
					onLive(live, func() { l.requestPledgePowerGradeList(live) })
				}
			case clientpackets.OpcodeRequestPledgeMemberPower, clientpackets.OpcodeRequestPledgeMemberDetail:
				req, err := decodeClientPacket(l, client, payload, func(p []byte) (clientpackets.RequestPledgeMemberName, error) {
					return clientpackets.DecodeRequestPledgeMemberName(p, second)
				})
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if live != nil {
					if second == clientpackets.OpcodeRequestPledgeMemberPower {
						onLive(live, func() { l.requestPledgeMemberPowerInfo(live, req) })
					} else {
						onLive(live, func() { l.requestPledgeMemberInfo(live, req) })
					}
				}
			case clientpackets.OpcodeRequestPledgeSetGrade:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPledgeSetMemberPowerGrade)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if live != nil {
					onLive(live, func() { l.requestPledgeSetMemberPowerGrade(live, req) })
				}
			case clientpackets.OpcodeRequestPledgeWarList:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPledgeWarList)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if live != nil {
					onLive(live, func() { l.requestPledgeWarList(live, req) })
				}
			case clientpackets.OpcodeRequestPledgeReorganizeMember:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPledgeReorganizeMember)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if live != nil {
					onLive(live, func() { l.requestPledgeReorganizeMember(live, req) })
				}
			case clientpackets.OpcodeRequestPledgeSetAcademyMaster:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPledgeSetAcademyMaster)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if live != nil {
					onLive(live, func() { l.requestPledgeSetAcademyMaster(live, req) })
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
					onLive(live, func() { l.handleMagicSkillUseGround(live, req) })
				}
			case clientpackets.OpcodeRequestConfirmTargetItem:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestConfirmTargetItem)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if live != nil {
					onLive(live, func() { l.confirmAugmentTarget(live, req) })
				}
			case clientpackets.OpcodeRequestConfirmRefinerItem:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestConfirmRefinerItem)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if live != nil {
					onLive(live, func() { l.confirmAugmentRefiner(live, req) })
				}
			case clientpackets.OpcodeRequestConfirmGemStone:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestConfirmGemStone)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if live != nil {
					onLive(live, func() { l.confirmAugmentGemstone(live, req) })
				}
			case clientpackets.OpcodeRequestRefine:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestRefine)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if live != nil {
					onLive(live, func() { l.refineAugment(live, req) })
				}
			case clientpackets.OpcodeRequestConfirmCancelItem:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestConfirmCancelItem)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if live != nil {
					onLive(live, func() { l.confirmAugmentCancel(live, req) })
				}
			case clientpackets.OpcodeRequestRefineCancel:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestRefineCancel)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if live != nil {
					onLive(live, func() { l.cancelAugment(live, req) })
				}
			case clientpackets.OpcodeRequestChangePartyLeader, clientpackets.OpcodeRequestExAskJoinMPCC,
				clientpackets.OpcodeRequestExAcceptJoinMPCC, clientpackets.OpcodeRequestExOustFromMPCC,
				clientpackets.OpcodeRequestExMPCCShowPartyMembersInfo:
				if !l.dispatchPartyExtended(client, live, second, payload) {
					return
				}
			case clientpackets.OpcodeRequestOustFromPartyRoom, clientpackets.OpcodeRequestDismissPartyRoom,
				clientpackets.OpcodeRequestWithdrawPartyRoom, clientpackets.OpcodeRequestAskJoinPartyRoom,
				clientpackets.OpcodeAnswerJoinPartyRoom, clientpackets.OpcodeRequestListPartyMatchingWaitingRoom,
				clientpackets.OpcodeRequestExitPartyMatchingWaitingRoom:
				if !l.dispatchPartyMatchExtended(client, live, second, payload) {
					return
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
					client.closeNow()
					return
				}
			}

		case clientpackets.OpcodeRequestSkillCoolTime:
			// This opcode is accepted and answered with nothing: reuse
			// timers reach the client unsolicited (e.g. in the EnterWorld
			// burst), never as a request reply.
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
				onLive(live, func() { l.handleMagicSkillUse(live, req) })
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
			onLive(live, func() {
				// A plain click on the already-selected object acts on it
				// (attack, sit, pick up), exactly like an attack request —
				// the client sends this second click expecting the action
				// to resolve, and locks its own input until Attack or
				// ActionFailed answers it.
				selected := live.Target() != nil && live.Target().ObjectID() == req.ObjectID
				l.handleTargetAction(ctx, live, req.ObjectID, selected, false, req.Shift)
			})

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
			onLive(live, func() {
				// A forced attack is an attack request on the object already
				// selected; on any other object it only selects it.
				selected := live.Target() != nil && live.Target().ObjectID() == req.ObjectID
				l.handleTargetAction(ctx, live, req.ObjectID, selected, selected, req.Shift)
			})

		case clientpackets.OpcodeLogout:
			if live == nil {
				// Logout from character select has no character to take out
				// of the world: it is ignored silently and the connection
				// stays open. The client registers no pending action for
				// it, so there is nothing to release.
				continue
			}
			refused := false
			onLive(live, func() {
				if block := l.exitBlockReason(live); block != exitAllowed {
					l.refuseExit(session, live, block, false)
					refused = true
					return
				}
				// LeaveWorld is the last packet: what detach sends
				// afterwards never reaches the client.
				session.sendLast(serverpackets.FrameLeaveWorld())
			})
			if refused {
				continue
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
			onLive(live, func() {
				l.moveLivePlayer(live,
					location.Location{X: int(req.TargetX), Y: int(req.TargetY), Z: int(req.TargetZ)},
					location.Location{X: int(req.OriginX), Y: int(req.OriginY), Z: int(req.OriginZ)},
				)
			})

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
			onLive(live, func() { l.stopLivePlayer(live) })

		case clientpackets.OpcodeValidatePosition:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeValidatePosition)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() {
					l.validateLivePlayerPosition(live, location.Location{X: int(req.X), Y: int(req.Y), Z: int(req.Z)})
				})
			}

		case clientpackets.OpcodeRequestItemList:
			if live == nil {
				continue
			}
			// Build and send on the player's queue. Handing the frame back
			// to the connection to send would let an inventory drain queued
			// behind this task overtake the snapshot it supersedes, leaving
			// the client on stale counts until that item changes again.
			var failed bool
			onLive(live, func() {
				// A shop or warehouse window just opened keeps the request
				// unanswered, as specified: no list, no
				// ActionFailed. The inventory button leaves no client
				// action pending.
				if live.inventoryDisabled.Load() {
					return
				}
				// Carried weight is recomputed on every item-list send, not
				// only at login.
				if inv := live.Inventory(); inv != nil {
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
				frame, err := live.buildItemList(l.itemTemplates, true)
				if err != nil {
					l.log.Error().Err(err).Msg("build ItemList")
					failed = true
					return
				}
				session.SendFrame(frame)
			})
			if failed {
				return
			}

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
			// The dice gate belongs to this read loop, which stays parked
			// in onLive while useItem runs, so the throw consults it there.
			rollDice := func() bool {
				return client.performFloodProtected(floodProtectorRollDice, l.playerConfig.RollDiceDelay, time.Now())
			}
			onLive(live, func() { l.useItem(live, req.ObjectID, req.CtrlPressed, rollDice) })

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
			onLive(live, func() { l.unequipItem(live, req.BodySlot) })

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
			onLive(live, func() { l.dropLiveItem(live, req) })

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
			onLive(live, func() { l.destroyLiveItem(live, req.ObjectID, int(req.Count)) })

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
			onLive(live, func() { l.crystallizeLiveItem(live, req) })

		case clientpackets.OpcodeRequestRecipeBookOpen:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestRecipeBookOpen)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.openRecipeBook(live, req) })
			}

		case clientpackets.OpcodeRequestRecipeBookDestroy:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestRecipeBookDestroy)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.destroyRecipe(live, req) })
			}

		case clientpackets.OpcodeRequestRecipeItemMakeInfo:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestRecipeItemMakeInfo)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.sendRecipeItemMakeInfo(live, req) })
			}

		case clientpackets.OpcodeRequestRecipeItemMakeSelf:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestRecipeItemMakeSelf)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			// A craft inside the manufacture reuse window is dropped
			// silently, before anything else is looked at.
			if !client.performFloodProtected(floodProtectorManufacture, l.playerConfig.ManufactureDelay, time.Now()) {
				continue
			}
			if live != nil {
				onLive(live, func() { l.makeRecipeSelf(live, req) })
			}

		case clientpackets.OpcodeRequestHennaItemList:
			_, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestHennaItemList)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.sendHennaEquipList(live) })
			}

		case clientpackets.OpcodeRequestHennaItemInfo:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestHennaItemInfo)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.sendHennaItemInfo(live, req) })
			}

		case clientpackets.OpcodeRequestHennaEquip:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestHennaEquip)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.drawHenna(live, req) })
			}

		case clientpackets.OpcodeRequestHennaUnequipList:
			_, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestHennaUnequipList)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { sendHennaUnequipList(live) })
			}

		case clientpackets.OpcodeRequestHennaUnequipInfo:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestHennaUnequipInfo)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.sendHennaUnequipInfo(live, req) })
			}

		case clientpackets.OpcodeRequestHennaUnequip:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestHennaUnequip)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.deleteHenna(live, req) })
			}

		case clientpackets.OpcodeRequestPrivateStoreManageSell, clientpackets.OpcodeSetPrivateStoreListSell,
			clientpackets.OpcodeRequestPrivateStoreQuitSell, clientpackets.OpcodeSetPrivateStoreMsgSell,
			clientpackets.OpcodeRequestPrivateStoreBuy, clientpackets.OpcodeRequestPrivateStoreManageBuy,
			clientpackets.OpcodeSetPrivateStoreListBuy, clientpackets.OpcodeRequestPrivateStoreQuitBuy,
			clientpackets.OpcodeSetPrivateStoreMsgBuy, clientpackets.OpcodeRequestPrivateStoreSell,
			clientpackets.OpcodeRequestRecipeShopMessageSet, clientpackets.OpcodeRequestRecipeShopListSet,
			clientpackets.OpcodeRequestRecipeShopManageQuit, clientpackets.OpcodeRequestRecipeShopMakeInfo,
			clientpackets.OpcodeRequestRecipeShopMakeItem, clientpackets.OpcodeRequestRecipeShopManagePrev:
			if !l.dispatchPrivateStore(client, live, opcode, payload) {
				return
			}

		case clientpackets.OpcodeMultiSellChoose:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeMultiSellChoose)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				// The reuse gate is consulted only with a player in the
				// world; a refused exchange still drops the open list.
				allowed := client.performFloodProtected(floodProtectorMultisell, l.playerConfig.MultisellDelay, time.Now())
				onLive(live, func() { l.requestMultiSellChoose(live, req, allowed) })
			}

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
			onLive(live, func() { l.enchantLiveItem(ctx, live, req) })

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
			onLive(live, func() {
				session.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
			})

		case clientpackets.OpcodeRequestAcquireSkillInfo:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestAcquireSkillInfo)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.sendAcquireSkillInfo(live, req) })
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
				onLive(live, func() { l.learnAcquireSkill(live, req) })
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
			onLive(live, func() {
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
			})

		case clientpackets.OpcodeRequestRestartPoint:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestRestartPoint)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.restartLivePlayer(live, req) })
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
				onLive(live, func() { l.broadcastLiveSocialAction(live, req.ActionID) })
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
				onLive(live, func() {
					// A rider keeps its stance; silent for the same reasons
					// as the action-bar toggle above.
					if live.Mounted() {
						return
					}
					l.changeLiveMoveType(live, req.Run)
				})
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
				onLive(live, func() { l.requestChangeWaitType(live, req.Stand) })
			}

		case clientpackets.OpcodeRequestLinkHtml:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestLinkHTML)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			onLive(live, func() { l.requestLinkHTML(live, req) })

		case clientpackets.OpcodeRequestBypassToServer:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestBypassToServer)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			// An empty command returns before the bypass reuse gate is
			// consulted, so it never consumes the cooldown; a refused
			// command is dropped silently.
			if req.Command != "" && !client.performFloodProtected(floodProtectorServerBypass, l.playerConfig.ServerBypassDelay, time.Now()) {
				continue
			}
			onLive(live, func() { l.requestBypassToServer(live, req) })
			l.finishPendingClassChange(live)

		case clientpackets.OpcodeRequestShowBoard:
			if _, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestShowBoard); err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestShowBoard(live) })
			}

		case clientpackets.OpcodeRequestBBSWrite:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestBBSWrite)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestBBSWrite(live, req) })
			}

		case clientpackets.OpcodeSendBypassBuildCmd:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeSendBypassBuildCmd)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.sendBypassBuildCmd(live, req) })
			}

		case clientpackets.OpcodeRequestGmList:
			// The request carries no body.
			if live != nil {
				onLive(live, func() { l.requestGmList(live) })
			}

		case clientpackets.OpcodeRequestPetition:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPetition)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestPetition(live, req) })
			}

		case clientpackets.OpcodeRequestPetitionCancel:
			// The request carries no body.
			if live != nil {
				onLive(live, func() { l.requestPetitionCancel(live) })
			}

		case clientpackets.OpcodePetitionVote:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodePetitionVote)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.petitionVote(live, req) })
			}

		case clientpackets.OpcodeRequestTargetCancel:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestTargetCancel)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestTargetCancel(live, req) })
			}

		case clientpackets.OpcodeAppearing:
			if _, err := decodeClientPacket(l, client, payload, clientpackets.DecodeAppearing); err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() {
					l.completeLivePlayerTeleport(live)
					live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
				})
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
			onLive(live, func() {
				l.broadcastLiveFrame(live, func() wire.Frame {
					return serverpackets.FrameStartRotation(live.ObjectID(), int(req.Degree), int(req.Side), 0)
				})
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
			onLive(live, func() {
				live.SetHeading(int(req.Degree))
				l.broadcastLiveFrame(live, func() wire.Frame {
					return serverpackets.FrameStopRotation(live.ObjectID(), int(req.Degree), 0)
				})
			})

		case clientpackets.OpcodeRequestRestart:
			if live == nil {
				continue
			}
			// The exit check and detach run on the player's queue; waiting for
			// the saves detach queued stays on this goroutine.
			var owners []int32
			refused := false
			// This is the one dispatch site that clears live, so a panic
			// here must be acted on before that happens: past `live = nil`
			// the guard at the top of the loop can never fire again, the
			// session would stay open with a half-detached character still
			// registered in the world, and Handle's deferred detach would
			// be skipped too. Leaving live in place and taking the guard
			// instead ends the session and finishes the teardown the
			// panicking task abandoned.
			if !onLive(live, func() {
				if block := l.exitBlockReason(live); block != exitAllowed {
					l.refuseExit(session, live, block, true)
					refused = true
					return
				}
				live.Character.DetachSession()
				owners = l.detachLivePlayer(live)
			}) {
				continue
			}
			if refused {
				continue
			}
			_ = l.awaitPersistence(conn, owners...)
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
			onLive(live, func() { l.sendPackageSendableItemList(live, req.ObjectID) })

		case clientpackets.OpcodeRequestPetUseItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPetUseItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			onLive(live, func() { l.petUseItem(ctx, live, req) })

		case clientpackets.OpcodeRequestGiveItemToPet:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestGiveItemToPet)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			onLive(live, func() { l.giveItemToPet(ctx, live, req) })

		case clientpackets.OpcodeRequestGetItemFromPet:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestGetItemFromPet)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			onLive(live, func() { l.getItemFromPet(ctx, live, req) })

		case clientpackets.OpcodeRequestPetGetItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPetGetItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			onLive(live, func() { l.petGetItem(ctx, live, req) })

		case clientpackets.OpcodeRequestJoinParty, clientpackets.OpcodeRequestAnswerJoinParty,
			clientpackets.OpcodeRequestWithdrawParty, clientpackets.OpcodeRequestOustPartyMember:
			if !l.dispatchParty(client, live, opcode, payload) {
				return
			}

		case clientpackets.OpcodeRequestListPartyWaiting, clientpackets.OpcodeRequestManagePartyRoom,
			clientpackets.OpcodeRequestJoinPartyRoom:
			if !l.dispatchPartyMatch(client, live, opcode, payload) {
				return
			}

		case clientpackets.OpcodeTradeRequest:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeTradeRequest)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.handleTradeRequest(live, req) })
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
				onLive(live, func() { l.handleAnswerTradeRequest(live, req) })
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
				onLive(live, func() { l.handleAddTradeItem(live, req) })
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
				onLive(live, func() { l.handleTradeDone(ctx, live, req) })
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
				onLive(live, func() { l.registerShortcut(live, req) })
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
				onLive(live, func() { l.deleteShortcut(live, req) })
			}

		case clientpackets.OpcodeRequestMakeMacro:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestMakeMacro)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.makeMacro(live, req) })
			}

		case clientpackets.OpcodeRequestDeleteMacro:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestDeleteMacro)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.deleteMacro(live, req) })
			}

		case clientpackets.OpcodeRequestEvaluate:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestEvaluate)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.evaluate(live, req) })
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
				// Runs its own queue hops: the pets-table read in the middle
				// stays here, off the queue (see handleRequestChangePetName).
				l.handleRequestChangePetName(ctx, live, req)
			}

		case clientpackets.OpcodeRequestBuyItem:
			req, err := decodeClientPacket(l, client, payload, func(p []byte) (clientpackets.RequestBuyItem, error) {
				return clientpackets.DecodeRequestBuyItem(p, l.playerConfig.InventorySlots.MaxItemInPacket())
			})
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestBuyItem(live, req) })
			}

		case clientpackets.OpcodeRequestPreviewItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPreviewItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestPreviewItem(live, req) })
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
				onLive(live, func() { l.handleDlgAnswer(live, req) })
			}

		case clientpackets.OpcodeRequestSellItem:
			req, err := decodeClientPacket(l, client, payload, func(p []byte) (clientpackets.RequestSellItem, error) {
				return clientpackets.DecodeRequestSellItem(p, l.playerConfig.InventorySlots.MaxItemInPacket())
			})
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestSellItem(live, req) })
			}

		case clientpackets.OpcodeSendWarehouseDeposit:
			req, err := decodeClientPacket(l, client, payload, func(p []byte) (clientpackets.SendWarehouseDepositList, error) {
				return clientpackets.DecodeSendWarehouseDepositList(p, l.playerConfig.InventorySlots.MaxItemInPacket())
			})
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestWarehouseDeposit(live, req) })
			}

		case clientpackets.OpcodeSendWarehouseWithdraw:
			req, err := decodeClientPacket(l, client, payload, func(p []byte) (clientpackets.SendWarehouseWithdrawList, error) {
				return clientpackets.DecodeSendWarehouseWithdrawList(p, l.playerConfig.InventorySlots.MaxItemInPacket())
			})
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestWarehouseWithdraw(live, req) })
			}

		case clientpackets.OpcodeRequestPackageSend:
			req, err := decodeClientPacket(l, client, payload, func(p []byte) (clientpackets.RequestPackageSend, error) {
				return clientpackets.DecodeRequestPackageSend(p, l.playerConfig.InventorySlots.MaxItemInPacket())
			})
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestPackageSend(live, req) })
			}

		case clientpackets.OpcodeRequestFriendInvite:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestFriendInvite)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.handleRequestFriendInvite(live, req) })
			}

		case clientpackets.OpcodeRequestAnswerFriendInvite:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestAnswerFriendInvite)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.handleRequestAnswerFriendInvite(live, req) })
			}

		case clientpackets.OpcodeRequestFriendList:
			// The request carries no body.
			if live != nil {
				onLive(live, func() { l.handleRequestFriendList(live) })
			}

		case clientpackets.OpcodeRequestFriendDel:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestFriendDel)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.handleRequestFriendDel(live, req) })
			}

		case clientpackets.OpcodeRequestBlock:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestBlock)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.handleRequestBlock(live, req) })
			}

		case clientpackets.OpcodeRequestSendL2FriendSay:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestSendL2FriendSay)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.handleRequestSendL2FriendSay(live, req) })
			}

		case clientpackets.OpcodeRequestJoinPledge:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestJoinPledge)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestJoinPledge(live, req) })
			}

		case clientpackets.OpcodeRequestAnswerJoinPledge:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestAnswerJoinPledge)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestAnswerJoinPledge(live, req) })
			}

		case clientpackets.OpcodeRequestOustPledgeMember:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestOustPledgeMember)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestOustPledgeMember(live, req) })
			}

		case clientpackets.OpcodeRequestPledgeInfo:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPledgeInfo)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestPledgeInfo(live, req) })
			}

		case clientpackets.OpcodeRequestPledgePower:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPledgePower)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestPledgePower(live, req) })
			}

		case clientpackets.OpcodeRequestWithdrawPledge:
			if live != nil {
				onLive(live, func() { l.requestWithdrawPledge(live) })
			}

		case clientpackets.OpcodeRequestPledgeMemberList:
			if live != nil {
				onLive(live, func() { l.requestPledgeMemberList(live) })
			}

		case clientpackets.OpcodeRequestStartPledgeWar, clientpackets.OpcodeRequestStopPledgeWar,
			clientpackets.OpcodeRequestSurrenderPledgeWar, clientpackets.OpcodeRequestSurrenderPersonally:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPledgeWarName)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				switch opcode {
				case clientpackets.OpcodeRequestStartPledgeWar:
					onLive(live, func() { l.requestStartPledgeWar(live, req) })
				case clientpackets.OpcodeRequestStopPledgeWar:
					onLive(live, func() { l.requestStopPledgeWar(live, req) })
				case clientpackets.OpcodeRequestSurrenderPersonally:
					onLive(live, func() { l.requestSurrenderPersonally(live, req) })
				default:
					onLive(live, func() { l.requestSurrenderPledgeWar(live, req) })
				}
			}

		case clientpackets.OpcodeRequestReplyStartPledgeWar, clientpackets.OpcodeRequestReplyStopPledgeWar,
			clientpackets.OpcodeRequestReplySurrenderPledgeWar:
			if _, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPledgeWarReply); err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestReplyPledgeWar(live, opcode) })
			}

		case clientpackets.OpcodeRequestShowMiniMap:
			// The request carries no body; with no player in the world
			// nothing answers, as the specified handler does.
			if live != nil {
				onLive(live, func() { l.showMiniMap(live, serverpackets.RegularMapID) })
			}

		case clientpackets.OpcodeRequestRecordInfo:
			// The request carries no body; with no player in the world
			// nothing answers, as the specified handler does.
			if live != nil {
				onLive(live, func() { l.requestRecordInfo(live) })
			}

		case clientpackets.OpcodeSay2:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeSay2)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.handleSay2(client, live, req) })
			}

		case clientpackets.OpcodeRequestUserCommand:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestUserCommand)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if live != nil {
				onLive(live, func() { l.requestUserCommand(live, req.CommandID) })
			}

		case clientpackets.OpcodeDummy1A,
			clientpackets.OpcodeDummy23,
			clientpackets.OpcodeDummy2E,
			clientpackets.OpcodeDummy34,
			clientpackets.OpcodeDummy3E,
			clientpackets.OpcodeRequestGetOnVehicle,
			clientpackets.OpcodeRequestGetOffVehicle,
			clientpackets.OpcodeRequestMoveInVehicle,
			clientpackets.OpcodeCannotMoveInVehicle,
			clientpackets.OpcodeRequestQuestListInGame,
			clientpackets.OpcodeRequestQuestAbort,
			clientpackets.OpcodeGameGuardReply:
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
		clientpackets.OpcodeRequestPledgeCrest, clientpackets.OpcodeAppearing,
		clientpackets.OpcodeRequestPledgeInfo:
		return false
	default:
		return true
	}
}

func normalReadFrameError(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrDeadlineExceeded)
}

// authenticate validates req against the login server over the game
// server's current link, advancing client to StateAuthed on success.
// AuthLoginFail (and the connection close that follows) is the caller's
// job for every false/error result except the login-link-down case, which
// authenticate handles itself since there is no in-flight validation to
// fail.
