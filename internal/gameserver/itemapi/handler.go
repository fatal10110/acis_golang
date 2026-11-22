// Package itemapi serves the character item API on the debug listener: an
// operator lists and changes a character's inventory and paperdoll while the
// server runs, in or out of the world.
package itemapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/fatal10110/acis_golang/internal/commons/debughttp"
	"github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// Item API paths on the debug listener.
const (
	Path        = "/debug/items"
	AddPath     = Path + "/add"
	EditPath    = Path + "/edit"
	DeletePath  = Path + "/delete"
	EquipPath   = Path + "/equip"
	UnequipPath = Path + "/unequip"
)

// Editor lists and edits a character's items.
type Editor interface {
	CharacterItems(ctx context.Context, characterID int32) (inventory.CharacterItems, error)
	EditCharacterItems(ctx context.Context, edit inventory.ItemEdit) (inventory.CharacterItems, error)
}

// Handler serves the item API over editor: GET Path?character_id=N lists a
// character's items, and POST AddPath, EditPath, DeletePath, EquipPath and
// UnequipPath change them, each answering with the items as they stand
// after the change. Writes take an application/json body. Only loopback
// clients are served.
func Handler(editor Editor) http.Handler {
	return &handler{editor: editor}
}

type handler struct {
	editor Editor
}

// itemRow is one item in a list. Slot is the paperdoll slot of a worn item,
// else -1; Mana is a shadow item's remaining mana, else -1.
type itemRow struct {
	ObjectID  int32  `json:"object_id"`
	ItemID    int32  `json:"item_id"`
	Count     int    `json:"count"`
	Enchant   int    `json:"enchant"`
	Location  string `json:"location"`
	Slot      int    `json:"slot"`
	Augmented bool   `json:"augmented"`
	Mana      int    `json:"mana"`
}

type itemList struct {
	CharacterID int32     `json:"character_id"`
	Name        string    `json:"name"`
	Online      bool      `json:"online"`
	Items       []itemRow `json:"items"`
}

type addRequest struct {
	CharacterID int32 `json:"character_id"`
	ItemID      int32 `json:"item_id"`
	Count       int   `json:"count"`
	Enchant     int   `json:"enchant"`
}

// expect is what the caller last listed of the item it changes.
type expect struct {
	ItemID   int32  `json:"item_id"`
	Count    int    `json:"count"`
	Enchant  int    `json:"enchant"`
	Location string `json:"location"`
}

type editRequest struct {
	CharacterID int32   `json:"character_id"`
	ObjectID    int32   `json:"object_id"`
	Expect      *expect `json:"expect"`
	Count       *int    `json:"count"`
	Enchant     *int    `json:"enchant"`
}

type targetRequest struct {
	CharacterID int32   `json:"character_id"`
	ObjectID    int32   `json:"object_id"`
	Expect      *expect `json:"expect"`
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !debughttp.LoopbackPeer(r.RemoteAddr) {
		debughttp.WriteError(w, http.StatusForbidden, "loopback clients only")
		return
	}
	switch r.URL.Path {
	case Path:
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			debughttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		id, err := strconv.ParseInt(r.URL.Query().Get("character_id"), 10, 32)
		if err != nil || id <= 0 {
			debughttp.WriteError(w, http.StatusBadRequest, "character_id must be a positive object id")
			return
		}
		h.answer(w)(h.editor.CharacterItems(r.Context(), int32(id)))
	case AddPath:
		var req addRequest
		if debughttp.DecodePost(w, r, &req) {
			h.answer(w)(h.editor.EditCharacterItems(r.Context(), inventory.ItemEdit{
				Op: inventory.EditAdd, CharacterID: req.CharacterID, ItemID: req.ItemID, Count: req.Count, Enchant: req.Enchant,
			}))
		}
	case EditPath:
		var req editRequest
		if !debughttp.DecodePost(w, r, &req) {
			return
		}
		edit, ok := target(w, inventory.EditSet, req.CharacterID, req.ObjectID, req.Expect)
		if !ok {
			return
		}
		if req.Count != nil {
			edit.SetCount, edit.Count = true, *req.Count
		}
		if req.Enchant != nil {
			edit.SetEnchant, edit.Enchant = true, *req.Enchant
		}
		h.answer(w)(h.editor.EditCharacterItems(r.Context(), edit))
	case DeletePath, EquipPath, UnequipPath:
		var req targetRequest
		if !debughttp.DecodePost(w, r, &req) {
			return
		}
		op := map[string]inventory.EditOp{DeletePath: inventory.EditDelete, EquipPath: inventory.EditEquip, UnequipPath: inventory.EditUnequip}[r.URL.Path]
		if edit, ok := target(w, op, req.CharacterID, req.ObjectID, req.Expect); ok {
			h.answer(w)(h.editor.EditCharacterItems(r.Context(), edit))
		}
	default:
		debughttp.WriteError(w, http.StatusNotFound, "not found")
	}
}

