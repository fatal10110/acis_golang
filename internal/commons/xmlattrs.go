package commons

import "encoding/xml"

// StatSetFromXMLAttrs folds an XML element's attributes into a StatSet,
// keyed by local attribute name. It mirrors IXmlReader.parseAttributes
// (commons/data/xml/IXmlReader.java), the seam every Java XML loader uses
// between wire parsing and model construction: loaders decode elements,
// model constructors consume StatSets.
func StatSetFromXMLAttrs(attrs []xml.Attr) *StatSet {
	set := NewStatSetWithCapacity(len(attrs))
	for _, a := range attrs {
		set.Set(a.Name.Local, a.Value)
	}
	return set
}
