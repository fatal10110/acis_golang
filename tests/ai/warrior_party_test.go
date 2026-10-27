package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The party Warriors' NPC ids the scenarios spawn.
const (
	leaderID         = int32(27036) // PartyLeaderWarrior
	summonerLeaderID = int32(27062) // PartyLeaderWarrior
	shoutingLeaderID = int32(27065) // PartyLeaderWarrior
	partyPrivateID   = int32(27037) // PartyPrivateWarrior
)

// partyScripts are warriorScripts with the party Warriors, in scripts.xml
// order after Warrior.
func partyScripts() ([]script.Listing, script.Catalog) {
	list, catalog := warriorScripts()
	for path, ctor := range map[string]func() script.Script{
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderWarrior":   warrior.PartyLeader,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateWarrior": warrior.PartyPrivate,
	} {
		catalog[path] = ctor
	}
	at := slices.IndexFunc(list, func(l script.Listing) bool {
		return l.Path == "script.ai.individual.Monster.WarriorBase.Warrior.Warrior"
	}) + 1
	return slices.Insert(list, at,
		script.Listing{Path: "script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderWarrior"},
		script.Listing{Path: "script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateWarrior"},
	), catalog
}

// bootParty is bootWarriors with the party Warriors registered, and the
// npcmakers of makers spawned at boot (none for nil).
func bootParty(t *testing.T, makers *spawn.Table, templates ...*npc.Template) *warriorWorld {
	t.Helper()
	return bootPartyWith(t, makers, nil, templates...)
}

// bootPartyWith is bootParty with extra boot options, applied last.
func bootPartyWith(t *testing.T, makers *spawn.Table, extra []gameservertest.Option, templates ...*npc.Template) *warriorWorld {
	t.Helper()
	list, catalog := partyScripts()
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(makers),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(kinds, list, catalog),
	}, extra...)...)
	c := srv.Client
	c.Send(encodeGameStart())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("no CharSelected within 100 frames")
		}
		if c.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	srv.ReadQueued(t, c)
	id := srv.SoleObjectID(t)
	x, y, z := srv.PlayerPosition(t, id)
	obj, ok := srv.State.Object(id)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	return &warriorWorld{srv: srv, at: location.Location{X: x, Y: y, Z: z}, player: obj.(attackable.Combatant)}
}

// leaderTemplate is a fixture party leader of id with ai as its AI
// parameters, declaring privates of the fixture private, each respawning
// after 240 s.
func leaderTemplate(id int32, ai npc.AIParams, privates int) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	for range privates {
		tmpl.Privates = append(tmpl.Privates, npc.PrivateEntry{NpcID: int(partyPrivateID), Weight: 1, RespawnDelay: 240 * time.Second})
	}
	return tmpl
}

// privatesOf returns master's privates, sorted by object id.
func privatesOf(t *testing.T, master *npc.Hostile, want int) []*npc.Hostile {
	t.Helper()
	minions := master.Minions()
	if len(minions) != want {
		t.Fatalf("master %d privates = %d, want %d", master.NpcID(), len(minions), want)
	}
	privates := make([]*npc.Hostile, 0, len(minions))
	for _, m := range minions {
		privates = append(privates, m.(*npc.Hostile))
	}
	slices.SortFunc(privates, func(a, b *npc.Hostile) int { return int(a.ObjectID() - b.ObjectID()) })
	return privates
}

// kill makes h die, killed by nobody, on its queue: its party learns of it
// and nobody gains hate.
func (w *warriorWorld) kill(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.onQueue(t, h, func() { script.NPCOf(h).DoDie(nil) })
	w.srv.ReadQueued(t, w.srv.Client)
}

// armedPrivateRespawn returns the key of the one script private's slot
// with a respawn pending.
func (w *warriorWorld) armedPrivateRespawn(t *testing.T) string {
	t.Helper()
	var armed []string
	for i := 1; i <= 64; i++ {
		if key := fmt.Sprintf("private#%d", i); w.srv.NpcRespawns.Tracked(key) {
			armed = append(armed, key)
		}
	}
	if len(armed) != 1 {
		t.Fatalf("script privates with a respawn pending = %q, want one", armed)
	}
	return armed[0]
}

