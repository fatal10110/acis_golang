package effect

func abortAll(target any) {
	if target, ok := target.(aborter); ok {
		target.AbortAll(false)
	}
}

func refresh(target any) {
	if target, ok := target.(abnormalUpdater); ok {
		target.UpdateAbnormalEffect()
	}
}

func fearImmune(target any) bool {
	t, ok := target.(fearImmuneTarget)
	return ok && t.FearImmune()
}

func isAfraid(target any) bool {
	t, ok := target.(afraidTarget)
	return ok && t.Afraid()
}

func isPlayable(target any) bool {
	t, ok := target.(playableTarget)
	return ok && t.Playable()
}

func isPlayer(target any) bool {
	t, ok := target.(playerTarget)
	return ok && t.IsPlayer()
}

// statFuncs builds the stat functions templates describes, attributed to
// owner. owner is opaque here (see basefunc.Func.Owner): a running buff
// passes itself, a passive skill passes an identity stable for as long as
// it stays learned.
