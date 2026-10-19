package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
)

// The stadium lists: the Olympiad manager's, and the one a spectator opens.
const (
	stadiumListFile      = "data/html/olympiad/olympiad_observe_list.htm"
	arenaObserveListFile = "data/html/olympiad/olympiad_arena_observe_list.htm"
)

// spectatorSpawn is the index, among a stadium's normal spawns, of where its
// spectators watch from: past the two sides' spawns.
const spectatorSpawn = 2

// bypassArenaChange answers "arenachange <n>": a player whose last selected
// NPC is an Olympiad manager, or one watching a stadium, goes to watch
// stadium n; one on a waiting list or in a match is told it may not watch.
// Anyone else, a player mid-teleport, a number that does not parse and a
// stadium that is not answer nothing: the link click leaves no client
// action pending. The teleport gate keeps a refused jump from leaving the
// player a spectator wherever it stands.
func (l *GameClientLink) bypassArenaChange(live *livePlayer, command string) {
	f := live.currentFolk.Load()
	if (f == nil || !f.OlympiadManager()) && (!live.ObserverMode() || live.OlympiadGameID() < 0) {
		return
	}
	if l.olympiadRegistered(live) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageWaitingListCannotWatchGame))
		return
	}
	id, ok := arenaChangeID(command)
	if !ok || live.Teleporting() {
		return
	}
	l.enterOlympiadObserverMode(live, id)
}

// arenaChangeID reads the stadium of an "arenachange" command: what
// follows its twelfth UTF-16 unit, trimmed, as a 32-bit number.
func arenaChangeID(command string) (int, bool) {
	units := commons.UTF16Units(command)
	if len(units) < 12 {
		return 0, false
	}
	n, err := commons.ParseInt(strings.TrimFunc(commons.UTF16String(units[12:]), isCtrlOrSpace), 32)
	if err != nil {
		return 0, false
	}
	return int(n), true
}

// enterOlympiadObserverMode sends live to watch stadium id: its summon and
// cubics go, it leaves its party, stands up, keeps where it stands for its
// return unless it already watches a stadium, drops its selection and,
// hidden and unhurtable, lands on the stadium's spectator spawn, its client
// switched to spectating. A stadium that is not answers nothing.
func (l *GameClientLink) enterOlympiadObserverMode(live *livePlayer, id int) {
	if l.olympiad == nil {
		return
	}
	if _, ok := l.olympiad.Stadium(id); !ok {
		return
	}
	z, ok := l.stadiumZone(id)
	if !ok {
		return
	}
	spawns := z.Spawn(zone.SpawnNormal)
	if len(spawns) <= spectatorSpawn {
		l.log.Warn().Int("stadium", id).Int("spawns", len(spawns)).Msg("olympiad: stadium has no spectator spawn")
		return
	}
	l.dropAllSummons(live)
	if l.parties != nil {
		l.applyPartyNotices(l.parties.Leave(live, party.Expelled))
	}
	live.SetOlympiadGameID(id)
	live.Character.StandUp()
	if !live.ObserverMode() {
		live.SetSavedLocation(live.CurrentLocation())
	}
	l.clearLiveTarget(live)
	live.SetInvul(true)
	live.SetInvisible(true)
	l.teleportLivePlayer(live, spawns[spectatorSpawn], 0)
	live.SendFrame(serverpackets.FrameExOlympiadMode(serverpackets.OlympiadModeObserver))
}

// requestOlympiadObserverEnd answers RequestOlympiadObserverEnd: a stadium's
// spectator goes back to where it left. Anyone else hears nothing, as the
// spectator window's leave button waits on no answer; so does a spectator
// whose jump in its client has not reported landed, as the jump back would
// be refused and leave it in the stadium as a plain player.
func (l *GameClientLink) requestOlympiadObserverEnd(live *livePlayer) {
	if live.ObserverMode() && !live.Teleporting() {
		l.leaveOlympiadObserverMode(live)
	}
}

// leaveOlympiadObserverMode brings stadium spectator live back to the
// position it left to watch, shown and hurtable again. A player watching no
// stadium is left alone.
func (l *GameClientLink) leaveOlympiadObserverMode(live *livePlayer) {
	if live.OlympiadGameID() == -1 {
		return
	}
	live.SetOlympiadGameID(-1)
	live.tryToIdle(live.DenyAIAction())
	l.clearLiveTarget(live)
	live.SetInvisible(false)
	live.SetInvul(false)
	live.SendFrame(serverpackets.FrameExOlympiadMode(serverpackets.OlympiadModeNone))
	saved, _ := live.SavedLocation()
	l.teleportLivePlayer(live, saved, 0)
	live.ClearSavedLocation()
}

