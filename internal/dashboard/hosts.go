package dashboard

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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

const maxHostNameLen = 64

type hostView struct {
	ID string `json:"id"`
	// Name is the display name; empty means the frontend shows the hostname.
	Name     string       `json:"name"`
	HostInfo *v1.HostInfo `json:"host_info"`
	Report   *reportView  `json:"report"`
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
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.store.Hosts(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	resp := statusResponse{DashboardVersion: s.version, Hosts: make([]hostView, 0, len(hosts))}
	for _, h := range hosts {
		v := hostView{ID: h.ID, Name: h.Name}
		if h.HostInfo != nil {
			var info v1.HostInfo
			if err := json.Unmarshal(h.HostInfo, &info); err == nil {
				v.HostInfo = &info
				v.AgentOutdated = version.Older(info.AgentVersion, s.version)
			}
		}
		if rep, ok := s.latest.get(h.ID); ok {
			v.Report = &rep
		}
		s.addAddresses(&v, h)
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
	secret, err := newToken()
	if err != nil {
		s.internalError(w, err)
		return
	}
	token := HostTokenPrefix + secret
	host := store.Host{ID: id, Name: name, CreatedAt: s.now()}
	if err := s.store.CreateHost(r.Context(), host, hashToken(token)); err != nil {
		s.internalError(w, err)
		return
	}
	s.hub.publish(liveMessage{Type: "hosts_changed"})
	writeJSON(w, http.StatusCreated, addHostResponse{ID: id, Name: name, Token: token})
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

type setCountryRequest struct {
	Country string `json:"country"`
}

// handleSetCountry sets or clears a Host's manual country override.
func (s *Server) handleSetCountry(w http.ResponseWriter, r *http.Request) {
	var req setCountryRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	country := strings.ToUpper(strings.TrimSpace(req.Country))
	if country != "" && !isCountryCode(country) {
		writeError(w, http.StatusBadRequest, "invalid_country")
		return
	}
	found, err := s.store.SetCountryOverride(r.Context(), r.PathValue("id"), country)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	s.hub.publish(liveMessage{Type: "hosts_changed"})
	w.WriteHeader(http.StatusNoContent)
}

func isCountryCode(c string) bool {
	return len(c) == 2 && c[0] >= 'A' && c[0] <= 'Z' && c[1] >= 'A' && c[1] <= 'Z'
}
