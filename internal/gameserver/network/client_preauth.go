package network

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// noPayload decodes a frame whose opcode carries no fields: whatever
// follows the opcode is ignored, so it never fails.
func noPayload([]byte) (struct{}, error) { return struct{}{}, nil }

// handleProtocolVersion answers a supported protocol revision with
// VersionCheck, which carries the cipher key, and closes the connection on
// any other.
func (l *GameClientLink) handleProtocolVersion(_ context.Context, cs *connState, req clientpackets.ProtocolVersion) error {
	session := cs.client.Session
	if !validProtocolRevision(req.Revision) {
		return errDisconnect
	}
	if !session.SendFrame(serverpackets.FrameVersionCheck(cs.key, !l.noCipher)) {
		return errDisconnect
	}
	// The key is out; every later frame crosses encrypted: crypt
	// starts only after VersionCheck.
	if !l.noCipher {
		session.EnableCrypt()
	}
	return nil
}

// handleAuthLogin validates the session key with the login server and, once
// accepted, sends the account's character list.
func (l *GameClientLink) handleAuthLogin(ctx context.Context, cs *connState, req clientpackets.AuthLogin) error {
	client, session := cs.client, cs.client.Session
	ok, err := l.authenticate(ctx, client, req)
	if err != nil || !ok {
		// A rejection already answered AuthLoginFail; it also closes
		// with ServerClose. A canceled wait or a lost login link
		// closes silently.
		if err == nil {
			client.closeNow()
		}
		return errDisconnect
	}
	session.CompleteHandshake()
	list, err := l.sendCharSelectInfo(ctx, client)
	if err != nil {
		l.log.Error().Err(err).Str("account", client.AccountName()).Msg("list characters")
		return errDisconnect
	}
	cs.chars = list
	return nil
}

// handleRequestCharacterCreate creates a character on the account and
// answers with the outcome and the refreshed character list.
func (l *GameClientLink) handleRequestCharacterCreate(ctx context.Context, cs *connState, req clientpackets.RequestCharacterCreate) error {
	client, session := cs.client, cs.client.Session
	sex, err := player.ParseSex(req.Sex)
	if err != nil {
		session.SendFrame(serverpackets.FrameCharCreateFail(serverpackets.CharCreateFailReasonCreationFailed))
		return nil
	}
	created, outcome, err := l.roster.Create(ctx, client.AccountName(), manager.CreateRequest{
		Name: req.Name, ClassID: int(req.ClassID), Race: int(req.Race), Sex: sex,
		HairStyle: req.HairStyle, HairColor: req.HairColor, Face: req.Face,
	})
	if err != nil {
		l.log.Error().Err(err).Str("account", client.AccountName()).Msg("create character")
		return errDisconnect
	}
	if outcome != manager.CreateOK {
		session.SendFrame(serverpackets.FrameCharCreateFail(createFailReason(outcome)))
		return nil
	}
	session.SendFrame(serverpackets.FrameCharCreateOk())
	if l.scripts != nil {
		l.scripts.CharacterCreated(created)
	}
	list, err := l.sendCharSelectInfo(ctx, client)
	if err != nil {
		l.log.Error().Err(err).Msg("list characters")
		return errDisconnect
	}
	cs.chars = list
	return nil
}

// handleRequestCharacterDelete marks the character in the addressed slot
// for deletion, or refuses it, and always ends with the refreshed character
// list unless the reuse gate refused the request.
func (l *GameClientLink) handleRequestCharacterDelete(ctx context.Context, cs *connState, req clientpackets.RequestCharacterDelete) error {
	client, session := cs.client, cs.client.Session
	// The character-select reuse gate answers a refused delete
	// with the deletion-failed reply; nothing else runs.
	if !client.performFloodProtected(floodProtectorCharacterSelect, l.playerConfig.CharacterSelectDelay, time.Now()) {
		session.SendFrame(serverpackets.FrameCharDeleteFail(serverpackets.CharDeleteFailReasonDeletionFailed))
		return nil
	}
	c, ok := slotCharacter(cs.chars, req.Slot)
	if !ok {
		// An unknown slot sends no failure reply — only the
		// refreshed character list every delete attempt ends with.
		list, err := l.sendCharSelectInfo(ctx, client)
		if err != nil {
			l.log.Error().Err(err).Msg("list characters")
			return errDisconnect
		}
		cs.chars = list
		return nil
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
	} else if l.roster.PurgesOnDelete() && l.awaitPersistence(cs.conn) != nil {
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
		return errDisconnect
	}
	cs.chars = list
	return nil
}

