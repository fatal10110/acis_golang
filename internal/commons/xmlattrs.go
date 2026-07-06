package commons

import "encoding/xml"

// StatSetFromXMLAttrs folds an XML element's attributes into a StatSet,
// keyed by local attribute name. It is the handoff between wire-shape
// decoding in the data loaders and the StatSet-consuming constructors on
// the model side: loaders decode elements, model types parse their own
// fields.
func StatSetFromXMLAttrs(attrs []xml.Attr) *StatSet {
	set := NewStatSetWithCapacity(len(attrs))
	for _, a := range attrs {
		set.Set(a.Name.Local, a.Value)
	}
	return set
}
