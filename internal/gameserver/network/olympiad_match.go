package network

import (
	"cmp"
	"slices"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// grandOlympiadManagerID is the Grand Olympiad Manager, the NPC that shouts
// the matches.
const grandOlympiadManagerID = 31688

// Olympiad buffs: a competitor gets Wind Walk, and a fighter Haste too.
var (
	olympiadWindWalk = modelskill.Ref{ID: 1204, Level: 2}
	olympiadHaste    = modelskill.Ref{ID: 1086, Level: 1}
)

// olympiadArena carries out the Olympiad's matches on live players and the
// stadium zones. Every effect on a player runs on that player's queue.
type olympiadArena struct{ l *GameClientLink }

var _ olympiad.Arena = olympiadArena{}

// bindOlympiadStadiums gives the Olympiad one stadium per stadium zone, in
// orderedStadiums order, draws made with rnd.
func (l *GameClientLink) bindOlympiadStadiums(rnd func(n int) int) {
	if l.olympiad == nil || l.queues == nil || l.zones == nil {
		return
	}
	l.olympiad.BindStadiums(len(orderedStadiums(l.zones)), olympiadArena{l}, l.queues.NewQueue, rnd)
}

// orderedStadiums are ix's stadium zones in the order the stadiums are
// numbered: by id modulo the size of a hash table holding them, a power of
// two of at least 16 filled at most to three quarters, then in load order.
func orderedStadiums(ix *zone.Index) []*zone.Olympiad {
	zs := zone.OfKind[*zone.Olympiad](ix)
	size := 16
	for len(zs) > size*3/4 {
		size *= 2
	}
	bucket := func(z *zone.Olympiad) int {
		id := z.ID()
		return (id ^ int(uint32(id)>>16)) & (size - 1)
	}
	slices.SortStableFunc(zs, func(a, b *zone.Olympiad) int { return cmp.Compare(bucket(a), bucket(b)) })
	return zs
}

// stadiumZone is the zone of stadium id.
func (l *GameClientLink) stadiumZone(id int) (*zone.Olympiad, bool) {
	if l.zones == nil {
		return nil, false
	}
	zs := orderedStadiums(l.zones)
	if id < 0 || id >= len(zs) {
		return nil, false
	}
	return zs[id], true
}

// wireStadiumZones gives the stadium zones of ix their match: whether its
// fight runs, the combat notices, the match window of a player walking in
// and the expulsion of anyone who has no business there.
func (l *GameClientLink) wireStadiumZones(ix *zone.Index) {
	for i, z := range orderedStadiums(ix) {
		z.BattleStarted = func() bool {
			if l.olympiad == nil {
				return false
			}
			s, ok := l.olympiad.Stadium(i)
			return ok && s.BattleStarted()
		}
		z.CombatNotice = stadiumCombatNotice
		z.SendMatchInfo = func(a zone.Actor) { l.sendStadiumMatchInfo(i, a) }
		z.ExpelUninvited = l.expelFromStadium
	}
}

// stadiumCombatNotice tells a player it entered or left a fighting
// stadium; leaving also closes its match window. The notice goes through
// the player's queue, behind what the stadium's match already sent it.
func stadiumCombatNotice(a zone.Actor, entering bool) {
	za, ok := a.(*liveZoneActor)
	if !ok {
		return
	}
	live := za.live
	postLive(live, func() {
		if entering {
			live.SendFrame(serverpackets.FrameSystemMessage(siege.MsgEnteredCombatZone))
			return
		}
		live.SendFrame(serverpackets.FrameSystemMessage(siege.MsgLeftCombatZone))
		live.SendFrame(serverpackets.FrameExOlympiadMatchEnd())
	})
}

// sendStadiumMatchInfo shows a player walking into stadium id while its
// fight runs both competitors' gauges, each followed by that competitor's
// effect icon pass, through the player's queue behind its combat notice.
func (l *GameClientLink) sendStadiumMatchInfo(id int, a zone.Actor) {
	za, ok := a.(*liveZoneActor)
	if !ok || l.olympiad == nil {
		return
	}
	s, ok := l.olympiad.Stadium(id)
	if !ok {
		return
	}
	g := s.Game()
	if g == nil {
		return
	}
	live := za.live
	for _, p := range g.Participants() {
		c, ok := l.onlineLive(p.ObjectID)
		if !ok {
			continue
		}
		info := olympiadUserInfo(c)
		postLive(live, func() { live.SendFrame(serverpackets.FrameExOlympiadUserInfo(info)) })
		postLive(c, func() { l.updateEffectIcons(c) })
	}
}

// expelFromStadium sends a player who walked into a stadium, or whose
// summon did, to town, its summon dismissed, unless it is a game master, a
// competitor or an observer.
func (l *GameClientLink) expelFromStadium(a zone.Actor) {
	var owner *livePlayer
	if za, ok := a.(*liveZoneActor); ok {
		owner = za.live
	} else if s, ok := l.zoneSummon(a); ok {
		owner, _ = liveSummonOwner(s)
	}
	if owner == nil || owner.accessLevel().IsGM || owner.OlympiadMode() || owner.ObserverMode() {
		return
	}
	postLive(owner, func() {
		if s := l.liveSummon(owner); s != nil {
			s.Unsummon()
		}
		if dest, ok := l.restartDestination(owner); ok {
			l.teleportLivePlayer(owner, dest, 20)
		}
	})
}

// onlineLive is the player objectID while it is in the world and not
// leaving it.
func (l *GameClientLink) onlineLive(objectID int32) (*livePlayer, bool) {
	live, ok := l.livePlayerByID(objectID)
	if !ok || live.Departed() {
		return nil, false
	}
	return live, true
}

// olympiadUserInfo is live as an Olympiad match window shows it.
func olympiadUserInfo(live *livePlayer) serverpackets.OlympiadUserInfo {
	res := live.ResourceValues()
	return serverpackets.OlympiadUserInfo{
		Side: live.OlympiadSide(), ObjectID: live.ObjectID(), Name: live.CharacterName(), ClassID: int32(live.ClassID()),
		HP: int32(res.CurrentHP), MaxHP: int32(res.MaxHP), CP: int32(res.CurrentCP), MaxCP: int32(res.MaxCP),
	}
}

// olympiadMessageFrame builds m's system message.
func olympiadMessageFrame(m olympiad.Message) wire.Frame {
	switch m.Kind {
	case olympiad.MessageEnterStadiumIn:
		return serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageEnterOlympiadStadiumInS1Seconds, int32(m.Number))
	case olympiad.MessageGameStartsIn:
		return serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageOlympiadGameStartsInS1Seconds, int32(m.Number))
	case olympiad.MessageGameStarted:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageOlympiadStartsTheGame)
	case olympiad.MessageMovedToTownIn:
		return serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageMovedToTownInS1Seconds, int32(m.Number))
	case olympiad.MessageOpponentLeft:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageOlympiadOpponentEndedTheGame)
	case olympiad.MessageOpponentUnfit:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageOlympiadOpponentUnfit)
	case olympiad.MessageUnfitDead:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotJoinOlympiadWhileDead)
	case olympiad.MessageUnfitSubclass:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageSubJobCannotJoinOlympiad)
	case olympiad.MessageUnfitCursedWeapon:
		return serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageCannotJoinOlympiadPossessingS1, int32(m.Number))
	case olympiad.MessageUnfitOverweight:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageInventoryTooFullForOlympiad)
	case olympiad.MessageWon:
		return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1HasWonTheGame, m.Name)
	case olympiad.MessageTie:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageOlympiadGameEndedInATie)
	case olympiad.MessagePointsGained:
		return serverpackets.FrameSystemMessageStringNumber(serverpackets.SystemMessageS1HasGainedS2OlympiadPoints, m.Name, int32(m.Number))
	default:
		return serverpackets.FrameSystemMessageStringNumber(serverpackets.SystemMessageS1HasLostS2OlympiadPoints, m.Name, int32(m.Number))
	}
}

