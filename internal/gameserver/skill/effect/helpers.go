package effect

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor"

// refresh pushes target's abnormal-effect state to observers; a nil target
// (an effect with no source) is left alone.
func refresh(target Actor) {
	if target != nil {
		target.UpdateAbnormalEffect()
	}
}

func startAbnormalEffect(target Actor, mask int) {
	target.StartAbnormalEffect(mask)
	target.UpdateAbnormalEffect()
	if p, ok := asPlayer(target); ok {
		p.BroadcastAbnormalEffect()
	}
}

func stopAbnormalEffect(target Actor, mask int) {
	target.StopAbnormalEffect(mask)
	target.UpdateAbnormalEffect()
	if p, ok := asPlayer(target); ok {
		p.BroadcastAbnormalEffect()
	}
}

func isPlayable(target Actor) bool {
	return target != nil && target.Kind().Playable()
}

func isPlayer(target Actor) bool {
	return target != nil && target.Kind() == actor.KindPlayer
}

// statFuncs builds the stat functions templates describes, attributed to
// owner. owner identifies whatever attached the Mod (see ModOwner): a running buff
// passes itself, a passive skill passes an identity stable for as long as
// it stays learned.
