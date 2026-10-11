package script

import "fmt"

// HTMLText returns the text of the script's page file, read as the dialog
// reads it (Quest.getHtmlText): a real quest's from quest/<Name>, any other
// script's from <Dir>/<Name>. A page that does not exist gives the
// missing-page notice naming its path, as the dialog shows for a missing
// page file. A hook that fills the text's placeholders and returns it
// answers with that whole page.
func (s *Script) HTMLText(file string) string {
	path := s.pagePath(file)
	if s.env.Page != nil {
		if page, ok := s.env.Page(path); ok {
			return page
		}
	}
	return "<html><body>My html is missing:<br>" + path + "</body></html>"
}

// ItemName returns the name of the item template itemID
// (ItemData.getTemplate(itemID).getName()). An id with no template panics,
// so the invocation aborts there.
func (s *Script) ItemName(itemID int32) string {
	if s.env.Items != nil {
		if tmpl, ok := s.env.Items.Get(itemID); ok && tmpl != nil {
			return tmpl.Name
		}
	}
	panic(fmt.Sprintf("script: no item template %d", itemID))
}
