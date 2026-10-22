package dashboard

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/Doublefire-Chen/Telltale/internal/dashboard/store"
)

// HostTokenPrefix starts every Host token, so leaked tokens are easy to recognise.
const HostTokenPrefix = "tt_"

const maxHostNameLen = 64

type hostView struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Report *reportView `json:"report"`
}

type statusResponse struct {
	Hosts []hostView `json:"hosts"`
}

// handleStatus serves the Status page: every Host with its latest Report.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.store.Hosts(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	resp := statusResponse{Hosts: make([]hostView, 0, len(hosts))}
	for _, h := range hosts {
		v := hostView{ID: h.ID, Name: h.Name}
		if rep, ok := s.latest.get(h.ID); ok {
			v.Report = &rep
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
	name := strings.TrimSpace(req.Name)
	if name == "" || utf8.RuneCountInString(name) > maxHostNameLen {
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