// postOnline runs fn on objectID's queue while it is online.
func (a olympiadArena) postOnline(objectID int32, fn func(live *livePlayer)) bool {
	live, ok := a.l.onlineLive(objectID)
	if !ok {
		return false
	}
	postLive(live, func() { fn(live) })
	return true
}

// stadiumPlayers are the players inside stadium id.
func (a olympiadArena) stadiumPlayers(id int) []*livePlayer {
	z, ok := a.l.stadiumZone(id)
	if !ok {
		return nil
	}
	var out []*livePlayer
	for _, occ := range z.Occupants() {
		if za, ok := occ.(*liveZoneActor); ok {
			out = append(out, za.live)
		}
	}
	return out
}

// toStadium sends each player inside stadium id its own copy of one frame,
// on its queue.
func (a olympiadArena) toStadium(id int, build func() wire.Frame) {
	for _, p := range a.stadiumPlayers(id) {
		postLive(p, func() { p.SendFrame(build()) })
	}
}

func (a olympiadArena) Competitor(objectID int32) (olympiad.Competitor, bool) {
	live, ok := a.l.onlineLive(objectID)
	if !ok {
		return olympiad.Competitor{}, false
	}
	return olympiad.Competitor{Name: live.CharacterName(), BaseClass: live.BaseClassID()}, true
}

