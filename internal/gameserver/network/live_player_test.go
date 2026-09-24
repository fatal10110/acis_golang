package network

import "testing"

func TestLivePlayerMarkDetaching(t *testing.T) {
	live := &livePlayer{}
	live.markDetaching()
	if !live.detached() {
		t.Fatal("delivery remains enabled after detachment")
	}
}
