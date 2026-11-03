package player

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// equippedTemplates maps each worn instance whose equip stats are attached
// to the template they attached with. mu is a leaf.
type equippedTemplates struct {
	mu     sync.Mutex
	byInst map[*item.Instance]*item.Template
}

// SetEquippedTemplate records tmpl as the template inst's equip stats
// attached with.
func (c *Character) SetEquippedTemplate(inst *item.Instance, tmpl *item.Template) {
	if inst == nil || tmpl == nil {
		return
	}
	c.equipped.mu.Lock()
	defer c.equipped.mu.Unlock()
	if c.equipped.byInst == nil {
		c.equipped.byInst = make(map[*item.Instance]*item.Template)
	}
	c.equipped.byInst[inst] = tmpl
}

// EquippedTemplate returns the template inst's equip stats attached with.
// It is what detaching them goes by: a reload that replaces or drops the
// template id does not change what the worn item brought.
func (c *Character) EquippedTemplate(inst *item.Instance) (*item.Template, bool) {
	c.equipped.mu.Lock()
	defer c.equipped.mu.Unlock()
	tmpl, ok := c.equipped.byInst[inst]
	return tmpl, ok
}

// ForgetEquippedTemplate drops inst's record once its equip stats have
// detached for good.
func (c *Character) ForgetEquippedTemplate(inst *item.Instance) {
	c.equipped.mu.Lock()
	defer c.equipped.mu.Unlock()
	delete(c.equipped.byInst, inst)
}