func (a olympiadArena) Online(objectID int32) bool {
	_, ok := a.l.onlineLive(objectID)
	return ok
}

func (a olympiadArena) Fitness(objectID int32) olympiad.Fitness {
	live, ok := a.l.onlineLive(objectID)
	if !ok {
		return olympiad.Fitness{}
	}
	return olympiad.Fitness{
		Online: true, Detached: live.clientDetached(), Observer: live.ObserverMode(), Dead: live.Dead(),
		Subclass: live.SubclassActive(), CursedWeaponID: live.CursedWeaponID(), Overweight: live.Overweight(),
	}
}

func (a olympiadArena) Standing(objectID int32) olympiad.Standing {
	live, ok := a.l.onlineLive(objectID)
	if !ok {
		return olympiad.Standing{GameID: -1}
	}
	res := live.ResourceValues()
	return olympiad.Standing{Online: true, Dead: live.Dead(), HP: res.CurrentHP, CP: res.CurrentCP, GameID: live.OlympiadGameID()}
}

func (a olympiadArena) Draft(objectID int32, stadium int) {
	if live, ok := a.l.onlineLive(objectID); ok {
		live.SetOlympiadGameID(stadium)
	}
}

func (a olympiadArena) Tell(objectID int32, m olympiad.Message) {
	a.postOnline(objectID, func(live *livePlayer) { live.SendFrame(olympiadMessageFrame(m)) })
}

func (a olympiadArena) Announce(stadium int, m olympiad.Message) {
	a.toStadium(stadium, func() wire.Frame { return olympiadMessageFrame(m) })
}

// Enter keeps where p stands for its return, clears its selection, makes it
// a competitor of stadium on its side, not yet fighting, and teleports it
// onto its side's spawn, then switches its client to its side.
func (a olympiadArena) Enter(stadium int, p olympiad.Participant) bool {
	z, ok := a.l.stadiumZone(stadium)
	if !ok {
		return false
	}
	spawns := z.Spawn(zone.SpawnNormal)
	if p.Side < 1 || p.Side > len(spawns) {
		return false
	}
	at := spawns[p.Side-1]
	return a.postOnline(p.ObjectID, func(live *livePlayer) {
		live.SetSavedLocation(live.CurrentLocation())
		a.l.clearLiveTarget(live)
		live.SetOlympiadGameID(stadium)
		live.SetOlympiadMode(true)
		live.SetOlympiadStarted(false)
		live.SetOlympiadSide(p.Side)
		a.l.teleportLivePlayer(live, at, 0)
		live.SendFrame(serverpackets.FrameExOlympiadMode(p.Side))
	})
}

func (a olympiadArena) Strip(objectID int32) {
	a.postOnline(objectID, a.l.stripCompetitor)
}

