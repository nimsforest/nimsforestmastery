package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/nimsforest/nimsforestmastery/internal/api"
	"github.com/nimsforest/nimsforestmastery/internal/doctrine"
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

const (
	promptText        = "Be the developer. Ship working software."
	ceremonyText      = "Audit ceremony: check the work."
	skillContent      = "Full create-issue skill content body."
	humanSkillContent = "Full review-code drill content body."
	validSession      = "0000000000000000000000000000000000000000000000000000000000000000"
)

func fixtureSoil() *fakeSoil {
	f := newFakeSoil()
	neo, _ := json.Marshal(map[string]any{
		"name": "neo", "role": "Software Development", "description": "The developer",
		"category": "Engineering", "long_description": "Neo builds the platform.",
		"subjects": []string{"message.neo"},
	})
	f.put("catalog.nims.neo", 11, string(neo))
	f.put("catalog.nims.nudge", 12, `{"name":"nudge","role":"Marketing","description":"Growth","category":"Growth"}`)
	f.put("prompts.nims.neo", 21, promptText)
	f.put("prompts.nims.neo.audit", 22, ceremonyText)
	f.put("config.nims.neo", 31, `{"agent_type":"opencode"}`)
	f.put("catalog.tools.bash", 41, `{"name":"Bash","description":"Execute shell commands","assigned_nims":["neo"]}`)
	f.put("catalog.skills.ai.create-issue", 51, `{"name":"create-issue","description":"File an issue","agent_type":"ai","path":"agents/ai/skills/create-issue/SKILL.md","assigned_nims":["neo"]}`)
	f.put("skills.content.ai.create-issue", 61, skillContent)
	f.put("catalog.skills.human.review-code", 52, `{"name":"review-code","description":"Review a change","agent_type":"human","path":"agents/human/skills/review-code/SKILL.md","assigned_nims":["neo"]}`)
	f.put("skills.content.human.review-code", 62, humanSkillContent)
	f.put("agenthuman.config", 71, `{"humans":{"cederik":{"backs":["neo"]}}}`)
	return f
}