// TestPartyLeaderLeadsPrivatesUntilItDies runs a party leader with no
// SummonPrivateRate: it creates its two declared privates as it spawns,
// at weight point 1, with its own weight point 10. With nothing to do, each
// private follows its live master with weight 50, and the 20 s check
// leaves it in the world. Once the leader is dead its privates wander (5,
// weight 5), and the next 20 s check, finding them neither attacking nor
// casting, hides them: they leave the world, sending DeleteObject, and no
// respawn is armed for them.
func TestPartyLeaderLeadsPrivatesUntilItDies(t *testing.T) {
	t.Parallel()
	w := bootParty(t, nil, leaderTemplate(leaderID, nil, 2), warriorTemplate(partyPrivateID, nil))
	leader := w.spawn(t, leaderID, 400, 0)
	privates := privatesOf(t, leader, 2)
	if got := leader.Scratch().Int(npc.IntWeightPoint); got != 10 {
		t.Fatalf("leader weight point = %d, want 10", got)
	}
	for _, p := range privates {
		if p.NpcID() != int(partyPrivateID) || p.Master() != leader || p.Scratch().Int(npc.IntWeightPoint) != 1 {
			t.Fatalf("private %d: npc %d, master %p (want %p), weight point %d (want 1)", p.ObjectID(), p.NpcID(), p.Master(), leader, p.Scratch().Int(npc.IntWeightPoint))
		}
	}

	w.ticks(t, 2)
	for _, p := range privates {
		d, ok := desire(p, actorai.IntentionFollow)
		if !ok || d.Weight != 50 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != leader.ObjectID() {
			t.Fatalf("private %d follow desire = %+v (%v), want its leader with weight 50", p.ObjectID(), d, ok)
		}
	}
	w.srv.Advance(t, 20*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	for _, p := range privates {
		if !w.inWorld(p) {
			t.Fatalf("the 20 s check hid private %d of a live leader", p.ObjectID())
		}
	}

	w.kill(t, leader)
	for _, p := range privates {
		if p.Master() != nil {
			t.Fatalf("private %d still has its dead leader as master", p.ObjectID())
		}
		p.AI().Desires().Clear()
	}
	w.ticks(t, 2)
	for _, p := range privates {
		if d, ok := desire(p, actorai.IntentionWander); !ok || d.Weight != 5 || d.Timer != 5 {
			t.Fatalf("masterless private %d wander desire = %+v (%v), want timer 5, weight 5", p.ObjectID(), d, ok)
		}
	}
	w.srv.Advance(t, 20*time.Second)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	for _, p := range privates {
		if w.inWorld(p) {
			t.Fatalf("the 20 s check kept private %d of a dead leader", p.ObjectID())
		}
		if got := framesOf(frames, serverpackets.OpcodeDeleteObject, p.ObjectID()); len(got) != 1 {
			t.Fatalf("DeleteObject frames for hidden private %d = %d, want 1", p.ObjectID(), len(got))
		}
	}
	for i := 1; i <= 64; i++ {
		if key := fmt.Sprintf("private#%d", i); w.srv.NpcRespawns.Tracked(key) {
			t.Fatalf("hidden private slot %s armed a respawn", key)
		}
	}
}

// TestPartyLeaderSummonsPrivatesOnAHit runs a party leader with a
// SummonPrivateRate of 100: it spawns alone, and the first hit (the draw 0
// under 100) creates its private and makes it say the first of its summon
// lines (1000294); a second hit summons nothing more. The hit that summons
// reaches the leader's party: the leader, now a master, adds the party
// formula for itself, damage / (level + 7) * damage * its weight point 10
// * 10, on top of the Warrior formula, and the new private, which is no
// master, damage / (level + 7) * damage * its weight point 1 * 10.
func TestPartyLeaderSummonsPrivatesOnAHit(t *testing.T) {
	t.Parallel()
	w := bootParty(t, nil, leaderTemplate(summonerLeaderID, npc.AIParams{"SummonPrivateRate": "100"}, 1), warriorTemplate(partyPrivateID, nil))
	leader := w.spawn(t, summonerLeaderID, 400, 0)
	if got := len(leader.Minions()); got != 0 {
		t.Fatalf("summoning leader privates before any hit = %d, want none", got)
	}

	w.hit(t, leader, 10)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), leader.ObjectID()); !slices.Equal(got, []string{script.NpcString(1000294)}) {
		t.Fatalf("leader lines on the summoning hit = %q, want the 1000294 line", got)
	}
	private := privatesOf(t, leader, 1)[0]
	// Warrior: 10 / 8 * 100; party, leader for itself: int(10 / 8 * 10 * 10 * 10).
	if got := leader.AggroHate(w.player); got != 125+1250 {
		t.Fatalf("leader hate after the summoning hit = %v, want 1375", got)
	}
	// Party, for the private: int(10 / 8 * 10 * 1 * 10).
	if got := private.AggroHate(w.player); got != 125 {
		t.Fatalf("new private hate after the summoning hit = %v, want 125", got)
	}

	w.hit(t, leader, 10)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), leader.ObjectID()); len(got) != 0 {
		t.Fatalf("leader lines on the second hit = %q, want none", got)
	}
	privatesOf(t, leader, 1)
}

