package skills

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	npcClanHealSkill = modelskill.ID(9103)
	npcClanHealPower = 40
)

func clanTemplate(id int, clans ...string) *npc.Template {
	return &npc.Template{
		ID: id, TemplateID: id, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		Clans: clans,
	}
}

// TestNPCClanSkillCoversOnlyClanmates has a monster cast a CLAN-targeted
// heal among three neighbors: one sharing a clan tag, one tagged with a
// different clan and one with none. The affected set is the caster followed
// by its clanmate, and only the clanmate is healed.
func TestNPCClanSkillCoversOnlyClanmates(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	startInWorld(t, srv.Client)
	defs := modelskill.NewTable([]modelskill.Definition{{
		ID: npcClanHealSkill, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetClan, Radius: 500, SkillType: "HEAL", Power: npcClanHealPower,
		StaticHitTime: true, StaticReuse: true,
	}})
	caster, aiCtl := srv.SpawnCastingHostileNPC(t, clanTemplate(100, "orc_clan", "wolf_clan"), defs)
	clanmate := srv.SpawnHostileNPCTemplateAt(t, clanTemplate(101, "wolf_clan"), location.Location{X: 160, Y: 20, Z: 30})
	outsider := srv.SpawnHostileNPCTemplateAt(t, clanTemplate(102, "elf_clan"), location.Location{X: 60, Y: 120, Z: 30})
	clanless := srv.SpawnHostileNPCTemplateAt(t, clanTemplate(103), location.Location{X: -40, Y: 20, Z: 30})
	// Every neighbor starts 100 HP short of its max, leaving room to heal.
	hurt := clanmate.MaxHP() - 100
	for _, h := range []*npc.Hostile{clanmate, outsider, clanless} {
		onNPCQueue(t, h, func() { h.SetCurrentHP(hurt) })
	}
	drainUntilQuiet(t, srv.Client)

	if !caster.Queue().Post(func() { aiCtl.Cast(caster, modelskill.Ref{ID: npcClanHealSkill, Level: 1}) }) {
		t.Fatal("post npc cast: queue closed")
	}
	frame := readUntil(t, srv.Client, serverpackets.OpcodeMagicSkillLaunched)
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // caster
	r.ReadInt32() // skill id
	r.ReadInt32() // level
	got := make([]int32, r.ReadInt32())
	for i := range got {
		got[i] = r.ReadInt32()
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read MagicSkillLaunched: %v", err)
	}
	if want := []int32{caster.ObjectID(), clanmate.ObjectID()}; !slices.Equal(got, want) {
		t.Fatalf("clan heal targets = %v, want caster then clanmate %v", got, want)
	}

	srv.AdvanceUntil(t, "clanmate healed", func() bool { return clanmate.CurrentHP() > hurt })
	if hp := clanmate.CurrentHP(); hp != hurt+npcClanHealPower {
		t.Fatalf("clanmate HP = %d, want %d", hp, hurt+npcClanHealPower)
	}
	for _, h := range []*npc.Hostile{outsider, clanless} {
		if hp := h.CurrentHP(); hp != hurt {
			t.Fatalf("non-clan NPC %d HP = %d, want untouched %d", h.ObjectID(), hp, hurt)
		}
	}
	drainUntilQuiet(t, srv.Client)
}
