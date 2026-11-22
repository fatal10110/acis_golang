package debughttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/netutil"
)

// Ban API paths on the debug listener.
const (
	BansPath       = "/debug/bans"
	BansAddPath    = "/debug/bans/add"
	BansRemovePath = "/debug/bans/remove"
)

// MaxBody caps a write request's body on the debug listener's APIs.
const MaxBody = 4 << 10

const (
	// maxBanSeconds is the longest temporary ban, ten years; longer is
	// asked for as permanent.
	maxBanSeconds = 10 * 365 * 24 * 60 * 60
)

// BanSource is a ban list the ban API lists and lifts bans in.
type BanSource interface {
	List() []netutil.BanEntry
	Remove(addr netip.Addr) bool
}

// banAdder is a BanSource that also takes new bans, for d or, when d <= 0,
// for good, reporting whether it banned addr.
type banAdder interface {
	Add(addr netip.Addr, d time.Duration) bool
}

// BanStore is a ban list served under Name. A Bans that also has an
// Add(netip.Addr, time.Duration) bool method takes new bans through the
// API; the others only list and lift them.
type BanStore struct {
	Name string
	Bans BanSource
}

// banRow is one ban in a list response.
type banRow struct {
	IP        string `json:"ip"`
	Store     string `json:"store"`
	Permanent bool   `json:"permanent"`
	Expires   string `json:"expires,omitempty"`
}

type storeInfo struct {
	Name string `json:"name"`
	Add  bool   `json:"add"`
}

type banList struct {
	Server string      `json:"server"`
	Stores []storeInfo `json:"stores"`
	Bans   []banRow    `json:"bans"`
}

type addRequest struct {
	IP        string `json:"ip"`
	Store     string `json:"store"`
	Permanent bool   `json:"permanent"`
	Seconds   int64  `json:"seconds"`
}

type removeRequest struct {
	IP    string `json:"ip"`
	Store string `json:"store"`
}

// BanHandler serves the ban API for server's stores: GET BansPath lists
// every ban, POST BansAddPath bans an address in a store that takes bans,
// and POST BansRemovePath lifts one. Writes take an application/json body.
// Only loopback clients are served. Each add and remove is logged to log.
func BanHandler(server string, stores []BanStore, log zerolog.Logger) http.Handler {
	return &banHandler{server: server, stores: stores, log: log}
}

type banHandler struct {
	server string
	stores []BanStore
	log    zerolog.Logger
}

func (h *banHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !LoopbackPeer(r.RemoteAddr) {
		WriteError(w, http.StatusForbidden, "loopback clients only")
		return
	}
	switch r.URL.Path {
	case BansPath:
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		WriteJSON(w, http.StatusOK, h.list())
	case BansAddPath:
		var req addRequest
		if DecodePost(w, r, &req) {
			h.add(w, req)
		}
	case BansRemovePath:
		var req removeRequest
		if DecodePost(w, r, &req) {
			h.remove(w, req)
		}
	default:
		WriteError(w, http.StatusNotFound, "not found")
	}
}

func (h *banHandler) list() banList {
	out := banList{Server: h.server, Stores: make([]storeInfo, 0, len(h.stores)), Bans: []banRow{}}
	for _, s := range h.stores {
		_, add := s.Bans.(banAdder)
		out.Stores = append(out.Stores, storeInfo{Name: s.Name, Add: add})
		bans := s.Bans.List()
		slices.SortFunc(bans, func(a, b netutil.BanEntry) int { return a.Addr.Compare(b.Addr) })
		for _, b := range bans {
			row := banRow{IP: b.Addr.String(), Store: s.Name, Permanent: b.Permanent()}
			if !row.Permanent {
				row.Expires = b.Expires.UTC().Format(time.RFC3339)
			}
			out.Bans = append(out.Bans, row)
		}
	}
	return out
}

func (h *banHandler) add(w http.ResponseWriter, req addRequest) {
	addr, store, ok := h.target(w, req.IP, req.Store)
	if !ok {
		return
	}
	adder, ok := store.Bans.(banAdder)
	if !ok {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("store %q does not take bans", store.Name))
		return
	}
	var d time.Duration
	switch {
	case req.Permanent && req.Seconds != 0:
		WriteError(w, http.StatusBadRequest, "give permanent or seconds, not both")
		return
	case req.Permanent:
	case req.Seconds <= 0 || req.Seconds > maxBanSeconds:
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("seconds must be 1 to %d, or permanent true", maxBanSeconds))
		return
	default:
		d = time.Duration(req.Seconds) * time.Second
	}
	added := adder.Add(addr, d)
	h.log.Warn().Str("ip", addr.String()).Str("store", store.Name).Bool("permanent", req.Permanent).
		Dur("duration", d).Bool("added", added).Msg("ip ban added through the ban api")
	WriteJSON(w, http.StatusOK, map[string]bool{"added": added})
}

func (h *banHandler) remove(w http.ResponseWriter, req removeRequest) {
	addr, store, ok := h.target(w, req.IP, req.Store)
	if !ok {
		return
	}
	removed := store.Bans.Remove(addr)
	h.log.Warn().Str("ip", addr.String()).Str("store", store.Name).Bool("removed", removed).
		Msg("ip ban removed through the ban api")
	WriteJSON(w, http.StatusOK, map[string]bool{"removed": removed})
}

// target resolves a request's literal IP address and store name, answering
// 400 when either is not valid.
func (h *banHandler) target(w http.ResponseWriter, ip, name string) (netip.Addr, BanStore, bool) {
	addr, err := netip.ParseAddr(ip)
	if err != nil || addr.Zone() != "" {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("ip %q is not a literal IP address", ip))
		return netip.Addr{}, BanStore{}, false
	}
	for _, s := range h.stores {
		if s.Name == name {
			return addr.Unmap(), s, true
		}
	}
	WriteError(w, http.StatusBadRequest, fmt.Sprintf("unknown store %q", name))
	return netip.Addr{}, BanStore{}, false
}

// DecodePost reads a POST application/json body of at most MaxBody
// bytes with no unknown fields into v, answering the error itself when it
// cannot. The content type check keeps a browser's cross-site form post,
// which cannot send application/json without a preflight, from writing.
func DecodePost(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return false
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		WriteError(w, http.StatusUnsupportedMediaType, "content type must be application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			WriteError(w, http.StatusRequestEntityTooLarge, "body too large")
			return false
		}
		WriteError(w, http.StatusBadRequest, "bad json: "+err.Error())
		return false
	}
	if dec.More() {
		WriteError(w, http.StatusBadRequest, "bad json: trailing data")
		return false
	}
	return true
}

// LoopbackPeer reports whether remote, a request's RemoteAddr, is a
// loopback address. Every debug API answers only such peers, even when the
// listener is bound wider.
func LoopbackPeer(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return false
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.Unmap().IsLoopback()
}

// WriteError answers status with a JSON {"error": msg} body.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// WriteJSON answers status with v encoded as JSON.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // the client is gone if this fails
}
