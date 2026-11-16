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

// rejectedFrameHeadBytes is how much of a frame the state gate rejected its
// log line shows: enough to identify the request, and a fixed bound however
// long a frame a client sends.
const rejectedFrameHeadBytes = 16

// decodeClientPacket disconnects buffer-underflow-equivalent pre-auth packets
// (insufficient bytes, wire.ErrShortPacket) immediately. Once authenticated,
// that same error class (char-select or in-game) is tolerated up to
// maxUnderflowsPerMin within a 60s sliding window, then disconnect. Other
// decode validation errors (e.g. an out-of-range field) are logged and the
// packet dropped without counting or disconnecting. A request the decoder
// discarded (clientpackets.ErrDiscarded) was read in full: it is dropped
// without a word, yet counts as the player acting.
func decodeClientPacket[T any](l *GameClientLink, client *Client, payload []byte, decode func([]byte) (T, error)) (T, error) {
	req, err := decode(payload)
	if errors.Is(err, clientpackets.ErrDiscarded) {
		return req, err
	}
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
	session, ok := l.openSession(conn)
	if !ok {
		return
	}
	client := NewClient(session)
	stopOnShutdown := closeOnShutdown(ctx, client, conn)
	defer stopOnShutdown()
	cs := &connState{client: client, conn: conn}
	defer l.endConnection(ctx, cs)

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
		// ends it: the deferred detach saves and detaches the character.
		// Timers and ticks keep the pool's own recovery — nothing waits on
		// them, and dropping a session over a tick would be a regression in
		// the other direction.
		if cs.live != nil && cs.live.handlerPanicked {
			l.log.Warn().Str("account", client.AccountName()).Msg("game client disconnected: panic in queued packet handler")
			endSession(ctx, client)
			return
		}
		payload, err := session.ReadFrame()
		if err != nil {
			// A read the client did not end by hanging up (the idle
			// deadline) leaves it connected: it reads ServerClose before the close, so it shows
			// the disconnect instead of waiting on a dead connection. The
			// player lingers as on a dropped connection.
			cs.lost = true
			l.logReadFrameError(client, err)
			if !hangUp(err) && !errors.Is(err, net.ErrClosed) && endSession(ctx, client) {
				_ = conn.Close()
			}
			return
		}
		// An evicted session serves its client nothing more: its last
		// frame is queued, and the connection closes once that is flushed,
		// however long a client that stopped reading keeps it waiting.
		if session.evictedNow() {
			return
		}
		if len(payload) == 0 && client.State() != StateConnected {
			// A frame with no body is skipped unread, and the client waits
			// on no answer to it. Before authentication gateFrame still
			// counts it toward the pre-auth packet cap, so empty frames
			// cannot hold an unauthenticated connection open.
			continue
		}
		serve, open := l.gateFrame(client, payload)
		if !open {
			return
		}
		if !serve {
			continue
		}
		// Once in the world, everything a frame does to the player runs as a
		// task on its queue (onLive), serialized with its timers and ticks.
		// The loop waits for each task before reading on, so frames are
		// still handled one at a time in read order; decoding and the
		// protection gates stay on this goroutine.
		cs.acted = clearsSpawnProtection(payload[0])
		if !l.dispatch(ctx, cs, payload) {
			endSession(ctx, client)
			return
		}
	}
}

// endSession makes ServerClose the last frame of an authenticated session
// the server ends, so its client shows the disconnect instead of hanging. It
// reports whether it did. A session that already sent its last frame (a
// rejection's ServerClose, a logout's LeaveWorld) keeps it. An
// unauthenticated connection just closes, as does every connection of a
// stopping server, whose own close decides what each client reads.
func endSession(ctx context.Context, client *Client) bool {
	if ctx.Err() != nil || client.State() == StateConnected {
		return false
	}
	client.closeNow()
	return true
}

// openSession builds the connection's cipher and session, logging and
// reporting false when either cannot be made. The cipher starts from a zero
// key: nothing crosses encrypted before ProtocolVersion sets the real one.
func (l *GameClientLink) openSession(conn *Conn) (session *Session, ok bool) {
	gameCipher, err := gamecipher.NewCipher(make([]byte, gamecipher.KeySize))
	if err != nil {
		l.log.Error().Err(err).Msg("build game cipher")
		return nil, false
	}
	session = NewSession(conn, gameCipher)
	session.idleTimeout = l.readIdleTimeout
	return session, true
}

