package player

// ChangeOccupation makes classID, played with tmpl, the class c plays, in
// place: the class index stays, an active subclass's slot takes the new
// class, and the level, experience and SP carry over. The level tables'
// maxima are those of tmpl from then on; current HP, MP and CP are kept.
// The base class moves only through SetBaseClass. It runs on c's queue.
func (c *Character) ChangeOccupation(classID int, tmpl *Template) {
	c.progressionMu.Lock()
	if index := c.ClassIndex(); index != 0 {
		if sub, ok := c.subclasses.slots[index]; ok {
			sub.ClassID = classID
			c.subclasses.slots[index] = sub
		}
	}
	c.SetClassID(classID)
	c.runtimeTemplate.Store(tmpl)
	c.progressionMu.Unlock()
	c.RestoreVitals(tmpl)
}

// SetBaseClass makes classID, with tmpl its body, c's base class.
func (c *Character) SetBaseClass(classID int, tmpl *Template) {
	c.SetBaseClassID(classID)
	c.baseTemplate.Store(tmpl)
}
