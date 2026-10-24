package dashboard

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Doublefire-Chen/Telltale/internal/dashboard/store"
	v1 "github.com/Doublefire-Chen/Telltale/internal/protocol/v1"
	"github.com/Doublefire-Chen/Telltale/internal/version"
)

// HostTokenPrefix starts every Host token, so leaked tokens are easy to recognise.
const HostTokenPrefix = "tt_"

const (
	maxHostNameLen  = 64
	maxGroupNameLen = 32
)

type hostView struct {
	ID string `json:"id"`
	// Name is the display name; empty means the frontend shows the hostname.
	Name     string       `json:"name"`
	HostInfo *v1.HostInfo `json:"host_info"`
	Report   *reportView  `json:"report"`
	// Online is set while the Host keeps reporting; false once it has
	// missed three intervals (Offline) or before its first Report.
	Online bool `json:"online"`
	// AgentOutdated is set when the Host's Agent is older than this Dashboard.
	AgentOutdated bool `json:"agent_outdated"`
	// Countries are the ISO codes to show as flags: the manual override if
	// set, else the distinct countries of the Host's IPv4 and IPv6 addresses.
	Countries []string `json:"countries"`
	// IPv4 and IPv6 are the Host's addresses (admin only).
	IPv4 string `json:"ipv4,omitempty"`
	IPv6 string `json:"ipv6,omitempty"`
	// CountryOverride is the admin's manual country, or "".
	CountryOverride string `json:"country_override"`
	// Group is the group the Host is shown under, or "".
	Group string `json:"group"`
	// Hidden Hosts are left off the public Status page.
	Hidden bool `json:"hidden"`
	// PublicHistory lets visitors of a public Status page open the Host's history.
	PublicHistory bool `json:"public_history"`
}

// addressMaxAge is how long an address stays shown without being seen again.
// Agents resend Host Info hourly, so an address missing for longer than this
// (e.g. a Host that lost IPv6) is gone.
const addressMaxAge = 3 * time.Hour

type statusResponse struct {
	// DashboardVersion is this Dashboard's release version.
	DashboardVersion string     `json:"dashboard_version"`
	Hosts            []hostView `json:"hosts"`
}

// handleStatus serves the Status page: every Host with its latest Report.
// Anonymous viewers of a public Status page get no Hidden Hosts, IP addresses
// or Dashboard-side settings; the filtering happens here, not in the browser.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	admin, err := s.authenticated(r)
	if err != nil {
		s.internalError(w, err)
		return
	}
	hosts, err := s.store.Hosts(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	resp := statusResponse{DashboardVersion: s.version, Hosts: make([]hostView, 0, len(hosts))}
	for _, h := range hosts {
		if h.Hidden && !admin {
			continue
		}
		v := hostView{ID: h.ID, Name: h.Name, Group: h.Group, Hidden: h.Hidden, PublicHistory: h.PublicHistory}
		if h.HostInfo != nil {
			var info v1.HostInfo
			if err := json.Unmarshal(h.HostInfo, &info); err == nil {
				v.HostInfo = &info
				v.AgentOutdated = version.Older(info.AgentVersion, s.version)
			}
		}
		if rep, ok := s.latest.get(h.ID); ok {
			v.Report = &rep
			v.Online = online(rep, s.now())
		}
		s.addAddresses(&v, h)
		if !admin {
			v.IPv4, v.IPv6, v.CountryOverride = "", "", ""
		}
		resp.Hosts = append(resp.Hosts, v)
	}
	writeJSON(w, http.StatusOK, resp)
}

type addHostRequest struct {
	Name string `json:"name"`
}

type addHostResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token"`
}

// handleAddHost registers a Host and returns its token. This is the only time
// the token is revealed; the Dashboard keeps just its hash.
func (s *Server) handleAddHost(w http.ResponseWriter, r *http.Request) {
	var req addHostRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	// An empty name is allowed: the Status page then shows the Host's hostname.
	name := strings.TrimSpace(req.Name)
	if utf8.RuneCountInString(name) > maxHostNameLen {
		writeError(w, http.StatusBadRequest, "invalid_name")
		return
	}

	id, err := newHostID()
	if err != nil {
		s.internalError(w, err)
		return
	}
	token, err := newHostToken()
	if err != nil {
		s.internalError(w, err)
		return
	}
	host := store.Host{ID: id, Name: name, CreatedAt: s.now()}
	if err := s.store.CreateHost(r.Context(), host, hashToken(token)); err != nil {
		s.internalError(w, err)
		return
	}
	s.hostsChanged()
	writeJSON(w, http.StatusCreated, addHostResponse{ID: id, Name: name, Token: token})
}

