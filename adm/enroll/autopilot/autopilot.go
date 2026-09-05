// Package autopilot drives Windows Autopilot through Microsoft Graph so a
// Windows device is registered (hardware hash), tagged and assigned a
// deployment profile before it is unboxed, and enrolls straight into ADM's
// MDM (Fleet's Entra automatic-enrollment endpoints) during OOBE.
//
// This complements what Fleet already does (reading Autopilot device
// identities, see server/microsoft/msgraph) with the write side: import,
// group tags, profile assignment and sync. Credentials are an Entra app
// registration with DeviceManagementServiceConfig.ReadWrite.All.
package autopilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultLoginHost = "https://login.microsoftonline.com"
	defaultGraphHost = "https://graph.microsoft.com"
	graphScope       = "https://graph.microsoft.com/.default"
)

// Credential is an Entra app registration.
type Credential struct {
	TenantID     string
	ClientID     string
	ClientSecret string
}

// Client talks to Microsoft Graph.
type Client struct {
	cred      Credential
	http      *http.Client
	loginHost string
	graphHost string

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// Option configures a Client.
type Option func(*Client)

// WithHosts overrides the login and Graph hosts (tests, sovereign clouds).
func WithHosts(login, graph string) Option {
	return func(c *Client) { c.loginHost, c.graphHost = login, graph }
}

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// New returns a client.
func New(cred Credential, opts ...Option) (*Client, error) {
	if cred.TenantID == "" || cred.ClientID == "" || cred.ClientSecret == "" {
		return nil, errors.New("autopilot: tenant id, client id and client secret are required")
	}
	c := &Client{cred: cred, http: &http.Client{Timeout: 60 * time.Second}, loginHost: defaultLoginHost, graphHost: defaultGraphHost}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Until(c.tokenExp) > time.Minute {
		return c.token, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {c.cred.ClientID}, "client_secret": {c.cred.ClientSecret}, "scope": {graphScope}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/%s/oauth2/v2.0/token", c.loginHost, url.PathEscape(c.cred.TenantID)), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request: status %d: %s", resp.StatusCode, truncate(body))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return "", errors.New("token request: malformed response")
	}
	c.token = tok.AccessToken
	c.tokenExp = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	return c.token, nil
}

// GraphError is a non-2xx Graph response.
type GraphError struct {
	Status int
	Code   string
	Msg    string
}

