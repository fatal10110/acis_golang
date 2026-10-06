package ai

import (
	"testing"
	"time"
)

// TestClaimTalkSocialSharesTheSocialClock pins the talk animation on the
// social desire's clock: a fresh NPC plays one, the next waits more than
// talkSocialInterval, a social desire's hold delays it until the interval
// has passed after the hold ends, and a talk animation does not hold desire
// selection.
func TestClaimTalkSocialSharesTheSocialClock(t *testing.T) {
	brain, _, _, _, now := fleeSocialAI(t)

	if !brain.ClaimTalkSocial() {
		t.Fatal("first talk animation refused")
	}
	if brain.selectionHeld() {
		t.Fatal("a talk animation holds desire selection")
	}
	*now = now.Add(talkSocialInterval)
	if brain.ClaimTalkSocial() {
		t.Fatal("talk animation exactly talkSocialInterval after the last one played")
	}
	*now = now.Add(time.Millisecond)
	if !brain.ClaimTalkSocial() {
		t.Fatal("talk animation past talkSocialInterval refused")
	}

	*now = now.Add(time.Minute)
	const hold = 30 * time.Second
	brain.AddSocialDesire(2, int(hold/time.Millisecond), 50)
	runAI(t, brain)
	if brain.ClaimTalkSocial() {
		t.Fatal("talk animation played inside the social desire's hold")
	}
	*now = now.Add(hold + talkSocialInterval)
	if brain.ClaimTalkSocial() {
		t.Fatal("talk animation played talkSocialInterval after the hold ended")
	}
	*now = now.Add(time.Millisecond)
	if !brain.ClaimTalkSocial() {
		t.Fatal("talk animation refused past talkSocialInterval after the hold")
	}
}
