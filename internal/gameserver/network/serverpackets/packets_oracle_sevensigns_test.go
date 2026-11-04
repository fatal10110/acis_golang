package serverpackets

import (
	"context"
	"strconv"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// oracleSevenSignsState is ProbeSevenSigns.State: the calendar, scores,
// seal owners, votes and member counts of one Seven Signs state.
type oracleSevenSignsState struct {
	name                     string
	period                   sevensigns.Period
	cycle                    int
	dawnStone, duskStone     float64
	dawnFestival             int
	duskFestival             int
	owners                   [3]sevensigns.Cabal
	dawnVotes, duskVotes     [3]int
	dawnMembers, duskMembers int
}

// oracleSevenSignsStates is ProbeSevenSigns.STATES.
var oracleSevenSignsStates = func() []oracleSevenSignsState {
	const n, dawn, dusk = sevensigns.NoCabal, sevensigns.Dawn, sevensigns.Dusk
	return []oracleSevenSignsState{
		{"empty", sevensigns.Recruiting, 1, 0, 0, 0, 0, [3]sevensigns.Cabal{n, n, n}, [3]int{0, 0, 0}, [3]int{0, 0, 0}, 0, 0},
		{"competition", sevensigns.Competition, 3, 30, 10, 5, 0, [3]sevensigns.Cabal{n, dusk, dawn}, [3]int{2, 1, 0}, [3]int{0, 1, 0}, 4, 2},
		{"float", sevensigns.Results, 7, 171, 29, 0, 0, [3]sevensigns.Cabal{dusk, dawn, n}, [3]int{1, 3, 4}, [3]int{3, 1, 0}, 8, 8},
		{"validation", sevensigns.SealValidation, 2, 0, 50, 100, 0, [3]sevensigns.Cabal{dawn, n, dusk}, [3]int{2, 0, 0}, [3]int{1, 1, 0}, 0, 3},
		{"tie", sevensigns.Competition, 4, 10, 10, 0, 0, [3]sevensigns.Cabal{n, dawn, dusk}, [3]int{0, 1, 0}, [3]int{0, 0, 0}, 10, 10},
		{"dawn-claims", sevensigns.SealValidation, 5, 100, 0, 0, 0, [3]sevensigns.Cabal{dusk, dusk, dawn}, [3]int{7, 6, 1}, [3]int{0, 1, 0}, 20, 10},
		{"dusk-claims", sevensigns.Competition, 6, 0, 100, 0, 0, [3]sevensigns.Cabal{dawn, dawn, n}, [3]int{0, 2, 0}, [3]int{7, 1, 9}, 20, 20},
		{"tie-owned", sevensigns.SealValidation, 8, 3, 3, 7, 7, [3]sevensigns.Cabal{dawn, dusk, n}, [3]int{1, 0, 0}, [3]int{0, 4, 0}, 20, 20},
		{"dawn-short", sevensigns.Competition, 9, 100, 0, 0, 0, [3]sevensigns.Cabal{n, dusk, dusk}, [3]int{3, 3, 0}, [3]int{0, 0, 0}, 10, 10},
		{"dusk-short", sevensigns.Results, 10, 0, 100, 0, 0, [3]sevensigns.Cabal{dawn, dusk, dusk}, [3]int{0, 0, 0}, [3]int{3, 1, 9}, 10, 10},
	}
}()

// oracleSevenSignsStore hands Restore one state: the status row and the
// sign-ups ProbeSevenSigns.load builds.
type oracleSevenSignsStore struct {
	row     sevensigns.StatusRow
	players []sevensigns.PlayerRow
}

func (s oracleSevenSignsStore) LoadStatus(context.Context) (sevensigns.StatusRow, bool, error) {
	return s.row, true, nil
}
func (oracleSevenSignsStore) SaveStatus(context.Context, sevensigns.StatusRow) error { return nil }
func (s oracleSevenSignsStore) LoadPlayers(context.Context) ([]sevensigns.PlayerRow, error) {
	return s.players, nil
}

func (oracleSevenSignsStore) InsertPlayer(context.Context, sevensigns.PlayerRow) error { return nil }

func (oracleSevenSignsStore) SavePlayers(context.Context, []sevensigns.PlayerRow) error { return nil }

// state restores s the way ProbeSevenSigns.load sets the manager up: player
// 1 is the first member (3 red, 2 green and 1 blue stones, 40 ancient adena
// to collect), the other members follow from object id 2, dawn before dusk.
func (s oracleSevenSignsState) state() *sevensigns.State {
	store := oracleSevenSignsStore{row: sevensigns.StatusRow{
		Cycle: s.cycle, Period: s.period,
		DawnStoneScore: s.dawnStone, DuskStoneScore: s.duskStone,
		DawnFestivalScore: s.dawnFestival, DuskFestivalScore: s.duskFestival,
		SealOwners: s.owners, DawnSealVotes: s.dawnVotes, DuskSealVotes: s.duskVotes,
	}}
	for i := range s.dawnMembers + s.duskMembers {
		p := sevensigns.PlayerRow{ObjectID: int32(i + 1), Cabal: sevensigns.Dusk, Seal: sevensigns.Seals[i%len(sevensigns.Seals)]}
		if i < s.dawnMembers {
			p.Cabal = sevensigns.Dawn
		}
		if i == 0 {
			p.RedStones, p.GreenStones, p.BlueStones, p.AncientAdena = 3, 2, 1, 40
		}
		store.players = append(store.players, p)
	}
	now := func() time.Time { return time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC) }
	st := sevensigns.NewState(store, nil, zerolog.Nop(), now, nil)
	if err := st.Restore(context.Background()); err != nil {
		panic(err)
	}
	return st
}

