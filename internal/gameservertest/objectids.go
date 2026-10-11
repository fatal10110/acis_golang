package gameservertest

// WithSingleItemDrop turns server.properties MultipleItemDrop off for kill
// rewards and party loot: a non-stackable item counted above one falls or
// is auto-looted as a single instance. Without it the setting is on, as
// shipped.
func WithSingleItemDrop() Option {
	return func(o *options) { o.singleItemDrop = true }
}

// HeldObjectIDs reports how many object ids the server's allocator has
// handed out and not taken back: comparing it around an action shows how
// many ids the action kept.
func (s *Server) HeldObjectIDs() int {
	s.ids.mu.Lock()
	defer s.ids.mu.Unlock()
	return s.ids.issued - s.ids.released
}
