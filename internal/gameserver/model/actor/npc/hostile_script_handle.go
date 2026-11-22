package npc

// ScriptHandle returns the script engine's handle on h, nil before it keeps
// one.
func (h *Hostile) ScriptHandle() any { return h.scriptHandle.Load() }

// KeepScriptHandle stores v as ScriptHandle unless a value is stored
// already, and returns the stored value. Every v must have the same
// concrete type.
func (h *Hostile) KeepScriptHandle(v any) any {
	if h.scriptHandle.CompareAndSwap(nil, v) {
		return v
	}
	return h.scriptHandle.Load()
}
