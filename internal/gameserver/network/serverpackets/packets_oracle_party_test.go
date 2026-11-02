package serverpackets

import (
	"math"
	"strconv"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// oracleVitals is ProbeParty.VITALS: the doubles a party window casts to an
// int, then NaN and values outside the int range, both signs.
var oracleVitals = []float64{
	0, 0.5, 1.1, -3.25, 1e10, math.NaN(), 1e20, -1e20, 2147483647.5, -2147483648.5, math.Inf(1),
}

// oraclePartyClassIDs is ProbeParty.CLASS_INDEX: the class id of member (row+k) mod 4.
var oraclePartyClassIDs = [4]int32{0, 18, 53, 88}

// partyProbe is ProbeParty.member: member k of row r with a given gauge triple.
type partyProbe struct {
	r, k int
	v    [10]int32
}

func (p partyProbe) id() int32     { return rowField(p.v, 1) + int32(p.k)*0x101 }
func (p partyProbe) name() string  { return oracleStrings[(p.r+p.k)%len(oracleStrings)].s }
func (p partyProbe) class() int32  { return oraclePartyClassIDs[(p.r+p.k)%len(oraclePartyClassIDs)] }
func (p partyProbe) active() int32 { return rowField(p.v, p.k+2) }
func (p partyProbe) level() int32  { return rowField(p.v, p.k+6) }

func (p partyProbe) row(cp, hp, mp float64) PartyMember {
	return PartyMember{
		ObjectID: p.id(), Name: p.name(),
		CP: cp, MaxCP: float64(rowField(p.v, p.k+3)),
		HP: hp, MaxHP: float64(rowField(p.v, p.k+4)),
		MP: mp, MaxMP: float64(rowField(p.v, p.k+5)),
		Level: p.level(), ClassID: p.class(), Race: int32((p.r + p.k) % 5),
	}
}

// vitals is the member with the row's own gauges (ProbeParty.member).
func (p partyProbe) vitals() PartyMember {
	d := func(k int) float64 { return oracleDoubles[(p.r+k)%len(oracleDoubles)] }
	return p.row(d(p.k), d(p.k+1), d(p.k+2))
}

func (p partyProbe) at() PartyMemberAt {
	return PartyMemberAt{ObjectID: p.id(), X: rowField(p.v, p.k+7), Y: rowField(p.v, p.k+8), Z: rowField(p.v, p.k+9)}
}

func (p partyProbe) roomRow(bbs, status int32) PartyRoomMember {
	return PartyRoomMember{ObjectID: p.id(), Name: p.name(), ClassID: p.active(), Level: p.level(), Location: bbs, Status: status}
}

// oracleRoomRegionFallback is the region a room row shows for a member whose
// position lies in no restart point.
const oracleRoomRegionFallback = 100

// oracleRoom is ProbeParty.room: the room's terms with memberCount members.
func oracleRoom(r int, v [10]int32, k, memberCount int) PartyRoom {
	return PartyRoom{
		ID: rowField(v, k), Title: oracleStrings[(r+k)%len(oracleStrings)].s,
		Location: rowField(v, k+1), MinLevel: rowField(v, k+2), MaxLevel: rowField(v, k+3),
		Members: int32(memberCount), MaxMembers: rowField(v, k+4),
		LeaderName: partyProbe{r, 10 + k, v}.name(),
	}
}

// oracleHashMapIDSets are the object id sets of ProbeParty.positionOrders.
var oracleHashMapIDSets = [][]int32{
	{33, 17, 1, 50, 34, 2, 100000, -5, 0x10010},
	{9, 8, 7, 6, 5, 4, 3, 2, 1},
	{0x7fffffff, 0x10001, 0x20002, 16, 32, 48, math.MinInt32, -1, 15},
	{1},
	{130, 114, 98, 82, 66, 50, 34, 18, 3, 1, 17, 33, 49},
	{},
	{1, 17, 33, 49, 65, 81, 97, 113, 129},
	{2, 1, 17, 33, 49, 65, 81, 97, 113, 129},
}

// oraclePartyCases are the party, command channel and party-matching packets
// (testdata/oracle/ProbeParty.java). Which members a window lists, and the
// status and region of a room row, are decided before the encoder, so each case
// hands the encoder the rows the probe's stand-ins hand the reference.
func oraclePartyCases() map[string]func() wire.Frame {
	m := map[string]func() wire.Frame{}
	for r, row := range oracleRows {
		v, n := row.v, "/"+row.name
		addPartyWindowCases(m, r, v, n)
		addPartySpelledCases(m, r, v, n)
		addPartyMatchCases(m, r, v, n)
		addPartyChannelCases(m, r, v, n)
		m["PartyMemberPosition"+n] = func() wire.Frame {
			at := make([]PartyMemberAt, 5)
			for k := range at {
				at[k] = partyProbe{r, k, v}.at()
			}
			return FramePartyMemberPosition(at)
		}
	}
	addPartyVitalsCases(m)
	for s, ids := range oracleHashMapIDSets {
		m["PartyMemberPosition/ids"+strconv.Itoa(s)] = func() wire.Frame {
			at := make([]PartyMemberAt, len(ids))
			for k, id := range ids {
				at[k] = partyProbe{0, k, oracleRows[0].v}.at()
				at[k].ObjectID = id
			}
			return FramePartyMemberPosition(at)
		}
	}
	m["ExOpenMPCC"] = FrameExOpenMPCC
	m["ExCloseMPCC"] = FrameExCloseMPCC
	m["ExClosePartyRoom"] = FrameExClosePartyRoom
	return m
}

func addPartyWindowCases(m map[string]func() wire.Frame, r int, v [10]int32, n string) {
	p := func(k int) partyProbe { return partyProbe{r, k, v} }
	loot := int32(r % 5)
	m["PartySmallWindowAll"+n] = func() wire.Frame {
		// The window of member 1 lists the others, in party order.
		return FramePartySmallWindowAll(p(0).id(), loot, []PartyMember{p(0).vitals(), p(2).vitals(), p(3).vitals()})
	}
	m["PartySmallWindowAll/pair"+n] = func() wire.Frame {
		return FramePartySmallWindowAll(p(5).id(), loot, []PartyMember{p(5).vitals()})
	}
	m["PartySmallWindowAdd"+n] = func() wire.Frame { return FramePartySmallWindowAdd(p(0).id(), loot, p(2).vitals()) }
	m["PartySmallWindowUpdate"+n] = func() wire.Frame { return FramePartySmallWindowUpdate(p(3).vitals()) }
	m["PartySmallWindowDelete"+n] = func() wire.Frame { return FramePartySmallWindowDelete(p(1).id(), p(1).name()) }
}

// addPartyVitalsCases are ProbeParty.vitals and .lootRules, both on row "a".
func addPartyVitalsCases(m map[string]func() wire.Frame) {
	v := oracleRows[0].v
	for i := range oracleVitals {
		name := "/d" + strconv.Itoa(i)
		cp, hp, mp := oracleVitals[i], oracleVitals[(i+1)%len(oracleVitals)], oracleVitals[(i+2)%len(oracleVitals)]
		self, other := partyProbe{0, 0, v}, partyProbe{0, 1, v}
		m["PartySmallWindowUpdate"+name] = func() wire.Frame { return FramePartySmallWindowUpdate(self.row(cp, hp, mp)) }
		m["PartySmallWindowAdd"+name] = func() wire.Frame { return FramePartySmallWindowAdd(other.id(), 0, self.row(cp, hp, mp)) }
		m["PartySmallWindowAll"+name] = func() wire.Frame {
			return FramePartySmallWindowAll(other.id(), 0, []PartyMember{self.row(cp, hp, mp)})
		}
	}
	for rule := range int32(5) {
		name := "/loot" + strconv.Itoa(int(rule))
		m0, m1 := partyProbe{0, 0, v}, partyProbe{0, 1, v}
		m["PartySmallWindowAll"+name] = func() wire.Frame {
			return FramePartySmallWindowAll(m0.id(), rule, []PartyMember{m1.vitals()})
		}
		m["PartySmallWindowAdd"+name] = func() wire.Frame { return FramePartySmallWindowAdd(m0.id(), rule, m1.vitals()) }
	}
}

func addPartySpelledCases(m map[string]func() wire.Frame, r int, v [10]int32, n string) {
	for i, kind := range []struct {
		name  string
		owner int32
	}{{"player", PartySpelledPlayer}, {"pet", PartySpelledPet}, {"servitor", PartySpelledServitor}} {
		id := rowField(v, i+1)
		m["PartySpelled/"+kind.name+"/empty"+n] = func() wire.Frame { return FramePartySpelled(kind.owner, id, nil) }
		m["PartySpelled/"+kind.name+n] = func() wire.Frame {
			effects := make([]AbnormalStatusEffect, 4)
			for k := range effects {
				effects[k] = AbnormalStatusEffect{
					SkillID: rowField(v, k+i), Level: rowField(v, k+i+1), DurationMillis: int(rowField(v, k+i+2)),
				}
			}
			return FramePartySpelled(kind.owner, id, effects)
		}
	}
	if r == 0 {
		m["PartySpelled/durations"] = func() wire.Frame {
			var effects []AbnormalStatusEffect
			for _, ms := range []int32{999, 1000, 1001, 15999, -1, -999, -1000, -1999, math.MinInt32, math.MaxInt32} {
				effects = append(effects, AbnormalStatusEffect{SkillID: 1000 + ms%100, Level: 3, DurationMillis: int(ms)})
			}
			return FramePartySpelled(PartySpelledPlayer, rowField(v, 1), effects)
		}
	}
}

func addPartyMatchCases(m map[string]func() wire.Frame, r int, v [10]int32, n string) {
	p := func(k int) partyProbe { return partyProbe{r, k, v} }
	m["PartyMatchList/empty"+n] = func() wire.Frame { return FramePartyMatchList(nil) }
	m["PartyMatchList"+n] = func() wire.Frame {
		rooms := make([]PartyRoom, 3)
		for k := range rooms {
			rooms[k] = oracleRoom(r, v, k, k+1)
		}
		return FramePartyMatchList(rooms)
	}
	m["PartyMatchDetail"+n] = func() wire.Frame {
		room := oracleRoom(r, v, 3, 1)
		return FramePartyMatchDetail(PartyRoomTerms{
			ID: room.ID, MaxMembers: room.MaxMembers, MinLevel: room.MinLevel, MaxLevel: room.MaxLevel,
			Loot: rowField(v, 8), Location: room.Location, Title: room.Title,
		})
	}

	// Room 5: the leader (15), a member of the leader's party (20), a member of
	// another party (21) and a member in no party (22) with no restart point.
	leader, mate, foreign, alone := p(15), p(20), p(21), p(22)
	roomRows := func() []PartyRoomMember {
		return []PartyRoomMember{
			leader.roomRow(rowField(v, 7), 1),
			mate.roomRow(rowField(v, 8), 2),
			foreign.roomRow(rowField(v, 9), 0),
			alone.roomRow(oracleRoomRegionFallback, 0),
		}
	}
	m["ExPartyRoomMember"+n] = func() wire.Frame { return FrameExPartyRoomMember(rowField(v, 0), roomRows()) }
	for i, name := range []string{"leader", "mate", "foreign", "alone"} {
		m["ExManagePartyRoomMember/"+name+n] = func() wire.Frame { return FrameExManagePartyRoomMember(rowField(v, 1), roomRows()[i]) }
	}
	// A leader outside any party puts nobody at status 2.
	m["ExManagePartyRoomMember/leader-alone"+n] = func() wire.Frame {
		return FrameExManagePartyRoomMember(rowField(v, 1), p(24).roomRow(oracleRoomRegionFallback, 0))
	}
	m["ExPartyRoomMember/leader-alone"+n] = func() wire.Frame {
		return FrameExPartyRoomMember(rowField(v, 2), []PartyRoomMember{
			p(16).roomRow(oracleRoomRegionFallback, 1), p(24).roomRow(oracleRoomRegionFallback, 0),
		})
	}
	m["ExPartyRoomMember/empty"+n] = func() wire.Frame { return FrameExPartyRoomMember(rowField(v, 0), nil) }

	m["ExListPartyMatchingWaitingRoom/empty"+n] = func() wire.Frame {
		return FrameExListPartyMatchingWaitingRoom(rowField(v, 0), nil)
	}
	m["ExListPartyMatchingWaitingRoom"+n] = func() wire.Frame {
		waiting := make([]WaitingPlayer, 3)
		for k := range waiting {
			w := p(30 + k)
			waiting[k] = WaitingPlayer{Name: w.name(), ClassID: w.active(), Level: w.level()}
		}
		return FrameExListPartyMatchingWaitingRoom(rowField(v, 0), waiting)
	}
}

func addPartyChannelCases(m map[string]func() wire.Frame, r int, v [10]int32, n string) {
	p := func(k int) partyProbe { return partyProbe{r, k, v} }
	// Party one is members 0 and 1; party two is members 2 to 4.
	first := ChannelParty{LeaderName: p(0).name(), LeaderID: p(0).id(), Members: 2}
	second := ChannelParty{LeaderName: p(2).name(), LeaderID: p(2).id(), Members: 3}
	m["ExMPCCShowPartyMemberInfo"+n] = func() wire.Frame {
		rows := make([]PartyChannelMember, 3)
		for i := range rows {
			member := p(2 + i)
			rows[i] = PartyChannelMember{Name: member.name(), ObjectID: member.id(), ClassID: member.class()}
		}
		return FrameExMPCCShowPartyMemberInfo(rows)
	}
	m["ExMultiPartyCommandChannelInfo/one"+n] = func() wire.Frame {
		return FrameExMultiPartyCommandChannelInfo(p(0).name(), 2, []ChannelParty{first})
	}
	m["ExMultiPartyCommandChannelInfo"+n] = func() wire.Frame {
		return FrameExMultiPartyCommandChannelInfo(p(0).name(), 5, []ChannelParty{first, second})
	}
	m["ExMPCCPartyInfoUpdate/add"+n] = func() wire.Frame {
		return FrameExMPCCPartyInfoUpdate(second.LeaderName, second.LeaderID, second.Members, true)
	}
	m["ExMPCCPartyInfoUpdate/remove"+n] = func() wire.Frame {
		return FrameExMPCCPartyInfoUpdate(first.LeaderName, first.LeaderID, first.Members, false)
	}
}
