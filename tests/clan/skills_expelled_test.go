package clan

import (
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestClanSkillChangeSkipsMemberExpelledFirst holds the recruit's queue
// while the leader learns Clan Vitality, which posts the recruit's share of
// it there, and then expels the recruit. Once the queue runs, the posted
// share finds the recruit out of the clan and does nothing: no clan skill,
// no clan skill list addition and, when the price took the score to 0, no
// notice that the clan's skills are off. The recruit only learns it was
// expelled, after any clan header queued while it was still a member.
func TestClanSkillChangeSkipsMemberExpelledFirst(t *testing.T) {
	cases := []struct {
		name       string
		reputation int
	}{
		{"skill given", 1000},
		{"score to zero", vitalityCost},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := append(clanSkillOptions(t), seedClan(t, clanSeed{level: 5, reputation: tc.reputation}), gameservertest.WithRealPool())
			w := bootClanWorldCarrying(t, 40, 0, map[int32]int32{vitalityItem: 1}, opts...)
			w.recruit(t)
			w.talkToMaster(t)
			drainFrames(t, w.member)
			memberHP := w.srv.PlayerMaxHP(t, w.memberID)

			queue := w.srv.PlayerQueue(t, w.memberID)
			held, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			letGo := func() { releaseOnce.Do(func() { close(release) }) }
			t.Cleanup(letGo) // a failure below must not leave the queue held at shutdown
			if !queue.Post(func() { close(held); <-release }) {
				t.Fatal("post to the recruit's queue: queue closed")
			}
			<-held

			// The recruit's queue is held, so the reads wait for the
			// leader's answers rather than for every queue to settle.
			w.leader.Send(encodeRequestAcquireSkill(vitalityID, 1, acquireSkillReq))
			readUntilFrame(t, w.leader, "the leader's learn answer", func(f []byte) bool {
				return f[0] == serverpackets.OpcodeAcquireSkillList
			})
			w.leader.Send(encodeRequestOustPledgeMember("Recruit"))
			readUntilFrame(t, w.leader, "the leader's SUCCEEDED_IN_EXPELLING_CLAN_MEMBER", func(f []byte) bool {
				return slices.Equal(messages(t, [][]byte{f}), []int{serverpackets.SystemMessageSucceededInExpellingClanMember})
			})

			letGo()
			frames := drainFrames(t, w.member)
			view := clanSkillView(t, frames)
			for _, unwanted := range []string{
				"PledgeSkillListAdd",
				"sm" + itoa(serverpackets.SystemMessageClanSkillS1Added),
				"sm" + itoa(serverpackets.SystemMessageReputationLowClanSkillsDeactivated),
			} {
				if slices.Contains(view, unwanted) {
					t.Fatalf("expelled recruit got %s: %v", unwanted, view)
				}
			}
			if ids := messages(t, frames); !slices.Equal(ids, []int{serverpackets.SystemMessageClanMembershipTerminated}) {
				t.Fatalf("expelled recruit's messages = %v, want CLAN_MEMBERSHIP_TERMINATED only", ids)
			}
			for _, f := range frames {
				if f[0] == serverpackets.OpcodeSkillList && skillListLevel(t, f, vitalityID) != 0 {
					t.Fatal("expelled recruit's SkillList lists Clan Vitality")
				}
			}
			if hp := w.srv.PlayerMaxHP(t, w.memberID); hp != memberHP {
				t.Fatalf("expelled recruit's max HP = %d, want %d", hp, memberHP)
			}
		})
	}
}

// readUntilFrame reads c's frames until one matches, failing the test if
// none has within 100 frames.
func readUntilFrame(t *testing.T, c *testsupport.ScriptedClient, what string, match func([]byte) bool) {
	t.Helper()
	for range 100 {
		if match(c.Read()) {
			return
		}
	}
	t.Fatalf("%s not among 100 frames", what)
}
