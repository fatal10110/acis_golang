package clan

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	// doomedClanID has no member online; its dissolution comes due a
	// minute after boot when the setup says so.
	doomedClanID int32 = 0x7f000403
	// fortressOfTheDead is the siegable hall 64.
	fortressOfTheDead int32 = 64
	// hallRecruitID is a Besiegers member, offline at boot.
	hallRecruitID int32 = 0x7f000404
)

func weekAhead() string { return strconv.FormatInt(time.Now().Add(7*24*time.Hour).UnixMilli(), 10) }

// TestHallSiegeRegistrationRefusals pins SiegableHall.registerClan's
// refusals, in its order, each answered with its message and then the
// hall's siege window, as RequestJoinSiege sends both: a clan below
// ChSiegeClanMinLevel (ONLY_CLAN_LEVEL_4_ABOVE_MAY_SIEGE), registrations
// closed (DEADLINE_FOR_SIEGE_S1_PASSED naming the hall), the siege fought
// (NOT_SIEGE_REGISTRATION_TIME2), the hall's owner
// (CLAN_THAT_OWNS_CASTLE_IS_AUTOMATICALLY_REGISTERED_DEFENDING), a clan
// owning another hall (CLAN_THAT_OWNS_CASTLE_CANNOT_PARTICIPATE_OTHER_SIEGE),
// a clan registered here (ALREADY_REQUESTED_SIEGE_BATTLE) or on another
// hall's siege (APPLICATION_DENIED_BECAUSE_ALREADY_SUBMITTED_A_REQUEST_FOR_
// ANOTHER_SIEGE_BATTLE), and a full attacker side (ATTACKER_SIDE_FULL). A
// dissolving clan is refused before (CANT_PARTICIPATE_IN_SIEGE_WHILE_
// DISSOLUTION_IN_PROGRESS), with no window. A hall no script runs the siege
// of answers nothing past the owner checks.
func TestHallSiegeRegistrationRefusals(t *testing.T) {
	t.Parallel()
	besiegers := itoa(besiegersClanID)
	for _, tc := range []struct {
		name   string
		setup  hallSiegeSetup
		before func(t *testing.T, w *hallSiegeWorld)
		want   [][]string
		window bool
	}{
		{
			name:   "clan level",
			setup:  hallSiegeSetup{extra: []gameservertest.Option{gameservertest.WithHallSiegeConfig(clanhall.SiegeConfig{MinClanLevel: 6, MaxAttackers: 10})}},
			want:   [][]string{{"645"}},
			window: true,
		},
		{
			name:  "registrations closed",
			setup: hallSiegeSetup{endDate: time.Now().Add(70 * time.Minute).UnixMilli()},
			before: func(t *testing.T, w *hallSiegeWorld) {
				w.srv.Advance(t, 11*time.Minute)
				drainFrames(t, w.c)
			},
			want:   [][]string{{"845", "34"}},
			window: true,
		},
		{
			name:  "siege fought",
			setup: hallSiegeSetup{owned: true},
			before: func(t *testing.T, w *hallSiegeWorld) {
				w.siege.Instant()
				w.srv.Advance(t, 0)
				drainFrames(t, w.c)
			},
			want:   [][]string{{"660"}},
			window: true,
		},
		{
			name:   "owner",
			setup:  hallSiegeSetup{seed: []string{"UPDATE clanhall SET ownerId = " + besiegers + " WHERE id = 34"}},
			want:   [][]string{{"688"}},
			window: true,
		},
		{
			name:   "owns another hall",
			setup:  hallSiegeSetup{seed: []string{"INSERT INTO clanhall (id, ownerId, paid, paidUntil) VALUES (22, " + besiegers + ", 1, " + weekAhead() + ")"}},
			want:   [][]string{{"689"}},
			window: true,
		},
		{
			name:   "registered here",
			setup:  hallSiegeSetup{seed: []string{"INSERT INTO clanhall_siege_attackers VALUES (34, " + besiegers + ")"}},
			want:   [][]string{{"638"}},
			window: true,
		},
		{
			name: "registered on another hall",
			setup: hallSiegeSetup{
				seed: []string{
					"INSERT INTO clanhall (id, endDate) VALUES (64, " + weekAhead() + ")",
					"INSERT INTO clanhall_siege_attackers VALUES (64, " + besiegers + ")",
				},
				extra: []gameservertest.Option{gameservertest.WithHallSiegeScript(fortressOfTheDead, (&standIn{}).script())},
			},
			want:   [][]string{{"639"}},
			window: true,
		},
		{
			name: "attacker side full",
			setup: hallSiegeSetup{
				seed:  []string{"INSERT INTO clanhall_siege_attackers VALUES (34, " + itoa(holdersClanID) + ")"},
				extra: []gameservertest.Option{gameservertest.WithHallSiegeConfig(clanhall.SiegeConfig{MinClanLevel: 4, MaxAttackers: 1})},
			},
			want:   [][]string{{"648"}},
			window: true,
		},
		{
			name:  "dissolving",
			setup: hallSiegeSetup{seed: []string{"UPDATE clan_data SET dissolving_expiry_time = " + weekAhead() + " WHERE clan_id = " + besiegers}},
			want:  [][]string{{"1114"}},
		},
		{
			name:  "no siege",
			setup: hallSiegeSetup{unbound: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootHallSiege(t, tc.setup)
			if tc.before != nil {
				tc.before(t, w)
			}
			attackers := w.siege.Attackers()
			frames := w.register(t, 1)
			if got := sysMsgParams(t, frames); !slices.EqualFunc(got, tc.want, slices.Equal) {
				t.Fatalf("messages = %v, want %v", got, tc.want)
			}
			if _, ok := firstOpcode(frames, serverpackets.OpcodeSiegeInfo); ok != tc.window {
				t.Fatalf("siege window sent = %v, want %v", ok, tc.window)
			}
			if got := w.siege.Attackers(); !slices.Equal(got, attackers) {
				t.Fatalf("attackers = %#x, want %#x", got, attackers)
			}
		})
	}
}

