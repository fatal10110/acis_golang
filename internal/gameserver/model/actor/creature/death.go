package creature

// DeathActor is the identity a dying actor or its killer expose to the
// shared death sequence.
type DeathActor interface {
	ObjectID() int32
}

// Mortal is implemented by actor types that track their own dead state.
type Mortal interface {
	DeathActor

	// MarkDead flips the actor into its dead state. It reports false when
	// the actor was already dead, so a repeated or concurrent kill is a
	// no-op — every actor's death needs exactly one winner.
	MarkDead() bool
}

// Rewarder computes and applies kill rewards (drops, experience, karma) for
// a defeated actor. Concrete actor types own their reward logic — the drop
// table and experience/SP systems land separately — so a nil Rewarder makes
// the reward step a no-op rather than an error.
type Rewarder interface {
	CalculateRewards(killer DeathActor)
}

// Die runs the shared, once-only death sequence for actor: the idempotent
// dead-state transition, then the reward hook. It reports whether the
// death was newly applied by this call.
func Die(actor Mortal, killer DeathActor, rewards Rewarder) bool {
	if actor == nil || !actor.MarkDead() {
		return false
	}
	if rewards != nil {
		rewards.CalculateRewards(killer)
	}
	return true
}
