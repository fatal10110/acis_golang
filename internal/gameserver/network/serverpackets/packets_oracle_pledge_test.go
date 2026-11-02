package serverpackets

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// oraclePledgeTypes is ProbePledge.PLEDGE_TYPES: the pledge type of roster
// member j.
var oraclePledgeTypes = [8]int32{0, 100, 0, -1, 1001, 0, 200, 2002}

// oraclePledgeUnits is ProbePledge.UNITS: the sub-units of the probed clan.
var oraclePledgeUnits = [5]int32{100, -1, 1001, 200, 2002}

// oraclePledgeRanks is the ranks of ProbePledge.privileges: ranks the clan has
// a mask for, then ranks it has none for.
var oraclePledgeRanks = [7]int32{1, 2, 3, 5, 7, 0, -1}

// pledgeProbe is ProbePledge's clan of row r: its eight members, sub-units,
// rank privileges and header.
type pledgeProbe struct {
	r int
	v [10]int32
}

func (p pledgeProbe) str(i int) string { return oracleStrings[i%len(oracleStrings)].s }
func (p pledgeProbe) pp(j int) partyProbe {
	return partyProbe{p.r, j, p.v}
}
func (p pledgeProbe) f(i int) int32 { return rowField(p.v, i) }

// online is ProbePledge.online.
func (p pledgeProbe) online(j int) bool { return j%3 != 1 }

// powerGrade is ProbePledge.powerGrade.
func (p pledgeProbe) powerGrade(j int) int32 {
	if j < 5 {
		return int32(j + 1)
	}
	return p.f(j + 4)
}

// hasSponsor and hasMentor report the mentor links of member j (by j % 4:
// none, a sponsor, an apprentice, both): a sponsor, and any link.
func (p pledgeProbe) hasSponsor(j int) bool { return j%4 == 1 || j%4 == 3 }
func (p pledgeProbe) hasMentor(j int) bool  { return j%4 != 0 }

// mentor is ClanMember.getApprenticeOrSponsorName: the apprentice's name, else
// the sponsor's, "Error" for one that is not in the roster.
func (p pledgeProbe) mentor(j int) string {
	switch j % 4 {
	case 2:
		if j == 6 {
			return "Error"
		}
		return p.pp(j + 1).name()
	case 3:
		if j == 7 {
			return "Error"
		}
		return p.pp(j + 2).name()
	case 1:
		return p.pp(j + 1).name()
	}
	return ""
}

func (p pledgeProbe) row(j int) PledgeMemberListMember {
	pp := p.pp(j)
	m := PledgeMemberListMember{
		Name: pp.name(), Level: pp.level(), ClassID: pp.class(),
		Sex: int32((p.r + j) % 3), Race: int32((p.r + j) % 5),
		PledgeType: oraclePledgeTypes[j], HasSponsor: p.hasMentor(j),
	}
	if p.online(j) {
		m.OnlineObjectID = pp.id()
	}
	return m
}

// updateRow is the row of PledgeShowMemberListUpdate(ClanMember): a member
// whose online field is zero (offline, or an online one with object id 0) shows
// no sex and race.
func (p pledgeProbe) updateRow(j int) PledgeMemberListMember {
	m := p.row(j)
	if m.OnlineObjectID == 0 {
		m.Sex, m.Race = 0, 0
	}
	return m
}

// roster is the clan's table (ProbePledge.fullClan) in the order its
// concurrent hash map lists it; row builds one member's row.
func (p pledgeProbe) roster(row func(j int) PledgeMemberListMember, count int) []PledgeMemberListMember {
	ids := make([]int32, count)
	for j := range ids {
		ids[j] = p.pp(j).id()
	}
	out := make([]PledgeMemberListMember, 0, count)
	for _, j := range oracleTableOrder(ids) {
		out = append(out, row(j))
	}
	return out
}

func (p pledgeProbe) dissolving() bool { return p.r%4 == 0 || p.r%4 == 3 }

// unitName is the name of sub-unit u (ProbePledge.fullClan).
func (p pledgeProbe) unitName(u int32) string {
	for i, id := range oraclePledgeUnits {
		if id == u {
			return p.str(p.r + i + 2)
		}
	}
	return ""
}

// unitLeader is Clan.getSubPledgeLeaderName: the captains of the units are
// members 1, 3 (the academy: never shown), none, one outside the roster, and
// member 7.
func (p pledgeProbe) unitLeader(u int32) string {
	switch u {
	case 0:
		return p.pp(0).name()
	case 100:
		return p.pp(1).name()
	case 2002:
		return p.pp(7).name()
	}
	return ""
}