// requestOlympiadMatchList answers RequestOlympiadMatchList: a spectator is
// shown every stadium's state, each a link to watch it. Anyone else hears
// nothing, as the spectator window's list button waits on no answer.
func (l *GameClientLink) requestOlympiadMatchList(live *livePlayer) {
	if !live.ObserverMode() {
		return
	}
	var sb strings.Builder
	for i, s := range l.olympiadStadiums() {
		phase, names := s.Phase()
		sb.WriteString(`<tr><td fixwidth=10><a action="bypass arenachange `)
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString(`">`)
		sb.WriteString(strconv.Itoa(i + 1))
		sb.WriteString(`</a></td><td fixwidth=80>`)
		if phase == olympiad.PhaseInitial {
			sb.WriteString("&$906;</td><td>&nbsp;")
		} else {
			sb.WriteString(phaseLabel(phase))
			sb.WriteString("</td><td>")
			sb.WriteString(names[0])
			sb.WriteString("&nbsp; / &nbsp;")
			sb.WriteString(names[1])
		}
		sb.WriteString(`</td><td><font color="aaccff"></font></td></tr>`)
	}
	page := strings.ReplaceAll(l.setPage(arenaObserveListFile), "%list%", sb.String())
	sendFilledHTML(live, 0, page, 0)
}

// showOlympiadStadiums opens Olympiad manager f's list of the stadiums for
// live, each with its state and a link to watch it.
func (l *GameClientLink) showOlympiadStadiums(live *livePlayer, f *npc.Folk) {
	var sb strings.Builder
	for i, s := range l.olympiadStadiums() {
		phase, names := s.Phase()
		sb.WriteString(`<a action="bypass arenachange `)
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString(`">Arena `)
		sb.WriteString(strconv.Itoa(i + 1))
		sb.WriteString("&nbsp;")
		if phase == olympiad.PhaseInitial {
			sb.WriteString("(&$906;)</td><td>&nbsp;")
		} else {
			sb.WriteString("(")
			sb.WriteString(phaseLabel(phase))
			sb.WriteString(")&nbsp;")
			sb.WriteString(names[0])
			sb.WriteString("&nbsp; : &nbsp;")
			sb.WriteString(names[1])
		}
		sb.WriteString("</a><br>")
	}
	page := strings.ReplaceAll(l.setPage(stadiumListFile), "%list%", sb.String())
	page = strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(f.ObjectID())))
	sendFilledHTML(live, f.ObjectID(), page, 0)
}

// olympiadStadiums are the Olympiad's stadiums, none without an Olympiad.
func (l *GameClientLink) olympiadStadiums() []*olympiad.Stadium {
	if l.olympiad == nil {
		return nil
	}
	return l.olympiad.Stadiums()
}

// phaseLabel is the client string naming a started match's phase.
func phaseLabel(p olympiad.Phase) string {
	switch p {
	case olympiad.PhaseCounting:
		return "&$907;" // Counting In Progress
	case olympiad.PhaseInProgress:
		return "&$829;" // In Progress
	default:
		return "&$908;" // Terminate
	}
}

// sendOlympiadSpelled shows the spectators inside live's stadium the icon
// list of live's effects, while live, a competitor, fights there.
func (l *GameClientLink) sendOlympiadSpelled(live *livePlayer) {
	if !live.OlympiadMode() || !live.OlympiadStarted() || l.olympiad == nil {
		return
	}
	s, ok := l.olympiad.Stadium(live.OlympiadGameID())
	if !ok || !s.BattleStarted() {
		return
	}
	var to []*livePlayer
	for _, p := range (olympiadArena{l}).stadiumPlayers(s.ID()) {
		if p.ObserverMode() {
			to = append(to, p)
		}
	}
	if len(to) == 0 {
		return
	}
	effects := partyIconEffects(live.EffectList(), live.Queue().Now())
	sendToMembers(to, func() wire.Frame { return serverpackets.FrameExOlympiadSpelledInfo(live.ObjectID(), effects) })
}
