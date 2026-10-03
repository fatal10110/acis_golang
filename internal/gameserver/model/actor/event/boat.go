package event

import "github.com/fatal10110/acis_golang/internal/gameserver/model/location"

// BoatDeparted reports a boat heading for its next route point from From:
// its observers are shown the move toward Destination, then the vehicle
// departure at the Speed and Rotation it now travels with.
type BoatDeparted struct {
	From, Destination location.Location
	Speed, Rotation   int
}

// BoatStarted reports a boat setting off on its route (Moving) or coming to
// a stop at the end of it.
type BoatStarted struct{ Moving bool }

// BoatShown reports that the boat's observers are shown it again where it
// stands, facing where it faces.
type BoatShown struct{}

// BoatAudience is who hears a boat's schedule: every online player standing,
// on the ground plane, strictly less than Radius from any of Centers.
type BoatAudience struct {
	Centers []location.Location
	Radius  int
}

// BoatAnnounced reports boat schedule lines, the system messages
// MessageIDs in order, told to Audience on the boat chat channel.
type BoatAnnounced struct {
	Audience   BoatAudience
	MessageIDs []int
}

// BoatSounded reports a boat sound, Sound bound to the boat and played at
// At, heard by Audience.
type BoatSounded struct {
	Audience BoatAudience
	Sound    string
	At       location.Location
}

func (BoatDeparted) event()  {}
func (BoatStarted) event()   {}
func (BoatShown) event()     {}
func (BoatAnnounced) event() {}
func (BoatSounded) event()   {}