// TestHallSiegeUnregister drops the Attacker's registration, stored, while
// the registrations are open; during the siege a drop changes nothing.
func TestHallSiegeUnregister(t *testing.T) {
	t.Parallel()
	w := bootHallSiege(t, hallSiegeSetup{})
	w.register(t, 1)
	if _, ok := firstOpcode(w.register(t, 0), serverpackets.OpcodeSiegeInfo); !ok {
		t.Fatal("the drop got no siege window")
	}
	if len(w.siege.Attackers()) != 0 || len(w.storedAttackers(t, devastatedCastle)) != 0 {
		t.Fatal("the drop kept the registration")
	}
	w.register(t, 1)
	w.siege.Instant()
	w.srv.Advance(t, 0)
	w.register(t, 0)
	if !slices.Equal(w.siege.Attackers(), []int32{besiegersClanID}) {
		t.Fatalf("attackers after a drop during the siege = %#x", w.siege.Attackers())
	}
}

// TestHallSiegeAttackersRestoredAtBind restores the stored attackers of a
// hall as a script binds its siege, leaving out a clan that does not
// exist; Rainbow Springs takes none of its rows, and a hall no script runs
// the siege of has no attacker. Neither's rows are touched.
func TestHallSiegeAttackersRestoredAtBind(t *testing.T) {
	t.Parallel()
	besiegers := itoa(besiegersClanID)
	w := bootHallSiege(t, hallSiegeSetup{
		seed: []string{
			"INSERT INTO clanhall (id, endDate) VALUES (62, " + weekAhead() + "), (35, " + weekAhead() + ")",
			"INSERT INTO clanhall_siege_attackers VALUES (34, " + besiegers + "), (34, 2130706431), (62, " + besiegers + "), (35, " + besiegers + ")",
		},
		extra: []gameservertest.Option{gameservertest.WithHallSiegeScript(clanhall.RainbowSprings, (&standIn{}).script())},
	})
	if got := w.siege.Attackers(); !slices.Equal(got, []int32{besiegersClanID}) {
		t.Fatalf("Devastated Castle's attackers = %#x, want Besiegers", got)
	}
	for _, id := range []int32{clanhall.RainbowSprings, 35} {
		s, ok := w.srv.Halls.Siege(id)
		if !ok {
			t.Fatalf("hall %d has no siege", id)
		}
		if len(s.Attackers()) != 0 {
			t.Fatalf("hall %d's attackers = %#x, want none", id, s.Attackers())
		}
		if got := w.storedAttackers(t, id); !slices.Equal(got, []int32{besiegersClanID}) {
			t.Fatalf("hall %d's stored attackers = %#x, want untouched", id, got)
		}
	}
	if s, _ := w.srv.Halls.Siege(35); s.Scripted() {
		t.Fatal("Bandit Stronghold's siege runs with no script")
	}
}

