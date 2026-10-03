package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// olympiadAnnouncer tells every player online about an Olympiad calendar
// change.
type olympiadAnnouncer struct {
	state *world.State
}

// NewOlympiadAnnouncer returns the announcer telling every player online in
// state about Olympiad calendar changes.
func NewOlympiadAnnouncer(state *world.State) olympiad.Announcer {
	return olympiadAnnouncer{state: state}
}

// olympiadNoticeMessages maps each notice to its system message.
var olympiadNoticeMessages = map[olympiad.Notice]int{
	olympiad.NoticeCompetitionStarted: serverpackets.SystemMessageTheOlympiadGameHasStarted,
	olympiad.NoticeCompetitionEnded:   serverpackets.SystemMessageTheOlympiadGameHasEnded,
	olympiad.NoticeRegistrationEnded:  serverpackets.SystemMessageOlympiadRegistrationPeriodEnded,
	olympiad.NoticeCycleStarted:       serverpackets.SystemMessageOlympiadPeriodS1HasStarted,
	olympiad.NoticeCycleEnded:         serverpackets.SystemMessageOlympiadPeriodS1HasEnded,
}

// Announce sends n's system message, numbering the cycle for a cycle's
// start or end, to every player online.
func (a olympiadAnnouncer) Announce(n olympiad.Notice, cycle int32) {
	if a.state == nil {
		return
	}
	id := olympiadNoticeMessages[n]
	build := func() wire.Frame { return serverpackets.FrameSystemMessage(id) }
	if n == olympiad.NoticeCycleStarted || n == olympiad.NoticeCycleEnded {
		build = func() wire.Frame { return serverpackets.FrameSystemMessageNumber(id, cycle) }
	}
	broadcastFrame(build, func(send func(frameReceiver)) {
		for _, p := range a.state.Players() {
			if listener, ok := p.(*livePlayer); ok {
				send(listener)
			}
		}
	})
}

// userCommandOlympiadStat (/olympiadstat) tells a noble its record for the
// running Olympiad cycle, all zeros while it has none; anyone else is
// refused.
func (l *GameClientLink) userCommandOlympiadStat(live *livePlayer, _ int32) {
	if !live.IsNoble() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoblesseOnly))
		return
	}
	var rec olympiad.Noble
	if l.olympiad != nil {
		rec, _ = l.olympiad.Noble(live.ObjectID())
	}
	live.SendFrame(serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageOlympiadRecordS1MatchesS2WinsS3Defeats,
		serverpackets.NumberParam(int32(rec.CompDone)), serverpackets.NumberParam(int32(rec.CompWon)),
		serverpackets.NumberParam(int32(rec.CompLost)), serverpackets.NumberParam(int32(rec.Points))))
}

// giveNobleSkills gives a noble c the noble skills, without storing them.
func (l *GameClientLink) giveNobleSkills(c *player.Character) {
	if !c.IsNoble() || l.skills == nil {
		return
	}
	if err := l.skills.GrantTransientSkills(c, modelskill.NobleSkills()); err != nil {
		l.log.Error().Err(err).Int32("object_id", c.ID).Msg("give noble skills")
	}
}
