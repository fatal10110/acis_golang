package zonepulse

import "github.com/fatal10110/acis_golang/internal/gameserver/model/zone"

// pulseEffects starts the pulses of their index and answers whether a
// castle's siege runs; every other zone effect does nothing.
type pulseEffects struct {
	zone.NoEffects
	pulses *Pulses
	siege  func() bool
}

func (f *pulseEffects) StartPulse(z zone.Kind) { f.pulses.Start(z) }

func (f *pulseEffects) CastleSiegeRunning(int) bool { return f.siege != nil && f.siege() }

// wire gives the zones of ix the pulses New builds from cfg; siege, when
// set, answers whether a castle's siege runs.
func wire(ix *zone.Index, siege func() bool, cfg Config) {
	ix.Wire(&pulseEffects{pulses: New(ix, cfg), siege: siege})
}
