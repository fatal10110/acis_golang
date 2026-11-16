package quest

import "testing"

// The newbie guide's first-talk payout is keyed on its own guideReward
// variable, not on the helper state's status the helper's first talk
// reopens: an owner-approved fix (#5159) so the guide pays once per
// character whatever order the guide and the helper are met in.

// TestNewbieGuidePaysOnceAfterHelperReopens: guide, helper, guide. The
// guide pays and completes the helper state, the helper's first talk
// starts it again, and the guide's second talk shows its page without
// paying again (#5159).
func TestNewbieGuidePaysOnceAfterHelperReopens(t *testing.T) {
	t.Parallel()
	w := bootNewbieHelper(t, 1, 0, nil)
	guide := w.spawnGuide(t, cnacelotID, 20, 0)
	carl := w.spawnGuide(t, carlID, 0, 20)

	expectLines(t, "guide, first talk", w.talk(t, guide.ObjectID()), "ActionFailed", "sound tutorial_voice_026", "page 30598.htm", "ActionFailed")
	expectCount(t, w, beginnerShots, 200)
	expectCount(t, w, travelToken, 12)
	expectVars(t, w, newbieHelper, map[string]string{"<state>": "COMPLETED", "guideReward": "1"})

	expectLines(t, "helper, first talk", w.talk(t, carl.ObjectID()), "ActionFailed", "page newbiehelper_fig_01.htm", "ActionFailed")
	expectVars(t, w, newbieHelper, map[string]string{"<state>": "STARTED", "step": "1", "guideReward": "1"})

	expectLines(t, "guide, after the helper", w.talk(t, guide.ObjectID()), "ActionFailed", "page 30598.htm", "ActionFailed")
	expectCount(t, w, beginnerShots, 200)
	expectCount(t, w, travelToken, 12)
	expectVars(t, w, newbieHelper, map[string]string{"<state>": "STARTED", "step": "1", "guideReward": "1"})
}

// TestNewbieGuidePaysOnceAfterHelper: helper, guide, guide. The guide pays
// on its first talk after the helper started the state, and not again on
// its second (#5159).
func TestNewbieGuidePaysOnceAfterHelper(t *testing.T) {
	t.Parallel()
	w := bootNewbieHelper(t, 1, 0, nil)
	guide := w.spawnGuide(t, cnacelotID, 20, 0)
	carl := w.spawnGuide(t, carlID, 0, 20)

	expectLines(t, "helper, first talk", w.talk(t, carl.ObjectID()), "ActionFailed", "page newbiehelper_fig_01.htm", "ActionFailed")
	expectVars(t, w, newbieHelper, map[string]string{"<state>": "STARTED", "step": "1"})

	expectLines(t, "guide, first talk", w.talk(t, guide.ObjectID()), "ActionFailed", "sound tutorial_voice_026", "page 30598.htm", "ActionFailed")
	expectCount(t, w, beginnerShots, 200)
	expectCount(t, w, travelToken, 12)
	expectVars(t, w, newbieHelper, map[string]string{"<state>": "COMPLETED", "step": "1", "guideReward": "1"})

	expectLines(t, "guide, second talk", w.talk(t, guide.ObjectID()), "ActionFailed", "page 30598.htm", "ActionFailed")
	expectCount(t, w, beginnerShots, 200)
	expectCount(t, w, travelToken, 12)
}
