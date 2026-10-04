package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/derby"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// DerbySink returns the sink that tells what the race track announces to
// every player standing inside one of zones' derby track zones.
func DerbySink(state *world.State, zones *zone.Index) event.Sink {
	return &derbySink{world: state, zones: zones}
}

type derbySink struct {
	world *world.State
	zones *zone.Index
}

// Emit sends ev's packets, zone by zone, each player of a zone hearing
// every part in order.
func (s *derbySink) Emit(ev event.Event) {
	announced, ok := ev.(event.DerbyAnnounced)
	if !ok || s.zones == nil || s.world == nil {
		return
	}
	for _, track := range zone.OfKind[*zone.DerbyTrack](s.zones) {
		var players []frameReceiver
		for _, a := range track.Occupants() {
			if a.Class() != zone.ClassPlayer {
				continue
			}
			if obj, ok := s.world.Object(a.ObjectID()); ok {
				if p, ok := obj.(*livePlayer); ok {
					players = append(players, p)
				}
			}
		}
		if len(players) == 0 {
			continue
		}
		for _, part := range announced.Parts {
			for _, build := range derbyFrames(part) {
				sendToEach(players, build)
			}
		}
	}
}

// derbyFrames builds the packets of one part of a race track
// announcement, in order.
func derbyFrames(part event.DerbyPart) []func() wire.Frame {
	switch p := part.(type) {
	case event.DerbyMessage:
		return []func() wire.Frame{func() wire.Frame {
			params := make([]serverpackets.SystemMessageParam, len(p.Numbers))
			for i, n := range p.Numbers {
				params[i] = serverpackets.NumberParam(n)
			}
			return serverpackets.FrameSystemMessageParams(p.ID, params...)
		}}
	case event.DerbySound:
		return []func() wire.Frame{func() wire.Frame {
			return serverpackets.FramePlaySoundAt(serverpackets.Sound{Type: p.Type, File: p.File})
		}}
	case event.DerbyRace:
		return []func() wire.Frame{func() wire.Frame { return serverpackets.FrameMonRaceInfo(p) }}
	case event.DerbyRunnersGone:
		out := make([]func() wire.Frame, 0, len(p.ObjectIDs))
		for _, id := range p.ObjectIDs {
			out = append(out, func() wire.Frame { return serverpackets.FrameDeleteObject(id, false) })
		}
		return out
	}
	return nil
}

// showDerbyRace shows p, coming to know race manager f, the race as it
// stands. It runs ahead of f's NpcInfo, the order a player walking up to
// the manager sees. A manager spawned in sight of p is shown the same way,
// although there the reference sends the race after the NpcInfo: Discover
// is not told which side moved, and managers spawn at boot, before anyone
// is in sight.
func (p *livePlayer) showDerbyRace(f *npc.Folk) {
	if p.link == nil || p.link.derby == nil || !f.DerbyTrackManager() {
		return
	}
	if race, ok := p.link.derby.RacePacket(); ok {
		p.sendVisibilityFrame(serverpackets.FrameMonRaceInfo(race))
	}
}

// hideDerbyRunners removes the race's runners from p, losing sight of race
// manager f, ahead of f itself: the order of a player walking away. A
// manager despawned in sight of p goes the same way, although there the
// reference deletes the manager before its runners.
func (p *livePlayer) hideDerbyRunners(f *npc.Folk) {
	if p.link == nil || p.link.derby == nil || !f.DerbyTrackManager() {
		return
	}
	for _, r := range p.link.derby.Runners() {
		p.sendVisibilityFrame(serverpackets.FrameDeleteObject(r.ObjectID, false))
	}
}

