package clan

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/rs/zerolog"
)

// Clan and leader ids of the alliance race world.
const (
	raceLeadClan   = 0x20000001
	raceLeadLeader = 0x20001001
	raceFirstClan  = 0x20000010
	raceFirstOther = 0x20100000
	raceContenders = 8
	raceRounds     = 50
	// raceOthers clans in no alliance fill the table, so each check walks
	// long enough for the contenders' checks to overlap: without allyMu
	// several of them would pass the check before any applies its change.
	raceOthers = 2000
)

// allianceRaceWorld restores a level 5 clan leading the alliance Ally,
// alone in it, beside raceContenders level 5 clans in no alliance and
// raceOthers more; the alliance holds maxClans clans. It returns the service and each contender
// clan's leader.
func allianceRaceWorld(t *testing.T, maxClans int, allied bool) (*Service, []*player.Character) {
	t.Helper()
	snap := Snapshot{}
	if allied {
		snap.Clans = append(snap.Clans, Row{ID: raceLeadClan, Name: "Lead", Level: 5, LeaderID: raceLeadLeader, AllyID: raceLeadClan, AllyName: "Ally"})
		snap.Members = append(snap.Members, MemberRow{ClanID: raceLeadClan, Member: Member{ObjectID: raceLeadLeader, Name: "LeadLeader"}})
	}
	leaders := make([]*player.Character, 0, raceContenders)
	for i := range raceContenders {
		id := int32(raceFirstClan + i)
		leader := id + 0x1000
		name := "Contender" + strconv.Itoa(i)
		snap.Clans = append(snap.Clans, Row{ID: id, Name: name, Level: 5, LeaderID: leader})
		snap.Members = append(snap.Members, MemberRow{ClanID: id, Member: Member{ObjectID: leader, Name: name + "Leader"}})
		leaders = append(leaders, inClan(leader, name+"Leader", id))
	}
	for i := range raceOthers {
		id := int32(raceFirstOther + i)
		name := "Other" + strconv.Itoa(i)
		snap.Clans = append(snap.Clans, Row{ID: id, Name: name, Level: 1, LeaderID: id + 0x100000})
		snap.Members = append(snap.Members, MemberRow{ClanID: id, Member: Member{ObjectID: id + 0x100000, Name: name + "Leader"}})
	}
	table := NewTable()
	table.Restore(snap, time.Now(), 1)
	cfg := DefaultConfig()
	cfg.MaxClansInAlly = maxClans
	return NewService(table, newFakeStore(), &laneWriter{}, nil, cfg, nil, zerolog.Nop()), leaders
}

// raceAll runs op for every index at once, released together, and waits
// for all of them.
func raceAll(n int, op func(i int)) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			op(i)
		}()
	}
	close(start)
	wg.Wait()
}

// TestJoinAllyRaceFillsTheLastSlotOnce has every contender clan accept an
// invitation into an alliance with one free slot at the same moment: the
// size check and the join are one step under allyMu, so exactly one clan
// joins and the alliance never exceeds its limit.
func TestJoinAllyRaceFillsTheLastSlotOnce(t *testing.T) {
	for round := range raceRounds {
		s, leaders := allianceRaceWorld(t, 2, true)
		results := make([]AllyJoinRefusal, len(leaders))
		raceAll(len(leaders), func(i int) {
			results[i] = s.JoinAlly(raceLeadLeader, leaders[i], time.Now()).Refusal
		})
		joined := 0
		for i, r := range results {
			switch r {
			case AllyJoinAllowed:
				joined++
			case AllyJoinFull:
			default:
				t.Fatalf("round %d: contender %d refused with %d, want joined or full", round, i, r)
			}
		}
		if allies := s.Table().Allies(raceLeadClan); joined != 1 || len(allies) != 2 {
			t.Fatalf("round %d: %d joins, alliance of %d clans; want 1 join and 2 clans", round, joined, len(allies))
		}
	}
}

// TestCreateAllyRaceTakesANameOnce has every contender clan found an
// alliance of the same name, in different cases, at the same moment: the
// name check and the founding are one step under allyMu, so exactly one
// clan founds it and the rest are told the name is taken.
func TestCreateAllyRaceTakesANameOnce(t *testing.T) {
	names := []string{"Ally", "ally", "ALLY", "aLLy"}
	for round := range raceRounds {
		s, leaders := allianceRaceWorld(t, 3, false)
		results := make([]AllyCreateResult, len(leaders))
		raceAll(len(leaders), func(i int) {
			results[i] = s.CreateAlly(leaders[i], names[i%len(names)], time.Now())
		})
		created, founders := 0, 0
		for i, r := range results {
			switch r {
			case AllyCreated:
				created++
			case AllyCreateNameTaken:
			default:
				t.Fatalf("round %d: contender %d founding = %d, want created or name taken", round, i, r)
			}
			cl, _ := s.Table().Get(leaders[i].ClanID())
			if cl.AllyID() != 0 {
				founders++
			}
		}
		if created != 1 || founders != 1 {
			t.Fatalf("round %d: %d foundings, %d clans in an alliance; want 1 and 1", round, created, founders)
		}
	}
}