func newHostToken() (string, error) {
	secret, err := newToken()
	if err != nil {
		return "", err
	}
	return HostTokenPrefix + secret, nil
}

func newHostID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Server) addAddresses(v *hostView, h store.Host) {
	now := s.now()
	v.CountryOverride = h.CountryOverride
	v.Countries = []string{}
	var auto []string
	for _, a := range []struct {
		addr netip.Addr
		seen time.Time
		dst  *string
	}{{h.IPv4, h.IPv4SeenAt, &v.IPv4}, {h.IPv6, h.IPv6SeenAt, &v.IPv6}} {
		if !a.addr.IsValid() || now.Sub(a.seen) > addressMaxAge {
			continue
		}
		*a.dst = a.addr.String()
		if c := s.geoip.Country(a.addr); c != "" && !slices.Contains(auto, c) {
			auto = append(auto, c)
		}
	}
	if h.CountryOverride != "" {
		v.Countries = []string{h.CountryOverride}
	} else if auto != nil {
		v.Countries = auto
	}
}

type updateHostRequest struct {
	Name            *string `json:"name"`
	Group           *string `json:"group"`
	Hidden          *bool   `json:"hidden"`
	PublicHistory   *bool   `json:"public_history"`
	CountryOverride *string `json:"country_override"`
}

// handleUpdateHost changes a Host's Dashboard-side settings: display name,
// group, Hidden flag, public history and country override. Nothing is sent to its Agent.
func (s *Server) handleUpdateHost(w http.ResponseWriter, r *http.Request) {
	var req updateHostRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	u := store.HostUpdate{Hidden: req.Hidden, PublicHistory: req.PublicHistory}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if utf8.RuneCountInString(name) > maxHostNameLen {
			writeError(w, http.StatusBadRequest, "invalid_name")
			return
		}
		u.Name = &name
	}
	if req.Group != nil {
		group := strings.TrimSpace(*req.Group)
		if utf8.RuneCountInString(group) > maxGroupNameLen {
			writeError(w, http.StatusBadRequest, "invalid_group")
			return
		}
		u.Group = &group
	}
	if req.CountryOverride != nil {
		country := strings.ToUpper(strings.TrimSpace(*req.CountryOverride))
		if country != "" && !isCountryCode(country) {
			writeError(w, http.StatusBadRequest, "invalid_country")
			return
		}
		u.CountryOverride = &country
	}
	found, err := s.store.UpdateHost(r.Context(), r.PathValue("id"), u)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	s.hostsChanged()
	w.WriteHeader(http.StatusNoContent)
}

type hostOrderRequest struct {
	IDs []string `json:"ids"`
}

// handleSetOrder sets the display order of all Hosts.
func (s *Server) handleSetOrder(w http.ResponseWriter, r *http.Request) {
	var req hostOrderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.store.SetHostOrder(r.Context(), req.IDs); errors.Is(err, store.ErrInvalidOrder) {
		writeError(w, http.StatusBadRequest, "invalid_order")
		return
	} else if err != nil {
		s.internalError(w, err)
		return
	}
	s.hostsChanged()
	w.WriteHeader(http.StatusNoContent)
}

type tokenResponse struct {
	Token string `json:"token"`
}

// handleRegenerateToken replaces a Host's token. The old one stops working at
// once; the new one is revealed only in this response.
func (s *Server) handleRegenerateToken(w http.ResponseWriter, r *http.Request) {
	token, err := newHostToken()
	if err != nil {
		s.internalError(w, err)
		return
	}
	found, err := s.store.SetHostToken(r.Context(), r.PathValue("id"), hashToken(token))
	if err != nil {
		s.internalError(w, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, tokenResponse{Token: token})
}

// handleDeleteHost removes a Host and everything the Dashboard holds about it.
func (s *Server) handleDeleteHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	found, err := s.store.DeleteHost(r.Context(), id)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	s.latest.delete(id)
	s.forget(id)
	s.markOnline(id)
	s.hostsChanged()
	w.WriteHeader(http.StatusNoContent)
}

func isCountryCode(c string) bool {
	return len(c) == 2 && c[0] >= 'A' && c[0] <= 'Z' && c[1] >= 'A' && c[1] <= 'Z'
}

type settingsBody struct {
	StatusPublic bool `json:"status_public"`
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	public, err := s.store.StatusPublic(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settingsBody{StatusPublic: public})
}

// handleSetSettings changes Dashboard settings. Making the Status page private
// disconnects every anonymous viewer at once.
func (s *Server) handleSetSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsBody
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.store.SetStatusPublic(r.Context(), req.StatusPublic); err != nil {
		s.internalError(w, err)
		return
	}
	if !req.StatusPublic {
		s.hub.dropAnonymous()
	}
	w.WriteHeader(http.StatusNoContent)
}