// derbyBypass runs race manager f's own command for live: its notice, the
// ticket bought or traded in, then its page or f's first chat page. It
// reports false when the command was too malformed to answer, so that
// nothing more is sent.
func (l *GameClientLink) derbyBypass(live *livePlayer, f *npc.Folk, command string) bool {
	if l.derby == nil {
		l.log.Debug().Int("npc_id", f.NpcID()).Str("command", command).Msg("bypass: no race track running")
		return true
	}
	reply := l.derby.Bypass(setPages{l.html}, f.NpcID(), f.ObjectID(), &live.derbyPicks, derbyHolder{live}, command)
	if reply.Aborted {
		return false
	}
	if reply.Notice != 0 {
		live.SendFrame(serverpackets.FrameSystemMessage(reply.Notice))
	}
	if buy := reply.Buy; buy != nil {
		if !reduceAdena(live, buy.Price) {
			return true
		}
		live.derbyPicks = derby.Picks{}
		l.giveDerbyTicket(live, buy)
	}
	if redeem := reply.Redeem; redeem != nil {
		l.redeemDerbyTicket(live, redeem)
	}
	if reply.Page != "" {
		sendFilledHTML(live, f.ObjectID(), reply.Page, 0)
		live.SendFrame(serverpackets.FrameActionFailed())
	}
	if reply.Chat {
		return l.folkBypass(live, f, "Chat 0")
	}
	return true
}

// giveDerbyTicket hands live the ticket of buy, tells live it was acquired
// and adds its price to its lane's stake.
func (l *GameClientLink) giveDerbyTicket(live *livePlayer, buy *derby.Purchase) {
	inv := live.Inventory()
	id, err := l.nextObjectID()
	if err != nil || inv == nil {
		l.log.Error().Err(err).Msg("derby: allocate race ticket")
		return
	}
	ticket := inv.AddNew(derby.TicketItemID, 1, id)
	if ticket == nil {
		l.log.Error().Msg("derby: create race ticket")
		return
	}
	inv.SetEnchantLevel(ticket, buy.Race)
	ticket.SetCustomType1(buy.Lane)
	ticket.SetCustomType2(buy.Price / 100)
	live.SendFrame(serverpackets.FrameSystemMessageNumberItemName(serverpackets.SystemMessageAcquiredS1S2, int32(buy.Race), derby.TicketItemID))
	l.derby.PlaceBet(buy.Lane, int64(buy.Price))
}

// redeemDerbyTicket destroys the ticket of redeem, naming it, then pays
// live its payout.
func (l *GameClientLink) redeemDerbyTicket(live *livePlayer, redeem *derby.Redemption) {
	inv := live.Inventory()
	if inv == nil {
		return
	}
	inst := inv.ItemByObjectID(redeem.ObjectID)
	if inst == nil {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
		return
	}
	count := inst.CountValue()
	if inv.DestroyItem(inst, count) == nil {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
		return
	}
	sendDestroyedMessage(live, inst.TemplateID, count)
	if redeem.Payout < 1 {
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageEarnedS1Adena, redeem.Payout))
		return
	}
	id, err := l.nextObjectID()
	if err != nil {
		l.log.Error().Err(err).Msg("derby: allocate race payout")
		return
	}
	live.AddRewardItem(item.AdenaID, int(redeem.Payout), id)
}

// derbyHolder reads a player's inventory for a race manager.
type derbyHolder struct{ live *livePlayer }

func (h derbyHolder) Tickets() []derby.Ticket {
	inv := h.live.Inventory()
	if inv == nil {
		return nil
	}
	var out []derby.Ticket
	for _, inst := range inv.ItemsByTemplateID(derby.TicketItemID) {
		out = append(out, derbyTicket(inst))
	}
	return out
}

func (h derbyHolder) Item(objectID int32) (derby.Ticket, bool) {
	inv := h.live.Inventory()
	if inv == nil {
		return derby.Ticket{}, false
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return derby.Ticket{}, false
	}
	return derbyTicket(inst), true
}

func derbyTicket(inst *item.Instance) derby.Ticket {
	st := inst.Snapshot()
	return derby.Ticket{ObjectID: st.ObjectID, Race: st.EnchantLevel, Lane: st.CustomType1, Hundreds: st.CustomType2}
}
