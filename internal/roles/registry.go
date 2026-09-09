package roles

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RegistryClient reads the nimregistry HTTP API. It is the fallback
// read path when Soil is not available; a bundle served through it
// carries revision 0 stamps and is marked degraded.
type RegistryClient struct {
	BaseURL string
	HTTP    *http.Client
}

// NewRegistryClient builds a client for the given base URL, for
// example "http://127.0.0.1:8101".
func NewRegistryClient(baseURL string) *RegistryClient {
	return &RegistryClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

// RegistryNim mirrors the nimregistry /api/nims response shape.
type RegistryNim struct {
	Name            string   `json:"name"`
	Role            string   `json:"role"`
	Description     string   `json:"description"`
	Category        string   `json:"category"`
	Subjects        []string `json:"subjects,omitempty"`
	LongDescription string   `json:"long_description,omitempty"`
}

const registryBodyLimit = 4 << 20

func (c *RegistryClient) get(path string) ([]byte, int, error) {
	resp, err := c.HTTP.Get(c.BaseURL + path)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, registryBodyLimit))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// ListNims returns all nims from GET /api/nims.
func (c *RegistryClient) ListNims() ([]RegistryNim, error) {
	body, status, err := c.get("/api/nims")
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("nimregistry: list nims: status %d", status)
	}
	var nims []RegistryNim
	if err := json.Unmarshal(body, &nims); err != nil {
		return nil, fmt.Errorf("nimregistry: list nims: %w", err)
	}
	return nims, nil
}

// GetNim returns one nim from GET /api/nims/{name}. It returns
// ErrRoleNotFound on 404, and for any name outside the strict role
// charset: a crafted name must never reach the request path.
func (c *RegistryClient) GetNim(name string) (*RegistryNim, error) {
	if !ValidRoleName(name) {
		return nil, ErrRoleNotFound
	}
	body, status, err := c.get("/api/nims/" + url.PathEscape(name))
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, ErrRoleNotFound
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("nimregistry: get nim %s: status %d", name, status)
	}
	var nim RegistryNim
	if err := json.Unmarshal(body, &nim); err != nil {
		return nil, fmt.Errorf("nimregistry: get nim %s: %w", name, err)
	}
	return &nim, nil
}

// GetPrompt returns the raw prompt markdown from
// GET /api/nims/{name}/prompt. A missing prompt returns "". A name
// outside the strict role charset never reaches the request path.
func (c *RegistryClient) GetPrompt(name string) (string, error) {
	if !ValidRoleName(name) {
		return "", nil
	}
	body, status, err := c.get("/api/nims/" + url.PathEscape(name) + "/prompt")
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound {
		return "", nil
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("nimregistry: get prompt %s: status %d", name, status)
	}
	return string(body), nil
}

// Health probes GET /health for the health surface.
func (c *RegistryClient) Health() error {
	_, status, err := c.get("/health")
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("nimregistry: health status %d", status)
	}
	return nil
}