// stripCompetitor readies live to compete: its effects and clan skills go,
// it stops what it does and shows itself, loses its hero skills and is
// healed; it gets off its mount, or its pet goes, or its servitor loses its
// effects and stops; other players' cubics, its party, the items it may not
// use there, its automatic shots and its weapon's charge go; then it gets
// its skill list.
func (l *GameClientLink) stripCompetitor(live *livePlayer) {
	live.EffectList().StopAllExceptThoseThatLastThroughDeath()
	l.broadcastCharacterInfo(live)
	cl, inClan := l.clanService().ClanOf(live.Character)
	if inClan && l.skills != nil {
		l.takeClanSkills(live, cl)
	}
	live.abortAll(true)
	live.SetInvisible(false)
	if live.IsHero() && l.skills != nil {
		for _, ref := range modelskill.HeroSkills() {
			l.removeLiveSkill(live, int(ref.ID), false)
		}
	}
	live.SetMaxCpHpMp()
	if live.Mounted() {
		live.Character.Dismount()
	} else if s := l.liveSummon(live); s != nil {
		if s.IsPet() {
			s.Unsummon()
		} else {
			s.EffectList().StopAllExceptThoseThatLastThroughDeath()
			sendSummonInfosToOwner(s)
			s.AbortAll(true)
		}
	}
	l.stopCubicsGivenByOthers(live)
	if l.parties != nil {
		l.applyPartyNotices(l.parties.Leave(live, party.Expelled))
	}
	l.unequipRestrictedItems(live)
	ids := live.AutoSoulShotIDs()
	for _, id := range ids {
		live.SendFrame(serverpackets.FrameExAutoSoulShot(id, false))
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageAutoUseOfItemCancelled, id))
	}
	for _, id := range ids {
		live.SetAutoSoulShot(id, false)
	}
	if inv := live.Inventory(); inv != nil {
		if weapon := inv.ItemAt(itemcontainer.RHand); weapon != nil {
			weapon.UnchargeAllShots()
		}
	}
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
}

// stopCubicsGivenByOthers ends the cubics other players gave live, telling
// its observers once when any ended.
func (l *GameClientLink) stopCubicsGivenByOthers(live *livePlayer) {
	live.cubicsMu.Lock()
	stopped := live.Character.StopCubicsGivenByOthers()
	for _, id := range stopped {
		if runtime := live.cubics[id]; runtime != nil {
			runtime.Stop()
			delete(live.cubics, id)
		}
	}
	live.cubicsMu.Unlock()
	if len(stopped) > 0 {
		l.broadcastCharacterInfo(live)
	}
}

func (a olympiadArena) Buff(objectID int32) {
	a.postOnline(objectID, func(live *livePlayer) {
		if a.l.skills == nil {
			return
		}
		refs := []modelskill.Ref{olympiadWindWalk}
		if !live.MageClass() {
			refs = append(refs, olympiadHaste)
		}
		for _, ref := range refs {
			if def, ok := a.l.skills.Definition(ref); ok {
				skillhandler.LandEffects(live.Character, live.Character, def)
			}
		}
	})
}

func (a olympiadArena) Heal(objectID int32) {
	a.postOnline(objectID, func(live *livePlayer) { live.SetMaxCpHpMp() })
}

func (a olympiadArena) Fight(objectID int32) {
	a.postOnline(objectID, func(live *livePlayer) { live.SetOlympiadStarted(true) })
}

// ShowCompetitors shows everyone inside stadium each competitor's gauges,
// each followed by that competitor's effect icon pass.
func (a olympiadArena) ShowCompetitors(stadium int, one, two int32) {
	for _, id := range []int32{one, two} {
		live, ok := a.l.onlineLive(id)
		if !ok {
			continue
		}
		info := olympiadUserInfo(live)
		a.toStadium(stadium, func() wire.Frame { return serverpackets.FrameExOlympiadUserInfo(info) })
		postLive(live, func() { a.l.updateEffectIcons(live) })
	}
}

func (a olympiadArena) RefreshCombat(stadium int) {
	if z, ok := a.l.stadiumZone(stadium); ok {
		z.UpdateCombatStatus()
	}
}

// Settle ends live's fight: it may no longer fight, stops what it does and
// goes idle, its death is lifted, its summon stops, and it is healed.
func (a olympiadArena) Settle(objectID int32) {
	a.postOnline(objectID, func(live *livePlayer) {
		live.SetOlympiadStarted(false)
		live.abortAll(false)
		a.l.clearLiveTarget(live)
		live.tryToIdle(live.DenyAIAction())
		if live.Dead() {
			live.ClearOlympiadDeath()
		}
		if s := a.l.liveSummon(live); s != nil && !s.Dead() {
			s.AbortAll(true)
		}
		live.SetMaxCpHpMp()
	})
}

