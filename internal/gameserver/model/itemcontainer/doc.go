// Package itemcontainer models the item collections a player, pet, or clan
// owns: a plain container (private warehouse, clan warehouse, freight) or
// an equip-capable inventory (player inventory, pet inventory). Every type
// here is decoupled from the world/network/persistence layers — callers
// supply pre-allocated object ids for new instances and are responsible
// for actually persisting changes and releasing ids no longer in use.
package itemcontainer