func (p pledgeProbe) list(pledgeType int32, members []PledgeMemberListMember) PledgeMemberList {
	name := p.str(p.r)
	if pledgeType != 0 {
		name = p.unitName(pledgeType)
	}
	return PledgeMemberList{
		ClanID: p.f(0), PledgeType: pledgeType, PledgeName: name, LeaderName: p.unitLeader(pledgeType),
		CrestID: p.f(1), Level: p.f(2), CastleID: p.f(3), ClanHallID: p.f(4), Rank: p.f(5), Reputation: p.f(6),
		Dissolving: p.dissolving(), AllyID: p.f(8), AllyName: p.str(p.r + 1), AllyCrestID: p.f(9), AtWar: p.r%2 == 0,
		Members: members,
	}
}

func (p pledgeProbe) header() PledgeHeader {
	return PledgeHeader{
		ClanID: p.f(0), CrestID: p.f(1), Level: p.f(2), CastleID: p.f(3), ClanHallID: p.f(4), Rank: p.f(5), Reputation: p.f(6),
		Dissolving: p.dissolving(), AllyID: p.f(8), AllyName: p.str(p.r + 1), AllyCrestID: p.f(9), AtWar: p.r%2 == 0,
	}
}

// privileges is Clan.getPrivilegesByRank: ranks 1 to 5 have a mask.
func (p pledgeProbe) privileges(rank int32) int32 {
	if rank >= 1 && rank <= 5 {
		return p.f(int(rank) + 5)
	}
	return 0
}

// oracleTableOrder returns the indices of ids in the order a hash table
// lists them after they were put in index order.
func oracleTableOrder(ids []int32) []int {
	at := make([]PartyMemberAt, len(ids))
	for i, id := range ids {
		at[i] = PartyMemberAt{ObjectID: id, X: int32(i)}
	}
	out := make([]int, len(ids))
	for i, m := range inHashMapOrder(at) {
		out[i] = int(m.X)
	}
	return out
}

// oraclePledgeCases are the pledge windows (testdata/oracle/ProbePledge.java).
// Which rows a window lists, and in which order, is decided before the encoder,
// so each case hands the encoder the rows the probe's stand-ins hand the
// reference. The table order of the rosters is covered by the clan registry
// oracle.
func oraclePledgeCases() map[string]func() wire.Frame {
	m := map[string]func() wire.Frame{}
	for r, row := range oracleRows {
		p, n := pledgeProbe{r, row.v}, "/"+row.name
		addPledgeRosterCases(m, p, n)
		addPledgeHeaderCases(m, p, n)
		addPledgeMemberCases(m, p, n)
		addPledgeAssetCases(m, p, n)
		addPledgeAllianceCases(m, p, n)
		addPledgeHeroCases(m, p, n)
		for period := range int32(4) {
			m["ShowMiniMap/"+strconv.Itoa(int(period))+n] = func() wire.Frame { return FrameShowMiniMap(p.f(0), period) }
		}
	}
	addPledgeFlagCases(m)
	addPledgeWarListCases(m)
	m["ShowMiniMap/regular"] = func() wire.Frame { return FrameShowMiniMap(RegularMapID, 0) }
	return m
}

func addPledgeRosterCases(m map[string]func() wire.Frame, p pledgeProbe, n string) {
	m["PledgeShowMemberListAll/0"+n] = func() wire.Frame {
		return FramePledgeShowMemberListAll(p.list(0, p.roster(p.row, 8)))
	}
	for _, u := range oraclePledgeUnits {
		m["PledgeShowMemberListAll/"+strconv.Itoa(int(u))+n] = func() wire.Frame {
			return FramePledgeShowMemberListAll(p.list(u, p.roster(p.row, 8)))
		}
	}
	// The game master's view lists every member whatever the sub-unit, and its
	// last field reports a sponsor only.
	m["GMViewPledgeInfo"+n] = func() wire.Frame { return FrameGMViewPledgeInfo(p.str(p.r+5), p.gmList(p.dissolving())) }
	for j := range oraclePledgeTypes {
		row := p.row(j)
		add := row
		add.HasSponsor = false
		upd := p.updateRow(j)
		jn := "/m" + strconv.Itoa(j) + n
		m["PledgeShowMemberListAdd"+jn] = func() wire.Frame { return FramePledgeShowMemberListAdd(add) }
		m["PledgeShowMemberListUpdate/player"+jn] = func() wire.Frame { return FramePledgeShowMemberListUpdate(row) }
		m["PledgeShowMemberListUpdate/member"+jn] = func() wire.Frame { return FramePledgeShowMemberListUpdate(upd) }
	}
}

// gmList is the clan of ProbePledge.fullClan as the game master's view lists
// it.
func (p pledgeProbe) gmList(dissolving bool) PledgeMemberList {
	l := p.list(0, p.roster(func(j int) PledgeMemberListMember {
		row := p.row(j)
		row.HasSponsor = p.hasSponsor(j)
		return row
	}, 8))
	l.Dissolving = dissolving
	return l
}

