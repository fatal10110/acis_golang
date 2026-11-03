package gameservertest

// WithShowNPCCrest turns on npcs.properties ShowNpcCrest (default: off): an
// NPC's NpcInfo shows the crests of the clan owning the residence it serves.
func WithShowNPCCrest() Option {
	return func(o *options) { o.showNPCCrest = true }
}
