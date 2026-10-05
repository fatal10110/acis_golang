package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/npcstring"
)

const (
	// townNameNpcString is the NpcString id of castle 0's town name; castle
	// n's is this plus n.
	townNameNpcString = 1001000
	// kingdomOfAdenNpcString and kingdomOfElmoreNpcString name the
	// territory a castle's town lies in: Elmore for castles past Aden (id 6).
	kingdomOfAdenNpcString   = 1001000
	kingdomOfElmoreNpcString = 1001100
	elmoreFirstCastleID      = 7
)

// npcCastle returns the castle inst belongs to. A maker whose spawn time
// names a residence gives its NPCs that castle, or none when it names a
// siegable clan hall instead; otherwise the NPC belongs to the first castle
// whose npcs list holds its template id.
//
// Artifacts, control towers and mercenary guards are given their castle
// as they spawn, by the siege systems that spawn them (#236, #238).
func (l *GameClientLink) npcCastle(inst *npc.Instance) (*castle.Castle, bool) {
	if inst == nil {
		return nil, false
	}
	if id, ok := inst.Maker.ResidenceParam(); ok {
		if c, ok := l.castles.Get(id); ok {
			return c, true
		}
		if l.clanHallData != nil {
			if h, ok := l.clanHallData.Get(id); ok && h.IsSiegable() {
				return nil, false
			}
		}
	}
	if inst.Template == nil {
		return nil, false
	}
	return l.castles.ByNPC(inst.Template.ID)
}

// npcTaxRate is the tax rate of the castle inst belongs to, 0 when it
// belongs to none. The castle need not have an owner.
func (l *GameClientLink) npcTaxRate(inst *npc.Instance) float64 {
	if c, ok := l.npcCastle(inst); ok {
		return c.TaxRate()
	}
	return 0
}

// npcOwnedCastleTaxRate is the tax rate of the castle inst belongs to when
// a clan owns it, 0 otherwise: the rate a multisell list applying taxes
// takes.
func (l *GameClientLink) npcOwnedCastleTaxRate(inst *npc.Instance) float64 {
	if c, ok := l.npcCastle(inst); ok && !c.IsFree() {
		return c.TaxRate()
	}
	return 0
}

// showTerritoryStatus answers f's TerritoryStatus command: the town and
// kingdom of f's castle, with its lord, lord's clan and tax rate in force
// when a clan owns it. An NPC of no castle answers nothing.
func (l *GameClientLink) showTerritoryStatus(live *livePlayer, f *npc.Folk) {
	c, ok := l.npcCastle(f.Instance)
	if !ok {
		return
	}
	var owner *clan.Clan
	if id := c.OwnerID(); id != 0 && l.clans != nil {
		owner, _ = l.clans.Table().Get(id)
	}
	var page string
	var pairs []string
	if owner != nil {
		page = l.setPage("data/html/territorystatus.htm")
		pairs = append(pairs,
			"%clanName%", owner.Name(),
			"%clanLeaderName%", owner.SubunitLeaderName(clan.SubunitMain),
			"%taxPercent%", strconv.Itoa(c.CurrentTaxPercent()),
		)
	} else {
		page = l.setPage("data/html/territorynoclan.htm")
	}
	kingdom := kingdomOfAdenNpcString
	if c.ID >= elmoreFirstCastleID {
		kingdom = kingdomOfElmoreNpcString
	}
	territory, _ := npcstring.Text(int32(kingdom))
	town, _ := npcstring.Text(int32(townNameNpcString + c.ID))
	pairs = append(pairs,
		"%territory%", territory,
		"%townName%", town,
		"%objectId%", strconv.Itoa(int(f.ObjectID())),
	)
	for i := 0; i < len(pairs); i += 2 {
		page = strings.ReplaceAll(page, pairs[i], pairs[i+1])
	}
	sendFilledHTML(live, f.ObjectID(), page, 0)
}
