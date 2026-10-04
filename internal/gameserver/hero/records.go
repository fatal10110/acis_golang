package hero

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// The pages' date layouts: a diary entry's hour, a fight's minute.
const (
	diaryDateLayout = "2006-01-02 15"
	fightDateLayout = "2006-01-02 15:04"
)

// DiaryRow is one stored heroes_diary row.
type DiaryRow struct {
	// At is when the entry was made, in Unix milliseconds.
	At     int64
	Action int
	Param  int
}

// FightRow is one stored olympiad_fights row with each side's current
// character name; a side whose character is gone has none.
type FightRow struct {
	OneID, TwoID       int32
	OneClass, TwoClass int
	OneName, TwoName   string
	OneFound, TwoFound bool
	// Winner is 1 or 2 for the side that won, 0 for a draw.
	Winner int
	// Start is when the match started, in Unix milliseconds; Time how long
	// it lasted, in milliseconds.
	Start, Time int64
	Classed     int
}

// Names names what a diary entry tells of.
type Names interface {
	// NpcName returns the name of the npc template npcID.
	NpcName(npcID int) (string, bool)
	// CastleName returns the name of castle castleID.
	CastleName(castleID int) (string, bool)
}

// diaryEntry is one line of a diary page. An action whose npc or castle
// is unknown has no text: a page showing it is never sent.
type diaryEntry struct {
	date, action string
	hasAction    bool
}

// fightEntry is one line of a fight history page. A fight whose winner is
// none of 0, 1 or 2 has no result: a page showing it is never sent.
type fightEntry struct {
	start, result, opponent, opponentClass, time string
	classed                                      int
	hasResult                                    bool
}

// fightCounts are a hero's victories, draws and losses.
type fightCounts struct {
	victories, draws, losses int
}

// records are the diaries, fight histories and messages the pages show.
// As in the reference, every hero's diary entries go to one list and
// every hero's fights to another: a hero's page shows all of them, and an
// election forgets which heroes have pages but keeps the lists.
type records struct {
	diary       []diaryEntry
	fights      []fightEntry
	diaryHeroes map[int32]struct{}
	fightCounts map[int32]fightCounts
	messages    map[int32]string
}

func newRecords() records {
	return records{diaryHeroes: map[int32]struct{}{}, fightCounts: map[int32]fightCounts{}, messages: map[int32]string{}}
}

// reset forgets which heroes have pages and their messages.
func (r *records) reset() {
	clear(r.diaryHeroes)
	clear(r.fightCounts)
	clear(r.messages)
}

// loadRecords appends objectID's fights, then its diary, to the shared
// lists, and gives it both pages.
func (m *Manager) loadRecords(ctx context.Context, objectID int32) {
	m.loadFights(ctx, objectID)
	m.loadDiary(ctx, objectID)
}

// loadFights appends objectID's fights started before this month's first
// day to the shared list and counts its results. The month starts at
// midnight, but at the current second, as the reference's does.
func (m *Manager) loadFights(ctx context.Context, objectID int32) {
	now := m.now()
	before := time.Date(now.Year(), now.Month(), 1, 0, 0, now.Second(), 0, now.Location()).UnixMilli()
	rows, err := m.store.LoadFights(ctx, objectID, before)
	if err != nil {
		m.log.Error().Err(err).Int32("object_id", objectID).Msg("hero: load fights")
		return
	}
	var counts fightCounts
	fights := make([]fightEntry, 0, len(rows))
	for _, r := range rows {
		var (
			name, class string
			found       bool
			won, lost   int
		)
		switch objectID {
		case r.OneID:
			name, found, class, won, lost = r.TwoName, r.TwoFound, player.ClassName(r.TwoClass), 1, 2
		case r.TwoID:
			name, found, class, won, lost = r.OneName, r.OneFound, player.ClassName(r.OneClass), 2, 1
		default:
			continue
		}
		if !found {
			continue
		}
		f := fightEntry{
			opponent: name, opponentClass: class, classed: r.Classed, hasResult: true,
			time:  fightTime(r.Time),
			start: time.UnixMilli(r.Start).In(now.Location()).Format(fightDateLayout),
		}
		switch r.Winner {
		case won:
			f.result = `<font color="00ff00">victory</font>`
			counts.victories++
		case lost:
			f.result = `<font color="ff0000">loss</font>`
			counts.losses++
		case 0:
			f.result = `<font color="ffff00">draw</font>`
			counts.draws++
		default:
			f.hasResult = false
		}
		fights = append(fights, f)
	}
	m.mu.Lock()
	m.records.fights = append(m.records.fights, fights...)
	m.records.fightCounts[objectID] = counts
	m.mu.Unlock()
}