// stubIamnim serves the two iamnim endpoints for one valid credential.
func stubIamnim(t *testing.T, valid string, orgs []string) *httptest.Server {
	t.Helper()
	authed := func(r *http.Request) bool {
		c, err := r.Cookie("iamnim_session")
		return err == nil && c.Value == valid
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		if !authed(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
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
		_ = json.NewEncoder(mux2json(w)).Encode(memberships)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func mux2json(w http.ResponseWriter) http.ResponseWriter {
	w.Header().Set("Content-Type", "application/json")
	return w
}

// contentDir writes one doctrine unit fixture and returns the dir.
func contentDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	docDir := filepath.Join(dir, "doctrine")
	if err := os.MkdirAll(docDir, 0o755); err != nil {
		t.Fatal(err)
	}
	unit := "# What a forest is\n\nAn organization has lands. Each land grows a forest.\n"
	if err := os.WriteFile(filepath.Join(docDir, "01-what-a-forest-is.md"), []byte(unit), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// newStack wires the full portal mux the same shape main.go serves:
// public directory and doctrine, SSO-wrapped role pages, Bearer-auth
// agent routes, and the /roles/ dispatch between them.
func newStack(t *testing.T, reader soil.Reader, iamnimURL, baseURL string) (http.Handler, *roles.Service) {
	t.Helper()
	service := roles.NewService(reader, roles.NewRegistryClient("http://127.0.0.1:1"))
	agentRoutes := api.NewAuth(iamnimURL, "acme").Wrap(api.NewServer(service, "acme", "test"))
	webServer := NewServer(service, doctrine.NewStore(contentDir(t)), "acme", "test")
	roleWebRoutes := NewAuth(iamnimURL, "acme", baseURL).Wrap(webServer.RolePages())

	mux := http.NewServeMux()
	mux.Handle("/api/", agentRoutes)
	mux.Handle("GET /llms.txt", agentRoutes)
	mux.Handle("/roles/", DispatchRoles(agentRoutes, roleWebRoutes))
	mux.Handle("/", webServer.Public())
	return mux, service
}

func get(t *testing.T, h http.Handler, path string, cookie string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPublicDirectoryEqualsEnumeration(t *testing.T) {
	iamnim := stubIamnim(t, validSession, []string{"acme"})
	stack, service := newStack(t, fixtureSoil(), iamnim.URL, "https://mastery.acme.example")

	rec := get(t, stack, "/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()

	// The testbook equality check: the rendered role list equals the
	// live catalog enumeration, link for link.
	list, err := service.ListRoles()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatal("enumeration is empty")
	}
	for _, role := range list {
		link := `href="/roles/` + role.Name + `"`
		if strings.Count(body, link) != 1 {
			t.Errorf("directory does not render exactly one link %s", link)
		}
	}
	if got := strings.Count(body, `href="/roles/`); got != len(list) {
		t.Errorf("directory renders %d role links, enumeration has %d", got, len(list))
	}
}

func TestPublicPagesLeakNoRoleContent(t *testing.T) {
	iamnim := stubIamnim(t, validSession, []string{"acme"})
	stack, _ := newStack(t, fixtureSoil(), iamnim.URL, "https://mastery.acme.example")

	for _, path := range []string{"/", "/doctrine", "/doctrine/what-a-forest-is"} {
		rec := get(t, stack, path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
		body := rec.Body.String()
		if strings.Contains(body, promptText) {
			t.Errorf("%s leaks the behavior prompt", path)
		}
		if strings.Contains(body, ceremonyText) {
			t.Errorf("%s leaks a ceremony prompt", path)
		}
		if strings.Contains(body, skillContent) || strings.Contains(body, humanSkillContent) {
			t.Errorf("%s leaks skill content", path)
		}
	}
}

func TestDoctrinePages(t *testing.T) {
	iamnim := stubIamnim(t, validSession, []string{"acme"})
	stack, _ := newStack(t, fixtureSoil(), iamnim.URL, "https://mastery.acme.example")

	rec := get(t, stack, "/doctrine", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("index status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `href="/doctrine/what-a-forest-is"`) {
		t.Errorf("doctrine index does not link the unit\n%s", rec.Body.String())
	}

	rec = get(t, stack, "/doctrine/what-a-forest-is", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("unit status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Each land grows a forest.") {
		t.Errorf("unit page does not render the content\n%s", rec.Body.String())
	}

	if rec = get(t, stack, "/doctrine/nope", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown unit status = %d, want 404", rec.Code)
	}
}

func TestRolePagesRedirectToLogin(t *testing.T) {
	iamnim := stubIamnim(t, validSession, []string{"acme"})
	stack, _ := newStack(t, fixtureSoil(), iamnim.URL, "https://mastery.acme.example")

	for _, path := range []string{"/roles/neo", "/roles/neo/learn", "/roles/neo/changelog"} {
		rec := get(t, stack, path, "")
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%s status = %d, want 303", path, rec.Code)
		}
		location := rec.Header().Get("Location")
		if !strings.HasPrefix(location, iamnim.URL+"/login?redirect_uri=") {
			t.Errorf("%s redirects to %q, not the iamnim login", path, location)
		}
		if !strings.Contains(location, "mastery.acme.example") {
			t.Errorf("%s login redirect misses the return target: %q", path, location)
		}
	}
}

func TestRolePagesRenderWithSession(t *testing.T) {
	iamnim := stubIamnim(t, validSession, []string{"acme"})
	stack, _ := newStack(t, fixtureSoil(), iamnim.URL, "https://mastery.acme.example")

	rec := get(t, stack, "/roles/neo", validSession)
	if rec.Code != http.StatusOK {
		t.Fatalf("meet status = %d, body %s", rec.Code, rec.Body.String())
	}
	meet := rec.Body.String()
	for _, want := range []string{
		"Software Development", "message.neo", "Neo builds the platform.", "catalog.nims.neo",
		"Backing humans", "cederik", // who already backs it, from agenthuman.config
	} {
		if !strings.Contains(meet, want) {
			t.Errorf("meet page is missing %q", want)
		}
	}

	rec = get(t, stack, "/roles/neo/learn", validSession)
	if rec.Code != http.StatusOK {
		t.Fatalf("learn status = %d", rec.Code)
	}
	learn := rec.Body.String()
	for _, want := range []string{
		promptText,
		ceremonyText,        // ceremony subkeys render in the behavior unit
		"review-code",       // the human-archetype drill unit
		humanSkillContent,   // rendered in full for the human learner
		"create-issue",      // the agent variant is listed
		"What a forest is",  // shared doctrine link
		"catalog.organize.", // the data context note
		"Bash",
	} {
		if !strings.Contains(learn, want) {
			t.Errorf("learn page is missing %q", want)
		}
	}
	// The agent-archetype skill is listed, never rendered as a human
	// unit: the design splits the Learn units per audience.
	if strings.Contains(learn, skillContent) {
		t.Error("learn page renders agent-archetype skill content as a human unit")
	}

	rec = get(t, stack, "/roles/neo/changelog", validSession)
	if rec.Code != http.StatusOK {
		t.Fatalf("changelog status = %d", rec.Code)
	}
	changelog := rec.Body.String()
	for _, want := range []string{
		"catalog.nims.neo", "prompts.nims.neo", "config.nims.neo",
		"prompts.nims.neo.audit", "agenthuman.config",
		"skills.content.ai.create-issue",
		">11<", ">21<", ">22<", ">31<", ">61<", ">71<", // the revision stamps
		"Soil history", // the honest diff note
	} {
		if !strings.Contains(changelog, want) {
			t.Errorf("changelog page is missing %q", want)
		}
	}

	if rec = get(t, stack, "/roles/nobody", validSession); rec.Code != http.StatusNotFound {
		t.Errorf("unknown role status = %d, want 404", rec.Code)
	}
}

// startLogin performs the redirect-to-login step and returns the
// state value from the redirect_uri and the state cookie it set.
func startLogin(t *testing.T, stack http.Handler, path string) (string, *http.Cookie) {
	t.Helper()
	rec := get(t, stack, path, "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("login redirect status = %d, want 303", rec.Code)
	}
	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	returnURL, err := url.Parse(location.Query().Get("redirect_uri"))
	if err != nil {
		t.Fatal(err)
	}
	state := returnURL.Query().Get("state")
	if state == "" {
		t.Fatal("login redirect_uri carries no state")
	}
	var stateCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == StateCookie {
			stateCookie = c
		}
	}
	if stateCookie == nil || stateCookie.Value != state {
		t.Fatalf("state cookie = %+v, want value %q", stateCookie, state)
	}
	return state, stateCookie
}

func TestLoginCallbackSetsCookie(t *testing.T) {
	iamnim := stubIamnim(t, validSession, []string{"acme"})
	stack, _ := newStack(t, fixtureSoil(), iamnim.URL, "https://mastery.acme.example")

	// Step 1: the portal starts the login and binds a state.
	state, stateCookie := startLogin(t, stack, "/roles/neo")

	// Step 2: iamnim returns with the token and echoes the state.
	req := httptest.NewRequest(http.MethodGet, "/roles/neo?state="+state+"&token="+validSession, nil)
	req.AddCookie(&http.Cookie{Name: StateCookie, Value: stateCookie.Value})
	rec := httptest.NewRecorder()
	stack.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303, body %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/roles/neo" {
		t.Errorf("redirect strips the token to %q, want /roles/neo", got)
	}
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookie {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("callback set no session cookie")
	}
	if cookie.Value != validSession || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie attributes = %+v", cookie)
	}
}

func TestCallbackWithoutStateSetsNoCookie(t *testing.T) {
	// A bare ?token= link (login CSRF shape) must never become the
	// browser's session: without a matching state the token is
	// ignored and a fresh login starts.
	iamnim := stubIamnim(t, validSession, []string{"acme"})
	stack, _ := newStack(t, fixtureSoil(), iamnim.URL, "https://mastery.acme.example")

	req := httptest.NewRequest(http.MethodGet, "/roles/neo?token="+validSession, nil)
	rec := httptest.NewRecorder()
	stack.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if !strings.HasPrefix(rec.Header().Get("Location"), iamnim.URL+"/login?redirect_uri=") {
		t.Errorf("unbound callback did not restart the login: %q", rec.Header().Get("Location"))
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookie && c.MaxAge >= 0 && c.Value != "" {
			t.Errorf("unbound callback set a session cookie: %+v", c)
		}
	}
}

func TestGarbageTokenCannotEvictValidSession(t *testing.T) {
	// A crafted ?token=garbage link (logout CSRF shape) on a browser
	// that holds a valid session keeps the session and serves.
	iamnim := stubIamnim(t, validSession, []string{"acme"})
	stack, _ := newStack(t, fixtureSoil(), iamnim.URL, "https://mastery.acme.example")

	req := httptest.NewRequest(http.MethodGet, "/roles/neo?token=garbage", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: validSession})
	rec := httptest.NewRecorder()
	stack.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/roles/neo" {
		t.Errorf("redirect = %q, want the same path without the query", got)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookie && c.MaxAge < 0 {
			t.Error("garbage token evicted the valid session cookie")
		}
	}

	// The session still serves after the strip redirect.
	rec = get(t, stack, "/roles/neo", validSession)
	if rec.Code != http.StatusOK {
		t.Fatalf("post-strip status = %d, want 200", rec.Code)
	}
}

func TestRolePagesDenyNonMember(t *testing.T) {
	iamnim := stubIamnim(t, validSession, []string{"otherorg"})
	stack, _ := newStack(t, fixtureSoil(), iamnim.URL, "https://mastery.acme.example")

	rec := get(t, stack, "/roles/neo", validSession)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), promptText) {
		t.Error("denied response leaks role content")
	}
}

