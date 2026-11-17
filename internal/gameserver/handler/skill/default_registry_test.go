package skill

// NewDefaultRegistry returns the shipped handlers without the signet handler
// and the handlers that need object ids, with MagicFailures on.
func NewDefaultRegistry() *Registry {
	return NewDefaultRegistryWithDefinitions(nil)
}

// NewDefaultRegistryWithDefinitions is NewDefaultRegistry with loaded skill
// definitions for the handlers that look up another skill's effects.
func NewDefaultRegistryWithDefinitions(defs Definitions) *Registry {
	return newDefaultRegistry(defs, true, nil)
}
