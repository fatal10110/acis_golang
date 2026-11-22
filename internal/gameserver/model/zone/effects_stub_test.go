package zone

// fxStub carries out the effects a test watches through its set funcs;
// every other effect does nothing.
type fxStub struct {
	NoEffects
	arenaNotice   func(a Actor, entering bool)
	siegeNotice   func(a Actor, entering bool)
	siegeUnsummon func(a Actor)
	startPulse    func(z Kind)
	fighting      func() bool
	siegeRunning  func() bool
}

func (f *fxStub) ArenaNotice(_ *Arena, a Actor, entering bool) {
	if f.arenaNotice != nil {
		f.arenaNotice(a, entering)
	}
}

func (f *fxStub) SiegeNotice(_ *Siege, a Actor, entering bool) {
	if f.siegeNotice != nil {
		f.siegeNotice(a, entering)
	}
}

func (f *fxStub) SiegeUnsummon(_ *Siege, a Actor) {
	if f.siegeUnsummon != nil {
		f.siegeUnsummon(a)
	}
}

func (f *fxStub) StartPulse(z Kind) {
	if f.startPulse != nil {
		f.startPulse(z)
	}
}

func (f *fxStub) StadiumFighting(*Olympiad) bool { return f.fighting != nil && f.fighting() }

func (f *fxStub) CastleSiegeRunning(int) bool { return f.siegeRunning != nil && f.siegeRunning() }

// wire gives zones ks the effects fx, as Index.Wire does for an index.
func wire(fx Effects, ks ...Kind) {
	for _, k := range ks {
		k.Core().fx = fx
	}
}
