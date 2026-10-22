package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"log/slog"
	"net"
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

// How often the Agent re-reads Host Info to look for changes, and how often it
// sends Host Info even if nothing changed (so a restarted Dashboard, or a
// changed IP address, catches up).
const (
	hostInfoCheckEvery   = time.Minute
	hostInfoRefreshEvery = time.Hour
)

// Sender sends Host Info and a Report every interval to the Dashboard.
type Sender struct {
	reportURL   string
	hostInfoURL string
	token       string
	interval    time.Duration
	version     string
	collector   Collector
	logger      *slog.Logger
	client      *http.Client
	now         func() time.Time

	hostInfoCheck, hostInfoRefresh time.Duration

	// lastStatus is the previous Report's outcome (-1 before the first send).
	lastStatus int

	// infoChannels send Host Info once over IPv4 and once over IPv6, so the
	// Dashboard learns both of the Host's addresses (ADR 0004).
	infoChannels  []*infoChannel
	infoCheckedAt time.Time
}

// infoChannel is Host Info delivery over one IP family.
type infoChannel struct {
	family string // "tcp4" or "tcp6"
	client *http.Client
	// delivered is the last Host Info the Dashboard accepted, at deliveredAt;
	// pending is Host Info waiting to be sent (nil if none), not before retryAt.
	delivered   []byte
	deliveredAt time.Time
	pending     []byte
	retryAt     time.Time
}

// NewSender builds a Sender for cfg. version is sent as the Agent's release version.
func NewSender(cfg Config, collector Collector, version string, logger *slog.Logger) *Sender {
	return newSender(cfg.DashboardURL, cfg.Token, cfg.Interval.Duration, version, collector, logger, nil)
}

func newSender(dashboardURL, token string, interval time.Duration, version string, collector Collector, logger *slog.Logger, rootCAs *x509.CertPool) *Sender {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	base := strings.TrimSuffix(dashboardURL, "/")
	return &Sender{
		reportURL:   base + v1.ReportPath,
		hostInfoURL: base + v1.HostInfoPath,
		token:       token,
		interval:    interval,
		version:     version,
		collector:   collector,
		logger:      logger,
		client:      newClient(rootCAs, "tcp"),
		infoChannels: []*infoChannel{
			{family: "tcp4", client: newClient(rootCAs, "tcp4")},
			{family: "tcp6", client: newClient(rootCAs, "tcp6")},
		},
		now:             time.Now,
		hostInfoCheck:   hostInfoCheckEvery,
		hostInfoRefresh: hostInfoRefreshEvery,
		lastStatus:      -1,
	}
}

// newClient returns an HTTP client that talks only to the Dashboard: it
// prefers HTTP/2, never follows redirects, and keeps no cookies. network is
// "tcp", or "tcp4"/"tcp6" to force one IP family.
func newClient(rootCAs *x509.CertPool, network string) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.ForceAttemptHTTP2 = true
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	tr.DialContext = func(ctx context.Context, _, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, addr)
	}
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

// Run sends Host Info and a Report immediately, then a Report every interval
// (with Host Info whenever it is due) until ctx ends.
func (s *Sender) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	s.checkHostInfo(ctx)
	for {
		s.sendHostInfo(ctx)
		s.sendReport(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if s.now().Sub(s.infoCheckedAt) >= s.hostInfoCheck {
			s.checkHostInfo(ctx)
		}
	}
}

// checkHostInfo reads Host Info and queues it on each IP family where it
// changed or a refresh is due.
func (s *Sender) checkHostInfo(ctx context.Context) {
	now := s.now()
	s.infoCheckedAt = now
	info := s.collector.HostInfo(ctx)
	info.AgentVersion = s.version
	body, err := json.Marshal(info)
	if err != nil {
		s.logger.Error("encode Host Info", "err", err)
		return
	}
	for _, ch := range s.infoChannels {
		if !bytes.Equal(body, ch.delivered) || now.Sub(ch.deliveredAt) >= s.hostInfoRefresh {
			ch.pending = body
		}
	}
}

// sendHostInfo sends queued Host Info on each IP family. It stays queued until
// the Dashboard accepts it. If a family cannot connect at all (no IPv6 route,
// say), it is retried at the next check rather than every interval.
func (s *Sender) sendHostInfo(ctx context.Context) {
	now := s.now()
	for _, ch := range s.infoChannels {
		if ch.pending == nil || now.Before(ch.retryAt) {
			continue
		}
		switch status := s.postWith(ctx, ch.client, s.hostInfoURL, ch.pending); {
		case status >= 200 && status < 300:
			ch.delivered, ch.deliveredAt, ch.pending = ch.pending, now, nil
		case status == 0:
			ch.retryAt = now.Add(s.hostInfoCheck)
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
	s.observe(s.postWith(ctx, s.client, s.reportURL, body))
}

// postWith sends one request body and returns the response status code, or 0
// if no response arrived. The status code is the only thing read from the response.
func (s *Sender) postWith(ctx context.Context, client *http.Client, url string, body []byte) int {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		s.logger.Error("build request", "err", err)
		return 0
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Debug("send to Dashboard", "url", url, "err", err)
		}
		return 0
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrain))
	resp.Body.Close()
	return resp.StatusCode
}

// observe logs when the outcome of sending Reports changes, rather than on every Report.
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