func addPledgeHeaderCases(m map[string]func() wire.Frame, p pledgeProbe, n string) {
	m["PledgeInfo"+n] = func() wire.Frame { return FramePledgeInfo(p.f(0), p.str(p.r), p.str(p.r+1)) }
	m["PledgeStatusChanged"+n] = func() wire.Frame {
		return FramePledgeStatusChanged(p.pp(0).id(), p.f(0), p.f(1), p.f(8), p.f(9))
	}
	m["PledgeShowInfoUpdate"+n] = func() wire.Frame { return FramePledgeShowInfoUpdate(p.header()) }
	for i, rank := range oraclePledgeRanks {
		m["ManagePledgePower/rank"+strconv.Itoa(i)+n] = func() wire.Frame {
			return FrameManagePledgePower(rank, p.f(i+3), p.privileges(rank))
		}
	}
	for _, u := range oraclePledgeUnits {
		m["PledgeReceiveSubPledgeCreated/"+strconv.Itoa(int(u))+n] = func() wire.Frame {
			return FramePledgeReceiveSubPledgeCreated(u, p.unitName(u), p.unitLeader(u))
		}
	}
}

func addPledgeMemberCases(m map[string]func() wire.Frame, p pledgeProbe, n string) {
	for j, pt := range oraclePledgeTypes {
		pp := p.pp(j)
		pledge := p.str(p.r)
		if pt != 0 {
			pledge = p.unitName(pt)
		}
		jn := "/m" + strconv.Itoa(j) + n
		m["PledgeReceiveMemberInfo"+jn] = func() wire.Frame {
			return FramePledgeReceiveMemberInfo(PledgeMemberInfo{
				PledgeType: pt, Name: pp.name(), Title: p.str(p.r + j + 1),
				PowerGrade: p.powerGrade(j), PledgeName: pledge, Mentor: p.mentor(j),
			})
		}
		m["PledgeReceivePowerInfo"+jn] = func() wire.Frame {
			return FramePledgeReceivePowerInfo(p.powerGrade(j), pp.name(), p.privileges(p.powerGrade(j)))
		}
	}
}

func addPledgeAssetCases(m map[string]func() wire.Frame, p pledgeProbe, n string) {
	m["PledgeSkillList/empty"+n] = func() wire.Frame { return FramePledgeSkillList(nil) }
	skills := func(count int) []SkillListEntry {
		out := make([]SkillListEntry, count)
		for i := range out {
			out[i] = SkillListEntry{ID: int32(370 + i), Level: p.f(i)}
		}
		return out
	}
	m["PledgeSkillList/1"+n] = func() wire.Frame { return FramePledgeSkillList(skills(1)) }
	m["PledgeSkillList/5"+n] = func() wire.Frame { return FramePledgeSkillList(skills(5)) }
	m["PledgeSkillListAdd"+n] = func() wire.Frame { return FramePledgeSkillListAdd(p.f(0), p.f(1)) }

	full := make([]byte, 256)
	for i := range full {
		full[i] = byte(i*7 + p.r)
	}
	// A crest of another size is not a pledge crest: the cache answers nothing.
	m["PledgeCrest/absent"+n] = func() wire.Frame { return FramePledgeCrest(p.f(1), nil) }
	m["PledgeCrest/data"+n] = func() wire.Frame { return FramePledgeCrest(p.f(1), full) }
	m["PledgeCrest/short"+n] = func() wire.Frame { return FramePledgeCrest(p.f(1), nil) }
	m["PledgeCrest/empty"+n] = func() wire.Frame { return FramePledgeCrest(p.f(1), nil) }
}

// allyClan is ProbePledge.allyClan: clan k of the alliance table.
func (p pledgeProbe) allyClan(k int) (id, ally int32, c AllianceClan) {
	id = p.f(0) + int32(k)*0x101
	ally = p.f(0)
	if k == 2 {
		ally = p.f(0) + 99
	}
	c = AllianceClan{
		Name: p.str(p.r + k), Level: p.f(k + 2), LeaderName: p.str(p.r + 8*k),
		Total: int32(k + 1), Online: int32(k),
	}
	if k == 3 {
		c.LeaderName = ""
	}
	return id, ally, c
}

func addPledgeAllianceCases(m map[string]func() wire.Frame, p pledgeProbe, n string) {
	ally := p.f(0)
	build := func(alone bool) Alliance {
		var ids []int32
		var allyIDs []int32
		var clans []AllianceClan
		for k := range 4 {
			id, a, c := p.allyClan(k)
			if alone && id != ally {
				continue
			}
			ids, allyIDs, clans = append(ids, id), append(allyIDs, a), append(clans, c)
		}
		out := Alliance{Name: p.str(p.r + 1), LeaderClan: p.str(p.r), LeaderName: p.str(p.r)}
		// The alliance 0 is no alliance: it lists no clan.
		if ally == 0 {
			return out
		}
		for _, i := range oracleTableOrder(ids) {
			if allyIDs[i] != ally {
				continue
			}
			out.Clans = append(out.Clans, clans[i])
			out.Total += clans[i].Total
			out.Online += clans[i].Online
		}
		return out
	}
	m["AllianceInfo"+n] = func() wire.Frame { return FrameAllianceInfo(build(false)) }
	m["AllianceInfo/alone"+n] = func() wire.Frame { return FrameAllianceInfo(build(true)) }
}

