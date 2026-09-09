package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/nimsforest/nimsforestmastery/internal/roles"
	"github.com/nimsforest/nimsforestmastery/internal/soil"
)

// fakeSoil implements soil.Reader over a fixture map.
type fakeSoil struct {
	entries map[string]soil.Entry
}

func newFakeSoil() *fakeSoil {
	return &fakeSoil{entries: make(map[string]soil.Entry)}
}

func (f *fakeSoil) put(key string, revision uint64, value string) {
	f.entries[key] = soil.Entry{Key: key, Value: []byte(value), Revision: revision}
}

func (f *fakeSoil) Get(key string) (soil.Entry, error) {
	entry, ok := f.entries[key]
	if !ok {
		return soil.Entry{}, soil.ErrNotFound
	}
	return entry, nil
}

func (f *fakeSoil) List(prefix string) ([]soil.Entry, error) {
	var entries []soil.Entry
	for key, entry := range f.entries {
		if strings.HasPrefix(key, prefix) {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return entries, nil
}

func (f *fakeSoil) Disabled() bool { return false }

const neoLongDescription = "Neo builds the platform.\n\n" +
	"Performance Metrics:\n\n" +
	"To evaluate Neo's effectiveness, the following KPIs will be tracked:\n\n" +
	"1. Development Velocity: Sprint completion rate.\n" +
	"2. Code Quality: Coverage by tests.\n"

func fixtureSoil() *fakeSoil {
	f := newFakeSoil()
	neo, _ := json.Marshal(map[string]any{
		"name": "neo", "role": "Software Development", "description": "The developer",
		"category": "Engineering", "long_description": neoLongDescription,
		"subjects": []string{"message.neo"},
	})
	f.put("catalog.nims.neo", 11, string(neo))
	f.put("prompts.nims.neo", 21, "Be the developer. Ship working software.")
	f.put("prompts.nims.neo.audit", 22, "Audit ceremony: check the work.")
	f.put("prompts.nims.neo.autonomate.intent", 23, "Autonomate intent ceremony body.")
	f.put("prompts.nims.neo.eos", 24, "EOS ceremony body.")
	f.put("config.nims.neo", 31, `{"agent_type":"opencode","model":"gpt-5","provider":"openai"}`)
	f.put("catalog.tools.bash", 41, `{"name":"Bash","description":"Execute shell commands","assigned_nims":["neo"]}`)
	f.put("catalog.skills.ai.create-issue", 51, `{"name":"create-issue","description":"File an issue","agent_type":"ai","path":"agents/ai/skills/create-issue/SKILL.md","excerpt":"File...","assigned_nims":["neo"]}`)
	f.put("skills.content.ai.create-issue", 61, "Full create-issue skill content body.")
	f.put("catalog.skills.human.campaign-brief", 52, `{"name":"campaign-brief","description":"Write a brief","agent_type":"human","path":"agents/human/skills/campaign-brief/SKILL.md","assigned_nims":["neo"]}`)
	f.put("agenthuman.config", 71, `{"humans":{"cederik":{"backs":["neo","nudge"]},"julie":{"backs":["nudge"]}}}`)
	return f
}

// stubIamnim serves the iamnim /api/me and /api/me/memberships shapes
// for one valid token and one org membership list.
func stubIamnim(t *testing.T, validToken string, orgs []string) *httptest.Server {
	t.Helper()
	authed := func(r *http.Request) bool {
		c, err := r.Cookie("iamnim_session")
		return err == nil && c.Value == validToken
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		if !authed(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_id":"user-1","email":"person@example.com","name":"Person"}`))
	})
	mux.HandleFunc("GET /api/me/memberships", func(w http.ResponseWriter, r *http.Request) {
		if !authed(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		memberships := make([]map[string]any, 0, len(orgs))
		for _, org := range orgs {
			memberships = append(memberships, map[string]any{"organization_slug": org, "is_admin": false})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(memberships)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// newPortal wires a full agent surface behind the auth middleware,
// the same shape main.go serves.
func newPortal(iamnimURL string) http.Handler {
	service := roles.NewService(fixtureSoil(), roles.NewRegistryClient("http://127.0.0.1:1"))
	auth := NewAuth(iamnimURL, "acme")
	return auth.Wrap(NewServer(service, "acme", "test"))
}

const validPAT = "inpat_" + "0000000000000000000000000000000000000000000000000000000000000000"

func TestAuthFailClosed(t *testing.T) {
	iamnim := stubIamnim(t, validPAT, []string{"acme"})

	cases := []struct {
		name       string
		iamnimURL  string
		bearer     string
		cookie     string
		wantStatus int
	}{
		{"no credential", iamnim.URL, "", "", http.StatusUnauthorized},
		{"invalid bearer", iamnim.URL, "inpat_wrong", "", http.StatusUnauthorized},
		{"valid bearer and member", iamnim.URL, validPAT, "", http.StatusOK},
		{"valid session cookie", iamnim.URL, "", validPAT, http.StatusOK},
		{"iamnim not configured", "", validPAT, "", http.StatusServiceUnavailable},
		{"iamnim unreachable", "http://127.0.0.1:1", validPAT, "", http.StatusServiceUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			portal := newPortal(tc.iamnimURL)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil)
			if tc.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "iamnim_session", Value: tc.cookie})
			}
			rec := httptest.NewRecorder()
			portal.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if got := rec.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			if rec.Code == http.StatusUnauthorized && rec.Header().Get("WWW-Authenticate") == "" {
				t.Error("401 without a WWW-Authenticate challenge")
			}
			if rec.Code != http.StatusOK {
				var body map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" {
					t.Errorf("error body is not honest JSON: %s", rec.Body.String())
				}
			}
		})
	}
}

func TestAuthDeniesNonMember(t *testing.T) {
	// The identity is valid but the membership list does not contain
	// the portal's organization.
	iamnim := stubIamnim(t, validPAT, []string{"otherorg"})
	portal := newPortal(iamnim.URL)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil)
	req.Header.Set("Authorization", "Bearer "+validPAT)
	rec := httptest.NewRecorder()
	portal.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body %s", rec.Code, rec.Body.String())
	}
}

func TestAuthFailsClosedOnIdentityServerError(t *testing.T) {
	// A broken identity service must never become an allow.
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(broken.Close)

	portal := newPortal(broken.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil)
	req.Header.Set("Authorization", "Bearer "+validPAT)
	rec := httptest.NewRecorder()
	portal.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body %s", rec.Code, rec.Body.String())
	}
}

func TestHealthStaysOutsideAuth(t *testing.T) {
	// main.go mounts /health on the outer mux, before the auth wrap.
	// This test pins the wiring shape: a request through the wrapped
	// surface is challenged, the outer health route is not.
	iamnim := stubIamnim(t, validPAT, []string{"acme"})
	portal := newPortal(iamnim.URL)

	outer := http.NewServeMux()
	outer.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	outer.Handle("/", portal)

	rec := httptest.NewRecorder()
	outer.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	outer.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated api status = %d, want 401", rec.Code)
	}
}