// Release ends live's standing as a competitor and switches its client
// back; its effects and charges go, its summon's effects too; its clan
// skills come back, healing it, then its hero skills; then it gets its
// skill list.
func (a olympiadArena) Release(objectID int32) {
	a.postOnline(objectID, func(live *livePlayer) {
		l := a.l
		live.SetOlympiadMode(false)
		live.SetOlympiadStarted(false)
		live.SetOlympiadSide(-1)
		live.SetOlympiadGameID(-1)
		live.SendFrame(serverpackets.FrameExOlympiadMode(serverpackets.OlympiadModeNone))
		live.EffectList().StopAllExceptThoseThatLastThroughDeath()
		l.broadcastCharacterInfo(live)
		live.ClearCharges()
		if s := l.liveSummon(live); s != nil && !s.Dead() {
			s.EffectList().StopAllExceptThoseThatLastThroughDeath()
			sendSummonInfosToOwner(s)
		}
		if cl, ok := l.clanService().ClanOf(live.Character); ok {
			l.giveClanSkills(live, cl, live.PledgeClass())
			live.SetMaxCpHpMp()
		}
		if live.IsHero() && l.skills != nil {
			if err := l.skills.GrantTransientSkills(live.Character, modelskill.HeroSkills()); err != nil {
				l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("olympiad: give hero skills back")
			}
		}
		live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	})
}

// SendBack returns live from where it entered the stadium to a random point
// of the restart point of that region, or to that very spot when it has
// none, and forgets it.
func (a olympiadArena) SendBack(objectID int32) {
	a.postOnline(objectID, func(live *livePlayer) {
		saved, ok := live.SavedLocation()
		if !ok {
			return
		}
		dest := saved
		if a.l.restarts != nil {
			if point, ok := a.l.restarts.PointAt(saved); ok && len(point.Points) > 0 {
				dest = point.Points[a.l.olympiadRoll(len(point.Points))]
			}
		}
		a.l.teleportLivePlayer(live, dest, 0)
		live.ClearSavedLocation()
	})
}

// Reward gives live each reward and tells it what it earned.
func (a olympiadArena) Reward(objectID int32, rewards []olympiad.Reward) {
	a.postOnline(objectID, func(live *livePlayer) {
		for _, r := range rewards {
			if live.AddCraftedItem(r.ItemID, r.Count, a.l.nextObjectID) == 0 {
				continue
			}
			live.SendFrame(serverpackets.FrameSystemMessageItemNameNumber(serverpackets.SystemMessageEarnedS2S1S, r.ItemID, int32(r.Count)))
		}
	})
}

// Shout has every Grand Olympiad Manager in the world shout text.
func (a olympiadArena) Shout(text string) {
	if a.l.world == nil {
		return
	}
	for _, obj := range a.l.world.Objects() {
		f, ok := obj.(*npc.Folk)
		if !ok || f.NpcID() != grandOlympiadManagerID {
			continue
		}
		if q := f.Queue(); q != nil {
			q.Post(func() { f.Shout(text) })
		} else {
			f.Shout(text)
		}
	}
}

// olympiadRoll draws an index below n for the Olympiad.
func (l *GameClientLink) olympiadRoll(n int) int {
	if l.olympiadDraw != nil {
		return l.olympiadDraw(n)
	}
	return rnd.Get(n)
}

// notifyCompetitorDamage counts damage attacker, a competitor, dealt the
// player targetID toward its match when both compete in the same stadium.
func (l *GameClientLink) notifyCompetitorDamage(attacker *livePlayer, targetID int32, damage int) {
	if l.olympiad == nil || attacker == nil || !attacker.OlympiadMode() {
		return
	}
	target, ok := l.livePlayerByID(targetID)
	if !ok || !target.OlympiadMode() || target.OlympiadGameID() != attacker.OlympiadGameID() {
		return
	}
	l.olympiad.CompetitorDamage(attacker.ObjectID(), attacker.OlympiadGameID(), damage)
}

// sendOlympiadVitals shows the new CP and HP of live, a fighting
// competitor, to everyone inside its stadium on the other side, when a
// gauge left its segment.
func (l *GameClientLink) sendOlympiadVitals(live *livePlayer, cpOrHP bool) {
	if !cpOrHP || !live.OlympiadMode() || !live.OlympiadStarted() || l.olympiad == nil {
		return
	}
	s, ok := l.olympiad.Stadium(live.OlympiadGameID())
	if !ok || !s.BattleStarted() {
		return
	}
	info := olympiadUserInfo(live)
	var to []*livePlayer
	for _, p := range (olympiadArena{l}).stadiumPlayers(s.ID()) {
		if p.ObserverMode() || p.OlympiadSide() != live.OlympiadSide() {
			to = append(to, p)
		}
	}
	l.broadcastToMembers(to, func() wire.Frame { return serverpackets.FrameExOlympiadUserInfo(info) })
}
