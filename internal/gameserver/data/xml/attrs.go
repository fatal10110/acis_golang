package xml

import (
	"encoding/xml"

	"github.com/fatal10110/acis_golang/internal/commons"
)

func mergeXMLAttrs(set *commons.StatSet, attrs []xml.Attr) {
	for _, a := range attrs {
		set.Set(a.Name.Local, a.Value)
	}
}
