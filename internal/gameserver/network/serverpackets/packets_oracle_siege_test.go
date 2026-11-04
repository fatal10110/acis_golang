package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// oracleSiegeDates is ProbeSiege.DATES: the siege dates in milliseconds.
var oracleSiegeDates = [...]int64{1_791_000_123_456, -1_500, 0, 3_000_000_000_999}

func oracleStr(k int) string { return oracleStrings[k%len(oracleStrings)].s }

// oracleSiegeClan is ProbeSiege.clan: clan k of row v.
func oracleSiegeClan(r int, v [10]int32, k int) SiegeClan {
	return SiegeClan{
		ClanID: rowField(v, k), Name: oracleStr(r + k), LeaderName: oracleStr(r + k + 2),
		CrestID: rowField(v, k+1), AllyID: rowField(v, k+2), AllyName: oracleStr(r + k + 1), AllyCrestID: rowField(v, k+3),
	}
}

// oracleSiegeInfo is the view the server builds of a residence with ownerID
// as a viewer of viewerClan sees it, when the clan table holds owner (nil:
// the owner is not a known clan). The current time is written as 0 on both
// sides (the probe checks the reference wrote the time it ran at).
func oracleSiegeInfo(id, ownerID int32, owner *SiegeClan, viewerClan int32, leader bool, dateMillis int64) func() wire.Frame {
	return func() wire.Frame {
		v := SiegeInfoView{
			CastleID: id, IsLord: ownerID == viewerClan && leader, OwnerID: ownerID,
			SiegeDate: int32(dateMillis / 1000),
		}
		if owner != nil && ownerID > 0 {
			v.Owner = true
			v.OwnerName, v.LeaderName, v.AllyID, v.AllyName = owner.Name, owner.LeaderName, owner.AllyID, owner.AllyName
		}
		return FrameSiegeInfo(v)
	}
}

// oracleSiegeCases replays ProbeSiege.
func oracleSiegeCases() map[string]func() wire.Frame {
	m := map[string]func() wire.Frame{}
	for r, row := range oracleRows {
		v, n := row.v, "/"+row.name
		owner := oracleSiegeClan(r, v, 0)
		castleDate, hallDate := oracleSiegeDates[r], oracleSiegeDates[(r+1)%len(oracleSiegeDates)]
		m["SiegeInfo/castle-lord"+n] = oracleSiegeInfo(rowField(v, 5), owner.ClanID, &owner, owner.ClanID, true, castleDate)
		m["SiegeInfo/castle-member"+n] = oracleSiegeInfo(rowField(v, 5), owner.ClanID, &owner, owner.ClanID, false, castleDate)
		m["SiegeInfo/castle-outsider"+n] = oracleSiegeInfo(rowField(v, 5), owner.ClanID, &owner, owner.ClanID^1, true, castleDate)
		m["SiegeInfo/hall-lord"+n] = oracleSiegeInfo(rowField(v, 6), owner.ClanID, &owner, owner.ClanID, true, hallDate)
		m["SiegeInfo/hall-outsider"+n] = oracleSiegeInfo(rowField(v, 6), owner.ClanID, &owner, owner.ClanID^1, true, hallDate)

		a, b, c := oracleSiegeClan(r, v, 0), oracleSiegeClan(r, v, 3), oracleSiegeClan(r, v, 6)
		m["SiegeAttackerList/castle"+n] = func() wire.Frame { return FrameSiegeAttackerList(rowField(v, 7), []SiegeClan{a, b}) }
		m["SiegeAttackerList/hall"+n] = func() wire.Frame { return FrameSiegeAttackerList(rowField(v, 8), []SiegeClan{c}) }
		m["SiegeDefenderList"+n] = func() wire.Frame {
			return FrameSiegeDefenderList(rowField(v, 7), []SiegeDefender{
				{SiegeClan: a, Side: SiegeDefenderOwner},
				{SiegeClan: b, Side: SiegeDefenderApproved},
				{SiegeClan: c, Side: SiegeDefenderPending},
			})
		}
	}

	v := oracleRows[0].v
	m["SiegeInfo/castle-unowned"] = oracleSiegeInfo(3, 0, nil, 0, true, oracleSiegeDates[0])
	m["SiegeInfo/castle-unknown-owner"] = oracleSiegeInfo(4, rowField(v, 1), nil, rowField(v, 1), true, oracleSiegeDates[0])
	m["SiegeInfo/hall-unowned"] = oracleSiegeInfo(21, 0, nil, 0, false, oracleSiegeDates[3])
	m["SiegeAttackerList/empty"] = func() wire.Frame { return FrameSiegeAttackerList(9, nil) }
	m["SiegeDefenderList/empty"] = func() wire.Frame { return FrameSiegeDefenderList(9, nil) }
	m["SiegeDefenderList/pending-only"] = func() wire.Frame {
		return FrameSiegeDefenderList(9, []SiegeDefender{{SiegeClan: oracleSiegeClan(0, v, 3), Side: SiegeDefenderPending}})
	}
	return m
}
