package cast

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestPlayerActorResourcesAndInventory(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 57, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	ch := &player.Character{ID: 1, CurHP: 12, CurMP: 7}
	inv := itemcontainer.NewPlayerInventory(ch.ID, templates)
	inv.AddNew(57, 5, 100)
	ch.AttachRuntime(&player.Template{}, inv)

	actor := PlayerActor{Character: ch}
	actor.ReduceMP(9)
	actor.ReduceHP(20)

	if ch.CurMP != 0 || ch.CurHP != 0 {
		t.Fatalf("resources = hp %.0f mp %.0f, want both clamped to 0", ch.CurHP, ch.CurMP)
	}
	if got := actor.ItemCount(57); got != 5 {
		t.Fatalf("ItemCount() = %d, want 5", got)
	}
	if !actor.ConsumeItem(57, 3) {
		t.Fatalf("ConsumeItem() = false, want true")
	}
	if got := actor.ItemCount(57); got != 2 {
		t.Fatalf("ItemCount() after consume = %d, want 2", got)
	}
}

func TestPlayerActorSkillReuseDelegatesToCharacter(t *testing.T) {
	ch := &player.Character{}
	actor := PlayerActor{Character: ch}
	ref := modelskill.Ref{ID: 10, Level: 2}
	key := int32(10*256 + 2)

	actor.AddSkillReuse(ref, key, time.Minute)

	if !actor.SkillDisabled(key) {
		t.Fatalf("SkillDisabled() = false, want true")
	}
}
