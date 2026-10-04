// Package classmaster is the class manager mod: an NPC that hands out the
// first, second and third occupation changes for configured item costs and
// rewards, noblesse status, and every skill the talker can learn.
package classmaster

import (
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// Item is a count of one item template.
type Item struct {
	ID    int32
	Count int
}

// Job is one configured occupation change: the items it takes and the
// items it hands out.
type Job struct {
	Required []Item
	Reward   []Item
}

// Config is the npcs.properties class master settings.
type Config struct {
	// AllowEntireTree lets a talker take any later occupation of its own
	// class line, whatever its level.
	AllowEntireTree bool
	// jobs are the configured occupation changes by tier (1 to 3).
	jobs map[int]Job
}

// ParseJobs reads a ConfigClassMaster line: ';'-separated groups of a tier,
// then its required items, then its reward items, each item list a
// bracketed, comma-separated list of id(count). Empty fields are skipped
// as separators, and a later group of a tier replaces an earlier one. A
// tier or an item that does not parse is an error.
func ParseJobs(line string) (map[int]Job, error) {
	jobs := map[int]Job{}
	tokens := splitTokens(strings.TrimSpace(line), ";")
	for i := 0; i < len(tokens); {
		tier, err := commons.ParseInt(tokens[i], 32)
		if err != nil {
			return nil, fmt.Errorf("class master tier %q: %w", tokens[i], err)
		}
		i++
		var job Job
		for _, list := range []*[]Item{&job.Required, &job.Reward} {
			*list = []Item{}
			if i >= len(tokens) {
				continue
			}
			if *list, err = parseItems(tokens[i]); err != nil {
				return nil, err
			}
			i++
		}
		jobs[int(tier)] = job
	}
	return jobs, nil
}

// NewConfig returns the settings of jobs, as ParseJobs read them.
func NewConfig(allowEntireTree bool, jobs map[int]Job) Config {
	return Config{AllowEntireTree: allowEntireTree, jobs: jobs}
}

// parseItems reads one bracketed item list.
func parseItems(token string) ([]Item, error) {
	items := []Item{}
	for _, entry := range splitTokens(token, "[],") {
		parts := splitTokens(entry, "()")
		if len(parts) < 2 {
			return nil, fmt.Errorf("class master item %q: want id(count)", entry)
		}
		id, err := commons.ParseInt(parts[0], 32)
		if err != nil {
			return nil, fmt.Errorf("class master item id %q: %w", parts[0], err)
		}
		count, err := commons.ParseInt(parts[1], 32)
		if err != nil {
			return nil, fmt.Errorf("class master item count %q: %w", parts[1], err)
		}
		items = append(items, Item{ID: int32(id), Count: int(count)})
	}
	return items, nil
}

// splitTokens splits s on any of the delimiter characters, dropping empty
// tokens.
func splitTokens(s, delims string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return strings.ContainsRune(delims, r) })
}

// Allowed reports whether occupation change tier is offered.
func (c Config) Allowed(tier int) bool {
	_, ok := c.jobs[tier]
	return ok
}

// Job returns the configured occupation change tier.
func (c Config) Job(tier int) (Job, bool) {
	job, ok := c.jobs[tier]
	return job, ok
}
