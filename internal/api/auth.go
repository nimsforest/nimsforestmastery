// Package api serves the agent surface of the mastery portal: the
// role directory, the six-slot bundle, the plain markdown role page,
// and llms.txt. Every route is generated at request time from the
// live catalog; nothing here is authored per nim.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// sessionCookie is the cookie iamnim itself reads. A browser session
// on the portal forwards its value; a PAT arrives as a Bearer token.
const sessionCookie = "iamnim_session"

// User is the validated identity for one request.
type User struct {
	UserID  string `json:"user_id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	IsAdmin bool   `json:"-"`
}

type userKey struct{}

// GetUser returns the validated identity from the request context,
// or nil outside the auth middleware.
func GetUser(r *http.Request) *User {
	u, _ := r.Context().Value(userKey{}).(*User)
	return u
}

// WithUser stores a validated identity in a context. The web surface
// uses it so GetUser works behind both middlewares.
func WithUser(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, userKey{}, user)
}

// Auth validates iamnim credentials with the fail-closed reference
// pattern: every protected request re-checks GET /api/me and
// GET /api/me/memberships, and the membership list must contain the
// configured organization. There is no cache, so a revoked token or
// membership cannot reuse a stored identity.
type Auth struct {
	iamnimURL string
	orgSlug   string
	client    *http.Client
}

// NewAuth builds the middleware. An empty iamnimURL or orgSlug makes
// every protected route answer 503: the portal never opens up when
// identity is not configured.
func NewAuth(iamnimURL, orgSlug string) *Auth {
	return &Auth{
		iamnimURL: strings.TrimRight(iamnimURL, "/"),
		orgSlug:   orgSlug,
		client: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Configured reports whether the identity check has both the iamnim
// URL and the organization slug. Unconfigured identity fails closed.
func (a *Auth) Configured() bool {
	return a.iamnimURL != "" && a.orgSlug != ""
}

// AuthError carries a deny status (401 or 403) from iamnim. Any other
// validation error means the identity service is unavailable, which
// callers must answer with 503, never with an allow.
type AuthError int

// Error implements error.
func (e AuthError) Error() string { return http.StatusText(int(e)) }

// Wrap protects an agent-surface handler. The surface is machine
// facing, so a missing or bad credential answers JSON, never a login
// redirect.
func (a *Auth) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")

		if a.iamnimURL == "" || a.orgSlug == "" {
			writeJSONError(w, http.StatusServiceUnavailable,
				"identity service not configured",
				"IAMNIM_URL or ORG_SLUG is not set; authenticated routes fail closed")
			return
		}

		token := bearerToken(r)
		if token == "" {
			if c, err := r.Cookie(sessionCookie); err == nil {
				token = c.Value
			}
		}
		if token == "" {
			a.challenge(w)
			writeJSONError(w, http.StatusUnauthorized,
				"authentication required",
				"send an iamnim PAT as a Bearer token, or a valid iamnim session cookie")
			return
		}

		user, err := a.Validate(r.Context(), token)
		if err != nil {
			var denied AuthError
			if errors.As(err, &denied) {
				status := int(denied)
				if status == http.StatusUnauthorized {
					a.challenge(w)
					writeJSONError(w, status, "invalid or expired credential", "")
					return
				}
				writeJSONError(w, http.StatusForbidden, "organization access denied", "")
				return
			}
			// Fail closed: an unreachable identity service never
			// becomes an allow.
			writeJSONError(w, http.StatusServiceUnavailable,
				"identity service unavailable", "retry shortly")
			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, user)))
	})
}

func (a *Auth) challenge(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="nimsforestmastery"`)
}

// bearerToken extracts an Authorization Bearer credential.
func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(token)
}

// Validate performs the two-call fail-closed check. The credential is
// forwarded as the iamnim_session cookie; iamnim's single authenticate
// choke point accepts both a session id and an inpat_ PAT there, so
// one path serves both credential kinds. A deny comes back as an
// AuthError; any other error means the identity service could not
// answer, which the caller must render as 503, never as an allow. The
// web surface reuses this method behind its login redirect flow.
func (a *Auth) Validate(ctx context.Context, token string) (*User, error) {
	fetch := func(path string, dst any) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.iamnimURL+path, nil)
		if err != nil {
			return err
		}
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		resp, err := a.client.Do(req)
		if err != nil {
			return fmt.Errorf("identity service unavailable")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				return AuthError(resp.StatusCode)
			}
			return fmt.Errorf("identity service status %d", resp.StatusCode)
		}
		return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(dst)
	}

	var user User
	if err := fetch("/api/me", &user); err != nil {
		return nil, err
	}
	if user.UserID == "" {
		return nil, fmt.Errorf("missing identity")
	}

	var memberships []struct {
		Slug    string `json:"organization_slug"`
		IsAdmin bool   `json:"is_admin"`
	}
	if err := fetch("/api/me/memberships", &memberships); err != nil {
		return nil, err
	}
	for _, m := range memberships {
		if m.Slug == a.orgSlug {
			user.IsAdmin = m.IsAdmin
			return &user, nil
		}
	}
	return nil, AuthError(http.StatusForbidden)
}
