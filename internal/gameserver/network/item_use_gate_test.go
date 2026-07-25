package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func TestSendUseConditionFailureUsesTextMessage(t *testing.T) {
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)

	sendUseConditionFailure(live, &item.Template{ID: 6611}, item.UseCondition{Message: "Only heroes may use this item."})

	if len(frames.frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(frames.frames))
	}
	assertSystemMessageStringFrame(t, frames.frames[0], 1987, "Only heroes may use this item.")
}

func TestPlayerUseConditionHoldsUsesHeroState(t *testing.T) {
	live := newTestLivePlayer(t, 1, &frameCapture{})
	heroOnly := item.Condition{Kind: "player", Attrs: map[string]string{"isHero": "true"}}
	nonHeroOnly := item.Condition{Kind: "player", Attrs: map[string]string{"isHero": "false"}}

	if itemUseConditionHolds(live, heroOnly) {
		t.Fatal("hero-only condition passed for a non-hero")
	}
	if !itemUseConditionHolds(live, nonHeroOnly) {
		t.Fatal("non-hero condition failed for a non-hero")
	}

	live.SetHero(true)

	if !itemUseConditionHolds(live, heroOnly) {
		t.Fatal("hero-only condition failed for a hero")
	}
	if itemUseConditionHolds(live, nonHeroOnly) {
		t.Fatal("non-hero condition passed for a hero")
	}
}
