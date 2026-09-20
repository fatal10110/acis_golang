package network

import "testing"

func TestLivePlayerMarkDetaching(t *testing.T) {
	live := &livePlayer{}
	live.markDetaching()
	if !live.detached() {
		t.Fatal("delivery remains enabled after detachment")
	}
	live.shadowExpiryMu.RLock()
	detaching := live.detaching
	live.shadowExpiryMu.RUnlock()
	if !detaching {
		t.Fatal("detaching = false after markDetaching")
	}
}
