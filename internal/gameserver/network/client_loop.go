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
		client.frameUnread = true
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
	// A server stop ends the connection from here. A client with a
	// character in the world (selected or entered) reads ServerClose as its
	// last frame and leaves the world; any other connection just closes once
	// what is queued is flushed. The read loop then ends as on a lost
	// connection.
	stopOnShutdown := context.AfterFunc(ctx, func() {
		if client.State() >= StateEntering {
			session.Close()
			return
		}
		_ = conn.Close()
	})
	defer stopOnShutdown()

	cs := &connState{}
	defer func() {
		// A connection lost between selection and EnterWorld takes the
		// selected player out of the world as a logout would.
		if leaving := cmp.Or(cs.live, cs.entering); leaving != nil {
			leaving.sessionEnded.Store(true)
			awaitDetachDelay(ctx, session, leaving, detachDelay(leaving, cs.lost))
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
		// The player the handler left in the world acted, unless the frame
		// never reached its handler or the handler panicked. A handler that
		// took the player out (restart to character select) leaves no one
		// to clear.
		if cs.acted && !client.frameUnread && cs.live != nil && !cs.live.handlerPanicked {
			onLive(cs.live, func() { l.clearSpawnProtectionOnAction(cs.live) })
		}
		cs.acted, client.frameUnread = false, false
		// A task this goroutine waited on panicked. sim recovered it, but the
		// handler stopped part-way through its mutation and the client was
		// told nothing, so the session ends exactly as a fatal decode error
		// ends it: the deferred detach above saves and detaches the
		// character. Timers and ticks keep the pool's own recovery — nothing
		// waits on them, and dropping a session over a tick would be a
		// regression in the other direction.
		if cs.live != nil && cs.live.handlerPanicked {
			l.log.Warn().Str("account", client.AccountName()).Msg("game client disconnected: panic in queued packet handler")
			return
		}
		payload, err := session.ReadFrame()
		if err != nil {
			cs.lost = true
			// ponytail: pre-auth EOF, whole or mid-frame, is a port probe
			// (admin panel dials 7777 every 5s, internet scanners send a
			// stray "GET /" read as a frame header) or an aborted handshake —
			// not worth a log line. Ceiling: a real client dropping
			// mid-handshake leaves no trace; log it again once pre-auth
			// disconnects need diagnosing.
			if hangUp(err) && client.State() == StateConnected {
				return
			}
			ev := l.log.Error()
			if normalReadFrameError(err) {
				ev = l.log.Debug()
			}
			ev.Err(err).Str("addr", session.remoteIP()).Str("account", client.AccountName()).Msg("Read frame")
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
		cs.acted = clearsSpawnProtection(opcode)
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
			cs.chars = list

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
			created, outcome, err := l.roster.Create(ctx, client.AccountName(), manager.CreateRequest{
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
			if l.scripts != nil {
				l.scripts.CharacterCreated(created)
			}
			list, err := l.sendCharSelectInfo(ctx, client)
			if err != nil {
				l.log.Error().Err(err).Msg("list characters")
				return
			}
			cs.chars = list

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
			c, ok := slotCharacter(cs.chars, req.Slot)
			if !ok {
				// An unknown slot sends no failure reply — only the
				// refreshed character list every delete attempt ends with.
				list, err := l.sendCharSelectInfo(ctx, client)
				if err != nil {
					l.log.Error().Err(err).Msg("list characters")
					return
				}
				cs.chars = list
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
			} else if l.roster.PurgesOnDelete() && l.awaitPersistence(conn) != nil {
				// An immediate purge deletes the character's rows, and its
				// pets rows, inline. A save still queued, such as the pets
				// row of a pet returned just before, would write its row back
				// after them, so the purge waits for every lane first: a
				// returned pet's row is on its collar's lane, which nothing
				// the character's restart waited on. A wait that gave up
				// fails the delete rather than purge under those saves.
				session.SendFrame(serverpackets.FrameCharDeleteFail(serverpackets.CharDeleteFailReasonDeletionFailed))
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
			cs.chars = list

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
			if c, ok := slotCharacter(cs.chars, req.Slot); ok {
				if err := l.roster.Restore(ctx, c.ID); err != nil {
					l.log.Error().Err(err).Msg("restore character")
				}
			}
			list, err := l.sendCharSelectInfo(ctx, client)
			if err != nil {
				l.log.Error().Err(err).Msg("list characters")
				return
			}
			cs.chars = list

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
			if cs.live != nil {
				onLive(cs.live, func() { l.requestSetPledgeCrest(cs.live, req) })
			}

		case clientpackets.OpcodeRequestJoinAlly:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestJoinAlly)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestJoinAlly(cs.live, req) })
			}

		case clientpackets.OpcodeRequestAnswerJoinAlly:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestAnswerJoinAlly)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestAnswerJoinAlly(cs.live, req) })
			}

		case clientpackets.OpcodeAllyLeave:
			if cs.live != nil {
				onLive(cs.live, func() { l.allyLeave(cs.live) })
			}

		case clientpackets.OpcodeAllyDismiss:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeAllyDismiss)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.allyDismiss(cs.live, req) })
			}

		case clientpackets.OpcodeRequestDismissAlly:
			if cs.live != nil {
				onLive(cs.live, func() { l.requestDismissAlly(cs.live) })
			}

		case clientpackets.OpcodeRequestSetAllyCrest:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestSetAllyCrest)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestSetAllyCrest(cs.live, req) })
			}

		case clientpackets.OpcodeRequestAllyInfo:
			if cs.live != nil {
				onLive(cs.live, func() { l.requestAllyInfo(cs.live) })
			}

		case clientpackets.OpcodeRequestAllyCrest:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestAllyCrest)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live == nil {
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
			c, ok := slotCharacter(cs.chars, req.Slot)
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
				if prev, ok := l.accountPlayerInWorld(cs.chars); ok {
					l.log.Info().Int32("object_id", prev.ObjectID()).
						Msg("game client: account character still in the world, closing previous session")
					prev.kickClient()
					continue
				}
			}
			selected, next := l.selectCharacter(ctx, conn, client, cs.chars, req.Slot, c)
			if selected != nil {
				cs.entering = selected
			}
			if next == selectionRefused {
				continue
			}
			if next == selectionStopped {
				return
			}

		case clientpackets.OpcodeEnterWorld:
			// Unreachable while the state gate admits EnterWorld only in
			// StateEntering, which character selection enters together with
			// entering; closed like any other failed entry if that changes.
			if cs.entering == nil {
				client.closeNow()
				return
			}
			// A failed entry leaves live attached and registered; the
			// deferred detachLivePlayer above is what releases it.
			cs.live, cs.entering = cs.entering, nil
			if !l.enterWorld(client, cs.live) {
				client.closeNow()
				return
			}
			client.SetState(StateInGame)

		case clientpackets.OpcodeExtended:
			r := wire.NewReader(payload[1:])
			second := r.ReadUint16()
			if r.Err() != nil {
				l.log.Warn().Str("state", client.State().String()).Msg("game client: extended opcode missing")
				cs.acted = false
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
				cs.acted = false
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
				if cs.live != nil {
					onLive(cs.live, func() { l.handleAutoSoulShot(cs.live, req) })
				}
			case clientpackets.OpcodeRequestExEnchantSkillInfo:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestExEnchantSkillInfo)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if cs.live != nil {
					onLive(cs.live, func() { l.sendEnchantSkillInfo(cs.live, req) })
				}
			case clientpackets.OpcodeRequestExEnchantSkill:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestExEnchantSkill)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if cs.live != nil {
					onLive(cs.live, func() { l.applyEnchantSkill(cs.live, req) })
				}
			case clientpackets.OpcodeRequestManorList:
				session.SendFrame(serverpackets.FrameExSendManorList())
			case clientpackets.OpcodeRequestSetSeed:
				req, err := decodeClientPacket(l, client, payload, func(p []byte) (clientpackets.RequestSetSeed, error) {
					return clientpackets.DecodeRequestSetSeed(p, l.playerConfig.InventorySlots.MaxItemInPacket())
				})
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					// A refused list answers nothing, as specified: the
					// setting window's submit leaves no client action
					// pending.
					continue
				}
				if cs.live != nil {
					onLive(cs.live, func() { l.requestSetSeed(cs.live, req) })
				}
			case clientpackets.OpcodeRequestSetCrop:
				req, err := decodeClientPacket(l, client, payload, func(p []byte) (clientpackets.RequestSetCrop, error) {
					return clientpackets.DecodeRequestSetCrop(p, l.playerConfig.InventorySlots.MaxItemInPacket())
				})
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if cs.live != nil {
					onLive(cs.live, func() { l.requestSetCrop(cs.live, req) })
				}
			case clientpackets.OpcodeRequestProcureCropList:
				req, err := decodeClientPacket(l, client, payload, func(p []byte) (clientpackets.RequestProcureCropList, error) {
					return clientpackets.DecodeRequestProcureCropList(p, l.playerConfig.InventorySlots.MaxItemInPacket())
				})
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if cs.live != nil {
					onLive(cs.live, func() { l.requestProcureCropList(cs.live, req) })
				}
			case clientpackets.OpcodeRequestExPledgeCrestLarge:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestExPledgeCrestLarge)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if cs.live == nil {
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
				if cs.live != nil {
					onLive(cs.live, func() { l.requestSetLargePledgeCrest(cs.live, req) })
				}
			case clientpackets.OpcodeRequestPledgePowerGrades:
				if cs.live != nil {
					onLive(cs.live, func() { l.requestPledgePowerGradeList(cs.live) })
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
				if cs.live != nil {
					if second == clientpackets.OpcodeRequestPledgeMemberPower {
						onLive(cs.live, func() { l.requestPledgeMemberPowerInfo(cs.live, req) })
					} else {
						onLive(cs.live, func() { l.requestPledgeMemberInfo(cs.live, req) })
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
				if cs.live != nil {
					onLive(cs.live, func() { l.requestPledgeSetMemberPowerGrade(cs.live, req) })
				}
			case clientpackets.OpcodeRequestPledgeWarList:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPledgeWarList)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if cs.live != nil {
					onLive(cs.live, func() { l.requestPledgeWarList(cs.live, req) })
				}
			case clientpackets.OpcodeRequestPledgeReorganizeMember:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPledgeReorganizeMember)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if cs.live != nil {
					onLive(cs.live, func() { l.requestPledgeReorganizeMember(cs.live, req) })
				}
			case clientpackets.OpcodeRequestPledgeSetAcademyMaster:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPledgeSetAcademyMaster)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if cs.live != nil {
					onLive(cs.live, func() { l.requestPledgeSetAcademyMaster(cs.live, req) })
				}
			case clientpackets.OpcodeRequestGetBossRecord:
				if _, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestGetBossRecord); err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if cs.live != nil {
					onLive(cs.live, func() { l.sendBossRecord(cs.live) })
				}
			case clientpackets.OpcodeRequestWriteHeroWords:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestWriteHeroWords)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if cs.live != nil {
					onLive(cs.live, func() { l.writeHeroWords(cs.live, req) })
				}
			case clientpackets.OpcodeRequestExFishRanking:
				if cs.live != nil {
					onLive(cs.live, func() { l.requestFishRanking(cs.live) })
				}
			case clientpackets.OpcodeRequestCursedWeaponList:
				if cs.live == nil {
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
				if cs.live != nil {
					onLive(cs.live, func() { l.handleMagicSkillUseGround(cs.live, req) })
				}
			case clientpackets.OpcodeRequestConfirmTargetItem:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestConfirmTargetItem)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if cs.live != nil {
					onLive(cs.live, func() { l.confirmAugmentTarget(cs.live, req) })
				}
			case clientpackets.OpcodeRequestConfirmRefinerItem:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestConfirmRefinerItem)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if cs.live != nil {
					onLive(cs.live, func() { l.confirmAugmentRefiner(cs.live, req) })
				}
			case clientpackets.OpcodeRequestConfirmGemStone:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestConfirmGemStone)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if cs.live != nil {
					onLive(cs.live, func() { l.confirmAugmentGemstone(cs.live, req) })
				}
			case clientpackets.OpcodeRequestRefine:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestRefine)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if cs.live != nil {
					onLive(cs.live, func() { l.refineAugment(cs.live, req) })
				}
			case clientpackets.OpcodeRequestConfirmCancelItem:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestConfirmCancelItem)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if cs.live != nil {
					onLive(cs.live, func() { l.confirmAugmentCancel(cs.live, req) })
				}
			case clientpackets.OpcodeRequestRefineCancel:
				req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestRefineCancel)
				if err != nil {
					if errors.Is(err, errMalformedPacketDisconnect) {
						return
					}
					continue
				}
				if cs.live != nil {
					onLive(cs.live, func() { l.cancelAugment(cs.live, req) })
				}
			case clientpackets.OpcodeRequestChangePartyLeader, clientpackets.OpcodeRequestExAskJoinMPCC,
				clientpackets.OpcodeRequestExAcceptJoinMPCC, clientpackets.OpcodeRequestExOustFromMPCC,
				clientpackets.OpcodeRequestExMPCCShowPartyMembersInfo:
				if !l.dispatchPartyExtended(client, cs.live, second, payload) {
					return
				}
			case clientpackets.OpcodeRequestDuelStart, clientpackets.OpcodeRequestDuelAnswerStart,
				clientpackets.OpcodeRequestDuelSurrender:
				if !l.dispatchDuel(client, cs.live, second, payload) {
					return
				}
			case clientpackets.OpcodeRequestOustFromPartyRoom, clientpackets.OpcodeRequestDismissPartyRoom,
				clientpackets.OpcodeRequestWithdrawPartyRoom, clientpackets.OpcodeRequestAskJoinPartyRoom,
				clientpackets.OpcodeAnswerJoinPartyRoom, clientpackets.OpcodeRequestListPartyMatchingWaitingRoom,
				clientpackets.OpcodeRequestExitPartyMatchingWaitingRoom:
				if !l.dispatchPartyMatchExtended(client, cs.live, second, payload) {
					return
				}
			case clientpackets.OpcodeRequestCursedWeaponLocation:
				if cs.live == nil {
					continue
				}
				onLive(cs.live, func() { l.sendCursedWeaponLocations(cs.live) })
			default:
				l.log.Info().
					Uint16("opcode2", second).
					Str("state", client.State().String()).
					Msg("game client: accepted extended opcode not implemented yet")
				// OpcodeExtended is only ever allowed in StateEntering/StateInGame
				// (see allowedOpcodes), so unlike the top-level Accept gate there is
				// no pre-auth immediate-disconnect case here; just count toward the
				// same sliding-60s threshold as GameClient.onUnknownPacket.
				// An unknown packet never runs, so it is no action.
				cs.acted = false
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
			if cs.live != nil {
				onLive(cs.live, func() { l.handleMagicSkillUse(cs.live, req) })
			}

		case clientpackets.OpcodeAction:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeAction)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live == nil {
				continue
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

		case clientpackets.OpcodeAttackRequest:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeAttackRequest)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live == nil {
				continue
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

		case clientpackets.OpcodeLogout:
			if cs.live == nil {
				// Logout from character select has no character to take out
				// of the world: it is ignored silently and the connection
				// stays open. The client registers no pending action for
				// it, so there is nothing to release.
				continue
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
			if cs.live == nil {
				continue
			}
			if req.MoveMovement == 0 {
				session.SendFrame(serverpackets.FrameActionFailed())
				continue
			}
			onLive(cs.live, func() {
				l.moveLivePlayer(cs.live,
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
			if cs.live == nil {
				continue
			}
			onLive(cs.live, func() { l.stopLivePlayer(cs.live) })

		case clientpackets.OpcodeRequestMoveInVehicle:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestMoveToLocationInVehicle)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestMoveInVehicle(cs.live, req) })
			}

		case clientpackets.OpcodeCannotMoveInVehicle:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeCannotMoveAnymoreInVehicle)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.cannotMoveInVehicle(cs.live, req) })
			}

		case clientpackets.OpcodeRequestGetOnVehicle:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestGetOnVehicle)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestGetOnVehicle(cs.live, req) })
			}

		case clientpackets.OpcodeRequestGetOffVehicle:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestGetOffVehicle)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestGetOffVehicle(cs.live, req) })
			}

		case clientpackets.OpcodeValidatePosition:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeValidatePosition)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() {
					l.validateLivePlayerPosition(cs.live, location.Location{X: int(req.X), Y: int(req.Y), Z: int(req.Z)}, req.BoatID)
				})
			}

		case clientpackets.OpcodeRequestItemList:
			if cs.live == nil {
				continue
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
			if cs.live == nil {
				continue
			}
			// The dice gate belongs to this read loop, which stays parked
			// in onLive while useItem runs, so the throw consults it there.
			rollDice := func() bool {
				return client.performFloodProtected(floodProtectorRollDice, l.playerConfig.RollDiceDelay, time.Now())
			}
			onLive(cs.live, func() { l.useItem(cs.live, req.ObjectID, req.CtrlPressed, rollDice) })

		case clientpackets.OpcodeRequestUnEquipItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeUnequipItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live == nil {
				continue
			}
			onLive(cs.live, func() { l.unequipItem(cs.live, req.BodySlot) })

		case clientpackets.OpcodeRequestDropItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestDropItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live == nil {
				continue
			}
			onLive(cs.live, func() { l.dropLiveItem(cs.live, req) })

		case clientpackets.OpcodeRequestDestroyItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestDestroyItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live == nil {
				continue
			}
			onLive(cs.live, func() { l.destroyLiveItem(cs.live, req.ObjectID, int(req.Count)) })

		case clientpackets.OpcodeRequestCrystallizeItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestCrystallizeItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live == nil {
				continue
			}
			onLive(cs.live, func() { l.crystallizeLiveItem(cs.live, req) })

		case clientpackets.OpcodeRequestRecipeBookOpen:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestRecipeBookOpen)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.openRecipeBook(cs.live, req) })
			}

		case clientpackets.OpcodeRequestRecipeBookDestroy:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestRecipeBookDestroy)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.destroyRecipe(cs.live, req) })
			}

		case clientpackets.OpcodeRequestRecipeItemMakeInfo:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestRecipeItemMakeInfo)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.sendRecipeItemMakeInfo(cs.live, req) })
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
			if cs.live != nil {
				onLive(cs.live, func() { l.makeRecipeSelf(cs.live, req) })
			}

		case clientpackets.OpcodeRequestHennaItemList:
			_, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestHennaItemList)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.sendHennaEquipList(cs.live) })
			}

		case clientpackets.OpcodeRequestHennaItemInfo:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestHennaItemInfo)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.sendHennaItemInfo(cs.live, req) })
			}

		case clientpackets.OpcodeRequestHennaEquip:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestHennaEquip)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.drawHenna(cs.live, req) })
			}

		case clientpackets.OpcodeRequestHennaUnequipList:
			_, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestHennaUnequipList)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { sendHennaUnequipList(cs.live) })
			}

		case clientpackets.OpcodeRequestHennaUnequipInfo:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestHennaUnequipInfo)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.sendHennaUnequipInfo(cs.live, req) })
			}

		case clientpackets.OpcodeRequestHennaUnequip:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestHennaUnequip)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.deleteHenna(cs.live, req) })
			}

		case clientpackets.OpcodeRequestPrivateStoreManageSell, clientpackets.OpcodeSetPrivateStoreListSell,
			clientpackets.OpcodeRequestPrivateStoreQuitSell, clientpackets.OpcodeSetPrivateStoreMsgSell,
			clientpackets.OpcodeRequestPrivateStoreBuy, clientpackets.OpcodeRequestPrivateStoreManageBuy,
			clientpackets.OpcodeSetPrivateStoreListBuy, clientpackets.OpcodeRequestPrivateStoreQuitBuy,
			clientpackets.OpcodeSetPrivateStoreMsgBuy, clientpackets.OpcodeRequestPrivateStoreSell,
			clientpackets.OpcodeRequestRecipeShopMessageSet, clientpackets.OpcodeRequestRecipeShopListSet,
			clientpackets.OpcodeRequestRecipeShopManageQuit, clientpackets.OpcodeRequestRecipeShopMakeInfo,
			clientpackets.OpcodeRequestRecipeShopMakeItem, clientpackets.OpcodeRequestRecipeShopManagePrev:
			if !l.dispatchPrivateStore(client, cs.live, opcode, payload) {
				return
			}

		case clientpackets.OpcodeRequestSiegeAttackerList, clientpackets.OpcodeRequestSiegeDefenderList,
			clientpackets.OpcodeRequestJoinSiege, clientpackets.OpcodeRequestConfirmSiegeWaitingList:
			if !l.dispatchSiege(client, cs.live, opcode, payload) {
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
			if cs.live != nil {
				// The reuse gate is consulted only with a player in the
				// world; a refused exchange still drops the open list.
				allowed := client.performFloodProtected(floodProtectorMultisell, l.playerConfig.MultisellDelay, time.Now())
				onLive(cs.live, func() { l.requestMultiSellChoose(cs.live, req, allowed) })
			}

		case clientpackets.OpcodeRequestEnchantItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestEnchantItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live == nil {
				continue
			}
			onLive(cs.live, func() { l.enchantLiveItem(ctx, cs.live, req) })

		case clientpackets.OpcodeRequestSkillList:
			// While entering, 0x3f is the quest-list request the client
			// sends during loading: it is answered with the selected
			// character's quest list, or the quest panel stays empty.
			if client.State() == StateEntering {
				if cs.entering != nil {
					session.SendFrame(questListFrame(cs.entering.Character))
				}
				continue
			}
			if cs.live == nil {
				continue
			}
			onLive(cs.live, func() {
				session.SendFrame(serverpackets.FrameSkillList(skillListEntries(cs.live.Character, l.skills)))
			})

		case clientpackets.OpcodeRequestAcquireSkillInfo:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestAcquireSkillInfo)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.sendAcquireSkillInfo(cs.live, req) })
			}

		case clientpackets.OpcodeRequestAcquireSkill:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestAcquireSkill)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.learnAcquireSkill(cs.live, req) })
			}

		case clientpackets.OpcodeRequestActionUse:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestActionUse)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live == nil {
				continue
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

		case clientpackets.OpcodeRequestRestartPoint:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestRestartPoint)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.restartLivePlayer(cs.live, req) })
			}

		case clientpackets.OpcodeRequestSocialAction:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestSocialAction)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.broadcastLiveSocialAction(cs.live, req.ActionID) })
			}

		case clientpackets.OpcodeRequestChangeMoveType:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestChangeMoveType)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
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

		case clientpackets.OpcodeRequestChangeWaitType:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestChangeWaitType)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestChangeWaitType(cs.live, req.Stand) })
			}

		case clientpackets.OpcodeRequestLinkHtml:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestLinkHTML)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			onLive(cs.live, func() { l.requestLinkHTML(cs.live, req) })

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
			onLive(cs.live, func() { l.requestBypassToServer(cs.live, req) })
			l.finishPendingClassChange(cs.live)

		case clientpackets.OpcodeRequestShowBoard:
			if _, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestShowBoard); err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestShowBoard(cs.live) })
			}

		case clientpackets.OpcodeRequestBBSWrite:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestBBSWrite)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestBBSWrite(cs.live, req) })
			}

		case clientpackets.OpcodeSendBypassBuildCmd:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeSendBypassBuildCmd)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.sendBypassBuildCmd(cs.live, req) })
			}

		case clientpackets.OpcodeRequestGmList:
			// The request carries no body.
			if cs.live != nil {
				onLive(cs.live, func() { l.requestGmList(cs.live) })
			}

		case clientpackets.OpcodeRequestPetition:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPetition)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestPetition(cs.live, req) })
			}

		case clientpackets.OpcodeRequestPetitionCancel:
			// The request carries no body.
			if cs.live != nil {
				onLive(cs.live, func() { l.requestPetitionCancel(cs.live) })
			}

		case clientpackets.OpcodePetitionVote:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodePetitionVote)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.petitionVote(cs.live, req) })
			}

		case clientpackets.OpcodeRequestTargetCancel:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestTargetCancel)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestTargetCancel(cs.live, req) })
			}

		case clientpackets.OpcodeAppearing:
			if _, err := decodeClientPacket(l, client, payload, clientpackets.DecodeAppearing); err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() {
					l.completeLivePlayerTeleport(cs.live)
					cs.live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(cs.live)))
				})
			}

		case clientpackets.OpcodeObserverReturn:
			if _, err := decodeClientPacket(l, client, payload, clientpackets.DecodeObserverReturn); err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.observerReturn(cs.live) })
			}

		case clientpackets.OpcodeStartRotating:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeStartRotating)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live == nil {
				continue
			}
			onLive(cs.live, func() {
				l.broadcastLiveFrame(cs.live, func() wire.Frame {
					return serverpackets.FrameStartRotation(cs.live.ObjectID(), int(req.Degree), int(req.Side), 0)
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
			if cs.live == nil {
				continue
			}
			onLive(cs.live, func() {
				cs.live.SetHeading(int(req.Degree))
				l.broadcastLiveFrame(cs.live, func() wire.Frame {
					return serverpackets.FrameStopRotation(cs.live.ObjectID(), int(req.Degree), 0)
				})
			})

		case clientpackets.OpcodeRequestRestart:
			if cs.live == nil {
				continue
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
				continue
			}
			if refused {
				continue
			}
			_ = l.awaitPersistence(conn, owners...)
			cs.live = nil
			cs.entering = nil
			client.SetState(StateAuthed)
			session.SendFrame(serverpackets.FrameRestartResponse(true))
			list, err := l.sendCharSelectInfo(ctx, client)
			if err != nil {
				l.log.Error().Err(err).Msg("list characters")
				return
			}
			cs.chars = list

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
			onLive(cs.live, func() { l.sendPackageSendableItemList(cs.live, req.ObjectID) })

		case clientpackets.OpcodeRequestPetUseItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPetUseItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			onLive(cs.live, func() { l.petUseItem(ctx, cs.live, req) })

		case clientpackets.OpcodeRequestGiveItemToPet:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestGiveItemToPet)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			onLive(cs.live, func() { l.giveItemToPet(ctx, cs.live, req) })

		case clientpackets.OpcodeRequestGetItemFromPet:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestGetItemFromPet)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			onLive(cs.live, func() { l.getItemFromPet(ctx, cs.live, req) })

		case clientpackets.OpcodeRequestPetGetItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPetGetItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			onLive(cs.live, func() { l.petGetItem(ctx, cs.live, req) })

		case clientpackets.OpcodeRequestJoinParty, clientpackets.OpcodeRequestAnswerJoinParty,
			clientpackets.OpcodeRequestWithdrawParty, clientpackets.OpcodeRequestOustPartyMember:
			if !l.dispatchParty(client, cs.live, opcode, payload) {
				return
			}

		case clientpackets.OpcodeRequestListPartyWaiting, clientpackets.OpcodeRequestManagePartyRoom,
			clientpackets.OpcodeRequestJoinPartyRoom:
			if !l.dispatchPartyMatch(client, cs.live, opcode, payload) {
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
			if cs.live != nil {
				onLive(cs.live, func() { l.handleTradeRequest(cs.live, req) })
			}

		case clientpackets.OpcodeAnswerTradeRequest:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeAnswerTradeRequest)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.handleAnswerTradeRequest(cs.live, req) })
			}

		case clientpackets.OpcodeAddTradeItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeAddTradeItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.handleAddTradeItem(cs.live, req) })
			}

		case clientpackets.OpcodeTradeDone:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeTradeDone)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.handleTradeDone(ctx, cs.live, req) })
			}

		case clientpackets.OpcodeRequestShortCutReg:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestShortCutReg)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.registerShortcut(cs.live, req) })
			}

		case clientpackets.OpcodeRequestShortCutDel:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestShortCutDel)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.deleteShortcut(cs.live, req) })
			}

		case clientpackets.OpcodeRequestMakeMacro:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestMakeMacro)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.makeMacro(cs.live, req) })
			}

		case clientpackets.OpcodeRequestDeleteMacro:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestDeleteMacro)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.deleteMacro(cs.live, req) })
			}

		case clientpackets.OpcodeRequestEvaluate:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestEvaluate)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.evaluate(cs.live, req) })
			}

		case clientpackets.OpcodeRequestChangePetName:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestChangePetName)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				// Runs its own queue hops: the pets-table read in the middle
				// stays here, off the queue (see handleRequestChangePetName).
				l.handleRequestChangePetName(ctx, cs.live, req)
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
			if cs.live != nil {
				onLive(cs.live, func() { l.requestBuyItem(cs.live, req) })
			}

		case clientpackets.OpcodeRequestBuySeed:
			req, err := decodeClientPacket(l, client, payload, func(p []byte) (clientpackets.RequestBuySeed, error) {
				return clientpackets.DecodeRequestBuySeed(p, l.playerConfig.InventorySlots.MaxItemInPacket())
			})
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			// The reuse gate is spent before anything else, with or without
			// a player in the world. A gated purchase answers nothing, as
			// specified: the seed window's submit leaves no client action
			// pending.
			if !client.performFloodProtected(floodProtectorManor, l.playerConfig.ManorDelay, time.Now()) {
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestBuySeed(cs.live, req) })
			}

		case clientpackets.OpcodeRequestBuyProcure:
			// No window the server opens sends it, and no request decodes:
			// the refusal answers nothing, as specified, and leaves no
			// client action pending.
			req, err := decodeClientPacket(l, client, payload, func(p []byte) (clientpackets.RequestBuyProcure, error) {
				return clientpackets.DecodeRequestBuyProcure(p, l.playerConfig.InventorySlots.MaxItemInPacket())
			})
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			l.log.Warn().Int("rows", len(req.Items)).Int32("manor", req.ManorID).Msg("RequestBuyProcure: normally unused")

		case clientpackets.OpcodeRequestPreviewItem:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPreviewItem)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestPreviewItem(cs.live, req) })
			}

		case clientpackets.OpcodeRequestSSQStatus:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestSSQStatus)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestSSQStatus(cs.live, req) })
			}

		case clientpackets.OpcodeDlgAnswer:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeDlgAnswer)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.handleDlgAnswer(cs.live, req) })
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
			if cs.live != nil {
				onLive(cs.live, func() { l.requestSellItem(cs.live, req) })
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
			if cs.live != nil {
				onLive(cs.live, func() { l.requestWarehouseDeposit(cs.live, req) })
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
			if cs.live != nil {
				onLive(cs.live, func() { l.requestWarehouseWithdraw(cs.live, req) })
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
			if cs.live != nil {
				onLive(cs.live, func() { l.requestPackageSend(cs.live, req) })
			}

		case clientpackets.OpcodeRequestFriendInvite:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestFriendInvite)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.handleRequestFriendInvite(cs.live, req) })
			}

		case clientpackets.OpcodeRequestAnswerFriendInvite:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestAnswerFriendInvite)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.handleRequestAnswerFriendInvite(cs.live, req) })
			}

		case clientpackets.OpcodeRequestFriendList:
			// The request carries no body.
			if cs.live != nil {
				onLive(cs.live, func() { l.handleRequestFriendList(cs.live) })
			}

		case clientpackets.OpcodeRequestFriendDel:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestFriendDel)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.handleRequestFriendDel(cs.live, req) })
			}

		case clientpackets.OpcodeRequestBlock:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestBlock)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.handleRequestBlock(cs.live, req) })
			}

		case clientpackets.OpcodeRequestSendL2FriendSay:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestSendL2FriendSay)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.handleRequestSendL2FriendSay(cs.live, req) })
			}

		case clientpackets.OpcodeRequestJoinPledge:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestJoinPledge)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestJoinPledge(cs.live, req) })
			}

		case clientpackets.OpcodeRequestAnswerJoinPledge:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestAnswerJoinPledge)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestAnswerJoinPledge(cs.live, req) })
			}

		case clientpackets.OpcodeRequestOustPledgeMember:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestOustPledgeMember)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestOustPledgeMember(cs.live, req) })
			}

		case clientpackets.OpcodeRequestGiveNickName:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestGiveNickName)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestGiveNickName(cs.live, req) })
			}

		case clientpackets.OpcodeRequestPledgeInfo:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPledgeInfo)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestPledgeInfo(cs.live, req) })
			}

		case clientpackets.OpcodeRequestPledgePower:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestPledgePower)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestPledgePower(cs.live, req) })
			}

		case clientpackets.OpcodeRequestWithdrawPledge:
			if cs.live != nil {
				onLive(cs.live, func() { l.requestWithdrawPledge(cs.live) })
			}

		case clientpackets.OpcodeRequestPledgeMemberList:
			if cs.live != nil {
				onLive(cs.live, func() { l.requestPledgeMemberList(cs.live) })
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
			if cs.live != nil {
				switch opcode {
				case clientpackets.OpcodeRequestStartPledgeWar:
					onLive(cs.live, func() { l.requestStartPledgeWar(cs.live, req) })
				case clientpackets.OpcodeRequestStopPledgeWar:
					onLive(cs.live, func() { l.requestStopPledgeWar(cs.live, req) })
				case clientpackets.OpcodeRequestSurrenderPersonally:
					onLive(cs.live, func() { l.requestSurrenderPersonally(cs.live, req) })
				default:
					onLive(cs.live, func() { l.requestSurrenderPledgeWar(cs.live, req) })
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
			if cs.live != nil {
				onLive(cs.live, func() { l.requestReplyPledgeWar(cs.live, opcode) })
			}

		case clientpackets.OpcodeRequestShowMiniMap:
			// The request carries no body; with no player in the world
			// nothing answers, as the specified handler does.
			if cs.live != nil {
				onLive(cs.live, func() { l.showMiniMap(cs.live, serverpackets.RegularMapID) })
			}

		case clientpackets.OpcodeRequestRecordInfo:
			// The request carries no body; with no player in the world
			// nothing answers, as the specified handler does.
			if cs.live != nil {
				onLive(cs.live, func() { l.requestRecordInfo(cs.live) })
			}

		case clientpackets.OpcodeSay2:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeSay2)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.handleSay2(client, cs.live, req) })
			}

		case clientpackets.OpcodeRequestUserCommand:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestUserCommand)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.requestUserCommand(cs.live, req.CommandID) })
			}

		case clientpackets.OpcodeRequestQuestListInGame:
			if cs.live != nil {
				onLive(cs.live, func() { cs.live.SendFrame(questListFrame(cs.live.Character)) })
			}

		case clientpackets.OpcodeRequestTutorialLinkHTML, clientpackets.OpcodeRequestTutorialPassCmdToServer,
			clientpackets.OpcodeRequestTutorialQuestionMark, clientpackets.OpcodeRequestTutorialClientEvent:
			req, err := decodeClientPacket(l, client, payload, tutorialDecoder(opcode))
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.tutorialRequest(cs.live, req.Name) })
			}

		case clientpackets.OpcodeRequestQuestAbort:
			req, err := decodeClientPacket(l, client, payload, clientpackets.DecodeRequestQuestAbort)
			if err != nil {
				if errors.Is(err, errMalformedPacketDisconnect) {
					return
				}
				continue
			}
			if cs.live != nil {
				onLive(cs.live, func() { l.abortQuest(cs.live, req.QuestID) })
			}

		case clientpackets.OpcodeDummy1A,
			clientpackets.OpcodeDummy23,
			clientpackets.OpcodeDummy2E,
			clientpackets.OpcodeDummy34,
			clientpackets.OpcodeDummy3E,
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
		clientpackets.OpcodeRequestPledgeInfo, clientpackets.OpcodeSay2:
		return false
	default:
		return true
	}
}

// hangUp reports a read ended by the peer closing its side, between frames
// or part-way through one.
func hangUp(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

func normalReadFrameError(err error) bool {
	return hangUp(err) || errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrDeadlineExceeded)
}

// authenticate validates req against the login server over the game
// server's current link, advancing client to StateAuthed on success.
// AuthLoginFail (and the connection close that follows) is the caller's
// job for every false/error result except the login-link-down case, which
// authenticate handles itself since there is no in-flight validation to
// fail.
