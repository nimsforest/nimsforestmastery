package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/nimsforest/nimsforestmastery/internal/api"
)

// SessionCookie is the portal's own browser session cookie. Its value
// is the iamnim session token; the cookie exists so the token leaves
// the URL after the login callback.
const SessionCookie = "mastery_session"

// StateCookie binds the login callback to a login this middleware
// started: redirectToLogin sets a random state in this cookie and in
// the redirect_uri query, iamnim echoes the query back, and the
// callback only accepts ?token= when the two match. Without the
// binding, a crafted link could set an attacker's session on the
// victim's browser (login CSRF) or evict a valid session.
const StateCookie = "mastery_login_state"

// Auth is the human SSO middleware, the nimsforestadmin reference
// pattern: every protected request re-checks GET /api/me and
// GET /api/me/memberships against iamnim, with the configured
// organization required in the membership list. There is no cache, so
// a revoked token or membership cannot reuse a stored identity. A
// missing configuration answers 503, never an allow.
type Auth struct {
	identity *api.Auth
	iamnim   string
	baseURL  string
}

// NewAuth builds the middleware. iamnimURL and orgSlug drive the
// identity check; baseURL is this portal's own external URL, the
// login return target.
func NewAuth(iamnimURL, orgSlug, baseURL string) *Auth {
	return &Auth{
		identity: api.NewAuth(iamnimURL, orgSlug),
		iamnim:   strings.TrimRight(iamnimURL, "/"),
		baseURL:  strings.TrimRight(baseURL, "/"),
	}
}

// Wrap protects a human page handler with the login redirect flow.
// No credential sends the browser to iamnim with a fresh state; the
// state-bound callback ?token= is moved into a Secure HttpOnly
// SameSite=Lax cookie and stripped off the URL. An existing valid
// session cookie always wins over a query token, so a crafted
// ?token= link can neither replace nor evict a valid session.
func (a *Auth) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")

		if !a.identity.Configured() || a.baseURL == "" {
			http.Error(w, "sign-in is not configured; IAMNIM_URL, ORG_SLUG and BASE_URL must be set", http.StatusServiceUnavailable)
			return
		}

		// The existing session cookie is checked first and preferred.
		if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
			user, err := a.identity.Validate(r.Context(), c.Value)
			if err == nil {
				if r.URL.Query().Get("token") != "" {
					// A stray callback query on a valid session: strip
					// the sensitive URL, keep the session.
					clearStateCookie(w)
					http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
					return
				}
				next.ServeHTTP(w, r.WithContext(api.WithUser(r.Context(), user)))
				return
			}
			status := errStatus(err)
			if status != http.StatusUnauthorized {
				a.writeAuthError(w, status)
				return
			}
			// The stored session expired: drop it and continue into
			// the callback or login redirect flow.
			clearSessionCookie(w)
		}

		if token := r.URL.Query().Get("token"); token != "" {
			a.handleCallback(w, r, token)
			return
		}

		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			a.redirectToLogin(w, r)
			return
		}
		http.Error(w, "sign in required", http.StatusUnauthorized)
	})
}

// handleCallback finishes the login: the query token is accepted only
// when the query state matches the state cookie this middleware set
// when it started the login.
func (a *Auth) handleCallback(w http.ResponseWriter, r *http.Request, token string) {
	state := r.URL.Query().Get("state")
	cookie, err := r.Cookie(StateCookie)
	if state == "" || err != nil || cookie.Value == "" ||
		subtle.ConstantTimeCompare([]byte(state), []byte(cookie.Value)) != 1 {
		// Not a login this portal started: ignore the token and start
		// a fresh login instead of adopting a pushed session.
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			a.redirectToLogin(w, r)
			return
		}
		http.Error(w, "sign in required", http.StatusUnauthorized)
		return
	}

	if _, err := a.identity.Validate(r.Context(), token); err != nil {
		a.writeAuthError(w, errStatus(err))
		return
	}

	clearStateCookie(w)
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	// The callback URL carries the token and is sensitive in access
	// logs: redirect to the same path without it.
	http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
}

// errStatus maps a Validate error to the HTTP status the page must
// answer: 401 or 403 on a deny, 503 when the identity service could
// not answer. Never an allow.
func errStatus(err error) int {
	var denied api.AuthError
	if errors.As(err, &denied) {
		return int(denied)
	}
	return http.StatusServiceUnavailable
}

func (a *Auth) writeAuthError(w http.ResponseWriter, status int) {
	message := "organization access denied"
	if status == http.StatusServiceUnavailable {
		message = "identity service unavailable; retry shortly"
	}
	http.Error(w, message, status)
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     StateCookie,
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// newState returns a random login state.
func newState() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failing is unrecoverable for a login flow.
		panic(err)
	}
	return hex.EncodeToString(buf)
}

// redirectToLogin sends the browser to iamnim with this page as the
// return target and a fresh state bound to a short-lived cookie.
// iamnim preserves the redirect_uri query and appends &token=, so the
// callback carries both. iamnim returns with a GET, so a non-GET
// request must not become the target (#276): fall back to the
// same-origin Referer path, or the directory.
func (a *Auth) redirectToLogin(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Path
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		target = "/"
		if ref, err := url.Parse(r.Referer()); err == nil && ref.Path != "" &&
			(ref.Host == "" || a.baseURL == ref.Scheme+"://"+ref.Host) {
			target = ref.Path
		}
	}
	state := newState()
	http.SetCookie(w, &http.Cookie{
		Name:     StateCookie,
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	returnURL := a.baseURL + target + "?state=" + state
	http.Redirect(w, r, a.iamnim+"/login?redirect_uri="+url.QueryEscape(returnURL), http.StatusSeeOther)
}
