// Package datadiff compares two independently produced dumps of the same
// data category — typically a loader's in-memory result rendered to a
// canonical text form against an equivalent dump from another
// implementation — and reports whether they agree, record by record and
// field by field.
//
// The package is deliberately agnostic to any specific category: a
// category's records are reduced to a flat Record (an ID plus a map of
// named field values, both strings), and everything here operates on that
// shape. Producing Records for a given category — invoking its loader and
// picking which fields to compare — is the caller's job.
package datadiff
