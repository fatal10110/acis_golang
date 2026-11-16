package effect

import "testing"

// A class change brings the new class's saved effects back on a player in
// the world: each restored add tells the owner it feels the effect, as a
// cast does (the same messages Add sends in "later cast still announces"),
// and the icons still refresh once for the whole replay.
func TestListRestoreAnnouncedSendsTheFeltMessages(t *testing.T) {
	var events []string
	list := newTestList(iconEventOwner{eventOwner{events: &events}}, WithEnv(Env{KeepLesser: true}))
	weak := namedEffect("weak", 201, "speed_up", 1, false, &events)
	weak.Template.Icon = true
	strong := namedEffect("strong", 202, "speed_up", 2, false, &events)
	strong.Template.Icon = true

	list.RestoreAnnounced(func() {
		list.AddRestored(weak)
		list.AddRestored(strong)
	})

	requireEvents(t, events, []string{
		"weak:start",
		"owner:add",
		"felt:201:0",
		"owner:remove:weak",
		"weak:exit",
		"disappeared:201:0",
		"strong:start",
		"owner:add",
		"felt:202:0",
		"icons",
	})
	requireNames(t, list.All(), []string{"weak", "strong"})
}
