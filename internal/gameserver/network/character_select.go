package network

import (
	"context"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

// selectionOutcome is how a character selection ended for the read loop.
type selectionOutcome int

const (
	// selectionRefused aborts the selection silently, as the world checks
	// do; the connection stays at character select.
	selectionRefused selectionOutcome = iota
	// selectionFailed refuses a selection whose rows could not be settled or
	// read, and answers it with the character list; the connection stays at
	// character select.
	selectionFailed
	// selectionStopped ends the connection.
	selectionStopped
	// selectionEntering leaves the selected player registered, for
	// EnterWorld to spawn.
	selectionEntering
)

// selectCharacter selects c, the character in cs.chars at slot, unless
// another selection of it is under way or the world checks refuse it: it
// waits for the character's queued saves, restores it from its fresh row,
// answers CharSelected, registers the player and takes over what an earlier
// session left in the world. The restored player is cs.entering from the
// moment the restore hands it back, before anything else can fail or
// panic, so the read loop's teardown detaches it however the selection
// ends.
//
// From the wait for the saves until the selection is over, a pet corpse the
// character left behind that settles is held for this selection
// (selectingOwners), since the selection reads the rows such a settle writes.
func (l *GameClientLink) selectCharacter(ctx context.Context, cs *connState, slot int32, c *player.Character) selectionOutcome {
	conn, client, chars := cs.conn, cs.client, cs.chars
	objectID := c.ObjectID()
	// One selection of a character at a time: a second one while the first
	// is under way aborts silently, as the world checks below do. The claim
	// spans the checks, so a selection that registered its player before it
	// ended is found by them.
	if !l.flow.selections.begin(objectID) {
		return selectionRefused
	}
	defer l.flow.selections.end(objectID)
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
			return selectionRefused
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
			return selectionRefused
		}
	}
	// A previous session of this character has left the world, so every
	// save it queued is on the lane. Wait for them, then read the row fresh:
	// the character list may predate those saves, and selection restores the
	// character from its saved row. A wait that gave up refuses the
	// selection rather than load unwritten rows.
	if l.awaitSelectionSaves(conn, objectID) != nil {
		return selectionFailed
	}
	// Journal writes an earlier session could not land are applied before
	// the journal loads; if they still fail, the selection is refused, as
	// loading without them would offer one-time quest rewards again.
	if err := l.settleQuests(objectID); err != nil {
		l.log.Error().Err(err).Int32("object_id", objectID).Msg("select character: write quests")
		return selectionFailed
	}
	fresh, err := l.roster.Load(ctx, objectID)
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", objectID).Msg("select character: reload row")
		return selectionFailed
	}
	// The list may predate a ban or a deletion stored since; the row is what
	// counts, and both refuse silently, as the list's own checks do.
	if fresh.AccessLevel < 0 || fresh.DeleteAt > 0 {
		return selectionRefused
	}
	// A journal that does not load refuses the selection, as an unwritten
	// save does: entering with an empty journal would offer one-time quest
	// rewards again.
	if err := l.restoreQuests(ctx, fresh); err != nil {
		l.log.Error().Err(err).Int32("object_id", objectID).Msg("select character: load quests")
		return selectionFailed
	}
	// Memos record one-time rewards too: one that does not load refuses
	// the selection the same way.
	if err := l.restoreMemos(ctx, fresh); err != nil {
		l.log.Error().Err(err).Int32("object_id", objectID).Msg("select character: load memos")
		return selectionFailed
	}
	c = fresh
	chars[slot] = fresh
	l.clanService().RestoreMembership(c, time.Now())
	l.applyLoadedAccessLevel(c)
	if _, ok := l.templates.Get(c.ClassID()); !ok {
		l.log.Error().Int("class_id", c.ClassID()).Msg("select character: no template loaded")
		return selectionStopped
	}
	// The selection restores the character in full. A restore that fails
	// attaches nothing, and closes the connection.
	selected, ok := l.restoreSelected(ctx, client, c)
	if !ok {
		client.closeNow()
		return selectionStopped
	}
	cs.entering = selected
	session := client.Session
	session.SendFrame(ssqSkyFrame(l.sevenSigns))
	client.SetState(StateEntering)
	session.SendFrame(serverpackets.FrameCharSelected(serverpackets.CharSelectedSnapshot{
		Character: c, SessionID: client.SessionKey().PlayKey1,
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
		return selectionStopped
	}
	return selectionEntering
}

// awaitSelectionSaves waits for objectID's queued saves before a selection
// reads its rows. Owners share a persistence lane, so one owner's slow saves
// delay every selection on that lane: a wait that passes half its budget logs
// a warning naming the lane and the owner, while it is still waiting.
func (l *GameClientLink) awaitSelectionSaves(conn *Conn, objectID int32) error {
	budget := l.persistBudget()
	half := budget / 2
	var warnOnce sync.Once
	warn := func() {
		warnOnce.Do(func() {
			l.log.Warn().Uint32("lane", persist.LaneIndex(objectID)).Int32("owner_id", objectID).
				Dur("budget", budget).Msg("select character: wait for queued saves passed half its budget")
		})
	}
	start := time.Now()
	// The timer's callback only logs. It is stopped before this returns, and
	// the warnOnce call below waits out a callback already running, so no
	// warning outlives the wait.
	slow := time.AfterFunc(half, warn)
	err := l.awaitPersistence(conn, objectID)
	slow.Stop()
	if time.Since(start) >= half {
		warn()
	}
	return err
}
