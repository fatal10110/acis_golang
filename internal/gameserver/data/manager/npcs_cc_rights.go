package manager

import (
	"strings"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/npcstring"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// LootChannels resolves the command channel a raid boss's attacker fights
// in.
type LootChannels interface {
	// LootChannel returns the command channel the player's party is in.
	LootChannel(playerID int32) (LootChannel, bool)
}

// LootChannel is one command channel. Its values must be comparable: two
// of them are the same channel exactly when they compare equal.
type LootChannel interface {
	// MembersCount counts the members of every party in the channel.
	MembersCount() int
	// Leader returns the channel's leader.
	Leader() *player.Character
}

// Command channel loot rights timing: the rights lapse once the holding
// channel has not hit the boss for ccRightsLapse, checked every
// ccRightsCheck; the boss's announcements stay on screen for
// ccRightsScreenMs.
const (
	ccRightsLapse    = 5 * time.Minute
	ccRightsCheck    = time.Second
	ccRightsScreenMs = 10000
)

// bossLootRule is a boss's command channel loot rule: the NpcString ids of
// the on-screen texts announcing a channel winning and losing the rights,
// and the member count a channel must exceed to win them.
type bossLootRule struct {
	rightsMsg, noRightsMsg int32
	requiredMembers        int
}

// lootRule returns the loot rule of the boss of npc id npcID.
func lootRule(npcID int) bossLootRule {
	switch npcID {
	case 29001: // Queen Ant
		return bossLootRule{1800001, 1800005, 36}
	case 29006: // Core
		return bossLootRule{1800002, 1800006, 36}
	case 29014: // Orfen
		return bossLootRule{1800003, 1800007, 36}
	case 29022: // Zaken
		return bossLootRule{1800004, 1800008, 36}
	case 29020, 29028: // Baium, Valakas
		return bossLootRule{1800009, 1800010, 36}
	case 29019: // Antharas
		return bossLootRule{1800009, 1800010, 225}
	}
	return bossLootRule{1800009, 1800010, 18}
}

// ccLootRights are a raid or grand boss's command channel loot rights:
// the first channel larger than the boss's rule requires to hit it holds
// them, and its leader takes the drops, until that channel goes
// ccRightsLapse without a hit. Hits arrive on the attackers' queues and the
// lapse check runs on the boss's; mu guards the state they share.
type ccLootRights struct {
	boss     *npc.Hostile
	channels LootChannels
	rule     bossLootRule

	mu      sync.Mutex
	holder  LootChannel
	lastHit time.Time
	// check polls for the lapse while a channel holds the rights; nil
	// while none does.
	check *sim.Ticker
}

func newCCLootRights(boss *npc.Hostile, npcID int, channels LootChannels) *ccLootRights {
	return &ccLootRights{boss: boss, channels: channels, rule: lootRule(npcID)}
}

// Hit implements npc.HitObserver: a hit by a member of a large enough
// channel, or by a summon of one, wins the rights when nobody holds them
// and refreshes them when that channel holds them.
func (r *ccLootRights) Hit(attacker attackable.Combatant) {
	p, ok := actingCharacter(attacker)
	if !ok {
		return
	}
	channel, ok := r.channels.LootChannel(p.ObjectID())
	if !ok || channel.MembersCount() <= r.rule.requiredMembers {
		return
	}
	queue := r.boss.Queue()
	now := queue.Now()
	r.mu.Lock()
	if r.check != nil {
		if r.holder == channel {
			r.lastHit = now
		}
		r.mu.Unlock()
		return
	}
	r.holder, r.lastHit = channel, now
	r.check = queue.Every(ccRightsCheck, r.expire)
	r.mu.Unlock()
	r.announce(r.rule.rightsMsg, channel.Leader().CharacterName())
}

// expire drops the rights once their channel has gone ccRightsLapse
// without a hit, and announces it.
func (r *ccLootRights) expire() {
	now := r.boss.Queue().Now()
	r.mu.Lock()
	if r.check == nil || now.Sub(r.lastHit) <= ccRightsLapse {
		r.mu.Unlock()
		return
	}
	r.check.Stop()
	r.check, r.holder, r.lastHit = nil, nil, time.Time{}
	r.mu.Unlock()
	r.announce(r.rule.noRightsMsg)
}

// holderLeader returns the leader of the channel holding the rights.
func (r *ccLootRights) holderLeader() (*player.Character, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	holder := r.holder
	r.mu.Unlock()
	if holder == nil {
		return nil, false
	}
	return holder.Leader(), true
}

// announce shows the boss's observers the NpcString id, its %s parameters
// filled in order.
func (r *ccLootRights) announce(id int32, params ...string) {
	text, ok := npcstring.Text(id)
	if !ok {
		return
	}
	for _, p := range params {
		text = strings.Replace(text, "%s", p, 1)
	}
	r.boss.BroadcastOnScreen(ccRightsScreenMs, text)
}
