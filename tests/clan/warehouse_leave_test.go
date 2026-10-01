package clan

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: Clan.removeClanMember (Clan.java:700-711) sets the leaving
// player's active warehouse to null, whatever warehouse it is, so a deposit
// or withdrawal sent from the window it had open is dropped by
// SendWarehouseDepositList / SendWarehouseWithdrawList (no active
// warehouse), even once the player is back in the clan.

// bootStockedClanWarehouse boots a level 1 clan whose members may withdraw,
// with a join penalty of 0 days so a member may come back at once. The
// leader stocks the clan warehouse with 10 potions and the recruits hold
// the warehouse-search privilege. The recruit carries adena and a sword.
func bootStockedClanWarehouse(t *testing.T, extra ...gameservertest.Option) *whClanWorld {
	t.Helper()
	cfg := clan.DefaultConfig()
	cfg.MembersCanWithdrawFromWarehouse = true
	cfg.JoinDays = 0
	w := bootClanWarehouse(t,
		map[int32]int32{potionID: 10},
		map[int32]int32{item.AdenaID: whAdena, swordID: 1},
		append([]gameservertest.Option{gameservertest.WithClanConfig(cfg)}, extra...)...)
	w.raiseLevel(t)
	w.talkToKeeper(t, w.leader, w.leaderID)
	w.talkToKeeper(t, w.member, w.memberID)
	w.keeperCommand(t, w.leader, "DepositC")
	w.leader.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseDeposit, whRow{w.leaderItems[potionID], 10}))
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	w.setRecruitPrivileges(t, clan.PrivWarehouseSearch)
	if counts, _ := w.clanRows(t); counts[potionID] != 10 || len(counts) != 1 {
		t.Fatalf("stocked clan warehouse = %v, want 10 potions", counts)
	}
	return w
}

// openWithdrawal has c open the clan warehouse's withdrawal window.
func (w *whClanWorld) openWithdrawal(t *testing.T, c *testsupport.ScriptedClient, who string) {
	t.Helper()
	requireAnswer(t, who+" WithdrawC", w.keeperCommand(t, c, "WithdrawC"),
		serverpackets.OpcodeWarehouseWithdrawList, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed)
}

// requireRecruitWindowClosed has the recruit send a deposit of its sword
// and a withdrawal of 4 stored potions from the window it had open: neither
// is answered and nothing moves, in its inventory or at CLANWH.
func (w *whClanWorld) requireRecruitWindowClosed(t *testing.T, when string) {
	t.Helper()
	stored := w.leaderItems[potionID]
	w.member.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseDeposit, whRow{w.memberItems[swordID], 1}))
	w.member.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseWithdraw, whRow{stored, 4}))
	if frames := drainFrames(t, w.member); len(frames) != 0 {
		t.Fatalf("%s: requests from the old window answered %x, want nothing", when, opcodes(frames))
	}
	if s, p, a := w.held(t, w.memberID, swordID), w.held(t, w.memberID, potionID), w.held(t, w.memberID, item.AdenaID); s != 1 || p != 0 || a != whAdena {
		t.Fatalf("%s: recruit holds sword %d potions %d adena %d, want 1, 0 and %d", when, s, p, a, whAdena)
	}
	if counts, rows := w.clanRows(t); counts[potionID] != 10 || len(counts) != 1 || rows[potionID] != 1 {
		t.Fatalf("%s: clan warehouse rows = %v (rows %v), want one stack of 10 potions", when, counts, rows)
	}
}

// TestClanWarehouseClosedOnLeave has a recruit with the clan warehouse open
// leave the clan, or be expelled, then use the window it had open: nothing
// moves, whether it is still out of the clan or already taken back in. Back
// in the clan it passes the per-request membership check, so only leaving
// having dropped the window keeps it closed. A window it opens again works.
func TestClanWarehouseClosedOnLeave(t *testing.T) {
	for _, leave := range []struct {
		name string
		send func(w *whClanWorld)
	}{
		{"withdraw", func(w *whClanWorld) {
			w.member.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestWithdrawPledge).Bytes())
		}},
		{"oust", func(w *whClanWorld) {
			w.leader.Send(encodeRequestOustPledgeMember("Recruit"))
		}},
	} {
		for _, rejoin := range []bool{false, true} {
			name := leave.name
			if rejoin {
				name += "-rejoined"
			}
			t.Run(name, func(t *testing.T) {
				// skew moves the clan invitations' clock ahead of the wall
				// clock.
				var skew atomic.Int64
				w := bootStockedClanWarehouse(t, gameservertest.WithClanClock(func() time.Time {
					return time.Now().Add(time.Duration(skew.Load()))
				}))
				w.openWithdrawal(t, w.member, "recruit")

				leave.send(w)
				drainFrames(t, w.leader)
				drainFrames(t, w.member)
				if !rejoin {
					w.requireRecruitWindowClosed(t, "out of the clan")
					return
				}
				// The leader's invitation that took the recruit in first
				// lapses, and the recruit comes back.
				skew.Add(int64(clan.InviteTimeout))
				w.recruit(t)
				w.requireRecruitWindowClosed(t, "back in the clan")

				w.openWithdrawal(t, w.member, "rejoined recruit")
				w.member.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseWithdraw, whRow{w.leaderItems[potionID], 4}))
				if ids := messages(t, drainFrames(t, w.member)); len(ids) != 0 {
					t.Fatalf("withdrawal from a reopened window answered %v, want no message", ids)
				}
				if got := w.held(t, w.memberID, potionID); got != 4 {
					t.Fatalf("recruit potions after the reopened withdrawal = %d, want 4", got)
				}
				if counts, _ := w.clanRows(t); counts[potionID] != 6 {
					t.Fatalf("clan warehouse rows = %v, want 6 potions", counts)
				}
			})
		}
	}
}

// TestClanWarehouseConcurrentWithdrawals has the leader and a privileged
// recruit each ask for the whole stack of 10 stored potions, back to back
// before either answer is read. Exactly one gets them; the other gets
// nothing, and no potion is made or lost, in the inventories or the rows.
func TestClanWarehouseConcurrentWithdrawals(t *testing.T) {
	w := bootStockedClanWarehouse(t)
	w.openWithdrawal(t, w.leader, "leader")
	w.openWithdrawal(t, w.member, "recruit")

	stored := w.leaderItems[potionID]
	w.leader.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseWithdraw, whRow{stored, 10}))
	w.member.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseWithdraw, whRow{stored, 10}))
	for _, c := range []*testsupport.ScriptedClient{w.leader, w.member} {
		if ids := messages(t, drainFrames(t, c)); len(ids) != 0 {
			t.Fatalf("withdrawal answered messages %v, want none", ids)
		}
	}

	leader, recruit := w.held(t, w.leaderID, potionID), w.held(t, w.memberID, potionID)
	if (leader != 10 || recruit != 0) && (leader != 0 || recruit != 10) {
		t.Fatalf("potions held: leader %d recruit %d, want all 10 with one of them", leader, recruit)
	}
	if counts, _ := w.clanRows(t); len(counts) != 0 {
		t.Fatalf("clan warehouse rows = %v, want none", counts)
	}
	saved := 0
	for _, id := range []int32{w.leaderID, w.memberID} {
		for _, r := range w.savedRows(t, id) {
			if r.TemplateID == potionID {
				if r.Location != item.LocationInventory {
					t.Fatalf("potion row of %d at %v, want INVENTORY", id, r.Location)
				}
				saved += r.Count
			}
		}
	}
	if saved != 10 {
		t.Fatalf("saved potions = %d, want the 10 deposited", saved)
	}
}
