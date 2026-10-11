package event

// CameraShot is one cutscene camera move the client plays around an
// object, in the units the client takes them: a distance, angles in
// degrees and times in milliseconds.
type CameraShot struct {
	// Distance is how far the camera stands from the object.
	Distance int32
	// Yaw and Pitch aim the camera at the object.
	Yaw, Pitch int32
	// Time is how long the camera takes to reach the shot; Duration is
	// how long the shot lasts.
	Time, Duration int32
	// Turn and Rise move the camera around and above the object over the
	// shot.
	Turn, Rise int32
	// Widescreen, when 1, shows the shot letterboxed.
	Widescreen int32
	// Unknown is the shot's last field, passed to the client as given.
	Unknown int32
}

// CameraShotPlayed reports a cutscene camera shot, centred on the NPC that
// emits it, for the players that know the NPC.
type CameraShotPlayed struct{ Shot CameraShot }

// ScenePlayed reports the client's built-in scene played for the character
// alone.
type ScenePlayed struct{}

func (CameraShotPlayed) event() {}
func (ScenePlayed) event()      {}
