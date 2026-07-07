package datadiff

// Record is one entity's comparable field values within a data category —
// e.g. one item template, one experience-table row. ID distinguishes it
// from other records of the same category; Fields holds every value that
// should be compared, rendered as its canonical string form so two
// independently-generated dumps (from two different loaders, or from a
// loader and a stored fixture file) can be diffed as plain text.
type Record struct {
	ID     string
	Fields map[string]string
}