// TestPartyPrivateHatesForItsParty pins the party Warriors' hate formulas,
// for a hit of 10 on a private of a level 1 party: the private adds its
// attacked formula, damage / 8 * damage * weight point 1 * 10, then the
// Warrior one, damage / 8 * 100, and its own party call the same as its
// attacked formula; the leader adds its party call, weighted by the
// caller's weight point 1, and the other private its party call. The
// leader's death clears its privates' master, as the reference's does, so
// a private's dead-master formula never applies: a hit on a masterless
// private adds the same two attacked formulas, and reaches no party.
func TestPartyPrivateHatesForItsParty(t *testing.T) {
	t.Parallel()
	w := bootParty(t, nil, leaderTemplate(leaderID, nil, 2), warriorTemplate(partyPrivateID, nil))
	leader := w.spawn(t, leaderID, 400, 0)
	privates := privatesOf(t, leader, 2)
	victim, other := privates[0], privates[1]

	w.hit(t, victim, 10)
	for _, row := range []struct {
		what string
		h    *npc.Hostile
		want float64
	}{
		{"hit private", victim, 125 + 125 + 125},
		{"leader", leader, 125},
		{"other private", other, 125},
	} {
		if got := row.h.AggroHate(w.player); got != row.want {
			t.Errorf("%s hate after a hit of 10 = %v, want %v", row.what, got, row.want)
		}
	}

	w.kill(t, leader)
	before := other.AggroHate(w.player)
	w.hit(t, other, 10)
	if got := other.AggroHate(w.player) - before; got != 125+125 {
		t.Fatalf("masterless private hate gained from a hit of 10 = %v, want 250", got)
	}
	if got := victim.AggroHate(w.player); got != 375 {
		t.Fatalf("the other masterless private's hate = %v, want 375: the hit reached no party", got)
	}
}

// TestPartyLeaderShoutsTargetToPrivates runs a ShoutTarget leader, every
// draw 0: a hit by a player under 40% of its HP whom the leader already
// hates most makes the leader name the player in the first of its target
// lines (1000291), want to attack the player with weight 1000 only, keep
// the player's id as its flag and send event 10002 to the NPCs within 300.
// Its private, hating nobody, says the first of its lines (1000292) and
// wants to attack the flagged player with weight 2000. The first hit, which
// makes the player the most hated, shouts nothing.
func TestPartyLeaderShoutsTargetToPrivates(t *testing.T) {
	t.Parallel()
	w := bootParty(t, nil, leaderTemplate(shoutingLeaderID, npc.AIParams{"ShoutTarget": "1"}, 1), warriorTemplate(partyPrivateID, nil))
	leader := w.spawn(t, shoutingLeaderID, 400, 0)
	private := privatesOf(t, leader, 1)[0]

	w.hit(t, leader, 10)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), leader.ObjectID()); len(got) != 0 {
		t.Fatalf("leader lines on the first hit = %q, want none", got)
	}
	w.onQueue(t, private, func() { script.NPCOf(private).RemoveAllAttackDesire() })
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		c := w.player.(player.CharacterHolder).PlayerCharacter()
		c.SetHP(c.MaxHPValue() * 0.3)
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)

	w.hit(t, leader, 1)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got := says(frames, leader.ObjectID()); !slices.Equal(got, []string{script.NpcString(1000291, "Hunter")}) {
		t.Fatalf("leader lines on the shouting hit = %q, want the 1000291 line naming Hunter", got)
	}
	if got := says(frames, private.ObjectID()); !slices.Equal(got, []string{script.NpcString(1000292)}) {
		t.Fatalf("private lines on the leader's shout = %q, want the 1000292 line", got)
	}
	if got := leader.Scratch().Int(npc.IntFlag); got != w.player.ObjectID() {
		t.Fatalf("leader flag = %d, want the player's id %d", got, w.player.ObjectID())
	}
	// The leader's hate restarts at 1000; the Warrior and party formulas of
	// a hit of 1 add 12.5 and 12.
	if got := leader.AggroHate(w.player); got != 1000+12.5+12 {
		t.Fatalf("leader hate after the shout = %v, want 1024.5", got)
	}
	// The private's: 2000, then its party call, int(1 / 8 * 1 * 1 * 10).
	if got := private.AggroHate(w.player); got != 2000+1 {
		t.Fatalf("private hate after the shout = %v, want 2001", got)
	}
}