// TestHallSiegeDateSetAnewAtBoot boots Devastated Castle with its stored
// siege date passed: the next date is set from now (14 days on, 12:00) and
// stored with the owner, the lease columns untouched.
func TestHallSiegeDateSetAnewAtBoot(t *testing.T) {
	t.Parallel()
	before := time.Now()
	w := bootHallSiege(t, hallSiegeSetup{owned: true, endDate: 1})
	after := time.Now()
	owner, paidUntil, endDate := w.hallRow(t)
	if owner != int64(holdersClanID) || paidUntil != hallPaidUntil {
		t.Fatalf("hall row = owner %#x, paidUntil %d", owner, paidUntil)
	}
	lo := clanhall.NextSiegeDate(before, []int{14, 0, 0, 12, 0}).UnixMilli()
	hi := clanhall.NextSiegeDate(after, []int{14, 0, 0, 12, 0}).UnixMilli()
	if endDate < lo || endDate > hi || w.siege.Date() != endDate {
		t.Fatalf("next siege date = %d (siege %d), want within %d..%d", endDate, w.siege.Date(), lo, hi)
	}
}

// TestHallSiegeLoginTakesAttackerState logs a Besiegers member in during
// the siege its clan attacks: it takes the attacker state.
func TestHallSiegeLoginTakesAttackerState(t *testing.T) {
	t.Parallel()
	w := bootHallSiege(t, hallSiegeSetup{
		seed: []string{"UPDATE characters SET clanid = " + itoa(besiegersClanID) + " WHERE obj_Id = " + itoa(hallRecruitID)},
		extra: []gameservertest.Option{gameservertest.WithSeed(func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
			ch, err := player.NewCharacter(hallRecruitID, gameservertest.ClassTemplate(), "player2", "Recruit", 40, 0, 0, player.SexMale)
			if err != nil {
				t.Fatalf("seed Recruit: %v", err)
			}
			if err := chars.Create(context.Background(), ch); err != nil {
				t.Fatalf("seed Recruit: %v", err)
			}
		})},
	})
	w.register(t, 1)
	w.siege.Instant()
	w.srv.Advance(t, 0)
	recruit := w.srv.DialClient(t, "player2", 1)
	startInWorld(t, recruit)
	if got := w.srv.PlayerSiegeState(t, hallRecruitID); got != 1 {
		t.Fatalf("member's siege state at login = %d, want 1", got)
	}
}

