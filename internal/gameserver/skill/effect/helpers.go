package effect

func abortAll(target Participant) {
	if target, ok := target.(aborter); ok {
		target.AbortAll(false)
	}
}

func refresh(target Participant) {
	if target, ok := target.(abnormalUpdater); ok {
		target.UpdateAbnormalEffect()
	}
}

func startAbnormalEffect(target Participant, mask int) {
	if target, ok := target.(interface{ StartAbnormalEffect(int) }); ok {
		target.StartAbnormalEffect(mask)
	}
	refresh(target)
	if b, ok := target.(abnormalEffectBroadcaster); ok {
		b.BroadcastAbnormalEffect()
	}
}

func stopAbnormalEffect(target Participant, mask int) {
	if target, ok := target.(interface{ StopAbnormalEffect(int) }); ok {
		target.StopAbnormalEffect(mask)
	}
	refresh(target)
	if b, ok := target.(abnormalEffectBroadcaster); ok {
		b.BroadcastAbnormalEffect()
	}
}

func fearImmune(target Participant) bool {
	t, ok := target.(fearImmuneTarget)
	return ok && t.FearImmune()
}

func isAfraid(target Participant) bool {
	t, ok := target.(afraidTarget)
	return ok && t.Afraid()
}

func isPlayable(target Participant) bool {
	t, ok := target.(playableTarget)
	return ok && t.Playable()
}

func isPlayer(target Participant) bool {
	t, ok := target.(playerTarget)
	return ok && t.IsPlayer()
}

// statFuncs builds the stat functions templates describes, attributed to
// owner. owner identifies whatever attached the Mod (see ModOwner): a running buff
// passes itself, a passive skill passes an identity stable for as long as
// it stays learned.
