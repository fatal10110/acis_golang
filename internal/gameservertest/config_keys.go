package gameservertest

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
)

// configKeyOptions are the server.properties, players.properties and
// npcs.properties switches set by the options below.
type configKeyOptions struct {
	dropItemDelay         time.Duration
	warehouseDisabled     bool
	noSpawns              bool
	fakeDeathUpProtection *time.Duration
	showSummonCrest       bool
}

// WithDropItemDelay sets the reuse delay between two item drops on one
// client session (DropItemTime); the default 0 never gates them.
func WithDropItemDelay(d time.Duration) Option {
	return func(o *options) { o.configKeys.dropItemDelay = d }
}

// WithWarehouseDisabled turns server.properties AllowWarehouse off: a
// warehouse deposit request is ignored.
func WithWarehouseDisabled() Option {
	return func(o *options) { o.configKeys.warehouseDisabled = true }
}

// WithNoSpawns turns server.properties NoSpawns on: the WithNpcSpawns
// population is built but its spawn pass spawns nothing.
func WithNoSpawns() Option {
	return func(o *options) { o.configKeys.noSpawns = true }
}

// WithFakeDeathUpProtection sets how long, after a Fake Death ends, hostile
// NPCs do not aggro the player (PlayerFakeDeathUpProtection); the default
// is the shipped 5 seconds.
func WithFakeDeathUpProtection(d time.Duration) Option {
	return func(o *options) { o.configKeys.fakeDeathUpProtection = &d }
}

// WithShowSummonCrest turns npcs.properties ShowSummonCrest on: a summon's
// NpcInfo shows its owner's clan and alliance crests, and its owner is sent
// that NpcInfo too as it spawns and lands from a teleport.
func WithShowSummonCrest() Option {
	return func(o *options) { o.configKeys.showSummonCrest = true }
}

// apply puts the switches into the link's configuration.
func (c configKeyOptions) apply(cfg *network.GameClientLinkConfig) {
	cfg.PlayerConfig.DropItemDelay = c.dropItemDelay
	cfg.PlayerConfig.WarehouseDisabled = c.warehouseDisabled
	cfg.PlayerConfig.FakeDeathUpProtection = player.DefaultFakeDeathUpProtection
	if c.fakeDeathUpProtection != nil {
		cfg.PlayerConfig.FakeDeathUpProtection = *c.fakeDeathUpProtection
	}
	cfg.ShowSummonCrest = c.showSummonCrest
}
