package script

// Memo returns the player's memo key, kept across sessions.
func (p *Player) Memo(key string) (string, bool) {
	if c := p.character(); c != nil {
		return c.Memos().Get(key)
	}
	return "", false
}

// SetMemo sets the player's memo key to value and saves it.
func (p *Player) SetMemo(key, value string) {
	if c := p.character(); c != nil {
		c.Memos().Set(key, value)
	}
}

// UnsetMemo removes the player's memo key and deletes it.
func (p *Player) UnsetMemo(key string) {
	if c := p.character(); c != nil {
		c.Memos().Unset(key)
	}
}
