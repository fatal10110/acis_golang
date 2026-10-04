package npc

// Name is the NPC's name: the template's until SetName replaces it. The
// client shows it only for a template using its server-side name.
func (i *Instance) Name() string {
	if p := i.name.Load(); p != nil {
		return *p
	}
	return i.Template.Name
}

// SetName replaces the NPC's name.
func (i *Instance) SetName(name string) { i.name.Store(&name) }

// Title is the NPC's title: the template's until SetTitle replaces it. The
// client shows it only for a template using its server-side title.
func (i *Instance) Title() string {
	if p := i.title.Load(); p != nil {
		return *p
	}
	return i.Template.Title
}

// SetTitle replaces the NPC's title.
func (i *Instance) SetTitle(title string) { i.title.Store(&title) }
