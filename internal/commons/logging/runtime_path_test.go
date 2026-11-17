package logging

// Path returns the opened file path for sink.
func (r *Runtime) Path(sink Sink) string {
	return r.paths[sink]
}
