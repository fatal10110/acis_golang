// Package craft runs the recipe book rules: registering a recipe item into
// a player's book, deleting a recipe from it, and crafting a recipe for
// oneself. It decides and mutates; the caller turns the returned notices
// into client packets and queues the book's row writes.
package craft

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/recipe"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
)

// Service applies the recipe book rules against the loaded recipe table.
type Service struct {
	recipes *recipe.Table
	enabled bool
	roll    func(n int) int
	nextID  func() (int32, error)
}

// NewService returns a Service over recipes. enabled is the CraftingEnabled
// switch; nextID allocates a crafted product's object ids. A nil roll uses
// the shared random source; it returns a value in [0, n).
func NewService(recipes *recipe.Table, enabled bool, nextID func() (int32, error), roll func(n int) int) *Service {
	if roll == nil {
		roll = rnd.Get
	}
	return &Service{recipes: recipes, enabled: enabled, roll: roll, nextID: nextID}
}

// Recipe returns the loaded recipe with id.
func (s *Service) Recipe(id int) (recipe.Recipe, bool) {
	return s.recipes.Find(id)
}

// Notices a craft or a registration reports, in the order they happen.
type (
	// ActionFailed releases the client's pending action.
	ActionFailed struct{}
	// InCombat refuses a craft while the crafter is in combat.
	InCombat struct{}
	// MissingMaterial names a material the crafter lacks and how many
	// units short it is.
	MissingMaterial struct {
		ItemID int32
		Count  int
	}
	// NotEnoughMP refuses a craft the crafter cannot pay for.
	NotEnoughMP struct{}
	// CraftingDisabled refuses a craft while crafting is switched off.
	CraftingDisabled struct{}
	// MaterialConsumed names a material the craft used up.
	MaterialConsumed struct {
		ItemID int32
		Count  int
	}
	// ProductEarned names the product a successful craft handed over.
	ProductEarned struct {
		ItemID int32
		Count  int
	}
	// MixingFailed reports a craft whose success roll failed.
	MixingFailed struct{}

	// RegisterDisabled refuses a registration while crafting is
	// switched off.
	RegisterDisabled struct{}
	// AlreadyRegistered refuses a recipe the book already holds.
	AlreadyRegistered struct{}
	// NoCraftAbility refuses a recipe for a page the player cannot craft.
	NoCraftAbility struct{}
	// LevelTooLow refuses a recipe above the player's Create Item level.
	LevelTooLow struct{}
	// BookFull refuses a recipe for a page already at Limit recipes.
	BookFull struct{ Limit int }
	// RecipeAdded names the recipe item a registration used up.
	RecipeAdded struct{ ItemID int32 }
	// BookLocked refuses changing the recipe book while its owner runs a
	// workshop.
	BookLocked struct{}

	// NotEnoughAdena refuses a workshop order the customer cannot pay for.
	NotEnoughAdena struct{}
	// CraftedFor tells the crafter it made Count of ItemID for Customer at
	// Price.
	CraftedFor struct {
		Customer string
		ItemID   int32
		Count    int
		Price    int
	}
	// CraftedBy tells the customer Crafter made Count of ItemID for it at
	// Price.
	CraftedBy struct {
		Crafter string
		ItemID  int32
		Count   int
		Price   int
	}
	// CraftForFailed tells the crafter its craft of ItemID for Customer at
	// Price failed.
	CraftForFailed struct {
		Customer string
		ItemID   int32
		Price    int
	}
	// CraftByFailed tells the customer Crafter failed to craft ItemID for
	// it at Price.
	CraftByFailed struct {
		Crafter string
		ItemID  int32
		Price   int
	}
)

// Attempt is the outcome of one self-craft request.
type Attempt struct {
	// Notices are the messages to send, in order.
	Notices []any
	// Recipe is the recipe the request named, when it named a known one.
	Recipe recipe.Recipe
	// Made reports that a craft was attempted, so the craft window is
	// refreshed with Success after Notices.
	Made    bool
	Success bool
}

