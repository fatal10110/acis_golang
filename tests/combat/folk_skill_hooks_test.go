package combat

import (
	"fmt"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// TestFolkSkillDamageNamesTheSkillInItsHooks: a civilian NPC paying a
// skill's HP cost raises nothing; skill damage, a skill's damage-over-time
// tick and a lethal strike each run its attacked hooks and its clan calls
// naming the skill; a skill landing on it runs its attacked hooks only.
func TestFolkSkillDamageNamesTheSkillInItsHooks(t *testing.T) {
	t.Parallel()
	const caller, mate = int32(30051), int32(30052)
	log := newHookLog()
	srv := bootClanHooks(t, log, nil, []int32{caller, mate})
	startInWorld(t, srv.Client)
	p := liveCombatant(t, srv)
	log.name(p.ObjectID(), "p")
	dotCaster, ok := p.(effect.Actor)
	if !ok {
		t.Fatalf("player %T is not an effect actor", p)
	}

	f := srv.SpawnFolkNPCAt(t, clanTemplate(clanRole{id: caller, clan: "probe_clan", folk: true}), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	m := srv.SpawnFolkNPCAt(t, clanTemplate(clanRole{id: mate, clan: "probe_clan", folk: true}), location.Location{X: hostileX + 20, Y: hostileY, Z: hostileZ})
	same := srv.SpawnHostileNPCTemplateAt(t, clanTemplate(clanRoles["same"]), location.Location{X: hostileX + 40, Y: hostileY, Z: hostileZ})
	log.trackFolk("F", f)
	log.trackFolk("mate", m)
	log.track("same", same)

	before := f.HP()
	onAttackerQueue(t, srv, p.ObjectID(), func() { f.ConsumeHP(10) })
	if got, _ := log.take(); len(got) != 0 || f.HP() != before-10 {
		t.Fatalf("HP cost raised %q and left HP %v, want no hook and %v", got, f.HP(), before-10)
	}

	strike := skill.Definition{ID: 1177, Level: 1, Offensive: true}
	dot := skill.Ref{ID: 1178, Level: 2}
	lethal := skill.Definition{ID: 1179, Level: 1, Offensive: true}
	slow := skill.Definition{ID: 1160, Level: 1, Offensive: true, Debuff: true}
	var half int32
	onAttackerQueue(t, srv, p.ObjectID(), func() {
		f.ReduceHP(25, p, strike)
		f.ReduceHPBySkillDOT(7, dotCaster, dot)
		half = int32(f.HP() / 2)
		f.ApplyLethalOutcome(formulas.LethalHalf, p, lethal)
		f.SkillAttacked(p, slow)
	})
	got, _ := log.take()

	var want []string
	for _, hit := range []struct {
		damage int32
		skill  int32
	}{{25, 1177}, {7, 1178}, {half, 1179}} {
		want = append(want,
			fmt.Sprintf("ATTACKED npc=F attacker=p damage=%d skill=%d", hit.damage, hit.skill),
			fmt.Sprintf("CLAN_ATTACKED caller=F called=F attacker=p damage=%d skill=%d", hit.damage, hit.skill),
			fmt.Sprintf("CLAN_ATTACKED caller=F called=mate attacker=p damage=%d skill=%d", hit.damage, hit.skill),
			fmt.Sprintf("CLAN_ATTACKED caller=F called=same attacker=p damage=%d skill=%d", hit.damage, hit.skill),
		)
	}
	want = append(want, "ATTACKED npc=F attacker=p damage=120 skill=1160")
	if !sameLines(got, want) {
		t.Fatalf("hook calls =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}
