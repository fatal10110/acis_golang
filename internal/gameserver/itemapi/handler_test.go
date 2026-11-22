package itemapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/inventory"
)

type fakeEditor struct {
	err  error
	last inventory.ItemEdit
}

func (f *fakeEditor) CharacterItems(context.Context, int32) (inventory.CharacterItems, error) {
	return inventory.CharacterItems{}, f.err
}

func (f *fakeEditor) EditCharacterItems(_ context.Context, edit inventory.ItemEdit) (inventory.CharacterItems, error) {
	f.last = edit
	return inventory.CharacterItems{}, f.err
}

func serve(h http.Handler, method, path, remote, contentType, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = remote
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestHandlerRequestRules pins the request rules the item API shares with
// the ban API, and the decoding of an edit.
func TestHandlerRequestRules(t *testing.T) {
	const local = "127.0.0.1:5000"
	const edit = `{"character_id":7,"object_id":9,"expect":{"item_id":20,"count":5,"enchant":0,"location":"INVENTORY"},"count":3}`
	for _, tc := range []struct {
		name, method, path, remote, contentType, body string
		want                                          int
	}{
		{"remote peer", http.MethodGet, Path + "?character_id=7", "192.0.2.1:5000", "", "", http.StatusForbidden},
		{"list without id", http.MethodGet, Path, local, "", "", http.StatusBadRequest},
		{"post to list", http.MethodPost, Path, local, "application/json", "{}", http.StatusMethodNotAllowed},
		{"form post", http.MethodPost, EditPath, local, "application/x-www-form-urlencoded", edit, http.StatusUnsupportedMediaType},
		{"unknown field", http.MethodPost, DeletePath, local, "application/json", `{"character_id":7,"count":1}`, http.StatusBadRequest},
		{"bad location", http.MethodPost, EquipPath, local, "application/json", `{"character_id":7,"object_id":9,"expect":{"item_id":20,"count":5,"enchant":0,"location":"VOID"}}`, http.StatusBadRequest},
		{"unknown path", http.MethodGet, Path + "/x", local, "", "", http.StatusNotFound},
		{"edit", http.MethodPost, EditPath, local, "application/json", edit, http.StatusOK},
	} {
		f := &fakeEditor{}
		if rec := serve(Handler(f), tc.method, tc.path, tc.remote, tc.contentType, tc.body); rec.Code != tc.want {
			t.Fatalf("%s: status %d %s, want %d", tc.name, rec.Code, rec.Body, tc.want)
		}
		if tc.name == "edit" {
			want := inventory.ItemEdit{
				Op: inventory.EditSet, CharacterID: 7, ObjectID: 9, SetCount: true, Count: 3,
				Expect: inventory.ItemExpect{ItemID: 20, Count: 5, Location: 1},
			}
			if f.last != want {
				t.Fatalf("edit decoded as %+v, want %+v", f.last, want)
			}
		}
	}
}

// TestHandlerErrorStatus maps each edit fault to its status.
func TestHandlerErrorStatus(t *testing.T) {
	for fault, want := range map[inventory.EditFault]int{
		inventory.EditInvalid:     http.StatusBadRequest,
		inventory.EditNoCharacter: http.StatusNotFound,
		inventory.EditNoItem:      http.StatusNotFound,
		inventory.EditStale:       http.StatusConflict,
		inventory.EditRefused:     http.StatusUnprocessableEntity,
		inventory.EditBusy:        http.StatusServiceUnavailable,
		inventory.EditFailed:      http.StatusInternalServerError,
	} {
		f := &fakeEditor{err: &inventory.EditError{Fault: fault, Reason: "x"}}
		rec := serve(Handler(f), http.MethodGet, Path+"?character_id=7", "127.0.0.1:1", "", "")
		if rec.Code != want {
			t.Fatalf("fault %d: status %d, want %d", fault, rec.Code, want)
		}
		if retry := rec.Header().Get("Retry-After"); (fault == inventory.EditBusy) != (retry != "") {
			t.Fatalf("fault %d: Retry-After %q", fault, retry)
		}
	}
}
