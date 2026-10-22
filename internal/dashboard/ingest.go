package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Doublefire-Chen/Telltale/internal/dashboard/store"
	v1 "github.com/Doublefire-Chen/Telltale/internal/protocol/v1"
)

// authenticateAgent identifies the Host by its Bearer token, answering 401 if
// it is missing or unknown. Per ADR 0001 responses to Agents are bare status
// codes: Agents read nothing else, so nothing else is sent.
func (s *Server) authenticateAgent(w http.ResponseWriter, r *http.Request) (store.Host, bool) {
	token, ok := bearerToken(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return store.Host{}, false
	}
	host, ok, err := s.store.HostByTokenHash(r.Context(), hashToken(token))
	if err != nil {
		s.logger.Error("look up Host token", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return store.Host{}, false
	}
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return store.Host{}, false
	}
	return host, true
}

// decodeAgentBody reads a protocol v1 body into v, answering 400 if it is
// malformed. Unknown fields are ignored so that newer Agents keep working (ADR 0002).
func decodeAgentBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, v1.MaxBodyBytes)).Decode(v); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return false
	}
	return true
}

// handleReport accepts a Report from an Agent.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	host, ok := s.authenticateAgent(w, r)
	if !ok {
		return
	}
	var rep v1.Report
	if !decodeAgentBody(w, r, &rep) {
		return
	}
	// Never reject an Agent over its interval; treat anything too short as the minimum.
	if minMS := v1.MinInterval.Milliseconds(); rep.IntervalMS < minMS {
		rep.IntervalMS = minMS
	}

	prev, havePrev := s.latest.get(host.ID)
	view := withBandwidth(reportView{ReceivedAt: s.now().UnixMilli(), Report: rep}, prev, havePrev)
	s.latest.set(host.ID, view)
	s.hub.publish(liveMessage{Type: "report", HostID: host.ID, Report: &view})
	w.WriteHeader(http.StatusNoContent)
}

// handleHostInfo accepts Host Info from an Agent and stores it.
func (s *Server) handleHostInfo(w http.ResponseWriter, r *http.Request) {
	host, ok := s.authenticateAgent(w, r)
	if !ok {
		return
	}
	var info v1.HostInfo
	if !decodeAgentBody(w, r, &info) {
		return
	}
	stored, err := json.Marshal(info)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := s.store.SetHostInfo(r.Context(), host.ID, stored); err != nil {
		s.logger.Error("store Host Info", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	s.hub.publish(liveMessage{Type: "hosts_changed"})
	w.WriteHeader(http.StatusNoContent)
}

func bearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}