// MakeSelf crafts recipeID for c. busy reports that c is tied up in a trade
// or a trade request. A request for an unknown recipe, or for one that is
// not on the matching page of c's book, is dropped without a word.
func (s *Service) MakeSelf(c *player.Character, recipeID int, busy bool) Attempt {
	// A workshop owner crafts only for its customers.
	if c.OperateType() == privatestore.OperateManufacture {
		return Attempt{}
	}
	if c.InCombat() {
		return Attempt{Notices: []any{InCombat{}}}
	}
	r, ok := s.Recipe(recipeID)
	if !ok || !c.RecipeBook().HasOn(r.ID, r.Dwarven) {
		return Attempt{}
	}
	a := Attempt{Recipe: r, Made: true}
	switch {
	case c.AlikeDead(), busy, r.Level > c.CreateItemLevel(r.Dwarven):
		a.Notices = append(a.Notices, ActionFailed{})
		return a
	}
	if missing := missingMaterials(c, r); len(missing) > 0 {
		a.Notices = append(a.Notices, missing...)
		return a
	}
	if c.ResourceValues().CurrentMP < float64(r.MPCost) {
		a.Notices = append(a.Notices, NotEnoughMP{})
		return a
	}
	if !s.enabled {
		a.Notices = append(a.Notices, CraftingDisabled{})
		return a
	}

	if c.ReduceMP(float64(r.MPCost)) > 0 {
		c.BroadcastStatus()
	}
	// The materials are checked again once the MP is paid, as the
	// reference does before taking them.
	if missing := missingMaterials(c, r); len(missing) > 0 {
		a.Notices = append(a.Notices, missing...)
		return a
	}
	inv := c.Inventory()
	for _, m := range r.Materials {
		// A material that cannot be taken ends the craft with nothing made,
		// so a stack that shrank since the check never pays for a product.
		if m.Count > 0 && inv.DestroyByTemplateID(m.ItemID, m.Count) == nil {
			return a
		}
		a.Notices = append(a.Notices, MaterialConsumed{ItemID: m.ItemID, Count: m.Count})
	}
	if s.roll(100) >= r.SuccessRate {
		a.Notices = append(a.Notices, MixingFailed{})
		return a
	}
	c.AddCraftedItem(r.Product.ItemID, r.Product.Count, s.nextID)
	a.Notices = append(a.Notices, ProductEarned{ItemID: r.Product.ItemID, Count: r.Product.Count})
	a.Success = true
	return a
}

// missingMaterials names every material c holds too few of. Only the first
// held instance of a material counts, so a non-stackable material needed
// more than once always reads as short.
func missingMaterials(c *player.Character, r recipe.Recipe) []any {
	inv := c.Inventory()
	var out []any
	for _, m := range r.Materials {
		if m.Count <= 0 {
			continue
		}
		held := 0
		if inv != nil {
			if inst := inv.ItemByTemplateID(m.ItemID); inst != nil {
				held = inst.CountValue()
			}
		}
		if held < m.Count {
			out = append(out, MissingMaterial{ItemID: m.ItemID, Count: m.Count - held})
		}
	}
	return out
}

// Registration is the outcome of using a recipe item.
type Registration struct {
	// Notices are the messages to send, in order.
	Notices []any
	// Registered is the recipe the item was used up for, when it was.
	Registered *recipe.Recipe
	// Stored reports that the recipe went into the book. A character
	// playing a subclass uses the item up and is told the recipe was
	// added, but the book, the base class's alone, stays as it was.
	Stored bool
}

// RecipesHandler is the use-item handler name recipe items carry.
const RecipesHandler = "Recipes"

// Register uses recipe item inst, of template tmpl, from c's inventory:
// when c may take its recipe, the item is used up and the recipe written on
// its page. ok is false when tmpl is not a recipe item, which leaves the use
// to other handlers. A recipe item whose recipe is not loaded does nothing.
func (s *Service) Register(c *player.Character, inst *item.Instance, tmpl *item.Template) (reg Registration, ok bool) {
	if tmpl == nil || tmpl.EtcItem == nil || tmpl.EtcItem.Handler != RecipesHandler {
		return Registration{}, false
	}
	if !s.enabled {
		return Registration{Notices: []any{RegisterDisabled{}}}, true
	}
	r, found := s.recipes.FindByItemID(tmpl.ID)
	if !found {
		return Registration{}, true
	}
	book := c.RecipeBook()
	if book.Has(r.ID) {
		return Registration{Notices: []any{AlreadyRegistered{}}}, true
	}
	able, limit := c.HasCommonCraft(), c.CommonRecipeLimit()
	if r.Dwarven {
		able, limit = c.HasDwarvenCraft(), c.DwarfRecipeLimit()
	}
	switch {
	case !able:
		return Registration{Notices: []any{NoCraftAbility{}}}, true
	case c.OperateType() == privatestore.OperateManufacture:
		return Registration{Notices: []any{BookLocked{}}}, true
	case r.Level > c.CreateItemLevel(r.Dwarven):
		return Registration{Notices: []any{LevelTooLow{}}}, true
	case book.Count(r.Dwarven) >= limit:
		return Registration{Notices: []any{BookFull{Limit: limit}}}, true
	}
	if c.Inventory().DestroyByObjectID(inst.ObjectID, 1) == nil {
		return Registration{}, true
	}
	stored := !c.SubclassActive()
	if stored {
		book.Put(r)
	}
	return Registration{Notices: []any{RecipeAdded{ItemID: tmpl.ID}}, Registered: &r, Stored: stored}, true
}