func TestRolePagesFailClosedWithoutConfig(t *testing.T) {
	iamnim := stubIamnim(t, validSession, []string{"acme"})

	// BASE_URL missing: role pages answer 503, never open up.
	stack, _ := newStack(t, fixtureSoil(), iamnim.URL, "")
	rec := get(t, stack, "/roles/neo", validSession)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no base URL status = %d, want 503", rec.Code)
	}

	// IAMNIM_URL missing too.
	stack, _ = newStack(t, fixtureSoil(), "", "https://mastery.acme.example")
	rec = get(t, stack, "/roles/neo", validSession)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no iamnim status = %d, want 503", rec.Code)
	}
}

func TestDirectoryAnswers503WhenNoCatalog(t *testing.T) {
	iamnim := stubIamnim(t, validSession, []string{"acme"})
	stack, _ := newStack(t, soil.Disabled{Reason: "no nats"}, iamnim.URL, "https://mastery.acme.example")

	rec := get(t, stack, "/", "")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unavailable") {
		t.Errorf("503 page does not say why\n%s", rec.Body.String())
	}
}

func TestDispatchSendsAgentPageToAgentSurface(t *testing.T) {
	iamnim := stubIamnim(t, validSession, []string{"acme"})
	stack, _ := newStack(t, fixtureSoil(), iamnim.URL, "https://mastery.acme.example")

	// No credential on the markdown page: a JSON 401 with a Bearer
	// challenge, never a browser login redirect.
	rec := get(t, stack, "/roles/neo.md", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Error("agent page 401 without a Bearer challenge")
	}
}
