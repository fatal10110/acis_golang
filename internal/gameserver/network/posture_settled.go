package network

// settleLivePosture answers the end of a sit-down or stand-up transition,
// stoodUp naming a stand-up's end, a get-up out of fake death included.
//
// A stand-up's end frees the throne whatever the posture is by then: a
// get-up that ends after the player sat down again still releases the
// throne that later sit claimed.
//
// The intention queued behind the transition then runs at once, even while
// another transition is still under way: a fake-death get-up running beside
// a lie-down, or a lie-down or sit begun after a get-up that has yet to end.
// The queued intention runs as it would on a fresh request, its own gates
// deciding the outcome, so a stand queued during a lie-down that ends while
// a get-up still runs finds the player seated and still playing dead and
// gets it up again. A swing or cast still in flight keeps holding it for its
// own end.
func (l *GameClientLink) settleLivePosture(live *livePlayer, stoodUp bool) {
	if stoodUp {
		live.releaseChair()
	}
	if l.runDeferredAction(live, swingOrCastBusy) {
		return
	}
	l.finishDeferredPickup(live)
	if live.detached() || swingOrCastBusy(live) {
		return
	}
	l.runDeferredMagicSkill(live)
	l.runDeferredItemAICast(live)
	l.runDeferredFollow(live)
	l.runDeferredInteract(live)
	l.runDeferredUseItem(live, resumePosture)
}
