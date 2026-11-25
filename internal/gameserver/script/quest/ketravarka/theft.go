package ketravarka

import (
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Items of the totem thefts.
const (
	thiefKey    = 1661
	divineStone = 7081
)

// eyeLifetime is how long a box's eye stays once called.
const eyeLifetime = 10000 * time.Millisecond

// spawnedKey is the quest variable that holds whether the box's eye has been
// called.
const spawnedKey = "spawned"

// theftAlliance is the alliance level the thefts ask for.
const theftAlliance = 2

// theft is the quest that sends a level 74 ally to steal the other tribe's
// totem from its box. A warrior that sees the thief at the box calls the
// box's eye, and the theft then fails until the tribe's shaman lets the
// thief try again.
type theft struct {
	tribe
	name string
	eye  int32
	// called and seen are the NpcStrings the eye says: on being called, and
	// on dying while a thief of the quest stands by.
	called, seen int32
	// stolen is the stolen totem, totem the one the shaman gives for it.
	stolen, totem int32
}

func setSpawned(st *script.QuestState, v int) { st.Set(spawnedKey, strconv.Itoa(v)) }

// offered reports whether p may take the quest.
func (q theft) offered(p *script.Player) bool {
	return p.Level() >= 74 && q.allied(p, theftAlliance)
}

func (q theft) start(s *script.Script, p *script.Player, st *script.QuestState) {
	st.SetStatus(questlog.StatusStarted)
	st.SetCond(1)
	setSpawned(st, 0)
	s.PlaySound(p, script.SoundAccept)
}

// steal answers the box: refused once the eye has been called and without a
// Thief's Key, else the key is spent on the stolen totem. The pages are
// those of the box: seen, keyless and stolen.
func (q theft) steal(s *script.Script, p *script.Player, st *script.QuestState, seen, keyless, stolen string) string {
	switch {
	case st.GetInt(spawnedKey) == 1:
		return seen
	case !p.HasItem(thiefKey):
		return keyless
	}
	st.SetCond(3)
	s.PlaySound(p, script.SoundItemGet)
	s.TakeItems(p, thiefKey, 1)
	s.GiveItems(p, q.stolen, 1)
	return stolen
}

// shaman answers the tribe's shaman by the stage: it moves the thief on to
// the box, lets a seen thief try again, and trades the stolen totem for the
// totem and a Divine Stone of Wisdom. The pages are sent, waiting, again and
// done; the shaman says nothing else.
func (q theft) shaman(s *script.Script, p *script.Player, st *script.QuestState, sent, waiting, again, done string) string {
	switch cond := st.Cond(); {
	case cond == 1:
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		return sent
	case cond == 2:
		if st.GetInt(spawnedKey) == 0 {
			return waiting
		}
		setSpawned(st, 0)
		s.PlaySound(p, script.SoundMiddle)
		return again
	case cond == 3 && p.HasItem(q.stolen):
		s.TakeItems(p, q.stolen, 1)
		s.GiveItems(p, q.totem, 1)
		s.GiveItems(p, divineStone, 1)
		st.Unset(spawnedKey)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
		return done
	}
	return script.NoQuestMsg()
}

// onMyDying has the eye say a started quest taker's face is seen when it
// dies to the killer's party.
func (q theft) onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	e.NPC.Say(script.NpcString(q.seen))
}

// onSeeCreature calls the eye beside a thief at cond 2 once, and flags the
// quest as discovered.
func (q theft) onSeeCreature(s *script.Script, e script.SeeCreature) {
	p, ok := e.Creature.(*script.Player)
	if !ok {
		return
	}
	st := s.QuestState(p, q.name)
	if st == nil {
		return
	}
	if st.GetInt(spawnedKey) == 0 && st.Cond() == 2 {
		setSpawned(st, 1)
		if eye := s.Spawner().AddSpawn(q.eye, p, true, eyeLifetime); eye != nil {
			eye.Say(script.NpcString(q.called))
			s.PlaySound(p, script.SoundGiveUp)
		}
	}
}
