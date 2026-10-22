package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	v1 "github.com/Doublefire-Chen/Telltale/internal/protocol/v1"
)

const (
	requestTimeout = 10 * time.Second
	// maxDrain bounds how much of a response is read (and discarded) so the
	// connection can be reused.
	maxDrain = 4 << 10
)

// Sender sends a Report to the Dashboard every interval.
type Sender struct {
	reportURL string
	token     string
	interval  time.Duration
	collector Collector
	logger    *slog.Logger
	client    *http.Client
	now       func() time.Time

	// lastStatus is the previous send's outcome (-1 before the first send).
	lastStatus int
}

// NewSender builds a Sender for cfg.
func NewSender(cfg Config, collector Collector, logger *slog.Logger) *Sender {
	return newSender(cfg.DashboardURL, cfg.Token, cfg.Interval.Duration, collector, logger, nil)
}

func newSender(dashboardURL, token string, interval time.Duration, collector Collector, logger *slog.Logger, rootCAs *x509.CertPool) *Sender {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Sender{
		reportURL:  strings.TrimSuffix(dashboardURL, "/") + v1.ReportPath,
		token:      token,
		interval:   interval,
		collector:  collector,
		logger:     logger,
		client:     newClient(rootCAs),
		now:        time.Now,
		lastStatus: -1,
	}
}

// newClient returns an HTTP client that talks only to the Dashboard: it
// prefers HTTP/2, never follows redirects, and keeps no cookies.
func newClient(rootCAs *x509.CertPool) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.ForceAttemptHTTP2 = true
	if rootCAs != nil {
		tr.TLSClientConfig = &tls.Config{RootCAs: rootCAs}
	}
	return &http.Client{
		Transport: tr,
		Timeout:   requestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// Run sends a Report immediately and then every interval until ctx ends.
func (s *Sender) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		s.sendReport(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Sender) sendReport(ctx context.Context) {
	rep := s.collector.Sample(ctx)
	rep.Time = s.now().UnixMilli()
	rep.IntervalMS = s.interval.Milliseconds()
	body, err := json.Marshal(rep)
	if err != nil {
		s.logger.Error("encode Report", "err", err)
		return
	}
	s.observe(s.post(ctx, body))
}

// post sends one Report and returns the response status code, or 0 if no
// response arrived. The status code is the only thing read from the response.
func (s *Sender) post(ctx context.Context, body []byte) int {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.reportURL, bytes.NewReader(body))
	if err != nil {
		s.logger.Error("build request", "err", err)
		return 0
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Debug("send Report", "err", err)
		}
		return 0
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrain))
	resp.Body.Close()
	return resp.StatusCode
}

// observe logs when the outcome of sending changes, rather than on every Report.
func (s *Sender) observe(status int) {
	if status == s.lastStatus {
		return
	}
	s.lastStatus = status
	switch {
	case status >= 200 && status < 300:
		s.logger.Info("reporting to Dashboard", "url", s.reportURL)
	case status == http.StatusUnauthorized:
		s.logger.Error("Dashboard rejected the Host token; check token in the config file", "url", s.reportURL)
	case status == 0:
		s.logger.Warn("cannot reach Dashboard; will keep trying", "url", s.reportURL)
	default:
		s.logger.Warn("Dashboard refused Report", "status", status, "url", s.reportURL)
	}
}