// TestHallSiegeDissolvedClan destroys a clan whose dissolution came due:
// owning no hall, it leaves the hall sieges' attackers, stored; owning
// Devastated Castle, the hall is freed and its row stored.
func TestHallSiegeDissolvedClan(t *testing.T) {
	t.Parallel()
	past := strconv.FormatInt(time.Now().Add(-24*time.Hour).UnixMilli(), 10)
	doomed := []string{"INSERT INTO clan_data (clan_id, clan_name, clan_level, dissolving_expiry_time) VALUES (" + itoa(doomedClanID) + ", 'Doomed', 5, " + past + ")"}
	t.Run("attacker", func(t *testing.T) {
		t.Parallel()
		w := bootHallSiege(t, hallSiegeSetup{seed: append(doomed,
			"INSERT INTO clanhall_siege_attackers VALUES (34, "+itoa(doomedClanID)+"), (34, "+itoa(holdersClanID)+")")})
		w.srv.Advance(t, 61*time.Second)
		if _, ok := w.srv.Clans.Table().Get(doomedClanID); ok {
			t.Fatal("Doomed was not dissolved")
		}
		if got := w.siege.Attackers(); !slices.Equal(got, []int32{holdersClanID}) {
			t.Fatalf("attackers = %#x, want Holders", got)
		}
		if got := w.storedAttackers(t, devastatedCastle); !slices.Equal(got, []int32{holdersClanID}) {
			t.Fatalf("stored attackers = %#x, want Holders", got)
		}
	})
	t.Run("owner", func(t *testing.T) {
		t.Parallel()
		w := bootHallSiege(t, hallSiegeSetup{seed: append(doomed, "UPDATE clanhall SET ownerId = "+itoa(doomedClanID)+" WHERE id = 34")})
		if id, ok := w.srv.Halls.ByOwner(doomedClanID); !ok || id != devastatedCastle {
			t.Fatal("Doomed does not own Devastated Castle at boot")
		}
		w.srv.Advance(t, 61*time.Second)
		if _, ok := w.srv.Halls.ByOwner(doomedClanID); ok {
			t.Fatal("the dissolved clan kept its hall")
		}
		if owner, paidUntil, _ := w.hallRow(t); owner != 0 || paidUntil != hallPaidUntil {
			t.Fatalf("hall row = owner %#x, paidUntil %d; want none, untouched", owner, paidUntil)
		}
	})
}

// TestRainbowSpringsDecrees restores the stored Rainbow Springs decrees at
// boot, then stores each change: a clan's decrees added, another's
// dropped, then all of them cleared.
func TestRainbowSpringsDecrees(t *testing.T) {
	t.Parallel()
	w := bootHallSiege(t, hallSiegeSetup{seed: []string{"INSERT INTO rainbowsprings_attacker_list VALUES (" + itoa(holdersClanID) + ", 3)"}})
	stored := func() map[int32]int {
		t.Helper()
		w.srv.FlushPersistence(t)
		rows, err := w.srv.DB.QueryContext(context.Background(), "SELECT clanId, war_decrees_count FROM rainbowsprings_attacker_list")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[int32]int{}
		for rows.Next() {
			var id int32
			var n int
			if err := rows.Scan(&id, &n); err != nil {
				t.Fatal(err)
			}
			out[id] = n
		}
		return out
	}
	equal := func(a, b map[int32]int) bool {
		if len(a) != len(b) {
			return false
		}
		for k, v := range a {
			if b[k] != v {
				return false
			}
		}
		return true
	}
	if got := w.srv.Halls.RainbowDecrees(); !equal(got, map[int32]int{holdersClanID: 3}) {
		t.Fatalf("restored decrees = %v", got)
	}
	w.srv.Halls.AddRainbowDecrees(besiegersClanID, 7)
	want := map[int32]int{holdersClanID: 3, besiegersClanID: 7}
	if got := stored(); !equal(got, want) || !equal(w.srv.Halls.RainbowDecrees(), want) {
		t.Fatalf("decrees after an add = %v, stored %v", w.srv.Halls.RainbowDecrees(), got)
	}
	w.srv.Halls.RemoveRainbowDecrees(holdersClanID)
	want = map[int32]int{besiegersClanID: 7}
	if got := stored(); !equal(got, want) || !equal(w.srv.Halls.RainbowDecrees(), want) {
		t.Fatalf("decrees after a drop = %v, stored %v", w.srv.Halls.RainbowDecrees(), got)
	}
	w.srv.Halls.ClearRainbowDecrees()
	if got := stored(); len(got) != 0 || len(w.srv.Halls.RainbowDecrees()) != 0 {
		t.Fatalf("decrees after the clear = %v, stored %v", w.srv.Halls.RainbowDecrees(), got)
	}
}