// handleCharacterRestore cancels the pending deletion of the character in
// the addressed slot and sends the refreshed character list.
func (l *GameClientLink) handleCharacterRestore(ctx context.Context, cs *connState, req clientpackets.CharacterRestore) error {
	client := cs.client
	// The character-select reuse gate refuses a restore silently,
	// leaving the client's character list untouched.
	if !client.performFloodProtected(floodProtectorCharacterSelect, l.playerConfig.CharacterSelectDelay, time.Now()) {
		return nil
	}
	if c, ok := slotCharacter(cs.chars, req.Slot); ok {
		if err := l.roster.Restore(ctx, c.ID); err != nil {
			l.log.Error().Err(err).Msg("restore character")
		}
	}
	list, err := l.sendCharSelectInfo(ctx, client)
	if err != nil {
		l.log.Error().Err(err).Msg("list characters")
		return errDisconnect
	}
	cs.chars = list
	return nil
}

// handleRequestPledgeCrest answers with the requested clan crest.
func (l *GameClientLink) handleRequestPledgeCrest(_ context.Context, cs *connState, req clientpackets.RequestPledgeCrest) error {
	cs.client.Session.SendFrame(l.framePledgeCrest(req))
	return nil
}

// handleRequestAllyCrest answers an in-world player with the requested
// alliance crest, when there is one.
func (l *GameClientLink) handleRequestAllyCrest(_ context.Context, cs *connState, req clientpackets.RequestAllyCrest) error {
	if cs.live == nil {
		return nil
	}
	if frame, ok := l.frameAllyCrest(req); ok {
		cs.client.Session.SendFrame(frame)
	}
	return nil
}

// handleRequestNewCharacter answers with the creatable class templates.
func (l *GameClientLink) handleRequestNewCharacter(_ context.Context, cs *connState, _ struct{}) error {
	frame, err := serverpackets.FrameNewCharacterSuccess(l.templates)
	if err != nil {
		l.log.Error().Err(err).Msg("build NewCharacterSuccess")
		return errDisconnect
	}
	cs.client.Session.SendFrame(frame)
	return nil
}

// handleRequestGameStart selects the character in the addressed slot and
// readies it for EnterWorld.
func (l *GameClientLink) handleRequestGameStart(ctx context.Context, cs *connState, req clientpackets.RequestGameStart) error {
	client := cs.client
	// The character-select reuse gate refuses a selection silently.
	if !client.performFloodProtected(floodProtectorCharacterSelect, l.playerConfig.CharacterSelectDelay, time.Now()) {
		return nil
	}
	c, ok := slotCharacter(cs.chars, req.Slot)
	// An unknown slot or a banned character (AccessLevel < 0) aborts
	// the selection silently; the connection stays open.
	if !ok || c.AccessLevel < 0 {
		return nil
	}
	// A character already in the world belongs to another live
	// session: that session is closed with ServerClose and this
	// selection aborts silently.
	if l.world != nil {
		if obj, exists := l.world.Player(c.ObjectID()); exists {
			if prev, isLive := obj.(*livePlayer); isLive {
				l.log.Info().Int32("object_id", c.ObjectID()).
					Msg("game client: duplicate character login, closing previous session")
				prev.kickClient()
			}
			return nil
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
			return nil
		}
	}
	selected, next := l.selectCharacter(ctx, cs.conn, client, cs.chars, req.Slot, c)
	if selected != nil {
		cs.entering = selected
	}
	if next == selectionStopped {
		return errDisconnect
	}
	return nil
}

// handleEnterWorld spawns the selected character and moves the client in
// game.
func (l *GameClientLink) handleEnterWorld(_ context.Context, cs *connState, _ struct{}) error {
	client := cs.client
	// Unreachable while the state gate admits EnterWorld only in
	// StateEntering, which character selection enters together with
	// entering; closed like any other failed entry if that changes.
	if cs.entering == nil {
		client.closeNow()
		return errDisconnect
	}
	// A failed entry leaves live attached and registered; Handle's
	// deferred detachLivePlayer is what releases it.
	cs.live, cs.entering = cs.entering, nil
	if !l.enterWorld(client, cs.live) {
		client.closeNow()
		return errDisconnect
	}
	client.SetState(StateInGame)
	return nil
}
