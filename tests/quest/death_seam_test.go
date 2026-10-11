package quest

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	quest001 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q001"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// The death seam's probes: two quests with a death hook and Letters of
// Love, which has none. Journal order is name order at load, so Q001 sits
// between the two probes.
const (
	deathProbeA     = "Q962_DeathProbeA"
	deathProbeB     = "Q963_DeathProbeB"
	deathProbeMob   = 27901
	deathServitorID = 12564
)

// deathProbe is a real quest whose death hook plays, for its player, the
// sound death_<tag>_<killer object id>_<player object id>.
func deathProbe(id int32, tag string) func() script.Script {
	return func() script.Script {
		return script.Script{QuestID: id, Hooks: script.Hooks{OnDeath: func(s *script.Script, e script.Death) {
			s.PlaySound(e.Player, fmt.Sprintf("death_%s_%d_%d", tag, e.Killer.ObjectID(), e.Player.ObjectID()))
		}}}
	}
}

// deathWorld boots a character whose journal holds probe A completed,
// Letters of Love started and probe B created, with a servitor template
// and a monster to kill it.
func deathWorld(t *testing.T) *dialogWorld {
	t.Helper()
	return bootSeamQuest(t, 20, nil, func(srv *gameservertest.Server, objID int32) {
		seedJournal(t, srv, objID, []string{deathProbeA + ":COMPLETED", q001 + ":STARTED", deathProbeB + ":CREATED"})
	},
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(deathProbeMob), deathServitorTemplate()})),
		gameservertest.WithNPCScripts(nil,
			[]script.Listing{{Path: "quest." + q001}, {Path: "quest." + deathProbeA}, {Path: "quest." + deathProbeB}},
			script.Catalog{
				"quest." + q001:        quest001.New,
				"quest." + deathProbeA: deathProbe(962, "a"),
				"quest." + deathProbeB: deathProbe(963, "b"),
			},
		),
	)
}

func deathServitorTemplate() *npc.Template {
	return &npc.Template{
		ID: deathServitorID, TemplateID: deathServitorID, Type: "Servitor", Name: "Sin Eater", Level: 20,
		HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
}

// summonServitor summons the player's servitor and returns it once it is
// in the world.
func (w *dialogWorld) summonServitor(t *testing.T) *summon.Actor {
	t.Helper()
	w.onCharacter(t, func(c *player.Character) {
		c.SummonServitor(modelskill.Definition{ID: 1111, Level: 1, SkillType: "SUMMON", NpcID: deathServitorID, SummonTotalLifeTime: 1_200_000})
	})
	var servitor *summon.Actor
	w.srv.AdvanceUntil(t, "the servitor is in the world", func() bool {
		obj, ok := w.srv.State.Summon(w.player)
		if ok {
			servitor, ok = obj.(*summon.Actor)
		}
		return ok
	})
	w.srv.ReadQueued(t, w.srv.Client)
	return servitor
}

// onPlayerQueue runs fn on the player objID's queue and waits for it.
func onPlayerQueue(t *testing.T, srv *gameservertest.Server, objID int32, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !srv.PlayerQueue(t, objID).Post(func() {
		defer close(done)
		fn()
	}) {
		t.Fatal("the player's queue is closed")
	}
	<-done
}

// onlineCharacter returns the online player objID's character.
func onlineCharacter(t *testing.T, srv *gameservertest.Server, objID int32) *player.Character {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("player %d is not in the world", objID)
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %d (%T) is not an online character", objID, obj)
	}
	return c
}

// deathTrace renders frames as their opcode names, a sound as its file:
// "Die", "sound <file>", "CharInfo", "RelationChanged"; other frames are
// left out.
func deathTrace(t *testing.T, frames [][]byte) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeDie:
			out = append(out, "Die")
		case serverpackets.OpcodeCharInfo:
			out = append(out, "CharInfo")
		case serverpackets.OpcodeRelationChanged:
			out = append(out, "RelationChanged")
		case serverpackets.OpcodePlaySound:
			line, err := scriptcontract.Packet(f, nil)
			if err != nil {
				t.Fatalf("frame %x: %v", f, err)
			}
			_, rest, _ := strings.Cut(line, "file=")
			file, _, _ := strings.Cut(rest, " ")
			out = append(out, "sound "+file)
		}
	}
	return out
}

// probeSounds are the sounds the two probes play, in journal order, for a
// death of the player player caused by killer.
func probeSounds(killer, player int32) []string {
	return []string{
		fmt.Sprintf("sound death_a_%d_%d", killer, player),
		fmt.Sprintf("sound death_b_%d_%d", killer, player),
	}
}

