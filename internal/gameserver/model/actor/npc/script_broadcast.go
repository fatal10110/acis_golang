package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Say shows text as h's chat line to the players around it, named by h's
// npc id.
func (h *Hostile) Say(text string) {
	h.emit(event.NpcSay{NpcID: h.Instance.Template.ID, Text: text})
}

// Shout shouts text as h's chat line to the players around it, named by
// h's npc id.
func (h *Hostile) Shout(text string) {
	h.emit(event.NpcSay{NpcID: h.Instance.Template.ID, Text: text, Shout: true})
}

// Shout shouts text as f's chat line to the players around it, named by
// f's npc id.
func (f *Folk) Shout(text string) {
	f.emit(event.NpcSay{NpcID: f.Instance.Template.ID, Text: text, Shout: true})
}

// Say shows text as f's chat line to the players around it, named by f's
// npc id.
func (f *Folk) Say(text string) {
	f.emit(event.NpcSay{NpcID: f.Instance.Template.ID, Text: text})
}

// BroadcastOnScreen shows text in the middle of f's observers' screens
// for durationMs milliseconds.
func (f *Folk) BroadcastOnScreen(durationMs int32, text string) {
	f.emit(event.OnScreenMessage{Text: text, DurationMs: durationMs})
}

// SocialAction plays the social animation id on f for the players around
// it.
func (f *Folk) SocialAction(id int) {
	f.emit(event.SocialAction{ID: int32(id)})
}

// PlayCameraShot plays the cutscene camera shot, centred on h, for the
// players around it.
func (h *Hostile) PlayCameraShot(shot event.CameraShot) {
	h.emit(event.CameraShotPlayed{Shot: shot})
}

// PlayCameraShot plays the cutscene camera shot, centred on f, for the
// players around it.
func (f *Folk) PlayCameraShot(shot event.CameraShot) {
	f.emit(event.CameraShotPlayed{Shot: shot})
}

// PlayMusic plays the music file, bound to h where it stands now, for the
// players around it.
func (h *Hostile) PlayMusic(file string) {
	x, y, z := h.Position()
	h.emit(event.MusicPlayed{File: file, At: location.Location{X: x, Y: y, Z: z}})
}

// PlayMusic plays the music file, bound to f where it stands now, for the
// players around it.
func (f *Folk) PlayMusic(file string) {
	x, y, z := f.Position()
	f.emit(event.MusicPlayed{File: file, At: location.Location{X: x, Y: y, Z: z}})
}

// ShoutInRadius shouts text as h's chat line to the players within radius
// of it, named by h's npc id.
func (h *Hostile) ShoutInRadius(text string, radius int) {
	h.emit(event.NpcShout{NpcID: h.Instance.Template.ID, Text: text, Radius: radius})
}

// ShoutInRadius shouts text as f's chat line to the players within radius
// of it, named by f's npc id.
func (f *Folk) ShoutInRadius(text string, radius int) {
	f.emit(event.NpcShout{NpcID: f.Instance.Template.ID, Text: text, Radius: radius})
}