// fightTime is a match's length, in milliseconds, as mm:ss of its hour.
func fightTime(ms int64) string {
	s := ms / 1000
	return fmt.Sprintf("%02d:%02d", (s%3600)/60, s%60)
}

// loadDiary appends objectID's diary entries, oldest first, to the shared
// list.
func (m *Manager) loadDiary(ctx context.Context, objectID int32) {
	rows, err := m.store.LoadDiary(ctx, objectID)
	if err != nil {
		m.log.Error().Err(err).Int32("object_id", objectID).Msg("hero: load diary")
		return
	}
	entries := make([]diaryEntry, len(rows))
	for i, r := range rows {
		entries[i] = m.diaryEntry(r.At, r.Action, r.Param)
	}
	m.mu.Lock()
	m.records.diary = append(m.records.diary, entries...)
	m.records.diaryHeroes[objectID] = struct{}{}
	m.mu.Unlock()
}

// loadMessage reads objectID's message.
func (m *Manager) loadMessage(ctx context.Context, objectID int32) {
	msg, found, err := m.store.LoadMessage(ctx, objectID)
	if err != nil {
		m.log.Error().Err(err).Int32("object_id", objectID).Msg("hero: load message")
		return
	}
	if !found {
		return
	}
	m.mu.Lock()
	m.records.messages[objectID] = msg
	m.mu.Unlock()
}

// diaryEntry is the line a diary entry made at at, in Unix milliseconds,
// shows.
func (m *Manager) diaryEntry(at int64, action, param int) diaryEntry {
	e := diaryEntry{date: time.UnixMilli(at).In(m.now().Location()).Format(diaryDateLayout)}
	switch action {
	case DiaryRaidKilled:
		if m.names != nil {
			if name, ok := m.names.NpcName(param); ok {
				e.action, e.hasAction = name+" was defeated", true
			}
		}
	case DiaryHeroGained:
		e.action, e.hasAction = "Gained Hero status", true
	case DiaryCastleTaken:
		if m.names != nil {
			if name, ok := m.names.CastleName(param); ok {
				e.action, e.hasAction = name+" Castle was successfuly taken", true
			}
		}
	}
	return e
}

// noteDiaryEntry adds a raid boss kill or a castle taken, stored at at, to
// the shared diary when objectID has a diary page and the boss or castle
// is known.
func (m *Manager) noteDiaryEntry(objectID int32, at int64, action, param int) {
	if action != DiaryRaidKilled && action != DiaryCastleTaken {
		return
	}
	e := m.diaryEntry(at, action, param)
	if !e.hasAction {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.records.diaryHeroes[objectID]; ok {
		m.records.diary = append(m.records.diary, e)
	}
}

// SetMessage makes message objectID's hero message.
func (m *Manager) SetMessage(objectID int32, message string) {
	m.mu.Lock()
	m.records.messages[objectID] = message
	m.mu.Unlock()
}

// Shutdown stores every hero message, behind the heroes' other writes.
func (m *Manager) Shutdown() {
	m.write("save hero messages", func(ctx context.Context) error {
		m.mu.Lock()
		messages := maps.Clone(m.records.messages)
		m.mu.Unlock()
		return m.store.SaveMessages(ctx, messages)
	})
}

// HeroByClass returns the hero of the running era of classID, the lowest
// object id among several; found is false when there is none.
func (m *Manager) HeroByClass(classID int) (objectID int32, found bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, h := range m.heroes {
		if h.ClassID == classID && (!found || id < objectID) {
			objectID, found = id, true
		}
	}
	return objectID, found
}
