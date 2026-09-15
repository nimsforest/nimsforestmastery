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

func TestHealthFallbackDownIsDegraded(t *testing.T) {
	// Soil and iamnim healthy, only the unused nimregistry fallback down.
	//
	// This case previously answered 200 so that an orchestrator would not
	// restart a portal that serves every route. The protection against
	// that is the land role declaring no health block, which is the fleet
	// pattern for these tools, not a health surface that withholds the
	// truth from every other reader. The status code now matches the
	// status field, and the detail names the one path that is down.
	iamnim := stubServer(t, http.StatusUnauthorized)
	code, body := probeHealth(t, healthySoil{}, "http://127.0.0.1:1", iamnim.URL)
	if code != http.StatusServiceUnavailable {
		t.Errorf("code = %d, want 503 to match the degraded status", code)
	}
	if body["status"] != "degraded" {
		t.Errorf("status = %v, want degraded", body["status"])
	}
	checks, ok := body["checks"].(map[string]any)
	if !ok || checks["nimregistry"] == "ok" || checks["soil"] != "ok" {
		t.Errorf("checks = %v, want only nimregistry failing", body["checks"])
	}
}

func TestHealthIamnimDownIsDegraded(t *testing.T) {
	// Honest status: a portal whose identity service is down is degraded,
	// and says so in the status code as well as the detail. It keeps
	// serving; the land role declares no health block, so nothing
	// restarts it over this.
	registry := stubServer(t, http.StatusOK)
	code, body := probeHealth(t, healthySoil{}, registry.URL, "http://127.0.0.1:1")
	if code != http.StatusServiceUnavailable || body["status"] != "degraded" {
		t.Errorf("code = %d, status = %v, want 503 degraded", code, body["status"])
	}
	if checks, ok := body["checks"].(map[string]any); !ok || checks["iamnim"] == "ok" {
		t.Errorf("checks = %v, want iamnim named as the failure", body["checks"])
	}
}

func TestHealthNoReadPathIs503(t *testing.T) {
	// Soil disabled and the fallback down: the portal cannot serve roles
	// at all. Both failing checks are named, which is how an operator
	// tells this apart from a single degraded path.
	iamnim := stubServer(t, http.StatusUnauthorized)
	code, body := probeHealth(t, soil.Disabled{Reason: "no nats"}, "http://127.0.0.1:1", iamnim.URL)
	if code != http.StatusServiceUnavailable {
		t.Errorf("code = %d, want 503: no role read path is left", code)
	}
	checks, ok := body["checks"].(map[string]any)
	if !ok || checks["soil"] == "ok" || checks["nimregistry"] == "ok" {
		t.Errorf("checks = %v, want both soil and nimregistry named as failures", body["checks"])
	}
}

func TestHealthSoilDownFallbackUpIsDegraded(t *testing.T) {
	// The portal still serves through the fallback, and still reports the
	// truth: degraded, with soil named.
	registry := stubServer(t, http.StatusOK)
	iamnim := stubServer(t, http.StatusUnauthorized)
	code, body := probeHealth(t, soil.Disabled{Reason: "no nats"}, registry.URL, iamnim.URL)
	if code != http.StatusServiceUnavailable || body["status"] != "degraded" {
		t.Errorf("code = %d, status = %v, want 503 degraded", code, body["status"])
	}
	checks, ok := body["checks"].(map[string]any)
	if !ok || checks["soil"] == "ok" || checks["nimregistry"] != "ok" {
		t.Errorf("checks = %v, want soil failing and nimregistry ok", body["checks"])
	}
}
