package pets

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	// spellSeerID is the monster whose behavior records the spells it sees.
	spellSeerID = int32(20081)
	// ownerSummonHeal is the owner's good skill on its own summon.
	ownerSummonHeal = 4111
	// ownerSelfBuff is the owner's good self buff.
	ownerSelfBuff = 4112
)

// spellSeenLog records the see-spell hooks of the behavior bound to
// spellSeerID, naming creatures by the names given to their object ids.
type spellSeenLog struct {
	mu    sync.Mutex
	names map[int32]string
	lines []string
}

func (l *spellSeenLog) name(id int32, name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.names[id] = name
}

func (l *spellSeenLog) see(_ *script.Script, e script.SeeSpell) {
	l.mu.Lock()
	defer l.mu.Unlock()
	targets := make([]string, len(e.Targets))
	for i, c := range e.Targets {
		targets[i] = l.names[c.ObjectID()]
	}
	l.lines = append(l.lines, fmt.Sprintf("%s sees %d caster=%s targets=[%s] pet=%v",
		l.names[e.NPC.ObjectID()], e.Skill.ID, l.names[e.Caster.ObjectID()], strings.Join(targets, " "), e.IsPet))
}

// expect waits for the lines of want, lets the server settle and compares.
func (l *spellSeenLog) expect(t *testing.T, srv *gameservertest.Server, what string, want ...string) {
	t.Helper()
	srv.AdvanceUntil(t, what, func() bool {
		l.mu.Lock()
		defer l.mu.Unlock()
		return len(l.lines) >= len(want)
	})
	srv.Settle(t)
	l.mu.Lock()
	got := l.lines
	l.lines = nil
	l.mu.Unlock()
	if !slices.Equal(got, want) {
		t.Fatalf("%s: hooks = %q, want %q", what, got, want)
	}
}

// bootSpellSeenWolf brings the owner in, knowing ownerSummonHeal and
// ownerSelfBuff, with a wolf that knows strike, a spell-seeing monster
// beside it and the fixture monster targeted.
func bootSpellSeenWolf(t *testing.T, strike modelskill.Definition) (*petWorld, *spellSeenLog) {
	t.Helper()
	log := &spellSeenLog{names: map[int32]string{}}
	wolf := wolfTemplate()
	wolf.Skills = map[int]int{wolfStrikeSkill: 1}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		strike,
		{
			ID: ownerSummonHeal, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSummon,
			CastRange: 600, SkillType: "HEAL", Power: 1, HitTime: 500, StaticHitTime: true, StaticReuse: true,
		},
		{
			ID: ownerSelfBuff, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true,
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "probe_self", StackOrder: 1}},
		},
	}), gamesql.NewCharacterSkillStore(db))
	srv := bootPets(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf, treeTemplate()})),
		gameservertest.WithSkills(skills),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{spellSeerID: script.KindHostile},
			[]script.Listing{{Path: "ai.SpellSeer"}},
			script.Catalog{"ai.SpellSeer": func() script.Script {
				return script.Script{Behavior: true, NPCs: []int32{spellSeerID}, Hooks: script.Hooks{OnSeeSpell: log.see}}
			}}),
	)
	ownerID := srv.SoleObjectID(t)
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	for _, id := range []int{ownerSummonHeal, ownerSelfBuff} {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, id, 1); err != nil {
			t.Fatalf("seed known skill %d: %v", id, err)
		}
	}
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	startInWorld(t, h.client)
	petActor, _ := h.spawnWolf(t)
	// The monster stands off the wolf, which spawns beside its owner.
	x, y, z := srv.PlayerPosition(t, ownerID)
	at := location.Location{X: x + 300, Y: y, Z: z}
	hostile := srv.SpawnHostileNPCAt(t, at)
	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.ID, tmpl.TemplateID = int(spellSeerID), int(spellSeerID)
	seer := srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: x - 200, Y: y, Z: z})
	log.name(ownerID, "Owner")
	log.name(petActor.ObjectID(), "Wolf")
	log.name(hostile.ObjectID(), "Monster")
	log.name(seer.ObjectID(), "Seer")
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(hostile.ObjectID(), int32(at.X), int32(at.Y), int32(at.Z), false))
	drainFrames(t, h.client)
	return h, log
}

// TestSummonSkillSeenAsItsOwners (CreatureCast.callSkill): a summon's
// skill is seen by the NPCs around the summon as its owner's, marked as a
// pet's, on the targets it landed on, or on none when its launch found
// none.
func TestSummonSkillSeenAsItsOwners(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		strike modelskill.Definition
		want   string
	}{
		{
			name:   "on its target",
			strike: wolfStrike(),
			want:   fmt.Sprintf("Seer sees %d caster=Owner targets=[Monster] pet=true", wolfStrikeSkill),
		},
		{
			name: "on no one",
			strike: modelskill.Definition{
				ID: wolfStrikeSkill, Level: 1, Activation: modelskill.ActivationActive,
				Target: modelskill.TargetAura, Radius: 10, CastRange: 900, Offensive: true, SkillType: "PDAM",
				HitTime: wolfStrikeHitTime, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, Power: 1,
			},
			want: fmt.Sprintf("Seer sees %d caster=Owner targets=[] pet=true", wolfStrikeSkill),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, log := bootSpellSeenWolf(t, tc.strike)
			startWolfStrike(t, h)
			log.expect(t, h.srv, "the wolf's skill", tc.want)
		})
	}
}

// TestOwnerSkillOnItsSummonUnseen (CreatureCast.callSkill): an owner's
// skill on its summon alone is seen by no NPC; its next skill, on itself,
// is.
func TestOwnerSkillOnItsSummonUnseen(t *testing.T) {
	t.Parallel()
	h, log := bootSpellSeenWolf(t, wolfStrike())

	h.client.Send(encodeRequestMagicSkillUse(ownerSummonHeal))
	readUntilOpcode(t, h.client, serverpackets.OpcodeMagicSkillLaunched, "summon heal MagicSkillLaunched")
	h.srv.Advance(t, 2*time.Second)
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeRequestMagicSkillUse(ownerSelfBuff))
	log.expect(t, h.srv, "the owner's skills", fmt.Sprintf("Seer sees %d caster=Owner targets=[Owner] pet=false", ownerSelfBuff))
}
