package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"

	v1 "github.com/Doublefire-Chen/Telltale/internal/protocol/v1"
)

// handleReport accepts a Report from an Agent. Per ADR 0001 the response is a
// bare status code: Agents read nothing else, so nothing else is sent.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	token, ok := bearerToken(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	host, ok, err := s.store.HostByTokenHash(r.Context(), hashToken(token))
	if err != nil {
		s.logger.Error("look up Host token", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Unknown fields are ignored so that newer Agents keep working (ADR 0002).
	var rep v1.Report
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, v1.MaxBodyBytes)).Decode(&rep); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Never reject an Agent over its interval; treat anything too short as the minimum.
	if minMS := v1.MinInterval.Milliseconds(); rep.IntervalMS < minMS {
		rep.IntervalMS = minMS
	}

	view := reportView{ReceivedAt: s.now().UnixMilli(), Report: rep}
	s.latest.set(host.ID, view)
	s.hub.publish(liveMessage{Type: "report", HostID: host.ID, Report: &view})
	w.WriteHeader(http.StatusNoContent)
}

func bearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}
