package commons

import "encoding/xml"

// StatSetFromXMLAttrs folds an XML element's attributes into a StatSet,
// keyed by local attribute name. It is the handoff between wire-shape
// decoding in the data loaders and the StatSet-consuming constructors on
// the model side: loaders decode elements, model types parse their own
// fields.
func StatSetFromXMLAttrs(attrs []xml.Attr) *StatSet {
	set := NewStatSetWithCapacity(len(attrs))
	set.MergeXMLAttrs(attrs)
	return set
}

// MergeXMLAttrs folds attrs into s, keyed by local attribute name,
// overwriting any existing values for the same key. Callers building a
// StatSet from more than one XML element (e.g. an element split across
// several child tags) call this once per element to merge them all into
// one set.
func (s *StatSet) MergeXMLAttrs(attrs []xml.Attr) {
	for _, a := range attrs {
		s.Set(a.Name.Local, a.Value)
	}
}