// closeOnShutdown ends the connection when ctx (the server) stops. A client
// with a character in the world (selected or entered) reads ServerClose as
// its last frame and leaves the world; any other connection just closes
// once what is queued is flushed. The read loop then ends as on a lost
// connection. The returned func cancels the watch.
func closeOnShutdown(ctx context.Context, client *Client, conn *Conn) (stop func() bool) {
	return context.AfterFunc(ctx, func() {
		if client.State() >= StateEntering {
			client.Session.Close()
			return
		}
		_ = conn.Close()
	})
}

// endConnection runs once the read loop is done. A connection lost between
// selection and EnterWorld takes the selected player out of the world as a
// logout would.
func (l *GameClientLink) endConnection(ctx context.Context, cs *connState) {
	client := cs.client
	if leaving := cmp.Or(cs.live, cs.entering); leaving != nil {
		leaving.sessionEnded.Store(true)
		awaitDetachDelay(ctx, client.Session, leaving, detachDelay(leaving, cs.lost))
		var owners []int32
		onLive(leaving, func() { owners = l.detachLivePlayer(leaving) })
		_ = l.awaitPersistence(cs.conn, owners...)
	}
	// A connection replaced by a newer approved login leaves the account to
	// it: the login server already holds the account in play for the newer
	// one, so no logout is reported for the replaced connection.
	if l.clients != nil && !l.clients.Release(client.AccountName(), client) {
		return
	}
	l.notifyPlayerLogout(client.AccountName())
}

// logReadFrameError logs the read that ended the connection.
func (l *GameClientLink) logReadFrameError(client *Client, err error) {
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
	ev.Err(err).Str("addr", client.Session.remoteIP()).Str("account", client.AccountName()).Msg("Read frame")
}

// gateFrame runs a received frame through the packet-protection and state
// gates. serve reports that the frame goes on to its handler; open, that
// the connection keeps reading.
//
// The packet-protection gate comes ahead of the state gate: every received
// frame is counted, and while a flood is active frames are dropped,
// answering ActionFailed at most once per second of ongoing flood. After
// counting (so dropped frames never reach a handler), an excess of detected
// floods per minute disconnects, as does a hard cap on packets processed
// pre-auth. The queue-size accounting and burst cap of a queued packet
// reader have no counterpart here: the read loop processes each frame
// inline with its read, so there is no inbound queue to overflow or drain
// in batches.
func (l *GameClientLink) gateFrame(client *Client, payload []byte) (serve, open bool) {
	now := l.now
	if now == nil {
		now = time.Now
	}
	// A dropped frame never reaches a handler: that includes the
	// flood-onset packet itself, which answers ActionFailed and is
	// discarded like every other frame of the flood.
	if actionFailed, drop := client.stats.countIncomingPacket(now()); actionFailed || drop {
		if actionFailed {
			client.Session.SendFrame(serverpackets.FrameActionFailed())
		}
		return false, true
	} else if client.stats.floodsExceeded() {
		l.log.Warn().Str("state", client.State().String()).Msg("game client disconnected: too many packet floods")
		client.closeNow()
		return false, false
	} else if client.State() == StateConnected && client.stats.processedPackets > preAuthMaxProcessedPackets {
		l.log.Warn().Str("state", client.State().String()).Msg("game client disconnected: too many packets in non-authed state")
		client.closeNow()
		return false, false
	}
	if len(payload) == 0 {
		return false, true
	}
	if !client.Accept(payload[0]) {
		l.log.Warn().Str("state", client.State().String()).Str("opcode", fmt.Sprintf("%#02x", payload[0])).
			Int("length", len(payload)).Str("head", hex.EncodeToString(payload[:min(len(payload), rejectedFrameHeadBytes)])).
			Msg("Accept opcode")
		// Pre-auth, any rejected opcode disconnects immediately; once
		// authenticated, tolerate up to maxUnknownPerMin within a 60s
		// sliding window.
		if client.State() == StateConnected || client.countUnknownPacket() {
			client.closeNow()
			return false, false
		}
		return false, true
	}
	return true, true
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