func addPledgeHeroCases(m map[string]func() wire.Frame, p pledgeProbe, n string) {
	hero := func(j int) HeroListEntry {
		h := HeroListEntry{Name: p.str(p.r + j), ClassID: p.f(j + 3), Count: p.f(j + 6)}
		// The clan and alliance keys are absent for a hero out of a clan (odd j) or an alliance.
		if j%2 == 0 {
			h.ClanName, h.ClanCrest = p.str(p.r+j+1), p.f(j+5)
		}
		if j%4 == 0 {
			h.AllyName, h.AllyCrest = p.str(p.r+j+2), p.f(j+7)
		}
		return h
	}
	list := func(count int) []HeroListEntry {
		ids := make([]int32, count)
		for j := range ids {
			ids[j] = p.pp(j).id()
		}
		out := make([]HeroListEntry, 0, count)
		for _, j := range oracleTableOrder(ids) {
			out = append(out, hero(j))
		}
		return out
	}
	m["ExHeroList/empty"+n] = func() wire.Frame { return FrameExHeroList(nil) }
	m["ExHeroList/1"+n] = func() wire.Frame { return FrameExHeroList(list(1)) }
	m["ExHeroList/8"+n] = func() wire.Frame { return FrameExHeroList(list(8)) }
}

// addPledgeFlagCases are ProbePledge.headerFlags and .emptyClan, both on row "a".
func addPledgeFlagCases(m map[string]func() wire.Frame) {
	p := pledgeProbe{0, oracleRows[0].v}
	// The dissolving times 1, 0, -1, MIN and MAX: a time past zero is shown.
	dissolving := []bool{true, false, false, false, true}
	for t, d := range dissolving {
		for w := range 2 {
			n := "/t" + strconv.Itoa(t) + "w" + strconv.Itoa(w)
			atWar := w == 1
			header := p.header()
			header.Dissolving, header.AtWar = d, atWar
			m["PledgeShowInfoUpdate"+n] = func() wire.Frame { return FramePledgeShowInfoUpdate(header) }
			m["PledgeShowMemberListAll"+n] = func() wire.Frame {
				l := p.list(0, p.roster(p.row, 8))
				l.Dissolving, l.AtWar = d, atWar
				return FramePledgeShowMemberListAll(l)
			}
			m["GMViewPledgeInfo"+n] = func() wire.Frame {
				l := p.gmList(d)
				l.AtWar = atWar
				l.LeaderName = p.str(0)
				return FrameGMViewPledgeInfo(p.str(0), l)
			}
		}
	}
	// A clan of one: the leader alone, no sub-unit.
	solo := func() PledgeMemberList { return p.list(0, []PledgeMemberListMember{p.row(0)}) }
	m["PledgeShowMemberListAll/solo"] = func() wire.Frame { return FramePledgeShowMemberListAll(solo()) }
	m["GMViewPledgeInfo/solo"] = func() wire.Frame {
		l := solo()
		l.Members[0].HasSponsor = p.hasSponsor(0)
		return FrameGMViewPledgeInfo(p.str(0), l)
	}
	m["PledgeStatusChanged/solo"] = func() wire.Frame {
		return FramePledgeStatusChanged(p.pp(0).id(), p.f(0), p.f(1), p.f(8), p.f(9))
	}
}

// addPledgeWarListCases are ProbePledge.warLists: clans 1 to 40 are
// registered except every seventh, and a stored list names ids past them.
func addPledgeWarListCases(m map[string]func() wire.Frame) {
	for _, size := range []int{0, 1, 12, 13, 14, 26, 27, 40} {
		var names []string
		for i := range size {
			if id := (i*11)%40 + 1; id%7 != 0 {
				names = append(names, "clan"+strconv.Itoa(id))
			}
		}
		base := "PledgeReceiveWarList/s" + strconv.Itoa(size)
		m[base+"/declared"] = func() wire.Frame { return FramePledgeReceiveWarList(0, 0, size, names) }
		for tab := int32(1); tab <= 2; tab++ {
			for page := int32(0); page <= 3; page++ {
				m[base+"/t"+strconv.Itoa(int(tab))+"p"+strconv.Itoa(int(page))] = func() wire.Frame {
					return FramePledgeReceiveWarList(tab, page, size, names)
				}
			}
		}
	}
}
