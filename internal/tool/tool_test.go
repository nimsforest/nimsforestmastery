package tool

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nimsforest/nimsforestmastery/internal/roles"
	"github.com/nimsforest/nimsforestmastery/internal/soil"
)

// healthySoil is a Reader whose probe read succeeds.
type healthySoil struct{}

func (healthySoil) Get(string) (soil.Entry, error)    { return soil.Entry{}, soil.ErrNotFound }
func (healthySoil) List(string) ([]soil.Entry, error) { return nil, nil }
func (healthySoil) Disabled() bool                    { return false }

func stubServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return server
}

func probeHealth(t *testing.T, reader soil.Reader, registryURL, iamnimURL string) (int, map[string]any) {
	t.Helper()
	handler := HealthHandler(reader, roles.NewRegistryClient(registryURL), iamnimURL)
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("health body is not JSON: %s", rec.Body.String())
	}
	return rec.Code, body
}

func TestHealthAllChecksPass(t *testing.T) {
	registry := stubServer(t, http.StatusOK)
	iamnim := stubServer(t, http.StatusUnauthorized) // anonymous 401 is healthy
	code, body := probeHealth(t, healthySoil{}, registry.URL, iamnim.URL)
	if code != http.StatusOK || body["status"] != "ok" {
		t.Errorf("code = %d, status = %v, want 200 ok", code, body["status"])
	}
}

func TestHealthFallbackDownIsDegradedNotDead(t *testing.T) {
	// The reviewer repro: Soil and iamnim healthy, only the unused
	// nimregistry fallback down. The portal serves every route, so an
	// orchestrator must not restart it: 200 with status degraded.
	iamnim := stubServer(t, http.StatusUnauthorized)
	code, body := probeHealth(t, healthySoil{}, "http://127.0.0.1:1", iamnim.URL)
	if code != http.StatusOK {
		t.Errorf("code = %d, want 200: only the fallback is down", code)
	}
	if body["status"] != "degraded" {
		t.Errorf("status = %v, want degraded", body["status"])
	}
}

func TestHealthIamnimDownIsDegradedNotDead(t *testing.T) {
	registry := stubServer(t, http.StatusOK)
	code, body := probeHealth(t, healthySoil{}, registry.URL, "http://127.0.0.1:1")
	if code != http.StatusOK || body["status"] != "degraded" {
		t.Errorf("code = %d, status = %v, want 200 degraded", code, body["status"])
	}
}

func TestHealthNoReadPathIs503(t *testing.T) {
	// Soil disabled and the fallback down: the portal cannot serve
	// roles at all, the honest 503.
	iamnim := stubServer(t, http.StatusUnauthorized)
	code, body := probeHealth(t, soil.Disabled{Reason: "no nats"}, "http://127.0.0.1:1", iamnim.URL)
	if code != http.StatusServiceUnavailable {
		t.Errorf("code = %d, want 503: no role read path is left", code)
	}
	if body["status"] != "unavailable" {
		t.Errorf("status = %v, want unavailable", body["status"])
	}
}

func TestHealthSoilDownFallbackUpIsDegraded(t *testing.T) {
	registry := stubServer(t, http.StatusOK)
	iamnim := stubServer(t, http.StatusUnauthorized)
	code, body := probeHealth(t, soil.Disabled{Reason: "no nats"}, registry.URL, iamnim.URL)
	if code != http.StatusOK || body["status"] != "degraded" {
		t.Errorf("code = %d, status = %v, want 200 degraded", code, body["status"])
	}
}
