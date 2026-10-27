package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// newBareGameClientLink builds a link through NewGameClientLink from cfg,
// filling in the queues and effect activity the constructor requires when
// cfg leaves them out.
func newBareGameClientLink(t *testing.T, cfg GameClientLinkConfig) *GameClientLink {
	t.Helper()
	if cfg.Queues == nil {
		cfg.Queues = testQueues(t)
	}
	if cfg.Effects.Activity == nil {
		cfg.Effects.Activity = task.NewEffects()
	}
	link, err := NewGameClientLink(cfg)
	if err != nil {
		t.Fatalf("new game client link: %v", err)
	}
	return link
}
