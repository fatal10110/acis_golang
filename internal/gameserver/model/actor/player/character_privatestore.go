package player

import "github.com/fatal10110/acis_golang/internal/gameserver/privatestore"

// PrivateStore returns c's private store.
func (c *Character) PrivateStore() *privatestore.Store {
	return &c.store
}

// OperateType returns what c's private store is doing.
func (c *Character) OperateType() privatestore.OperateType {
	return c.store.OperateType()
}

// SetOperateType sets what c's private store is doing and reports whether
// it changed.
func (c *Character) SetOperateType(t privatestore.OperateType) bool {
	return c.store.SetOperateType(t)
}

// Operating reports whether c runs a private store or workshop, or is
// setting one up.
func (c *Character) Operating() bool {
	return c.store.OperateType().Operating()
}

// InStoreMode reports whether c's store or workshop is open for business.
func (c *Character) InStoreMode() bool {
	return c.store.OperateType().InStoreMode()
}

// InManageStoreMode reports whether c is setting up a store or workshop.
func (c *Character) InManageStoreMode() bool {
	return c.store.OperateType().InManageMode()
}
