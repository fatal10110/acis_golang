package combat

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	playermodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// killCorpses is a corpse-decay task on a clock the scenario moves by hand;
// a due corpse still in the world decays there, as the server's decay
// effect does.
type killCorpses struct {
	task *task.Decay

	mu    sync.Mutex
	now   time.Time
	state *world.State
}

func newKillCorpses(t *testing.T) *killCorpses {
	t.Helper()
	d := &killCorpses{now: time.Unix(1_700_000_000, 0)}
	decay, err := task.NewDecay(d, d.clock)
	if err != nil {
		t.Fatalf("task.NewDecay: %v", err)
	}
	d.task = decay
	return d
}

func (d *killCorpses) clock() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.now
}

func (d *killCorpses) Decay(actor task.DecayActor) {
	d.mu.Lock()
	state := d.state
	d.mu.Unlock()
	if obj, ok := state.Object(actor.ObjectID()); ok {
		if corpse, ok := obj.(interface {
			Decay(*world.State, func()) bool
		}); ok {
			corpse.Decay(state, nil)
		}
	}
}

// passAndTick moves the clock on by elapsed and runs one decay sweep.
func (d *killCorpses) passAndTick(t *testing.T, srv *gameservertest.Server, elapsed time.Duration) {
	t.Helper()
	d.mu.Lock()
	d.now = d.now.Add(elapsed)
	d.state = srv.State
	d.mu.Unlock()
	if err := d.task.Tick(); err != nil {
		t.Fatalf("decay tick: %v", err)
	}
	srv.Settle(t)
}

// rewardKindTemplate is a level 5 NPC of kind worth 4096 exp and 32 SP,
// with one guaranteed adena drop and a 7 s corpse time, so a reward or a
// drop roll that runs is observable.
func rewardKindTemplate(kind string) *npc.Template {
	return &npc.Template{
		ID: 100, TemplateID: 100, Type: kind, Level: 5, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		RewardExp: 4096, RewardSp: 32, CorpseTime: 7,
		Drops: []item.DropCategory{
			{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: item.AdenaID, Min: 10, Max: 10, Chance: 100}}},
		},
	}
}

// TestNonMonsterKillPaysNoReward pins that only a Monster-family NPC's kill
// pays a reward (Creature.doDie's calculateRewards is empty, and only
// Monster overrides it; Guard, SiegeGuard and FriendlyMonster extend
// Attackable). A karma player who kills a Guard, a SiegeGuard or a
// FriendlyMonster earns no exp or SP, loses no karma, rolls no drop and is
// told nothing; the corpse still decays at its corpse time (Npc.doDie's
// DecayTaskManager.add). The Monster control pays all of it.
func TestNonMonsterKillPaysNoReward(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		kind    string
		rewards bool
	}{
		{"Guard", false},
		{"SiegeGuard", false},
		{"FriendlyMonster", false},
		{"Monster", true},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			table, err := playermodel.NewLevelTable(map[int]playermodel.Level{
				1: {RequiredExpToLevelUp: 0},
				2: {RequiredExpToLevelUp: 1},
				3: {RequiredExpToLevelUp: 2},
				4: {RequiredExpToLevelUp: 3},
				5: {RequiredExpToLevelUp: 1000, KarmaModifier: 2.514219611},
				6: {RequiredExpToLevelUp: 3000},
				7: {RequiredExpToLevelUp: 1_000_000_000},
			})
			if err != nil {
				t.Fatalf("build level table: %v", err)
			}
			corpses := newKillCorpses(t)
			srv := gameservertest.Boot(t,
				gameservertest.WithWantChars(1),
				gameservertest.WithSeed(seedExperiencedCharacter(1500, 240)),
				gameservertest.WithLevels(table),
				gameservertest.WithDecay(corpses.task),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			startInWorld(t, c)
			target := srv.SpawnHostileNPCTemplateAt(t, rewardKindTemplate(tc.kind), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, c)
			obj, ok := srv.State.Player(objID)
			if !ok {
				t.Fatal("player missing from world state")
			}
			player, ok := network.OnlineCharacter(obj)
			if !ok {
				t.Fatalf("player %T is not an online character", obj)
			}

			if !target.TakeDamage(1_000_000, player) {
				t.Fatal("lethal hit did not kill the NPC")
			}
			srv.Settle(t)
			frames := readQuiet(c)
			if indexOf(frames, 0, serverpackets.OpcodeDie, target.ObjectID()) < 0 {
				t.Fatal("the kill sent no Die")
			}
			earned := indexOfSystemMessage(frames, 0, serverpackets.SystemMessageYouEarnedS1ExpAndS2SP) >= 0
			if earned != tc.rewards {
				t.Fatalf("YOU_EARNED_S1_EXP_AND_S2_SP sent = %v, want %v", earned, tc.rewards)
			}
			if indexOfSystemMessage(frames, 0, serverpackets.SystemMessageOverHit) >= 0 {
				t.Fatal("the kill sent an over-hit message")
			}
			if dropped := len(groundDrops(srv)) > 0; dropped != tc.rewards {
				t.Fatalf("ground drops after the kill = %v, want dropped %v", groundDrops(srv), tc.rewards)
			}

			// The corpse lasts its 7 s corpse time, then leaves the world.
			corpses.passAndTick(t, srv, 6*time.Second)
			if _, ok := srv.State.Object(target.ObjectID()); !ok || !target.HasCorpse() {
				t.Fatal("the corpse left before its corpse time")
			}
			corpses.passAndTick(t, srv, time.Second)
			if _, ok := srv.State.Object(target.ObjectID()); ok || !target.Decayed() {
				t.Fatal("the corpse did not decay at its corpse time")
			}
			if indexOf(readQuiet(c), 0, serverpackets.OpcodeDeleteObject, target.ObjectID()) < 0 {
				t.Fatal("the decay sent no DeleteObject")
			}

			logoutPersisted(t, srv, c)
			ch, err := srv.Chars.Get(context.Background(), objID)
			if err != nil {
				t.Fatalf("load saved character: %v", err)
			}
			if tc.rewards {
				if ch.Exp <= 1500 || ch.SP <= 0 || ch.KarmaPoints >= 240 {
					t.Fatalf("persisted character exp %d, SP %d, karma %d; want exp and SP earned and karma lost", ch.Exp, ch.SP, ch.KarmaPoints)
				}
				return
			}
			if ch.Exp != 1500 || ch.SP != 0 || ch.KarmaPoints != 240 {
				t.Fatalf("persisted character exp %d, SP %d, karma %d; want 1500, 0 and 240 untouched", ch.Exp, ch.SP, ch.KarmaPoints)
			}
		})
	}
}