// TestPlayerDeathReachesDeathTriggeredQuests: a monster's kill of the
// player runs the death hook of each quest of the journal that has one,
// once, in journal order and whatever the quest's status (completed,
// created), after the Die broadcast; Letters of Love, with no death hook,
// is passed over. A death no creature caused passes the player as its
// own killer (Playable.doDie).
func TestPlayerDeathReachesDeathTriggeredQuests(t *testing.T) {
	t.Parallel()
	t.Run("monster", func(t *testing.T) {
		t.Parallel()
		w := deathWorld(t)
		mob := w.spawnHostile(t, "mob", deathProbeMob, "Monster", 40)
		c := onlineCharacter(t, w.srv, w.player)
		onPlayerQueue(t, w.srv, w.player, func() { c.TakeDamage(1_000_000, mob) })
		got := deathTrace(t, w.srv.ReadQueued(t, w.srv.Client))
		want := append([]string{"Die"}, probeSounds(mob.ObjectID(), w.player)...)
		if !slices.Equal(got, want) {
			t.Fatalf("frames of the death = %q, want %q", got, want)
		}
	})
	t.Run("no killer", func(t *testing.T) {
		t.Parallel()
		w := deathWorld(t)
		c := onlineCharacter(t, w.srv, w.player)
		onPlayerQueue(t, w.srv, w.player, func() { c.Die(nil) })
		got := deathTrace(t, w.srv.ReadQueued(t, w.srv.Client))
		want := append([]string{"Die"}, probeSounds(w.player, w.player)...)
		if !slices.Equal(got, want) {
			t.Fatalf("frames of the death = %q, want %q", got, want)
		}
	})
}

// TestSummonDeathReachesItsOwnersQuests: the death of the player's
// servitor reaches the death hooks of the owner's quests as the owner's
// own death does, after the owner sees the servitor die, with the
// servitor's killer, or the servitor itself when no creature caused the
// death.
func TestSummonDeathReachesItsOwnersQuests(t *testing.T) {
	t.Parallel()
	t.Run("monster", func(t *testing.T) {
		t.Parallel()
		w := deathWorld(t)
		mob := w.spawnHostile(t, "mob", deathProbeMob, "Monster", 40)
		servitor := w.summonServitor(t)
		onPlayerQueue(t, w.srv, w.player, func() { servitor.TakeDamage(1_000_000, mob) })
		got := deathTrace(t, w.srv.ReadQueued(t, w.srv.Client))
		if want := append([]string{"Die"}, probeSounds(mob.ObjectID(), w.player)...); !slices.Equal(got, want) {
			t.Fatalf("owner frames of the servitor's death = %q, want %q", got, want)
		}
	})
	t.Run("no killer", func(t *testing.T) {
		t.Parallel()
		w := deathWorld(t)
		servitor := w.summonServitor(t)
		onPlayerQueue(t, w.srv, w.player, func() { servitor.Kill(nil) })
		got := deathTrace(t, w.srv.ReadQueued(t, w.srv.Client))
		if want := append([]string{"Die"}, probeSounds(servitor.ObjectID(), w.player)...); !slices.Equal(got, want) {
			t.Fatalf("owner frames of the servitor's death = %q, want %q", got, want)
		}
	})
}

// enterSecondPlayer brings a second character, account and name name,
// into the world beside the first and returns its object id and client.
func enterSecondPlayer(t *testing.T, srv *gameservertest.Server, name string) (int32, *testsupport.ScriptedClient) {
	t.Helper()
	id := srv.SeedCharacterFor(t, strings.ToLower(name), name, 20, 0).ID
	c := srv.DialClient(t, strings.ToLower(name), 1)
	c.Send(encodeRequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	srv.ReadQueued(t, c)
	srv.ReadQueued(t, srv.Client)
	return id, c
}

// TestPlayerKilledDeathHooksRunBeforeTheKillersKarma: when a player kills
// the character's servitor and then the character, each death's hooks run
// before the killer's karma for it is shown: the character sees the
// probes' sounds ahead of the killer's new appearance and relation.
func TestPlayerKilledDeathHooksRunBeforeTheKillersKarma(t *testing.T) {
	t.Parallel()
	w := deathWorld(t)
	killerID, killerClient := enterSecondPlayer(t, w.srv, "Killer")
	killer := playerCombatant(t, w.srv, killerID)
	servitor := w.summonServitor(t)
	w.srv.ReadQueued(t, killerClient)

	for _, tc := range []struct {
		name string
		kill func()
		want []string
	}{
		{"servitor", func() { servitor.TakeDamage(1_000_000, killer) }, []string{"Die"}},
		{"player", func() { onlineCharacter(t, w.srv, w.player).TakeDamage(1_000_000, killer) }, []string{"Die"}},
	} {
		onPlayerQueue(t, w.srv, w.player, tc.kill)
		got := deathTrace(t, w.srv.ReadQueued(t, w.srv.Client))
		want := append(tc.want, probeSounds(killerID, w.player)...)
		if len(got) <= len(want) || !slices.Equal(got[:len(want)], want) {
			t.Fatalf("%s death: frames %q, want %q then the killer's karma", tc.name, got, want)
		}
		for _, rest := range got[len(want):] {
			if rest != "CharInfo" && rest != "RelationChanged" {
				t.Fatalf("%s death: frames %q, want only the killer's karma after the sounds", tc.name, got)
			}
		}
		if karma := onlineCharacter(t, w.srv, killerID).Karma(); karma == 0 {
			t.Fatalf("%s death: killer karma 0, want a PK gain", tc.name)
		}
	}
}
