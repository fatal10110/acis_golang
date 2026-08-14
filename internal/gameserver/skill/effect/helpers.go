package effect

func abortAll(target participant) {
	if target, ok := target.(aborter); ok {
		target.AbortAll(false)
	}
}

func refresh(target participant) {
	if target, ok := target.(abnormalUpdater); ok {
		target.UpdateAbnormalEffect()
	}
}

func startAbnormalEffect(target participant, mask int) {
	if target, ok := target.(interface{ StartAbnormalEffect(int) }); ok {
		target.StartAbnormalEffect(mask)
	}
	refresh(target)
}

func stopAbnormalEffect(target participant, mask int) {
	if target, ok := target.(interface{ StopAbnormalEffect(int) }); ok {
		target.StopAbnormalEffect(mask)
	}
	refresh(target)
}

func fearImmune(target participant) bool {
	t, ok := target.(fearImmuneTarget)
	return ok && t.FearImmune()
}

func isAfraid(target participant) bool {
	t, ok := target.(afraidTarget)
	return ok && t.Afraid()
}

func isPlayable(target participant) bool {
	t, ok := target.(playableTarget)
	return ok && t.Playable()
}

func isPlayer(target participant) bool {
	t, ok := target.(playerTarget)
	return ok && t.IsPlayer()
}

// statFuncs builds the stat functions templates describes, attributed to
// owner. owner is opaque here (see basefunc.Func.Owner): a running buff
// passes itself, a passive skill passes an identity stable for as long as
// it stays learned.