// oracleFestivalMembers is ProbeSevenSigns.MEMBERS: each festival's best
// party per cabal (dusk, dawn), comma-joined as stored.
var oracleFestivalMembers = [festival.Count][2]string{
	{"", "Bo"},
	{"a,b,c", ",x"},
	{"a,,b", "a,b,"},
	{",", "Ünïcöde 日本語,ab"},
	{",,", "x"},
}

// oracleSevenSignsCases replays ProbeSevenSigns: the sky and the record's
// pages are read from a restored state, the festival page is built the way
// the server builds it from the festival scores.
func oracleSevenSignsCases() map[string]func() wire.Frame {
	m := map[string]func() wire.Frame{
		"SSQInfo/regular": FrameSSQInfo,
		"SSQInfo/dusk":    func() wire.Frame { return FrameSSQInfoSky(SSQSkyDusk) },
		"SSQInfo/dawn":    func() wire.Frame { return FrameSSQInfoSky(SSQSkyDawn) },
		"SSQInfo/red":     func() wire.Frame { return FrameSSQInfoSky(SSQSkyRed) },
	}
	for _, s := range oracleSevenSignsStates {
		n := "/" + s.name
		m["SSQInfo/sky"+n] = func() wire.Frame { return FrameSSQInfoSky(SSQSky(s.state().Sky())) }
		for _, page := range []byte{SSQPageRecord, SSQPageSeals, SSQPagePrediction} {
			m["SSQStatus/page"+strconv.Itoa(int(page))+n] = func() wire.Frame { return FrameSSQStatus(page, s.state().Record(1)) }
		}
	}
	competition := oracleSevenSignsStates[1]
	m["SSQStatus/page1/stranger"] = func() wire.Frame { return FrameSSQStatus(SSQPageRecord, competition.state().Record(99)) }
	m["SSQStatus/page0"] = func() wire.Frame { return FrameSSQStatus(0, competition.state().Record(1)) }
	m["SSQStatus/page5"] = func() wire.Frame { return FrameSSQStatus(5, competition.state().Record(1)) }

	for period := sevensigns.Recruiting; period <= sevensigns.SealValidation; period++ {
		m["SSQStatus/page2/"+strconv.Itoa(int(period))] = func() wire.Frame {
			festivals := make([]SSQFestival, festival.Count)
			for id := range festivals {
				dusk := festival.Score{Score: int(oracleRows[0].v[id]), Members: oracleFestivalMembers[id][0]}
				dawn := festival.Score{Score: int(oracleRows[3].v[id]), Members: oracleFestivalMembers[id][1]}
				festivals[id] = SSQFestival{
					MaxScore: int32(festival.MaxScores[id]),
					Dusk:     SSQFestivalBest{Score: int32(dusk.Score), Members: dusk.MemberNames()},
					Dawn:     SSQFestivalBest{Score: int32(dawn.Score), Members: dawn.MemberNames()},
				}
			}
			return FrameSSQStatusFestival(byte(period), festivals)
		}
	}
	return m
}