// TestPartyLeaderRespawnsADeadPrivate runs the leader's party-died hook: a
// private's death, with the leader alive, asks its spawn to bring it back
// after its declared delay (the delay itself is pinned by the population's
// own test). Once its corpse has decayed, the respawn brings a new private
// back around the leader, its master, at weight point 1, following it.
func TestPartyLeaderRespawnsADeadPrivate(t *testing.T) {
	t.Parallel()
	w := bootParty(t, nil, leaderTemplate(leaderID, nil, 1), warriorTemplate(partyPrivateID, nil))
	leader := w.spawn(t, leaderID, 400, 0)
	private := privatesOf(t, leader, 1)[0]

	w.kill(t, private)
	key := w.armedPrivateRespawn(t)
	w.onQueue(t, private, func() { private.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(private.ObjectID())) })
	w.srv.NpcSpawns.Respawn(key)
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)

	back := privatesOf(t, leader, 1)[0]
	if back.ObjectID() == private.ObjectID() || !w.inWorld(back) {
		t.Fatalf("leader's private after the respawn = %d (was %d), want a new one in the world", back.ObjectID(), private.ObjectID())
	}
	if back.Master() != leader || back.Scratch().Int(npc.IntWeightPoint) != 1 {
		t.Fatalf("respawned private: master %p (want %p), weight point %d (want 1)", back.Master(), leader, back.Scratch().Int(npc.IntWeightPoint))
	}
	w.ticks(t, 2)
	if d, ok := desire(back, actorai.IntentionFollow); !ok || d.Weight != 50 {
		t.Fatalf("respawned private follow desire = %+v (%v), want weight 50", d, ok)
	}
}

// partyDen is an npcmaker spawning one Party_Type 2 party leader that
// declares two privates on its spawn entry.
const partyDen = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="den" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="party_den" territory="den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="27036" total="1" pos="40;20;30;0" respawn="60sec">
			<privates>
				<private id="27037" weight="1" respawn="240sec"/>
				<private id="27037" weight="1" respawn="240sec"/>
			</privates>
		</npc>
	</npcmaker>
</list>`

// TestPartyLeaderSpawnsOnlyItsBehaviorsPrivates pins the spawn-list
// privates: a Party_Type 2 leader with a bound behavior gets exactly the two
// privates its behavior creates from its spawn entry; the spawn itself
// creates none.
func TestPartyLeaderSpawnsOnlyItsBehaviorsPrivates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(partyDen), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	w := bootParty(t, table, leaderTemplate(leaderID, npc.AIParams{"Party_Type": "2"}, 0), warriorTemplate(partyPrivateID, nil))
	leaders := w.liveNPCs(leaderID)
	if len(leaders) != 1 {
		t.Fatalf("den leaders = %d, want 1", len(leaders))
	}
	privatesOf(t, leaders[0], 2)
	if got := len(w.liveNPCs(partyPrivateID)); got != 2 {
		t.Fatalf("privates in the world = %d, want the 2 the behavior created", got)
	}
}
