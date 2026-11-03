package gameservertest

// WithSingleItemDrop turns server.properties MultipleItemDrop off for kill
// rewards, party loot, the scripts and item creation: a non-stackable item
// counted above one falls, is auto-looted or is given as a single instance. Without it the setting is on, as
// shipped.
func WithSingleItemDrop() Option {
	return func(o *options) { o.singleItemDrop = true }
}

// IssuedObjectIDs reports how many object ids the server's allocator has
// handed out. Like the production allocator's cursor it never goes back:
// an id given back is still counted, since nothing hands it out again.
// Comparing it around an action shows how many ids the action spent.
func (s *Server) IssuedObjectIDs() int {
	s.ids.mu.Lock()
	defer s.ids.mu.Unlock()
	return s.ids.issued
}