func (e *GraphError) Error() string {
	return fmt.Sprintf("graph: status %d %s: %s", e.Status, e.Code, e.Msg)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	full := path
	if !strings.HasPrefix(path, "http") {
		full = c.graphHost + "/v1.0" + path
	} else if !strings.HasPrefix(full, c.graphHost+"/") {
		return fmt.Errorf("graph: refusing to follow link off %s: %s", c.graphHost, full)
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	tok, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, full, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("graph %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		ge := &GraphError{Status: resp.StatusCode}
		var e struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil {
			ge.Code, ge.Msg = e.Error.Code, e.Error.Message
		} else {
			ge.Msg = truncate(raw)
		}
		return ge
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// Device is a registered Autopilot device identity.
type Device struct {
	ID              string `json:"id"`
	SerialNumber    string `json:"serialNumber"`
	GroupTag        string `json:"groupTag"`
	Model           string `json:"model"`
	Manufacturer    string `json:"manufacturer"`
	EntraDeviceID   string `json:"azureActiveDirectoryDeviceId"`
	EnrollmentState string `json:"enrollmentState"`
	LastContact     string `json:"lastContactedDateTime"`
	ProfileStatus   string `json:"deploymentProfileAssignmentStatus"`
}

type page[T any] struct {
	Value    []T    `json:"value"`
	NextLink string `json:"@odata.nextLink"`
}

// ListDevices returns every Autopilot device identity in the tenant.
func (c *Client) ListDevices(ctx context.Context) ([]Device, error) {
	var out []Device
	next := "/deviceManagement/windowsAutopilotDeviceIdentities"
	for next != "" {
		var p page[Device]
		if err := c.do(ctx, http.MethodGet, next, nil, &p); err != nil {
			return nil, err
		}
		out = append(out, p.Value...)
		next = p.NextLink
	}
	return out, nil
}

// ImportRequest registers a device by hardware hash.
type ImportRequest struct {
	SerialNumber string
	// HardwareHash is the OA3 hardware hash (base64), as exported by
	// Get-WindowsAutopilotInfo or the OEM.
	HardwareHash string
	GroupTag     string
	AssignedUser string
}

// Import is the result of an import request; Graph processes imports
// asynchronously, poll ImportStatus until State.DeviceImportStatus is
// "complete".
type Import struct {
	ID           string `json:"id"`
	SerialNumber string `json:"serialNumber"`
	GroupTag     string `json:"groupTag"`
	State        struct {
		DeviceImportStatus   string `json:"deviceImportStatus"`
		DeviceRegistrationID string `json:"deviceRegistrationId"`
		DeviceErrorCode      int    `json:"deviceErrorCode"`
		DeviceErrorName      string `json:"deviceErrorName"`
	} `json:"state"`
}

// ImportDevice submits a hardware hash for registration.
func (c *Client) ImportDevice(ctx context.Context, r ImportRequest) (*Import, error) {
	if r.SerialNumber == "" || r.HardwareHash == "" {
		return nil, errors.New("autopilot: serial number and hardware hash are required")
	}
	body := map[string]any{
		"@odata.type":        "#microsoft.graph.importedWindowsAutopilotDeviceIdentity",
		"serialNumber":       r.SerialNumber,
		"hardwareIdentifier": r.HardwareHash,
		"groupTag":           r.GroupTag,
		"state": map[string]any{
			"@odata.type":        "microsoft.graph.importedWindowsAutopilotDeviceIdentityState",
			"deviceImportStatus": "pending",
		},
	}
	if r.AssignedUser != "" {
		body["assignedUserPrincipalName"] = r.AssignedUser
	}
	var out Import
	if err := c.do(ctx, http.MethodPost, "/deviceManagement/importedWindowsAutopilotDeviceIdentities", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ImportStatus polls an import.
func (c *Client) ImportStatus(ctx context.Context, importID string) (*Import, error) {
	var out Import
	if err := c.do(ctx, http.MethodGet, "/deviceManagement/importedWindowsAutopilotDeviceIdentities/"+url.PathEscape(importID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WaitForImport polls until the import completes or ctx ends.
func (c *Client) WaitForImport(ctx context.Context, importID string, every time.Duration) (*Import, error) {
	for {
		imp, err := c.ImportStatus(ctx, importID)
		if err != nil {
			return nil, err
		}
		switch imp.State.DeviceImportStatus {
		case "complete":
			return imp, nil
		case "error":
			return imp, fmt.Errorf("autopilot import failed: %s (%d)", imp.State.DeviceErrorName, imp.State.DeviceErrorCode)
		}
		select {
		case <-ctx.Done():
			return imp, ctx.Err()
		case <-time.After(every):
		}
	}
}

// SetGroupTag updates a registered device's group tag (and optionally its
// display name / user), which is how deployment profiles are targeted.
func (c *Client) SetGroupTag(ctx context.Context, deviceID, groupTag string) error {
	return c.do(ctx, http.MethodPost, "/deviceManagement/windowsAutopilotDeviceIdentities/"+url.PathEscape(deviceID)+"/updateDeviceProperties", map[string]any{"groupTag": groupTag}, nil)
}

// Profile is an Autopilot deployment profile.
type Profile struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Language    string `json:"language"`
	OOBE        struct {
		UserType              string `json:"userType"`
		PrivacySettingsHidden bool   `json:"hidePrivacySettings"`
		EULAHidden            bool   `json:"hideEULA"`
		SkipKeyboard          bool   `json:"skipKeyboardSelectionPage"`
		DeviceUsage           string `json:"deviceUsageType"`
	} `json:"outOfBoxExperienceSettings"`
}

// ListProfiles returns the deployment profiles.
func (c *Client) ListProfiles(ctx context.Context) ([]Profile, error) {
	var out []Profile
	next := "/deviceManagement/windowsAutopilotDeploymentProfiles"
	for next != "" {
		var p page[Profile]
		if err := c.do(ctx, http.MethodGet, next, nil, &p); err != nil {
			return nil, err
		}
		out = append(out, p.Value...)
		next = p.NextLink
	}
	return out, nil
}

// AssignProfileToGroup assigns a deployment profile to an Entra group (a
// dynamic group keyed on the group tag is the usual pattern).
func (c *Client) AssignProfileToGroup(ctx context.Context, profileID, groupID string) error {
	body := map[string]any{"target": map[string]any{"@odata.type": "#microsoft.graph.groupAssignmentTarget", "groupId": groupID}}
	return c.do(ctx, http.MethodPost, "/deviceManagement/windowsAutopilotDeploymentProfiles/"+url.PathEscape(profileID)+"/assignments", body, nil)
}

// Sync asks Intune to synchronise Autopilot devices from the Microsoft
// device directory service now instead of on its schedule.
func (c *Client) Sync(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/deviceManagement/windowsAutopilotSettings/sync", nil, nil)
}

// DeleteDevice deregisters a device from Autopilot.
func (c *Client) DeleteDevice(ctx context.Context, deviceID string) error {
	return c.do(ctx, http.MethodDelete, "/deviceManagement/windowsAutopilotDeviceIdentities/"+url.PathEscape(deviceID), nil, nil)
}

func truncate(b []byte) string {
	s := string(b)
	if len(s) > 300 {
		return s[:300] + "..."
	}
	return s
}
