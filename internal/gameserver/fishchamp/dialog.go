package fishchamp

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// Championship pages, under data/html/fisherman/championship/.
const (
	// PageWinners is the last week's winners, the prizes and the minutes
	// left in the running week, with the claim link.
	PageWinners = "data/html/fisherman/championship/fish_event001.htm"
	// PageRunning is the running week's ranking and the prizes.
	PageRunning = "data/html/fisherman/championship/fish_event002.htm"
	// PageRefreshing says the running ranking is being taken.
	PageRefreshing = "data/html/fisherman/championship/fish_event003.htm"
	// PageRewarded thanks a winner who was paid a prize.
	PageRewarded = "data/html/fisherman/championship/fish_event_reward001.htm"
	// PageDisabled answers both fisherman commands while the championship
	// is disabled.
	PageDisabled = "data/html/fisherman/championship/no_fish_event001.htm"
	// PageNotWinner refuses a claim from a player not among the winners.
	PageNotWinner = "data/html/fisherman/championship/no_fish_event_reward001.htm"
)

// lengthText writes a catch's length as the pages show it.
func lengthText(length float64) string { return commons.JavaDouble(length) }

// FillWinners sets the winners page's placeholders: the winners' table,
// the prizes paid in itemName, the minutes left in the running week and
// the fisherman's object id.
func (c *Championship) FillWinners(page string, objectID int32, itemName string) string {
	page = c.fillPrizes(strings.ReplaceAll(page, "%TABLE%", table(c.Winners())), itemName)
	page = strings.ReplaceAll(page, "%refresh%", strconv.FormatInt(c.MinutesLeft(), 10))
	return strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(objectID)))
}

// FillRunning sets the running ranking page's placeholders: places as its
// table and the prizes paid in itemName.
func (c *Championship) FillRunning(page string, places [Places]Standing, itemName string) string {
	return c.fillPrizes(strings.ReplaceAll(page, "%TABLE%", table(places)), itemName)
}

// fillPrizes sets the prize item's name and each place's prize.
func (c *Championship) fillPrizes(page, itemName string) string {
	page = strings.ReplaceAll(page, "%prizeItem%", itemName)
	for i, key := range [Places]string{"%prizeFirst%", "%prizeTwo%", "%prizeThree%", "%prizeFour%", "%prizeFive%"} {
		page = strings.ReplaceAll(page, key, strconv.Itoa(int(c.cfg.Rewards[i])))
	}
	return page
}

// table writes places as the rows of a ranking table.
func table(places [Places]Standing) string {
	var sb strings.Builder
	for i, p := range places {
		sb.WriteString("<tr><td width=70 align=center>" + strconv.Itoa(i+1) + "</td>")
		sb.WriteString("<td width=110 align=center>" + p.Name + "</td>")
		sb.WriteString("<td width=80 align=center>" + p.Length + "</td></tr>")
	}
	return sb.String()
}
