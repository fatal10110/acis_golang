package network

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// selectionOutcome is how a character selection ended for the read loop.
type selectionOutcome int

const (
	// selectionRefused aborts the selection silently; the connection stays
	// at character select.
	selectionRefused selectionOutcome = iota
	// selectionStopped ends the connection.
	selectionStopped
	// selectionEntering leaves the selected player registered, for
	// EnterWorld to spawn.
	selectionEntering
)

// selectCharacter selects c, the character in chars at slot, once the world
// checks have let the selection through: it waits for the character's queued
// saves, restores it from its fresh row, answers CharSelected, registers the
// player and takes over what an earlier session left in the world. It returns
// the player once one is registered, even when the selection then ends the
// connection, so the read loop detaches it.
//
// From the wait for the saves until the selection is over, a pet corpse the
// character left behind that settles is held for this selection
// (selectingOwners), since the selection reads the rows such a settle writes.
func (l *GameClientLink) selectCharacter(ctx context.Context, conn *Conn, client *Client, chars []*player.Character, slot int32, c *player.Character) (*livePlayer, selectionOutcome) {
	objectID := c.ObjectID()
	l.selections.begin(objectID)
	defer l.selections.end(objectID)
	// A previous session of this character has left the world, so every
	// save it queued is on the lane. Wait for them, then read the row fresh:
	// the character list may predate those saves, and selection restores the
	// character from its saved row. A wait that gave up refuses the
	// selection silently, as the world checks do, rather than load unwritten
	// rows.
	if l.awaitPersistence(conn, objectID) != nil {
		return nil, selectionRefused
	}
	// Journal writes an earlier session could not land are applied before
	// the journal loads; if they still fail, the selection is refused, as
	// loading without them would offer one-time quest rewards again.
	if err := l.settleQuests(objectID); err != nil {
		l.log.Error().Err(err).Int32("object_id", objectID).Msg("select character: write quests")
		return nil, selectionRefused
	}
	fresh, err := l.roster.Load(ctx, objectID)
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", objectID).Msg("select character: reload row")
		return nil, selectionRefused
	}
	// The list may predate a ban stored since; the row is what counts.
	if fresh.AccessLevel < 0 {
		return nil, selectionRefused
	}
	// A journal that does not load refuses the selection silently, as an
	// unwritten save does: entering with an empty journal would offer
	// one-time quest rewards again.
	if err := l.restoreQuests(ctx, fresh); err != nil {
		l.log.Error().Err(err).Int32("object_id", objectID).Msg("select character: load quests")
		return nil, selectionRefused
	}
	// Memos record one-time rewards too: one that does not load refuses
	// the selection the same way.
	if err := l.restoreMemos(ctx, fresh); err != nil {
		l.log.Error().Err(err).Int32("object_id", objectID).Msg("select character: load memos")
		return nil, selectionRefused
	}
	c = fresh
	chars[slot] = fresh
	l.clanService().RestoreMembership(c, time.Now())
	l.applyLoadedAccessLevel(c)
	tmpl, ok := l.templates.Get(c.ClassID())
	if !ok {
		l.log.Error().Int("class_id", c.ClassID()).Msg("select character: no template loaded")
		return nil, selectionStopped
	}
	// The selection restores the character in full. A restore that fails
	// attaches nothing, and closes the connection.
	selected, ok := l.restoreSelected(ctx, client, c)
	if !ok {
		client.closeNow()
		return nil, selectionStopped
	}
	session := client.Session
	session.SendFrame(ssqSkyFrame(l.sevenSigns))
	client.SetState(StateEntering)
	session.SendFrame(serverpackets.FrameCharSelected(serverpackets.CharSelectedSnapshot{
		Character: c, Template: tmpl, SessionID: client.SessionKey().PlayKey1,
		GameTime: l.gameTime(),
	}))
	// From here on lookups by name and id find the character, as the world
	// checks do for a later selection; it is spawned only once EnterWorld
	// arrives. Registered after CharSelected, so nothing sent to it can reach
	// its client ahead of the selection's answer.
	if l.world != nil {
		l.world.AddPlayer(selected)
	}
	if !l.takeOverSelected(selected) {
		client.closeNow()
		return selected, selectionStopped
	}
	return selected, selectionEntering
}
