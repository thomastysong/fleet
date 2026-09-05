// Package fleet is the Fleet REST substrate: ADM drives a Fleet server the
// way cmd/fleet-mcp reads it, with an API-only user and the least role that
// covers the calls it makes.
package fleet

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fleetdm/fleet/v4/adm/device"
	"github.com/fleetdm/fleet/v4/adm/intent"
	"github.com/fleetdm/fleet/v4/adm/substrate"
)

// Client talks to a Fleet server.
type Client struct {
	base  string
	token string
	http  *http.Client
	// PageSize bounds host list pages.
	PageSize int

	mu     sync.Mutex
	fleets map[string]uint
	labels map[string]uint
}

// New returns a client for baseURL with an API token.
func New(baseURL, token string, opts ...Option) (*Client, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" || token == "" {
		return nil, errors.New("fleet: base URL and API token are required")
	}
	c := &Client{base: baseURL, token: token, http: &http.Client{Timeout: 90 * time.Second}, PageSize: 500}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// Name implements substrate.Substrate.
func (c *Client) Name() string { return "fleet" }

// APIError is a non-2xx Fleet response.
type APIError struct {
	Status int
	Path   string
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("fleet: %s: status %d: %s", e.Path, e.Status, e.Body)
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, in, out any) error {
	full := c.base + "/api/latest/fleet" + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	var body io.Reader
	contentType := ""
	switch v := in.(type) {
	case nil:
	case *multipartBody:
		body, contentType = v.r, v.contentType
	default:
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body, contentType = bytes.NewReader(b), "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, method, full, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("fleet: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := string(raw)
		if len(msg) > 300 {
			msg = msg[:300] + "..."
		}
		return &APIError{Status: resp.StatusCode, Path: path, Body: msg}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

type multipartBody struct {
	r           io.Reader
	contentType string
}

type namedID struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

func (c *Client) fleetID(ctx context.Context, name string) (uint, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fleets == nil {
		var out struct {
			Fleets []namedID `json:"fleets"`
			Teams  []namedID `json:"teams"`
		}
		if err := c.do(ctx, http.MethodGet, "/fleets", nil, nil, &out); err != nil {
			return 0, err
		}
		c.fleets = map[string]uint{}
		for _, f := range append(out.Fleets, out.Teams...) {
			c.fleets[strings.ToLower(f.Name)] = f.ID
		}
	}
	id, ok := c.fleets[strings.ToLower(name)]
	if !ok {
		return 0, fmt.Errorf("fleet: unknown fleet %q", name)
	}
	return id, nil
}

func (c *Client) labelID(ctx context.Context, name string) (uint, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.labels == nil {
		var out struct {
			Labels []namedID `json:"labels"`
		}
		if err := c.do(ctx, http.MethodGet, "/labels", nil, nil, &out); err != nil {
			return 0, err
		}
		c.labels = map[string]uint{}
		for _, l := range out.Labels {
			c.labels[strings.ToLower(l.Name)] = l.ID
		}
	}
	id, ok := c.labels[strings.ToLower(name)]
	if !ok {
		return 0, fmt.Errorf("fleet: unknown label %q", name)
	}
	return id, nil
}

type hostRow struct {
	ID             uint    `json:"id"`
	Hostname       string  `json:"hostname"`
	UUID           string  `json:"uuid"`
	Platform       string  `json:"platform"`
	OSVersion      string  `json:"os_version"`
	HardwareSerial string  `json:"hardware_serial"`
	Status         string  `json:"status"`
	TeamName       *string `json:"team_name"`
	FleetName      *string `json:"fleet_name"`
}

func (h hostRow) fleet() string {
	if h.FleetName != nil {
		return *h.FleetName
	}
	if h.TeamName != nil {
		return *h.TeamName
	}
	return ""
}

// ListHosts lists hosts matching the scope. Fleet filters by one fleet and
// one label server-side; the rest is filtered here.
func (c *Client) ListHosts(ctx context.Context, scope intent.Scope) ([]device.Host, error) {
	q := url.Values{"per_page": {strconv.Itoa(c.PageSize)}}
	if len(scope.Fleets) == 1 {
		id, err := c.fleetID(ctx, scope.Fleets[0])
		if err != nil {
			return nil, err
		}
		q.Set("team_id", strconv.FormatUint(uint64(id), 10))
		q.Set("fleet_id", strconv.FormatUint(uint64(id), 10))
	}
	if len(scope.Labels) >= 1 {
		id, err := c.labelID(ctx, scope.Labels[0])
		if err != nil {
			return nil, err
		}
		q.Set("label_id", strconv.FormatUint(uint64(id), 10))
	}
	var all []device.Host
	for page := 0; ; page++ {
		q.Set("page", strconv.Itoa(page))
		var out struct {
			Hosts []hostRow `json:"hosts"`
		}
		if err := c.do(ctx, http.MethodGet, "/hosts", q, nil, &out); err != nil {
			return nil, err
		}
		for _, h := range out.Hosts {
			all = append(all, device.Host{
				ID: h.ID, UUID: h.UUID, Hostname: h.Hostname, Serial: h.HardwareSerial,
				Platform: device.NormalizePlatform(h.Platform), OSVersion: h.OSVersion,
				Fleet: h.fleet(), Online: h.Status == "online", Labels: scope.Labels,
			})
		}
		if len(out.Hosts) < c.PageSize {
			break
		}
	}
	// Remaining selectors (extra labels are not known per host without a
	// second call per host; platforms, IDs and fleets are).
	rest := intent.Scope{Fleets: scope.Fleets, Platforms: scope.Platforms, HostIDs: scope.HostIDs}
	return device.Filter(all, rest), nil
}

// CountHosts returns the total host count.
func (c *Client) CountHosts(ctx context.Context) (int, error) {
	var out struct {
		Count int `json:"count"`
	}
	if err := c.do(ctx, http.MethodGet, "/hosts/count", nil, nil, &out); err != nil {
		return 0, err
	}
	return out.Count, nil
}

// LiveQuery runs SQL on the hosts through a temporary saved report, the
// same technique cmd/fleet-mcp uses, and deletes the report afterwards.
func (c *Client) LiveQuery(ctx context.Context, sql string, hostIDs []uint) (substrate.QueryResult, error) {
	res := substrate.QueryResult{Rows: map[uint][]map[string]string{}, Errors: map[uint]string{}}
	if len(hostIDs) == 0 {
		return res, nil
	}
	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	var created struct {
		Query  *namedID `json:"query"`
		Report *namedID `json:"report"`
	}
	if err := c.do(ctx, http.MethodPost, "/reports", nil, map[string]any{"name": "adm-temp-" + hex.EncodeToString(suffix), "query": sql, "description": "ADM verification (temporary)"}, &created); err != nil {
		return res, err
	}
	id := created.Report
	if id == nil {
		id = created.Query
	}
	if id == nil {
		return res, errors.New("fleet: create report returned no id")
	}
	defer func() {
		_ = c.do(context.WithoutCancel(ctx), http.MethodDelete, "/reports/id/"+strconv.FormatUint(uint64(id.ID), 10), nil, nil, nil)
	}()
	var out struct {
		Targeted  int `json:"targeted_host_count"`
		Responded int `json:"responded_host_count"`
		Results   []struct {
			HostID uint                `json:"host_id"`
			Rows   []map[string]string `json:"rows"`
			Error  *string             `json:"error"`
		} `json:"results"`
	}
	if err := c.do(ctx, http.MethodPost, "/reports/"+strconv.FormatUint(uint64(id.ID), 10)+"/run", nil, map[string]any{"host_ids": hostIDs}, &out); err != nil {
		return res, err
	}
	res.Targeted, res.Responded = out.Targeted, out.Responded
	for _, r := range out.Results {
		if r.Error != nil && *r.Error != "" {
			res.Errors[r.HostID] = *r.Error
			continue
		}
		res.Rows[r.HostID] = r.Rows
	}
	return res, nil
}

// AddProfile uploads a profile or declaration (multipart, like the UI).
func (c *Client) AddProfile(ctx context.Context, p substrate.Profile) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if p.Fleet != "" {
		id, err := c.fleetID(ctx, p.Fleet)
		if err != nil {
			return err
		}
		_ = w.WriteField("fleet_id", strconv.FormatUint(uint64(id), 10))
		_ = w.WriteField("team_id", strconv.FormatUint(uint64(id), 10))
	}
	for _, l := range p.Labels {
		_ = w.WriteField("labels_include_all", l)
	}
	name := p.Name
	switch {
	case p.Platform == intent.PlatformWindows && !strings.HasSuffix(name, ".xml"):
		name += ".xml"
	case bytes.HasPrefix(bytes.TrimSpace(p.Contents), []byte("{")) && !strings.HasSuffix(name, ".json"):
		name += ".json"
	case !strings.HasSuffix(name, ".mobileconfig") && !strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".xml"):
		name += ".mobileconfig"
	}
	fw, err := w.CreateFormFile("profile", name)
	if err != nil {
		return err
	}
	if _, err := fw.Write(p.Contents); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, "/mdm/profiles", nil, &multipartBody{r: &buf, contentType: w.FormDataContentType()}, nil)
}

