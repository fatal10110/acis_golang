package gameservertest

// WithShowNPCLevel turns on npcs.properties ShowNpcLevel (default: off): a
// monster's NpcInfo title starts with its level and, when it is
// aggressive, a star.
func WithShowNPCLevel() Option {
	return func(o *options) { o.showNPCLevel = true }
}

// WithMaxDriftRange sets npcs.properties MaxDriftRange (default: 200), the
// drift range the WithNpcSpawns population spawns with.
func WithMaxDriftRange(r int) Option {
	return func(o *options) { o.maxDriftRange = r }
}
