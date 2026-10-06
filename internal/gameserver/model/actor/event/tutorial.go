package event

// TutorialPageShown reports that the character's tutorial window opens on
// the datapack page File.
type TutorialPageShown struct{ File string }

// TutorialPageClosed reports that the character's tutorial window closes.
type TutorialPageClosed struct{}

// TutorialQuestionMarkShown reports that the character is shown the
// tutorial question mark ID.
type TutorialQuestionMarkShown struct{ ID int32 }

// TutorialClientEventEnabled reports that the character's client is to
// report the tutorial client event ID.
type TutorialClientEventEnabled struct{ ID int32 }

// TutorialVoicePlayed reports that the character hears the tutorial voice
// Voice, from where it stands.
type TutorialVoicePlayed struct{ Voice string }

// RadarMarkerAdded reports that the character's radar marks X, Y, Z.
type RadarMarkerAdded struct{ X, Y, Z int32 }

// RadarMarkerRemoved reports that the character's radar no longer marks X,
// Y, Z.
type RadarMarkerRemoved struct{ X, Y, Z int32 }

func (TutorialPageShown) event()          {}
func (TutorialPageClosed) event()         {}
func (TutorialQuestionMarkShown) event()  {}
func (TutorialClientEventEnabled) event() {}
func (TutorialVoicePlayed) event()        {}
func (RadarMarkerAdded) event()           {}
func (RadarMarkerRemoved) event()         {}