// RunCommand enqueues an MDM command (Apple plist or Windows SyncML).
func (c *Client) RunCommand(ctx context.Context, cmd substrate.Command) error {
	return c.do(ctx, http.MethodPost, "/commands/run", nil, map[string]any{"command": base64.StdEncoding.EncodeToString(cmd.Raw), "host_uuids": cmd.HostUUIDs}, nil)
}

// UpsertPolicy creates a policy (global or per fleet). Fleet rejects
// duplicate names; a conflict is treated as already present.
func (c *Client) UpsertPolicy(ctx context.Context, p substrate.Policy) error {
	body := map[string]any{"name": p.Name, "query": p.Query, "description": p.Description, "resolution": p.Resolution, "platform": fleetPlatform(p.Platform)}
	path := "/policies"
	if p.Fleet != "" {
		id, err := c.fleetID(ctx, p.Fleet)
		if err != nil {
			return err
		}
		path = "/fleets/" + strconv.FormatUint(uint64(id), 10) + "/policies"
	}
	err := c.do(ctx, http.MethodPost, path, nil, body, nil)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict {
		return nil
	}
	return err
}

func fleetPlatform(p intent.Platform) string {
	switch p {
	case intent.PlatformDarwin, intent.PlatformWindows, intent.PlatformLinux:
		return string(p)
	case intent.PlatformChromeOS:
		return "chrome"
	}
	return ""
}

// RunScript runs a script synchronously on one host (the fallback path).
func (c *Client) RunScript(ctx context.Context, hostID uint, script string) (substrate.ScriptResult, error) {
	var out struct {
		ExitCode *int   `json:"exit_code"`
		Output   string `json:"output"`
	}
	if err := c.do(ctx, http.MethodPost, "/scripts/run/sync", nil, map[string]any{"host_id": hostID, "script_contents": script}, &out); err != nil {
		return substrate.ScriptResult{}, err
	}
	code := -1
	if out.ExitCode != nil {
		code = *out.ExitCode
	}
	return substrate.ScriptResult{ExitCode: code, Output: out.Output}, nil
}

// Nudge is not exposed by Fleet's public API yet (ADR-0011 nudges are
// server-internal); verification proceeds on the agents' own cadence.
func (c *Client) Nudge(context.Context, []uint) error { return substrate.ErrUnsupported }
