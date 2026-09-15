// Package tool embeds github.com/nimsforest/nimsforesttool: the
// joining contract (announce, heartbeat, deregister), single tenancy
// through RequireOrg, and the honest health surface with per-check
// detail.
package tool

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	nftool "github.com/nimsforest/nimsforesttool"

	"github.com/nimsforest/nimsforestmastery/internal/roles"
	"github.com/nimsforest/nimsforestmastery/internal/soil"
)

// Name is this service's name on the forest bus and in health output.
const Name = "nimsforestmastery"

// RequireOrg enforces single tenancy: ORG_SLUG must be set and the
// flag value may only agree with it. Call it first and fail fast.
func RequireOrg(expected string) (string, error) {
	return nftool.RequireOrg(expected)
}

// Joiner holds the mycelium registration for one running instance.
type Joiner struct {
	registration *nftool.Registration
}

// Join announces the service on forest.mycelium.register and starts
// the 30 second heartbeat. Call Leave on shutdown.
func Join(nc *nats.Conn, org, version string) (*Joiner, error) {
	registration, err := nftool.RegisterConn(nc, nftool.Info{
		Name:       Name,
		OrgSlug:    org,
		Kind:       "service",
		Version:    version,
		Publishes:  []string{},
		Subscribes: []string{},
	})
	if err != nil {
		return nil, err
	}
	return &Joiner{registration: registration}, nil
}

// Leave stops the heartbeat and drops the deregister leaf. It is safe
// on a nil Joiner, so a failed Join needs no special case.
func (j *Joiner) Leave() {
	if j == nil || j.registration == nil {
		return
	}
	j.registration.Stop()
}

// HealthHandler serves the standard supporting-tool health surface: the
// Soil read path, the nimregistry fallback, and iamnim reachability,
// each with per-check detail.
//
// It delegates to the component rather than reimplementing it. The local
// version answered 200 while degraded, so that a probe would not restart
// a serving instance over a down fallback. That reasoning no longer
// holds: the land role declares no health block, so nothing restarts
// this container on a 503, and answering 200 meant any probe reading
// only the status code saw a portal with its identity service down as
// healthy. Severity is not lost, because the per-check detail still
// names exactly which path failed.
func HealthHandler(reader soil.Reader, registry *roles.RegistryClient, iamnimURL string) http.HandlerFunc {
	return nftool.HealthHandler(Name, map[string]nftool.Check{
		"soil":        soilCheck(reader),
		"nimregistry": registry.Health,
		"iamnim":      iamnimCheck(iamnimURL),
	})
}

// soilCheck reports whether the Soil read path works. In the disabled
// state it says why; when connected it performs one real read so a
// dropped connection shows up.
func soilCheck(reader soil.Reader) nftool.Check {
	return func() error {
		if disabled, ok := reader.(soil.Disabled); ok {
			return errors.New("soil disabled, serving through the nimregistry fallback: " + disabled.Reason)
		}
		if reader.Disabled() {
			return errors.New("soil disabled, serving through the nimregistry fallback")
		}
		_, err := reader.Get("catalog.nims.__health_probe")
		if err != nil && !errors.Is(err, soil.ErrNotFound) {
			return fmt.Errorf("soil read failed: %v", err)
		}
		return nil
	}
}

var iamnimClient = &http.Client{
	Timeout: 5 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// iamnimCheck probes iamnim reachability. An anonymous GET /api/me
// that answers 401 proves the identity service is up; only a network
// failure or a server error degrades the check.
func iamnimCheck(baseURL string) nftool.Check {
	return func() error {
		if baseURL == "" {
			return errors.New("IAMNIM_URL not configured, authenticated routes fail closed")
		}
		resp, err := iamnimClient.Get(strings.TrimRight(baseURL, "/") + "/api/me")
		if err != nil {
			return fmt.Errorf("iamnim unreachable: %v", err)
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
		switch resp.StatusCode {
		case http.StatusOK, http.StatusUnauthorized, http.StatusForbidden:
			return nil
		}
		return fmt.Errorf("iamnim status %d", resp.StatusCode)
	}
}