// target builds an edit of one item, answering 400 when the request does not
// name the item and what the caller expects it to be.
func target(w http.ResponseWriter, op inventory.EditOp, characterID, objectID int32, want *expect) (inventory.ItemEdit, bool) {
	if objectID <= 0 || want == nil {
		debughttp.WriteError(w, http.StatusBadRequest, "give object_id and expect {item_id, count, enchant, location}")
		return inventory.ItemEdit{}, false
	}
	var loc item.Location
	switch want.Location {
	case item.LocationInventory.String():
		loc = item.LocationInventory
	case item.LocationPaperdoll.String():
		loc = item.LocationPaperdoll
	default:
		debughttp.WriteError(w, http.StatusBadRequest, `expect.location must be "INVENTORY" or "PAPERDOLL"`)
		return inventory.ItemEdit{}, false
	}
	return inventory.ItemEdit{
		Op: op, CharacterID: characterID, ObjectID: objectID,
		Expect: inventory.ItemExpect{ItemID: want.ItemID, Count: want.Count, Enchant: want.Enchant, Location: loc},
	}, true
}

// answer writes an editor's result: the items, or the error's status.
func (h *handler) answer(w http.ResponseWriter) func(inventory.CharacterItems, error) {
	return func(items inventory.CharacterItems, err error) {
		if err != nil {
			status, msg := errorStatus(err)
			if status == http.StatusServiceUnavailable {
				w.Header().Set("Retry-After", "1")
			}
			debughttp.WriteError(w, status, msg)
			return
		}
		out := itemList{CharacterID: items.CharacterID, Name: items.Name, Online: items.Online, Items: make([]itemRow, 0, len(items.Items))}
		for _, st := range items.Items {
			row := itemRow{
				ObjectID: st.ObjectID, ItemID: st.TemplateID, Count: st.Count, Enchant: st.EnchantLevel,
				Location: st.Location.String(), Slot: -1, Augmented: st.Augmentation != nil, Mana: st.ManaLeft,
			}
			if st.Equipped() {
				row.Slot = st.LocationData
			}
			out.Items = append(out.Items, row)
		}
		debughttp.WriteJSON(w, http.StatusOK, out)
	}
}

// errorStatus maps an editor error to its status and message.
func errorStatus(err error) (int, string) {
	var edit *inventory.EditError
	if !errors.As(err, &edit) {
		return http.StatusInternalServerError, err.Error()
	}
	switch edit.Fault {
	case inventory.EditInvalid:
		return http.StatusBadRequest, edit.Reason
	case inventory.EditNoCharacter, inventory.EditNoItem:
		return http.StatusNotFound, edit.Reason
	case inventory.EditStale:
		return http.StatusConflict, edit.Reason
	case inventory.EditRefused:
		return http.StatusUnprocessableEntity, edit.Reason
	case inventory.EditBusy:
		return http.StatusServiceUnavailable, edit.Reason
	}
	return http.StatusInternalServerError, edit.Reason
}
