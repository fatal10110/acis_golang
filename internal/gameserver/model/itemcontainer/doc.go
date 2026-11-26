// Package itemcontainer models the item collections a player, pet, or clan
// owns: a plain container (private warehouse, clan warehouse, freight) or
// an equip-capable inventory (player inventory, pet inventory). Every type
// here is decoupled from the world/network/persistence layers — new
// instances take their object ids through AddCreated's nextID (object ids
// are not reused within a session), and callers are responsible for
// actually persisting changes.
package itemcontainer