// Forget deletes recipeID from c's book. ok is false for an unknown recipe,
// which changes nothing; a known recipe the book does not hold is still
// reported deleted. removed is false for a character playing a subclass:
// the book is the base class's, and nothing leaves it then, though the
// deletion is still reported.
func (s *Service) Forget(c *player.Character, recipeID int) (r recipe.Recipe, removed, ok bool) {
	r, ok = s.Recipe(recipeID)
	if !ok {
		return recipe.Recipe{}, false, false
	}
	if c.SubclassActive() {
		return r, false, true
	}
	c.RecipeBook().Remove(r.ID)
	return r, true, true
}

// ShopAttempt is the outcome of one workshop order.
type ShopAttempt struct {
	// ToCrafter and ToCustomer are the messages each side reads, in order.
	ToCrafter  []any
	ToCustomer []any
	// Success reports a craft whose product reached the customer.
	Success bool
}

// MakeFor crafts r at crafter's workshop for customer, who pays price in
// adena through pay and supplies the materials. busy reports that either
// side is tied up in a trade or a trade request. The customer's workshop
// craft window is refreshed after every outcome.
//
// The order is refused before anything is spent when either side is dead,
// busy, the crafter's Create Item level is below the recipe's, the customer
// holds less adena than price or misses a material, or the crafter lacks
// the MP. Otherwise the crafter spends the MP, the customer pays and gives
// up the materials, and the success roll decides whether the product
// reaches the customer.
func (s *Service) MakeFor(crafter, customer *player.Character, r recipe.Recipe, price int, busy bool, pay func(price int) bool) ShopAttempt {
	var a ShopAttempt
	switch {
	case crafter.AlikeDead() || customer.AlikeDead():
		a.ToCrafter = append(a.ToCrafter, ActionFailed{})
		return a
	case busy:
		a.ToCustomer = append(a.ToCustomer, ActionFailed{})
		return a
	case r.Level > crafter.CreateItemLevel(r.Dwarven):
		a.ToCrafter = append(a.ToCrafter, ActionFailed{})
		return a
	}
	if customer.Inventory().Adena() < price {
		a.ToCustomer = append(a.ToCustomer, NotEnoughAdena{})
		return a
	}
	if missing := missingMaterials(customer, r); len(missing) > 0 {
		a.ToCustomer = append(a.ToCustomer, missing...)
		return a
	}
	if crafter.ResourceValues().CurrentMP < float64(r.MPCost) {
		a.ToCustomer = append(a.ToCustomer, NotEnoughMP{})
		return a
	}
	if !s.enabled {
		a.ToCustomer = append(a.ToCustomer, CraftingDisabled{})
		return a
	}

	if crafter.ReduceMP(float64(r.MPCost)) > 0 {
		crafter.BroadcastStatus()
	}
	if price > 0 && !pay(price) {
		a.ToCustomer = append(a.ToCustomer, NotEnoughAdena{})
		return a
	}
	if missing := missingMaterials(customer, r); len(missing) > 0 {
		a.ToCustomer = append(a.ToCustomer, missing...)
		return a
	}
	inv := customer.Inventory()
	for _, m := range r.Materials {
		if m.Count > 0 && inv.DestroyByTemplateID(m.ItemID, m.Count) == nil {
			return a
		}
		a.ToCustomer = append(a.ToCustomer, MaterialConsumed{ItemID: m.ItemID, Count: m.Count})
	}
	product := r.Product
	if s.roll(100) >= r.SuccessRate {
		a.ToCrafter = append(a.ToCrafter, CraftForFailed{Customer: customer.Name, ItemID: product.ItemID, Price: price})
		a.ToCustomer = append(a.ToCustomer, CraftByFailed{Crafter: crafter.Name, ItemID: product.ItemID, Price: price})
		return a
	}
	customer.AddCraftedItem(product.ItemID, product.Count, s.nextID)
	a.ToCrafter = append(a.ToCrafter, CraftedFor{Customer: customer.Name, ItemID: product.ItemID, Count: product.Count, Price: price})
	a.ToCustomer = append(a.ToCustomer,
		CraftedBy{Crafter: crafter.Name, ItemID: product.ItemID, Count: product.Count, Price: price},
		ProductEarned{ItemID: product.ItemID, Count: product.Count})
	a.Success = true
	return a
}
