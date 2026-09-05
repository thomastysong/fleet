package autopilot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newServer(t *testing.T) (*httptest.Server, *atomic.Int32, map[string]any) {
	t.Helper()
	var tokens atomic.Int32
	state := map[string]any{}
	mux := http.NewServeMux()
	mux.HandleFunc("/tenant-1/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("scope") != graphScope || r.Form.Get("client_secret") != "s3cret" {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		tokens.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok" {
				http.Error(w, `{"error":{"code":"InvalidAuthenticationToken","message":"nope"}}`, http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/v1.0/deviceManagement/windowsAutopilotDeviceIdentities", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			_ = json.NewEncoder(w).Encode(map[string]any{"value": []map[string]any{{"id": "d2", "serialNumber": "S2", "groupTag": "eng"}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": []map[string]any{{"id": "d1", "serialNumber": "S1"}}, "@odata.nextLink": "http://" + r.Host + "/v1.0/deviceManagement/windowsAutopilotDeviceIdentities?page=2"})
	}))
	mux.HandleFunc("/v1.0/deviceManagement/importedWindowsAutopilotDeviceIdentities", auth(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		state["import"] = body
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "imp-1", "serialNumber": body["serialNumber"], "groupTag": body["groupTag"], "state": map[string]any{"deviceImportStatus": "pending"}})
	}))
	polls := 0
	mux.HandleFunc("/v1.0/deviceManagement/importedWindowsAutopilotDeviceIdentities/imp-1", auth(func(w http.ResponseWriter, r *http.Request) {
		polls++
		status := "pending"
		if polls >= 2 {
			status = "complete"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "imp-1", "state": map[string]any{"deviceImportStatus": status, "deviceRegistrationId": "d9"}})
	}))
	mux.HandleFunc("/v1.0/deviceManagement/windowsAutopilotDeviceIdentities/d1/updateDeviceProperties", auth(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		state["groupTag"] = body["groupTag"]
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("/v1.0/deviceManagement/windowsAutopilotDeploymentProfiles", auth(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"value": []map[string]any{{"id": "p1", "displayName": "ADM standard", "outOfBoxExperienceSettings": map[string]any{"userType": "standard"}}}})
	}))
	mux.HandleFunc("/v1.0/deviceManagement/windowsAutopilotDeploymentProfiles/p1/assignments", auth(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		state["assignment"] = body
		w.WriteHeader(http.StatusCreated)
	}))
	mux.HandleFunc("/v1.0/deviceManagement/windowsAutopilotSettings/sync", auth(func(w http.ResponseWriter, r *http.Request) {
		state["synced"] = true
		w.WriteHeader(http.StatusNoContent)
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &tokens, state
}

func TestClientFlow(t *testing.T) {
	srv, tokens, state := newServer(t)
	c, err := New(Credential{TenantID: "tenant-1", ClientID: "app", ClientSecret: "s3cret"}, WithHosts(srv.URL, srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	devs, err := c.ListDevices(ctx)
	if err != nil || len(devs) != 2 || devs[1].GroupTag != "eng" {
		t.Fatalf("%v %v", devs, err)
	}
	imp, err := c.ImportDevice(ctx, ImportRequest{SerialNumber: "S3", HardwareHash: "AAAA", GroupTag: "finance"})
	if err != nil || imp.ID != "imp-1" {
		t.Fatalf("%v %v", imp, err)
	}
	if body := state["import"].(map[string]any); body["hardwareIdentifier"] != "AAAA" || body["@odata.type"] != "#microsoft.graph.importedWindowsAutopilotDeviceIdentity" {
		t.Fatalf("import body %v", body)
	}
	done, err := c.WaitForImport(ctx, "imp-1", time.Millisecond)
	if err != nil || done.State.DeviceImportStatus != "complete" || done.State.DeviceRegistrationID != "d9" {
		t.Fatalf("%v %v", done, err)
	}
	if err := c.SetGroupTag(ctx, "d1", "eng"); err != nil || state["groupTag"] != "eng" {
		t.Fatalf("%v %v", err, state)
	}
	profiles, err := c.ListProfiles(ctx)
	if err != nil || len(profiles) != 1 || profiles[0].OOBE.UserType != "standard" {
		t.Fatalf("%v %v", profiles, err)
	}
	if err := c.AssignProfileToGroup(ctx, "p1", "g1"); err != nil {
		t.Fatal(err)
	}
	if err := c.Sync(ctx); err != nil || state["synced"] != true {
		t.Fatalf("%v", err)
	}
	if tokens.Load() != 1 {
		t.Fatalf("token should be cached, minted %d times", tokens.Load())
	}
	if _, err := c.ImportDevice(ctx, ImportRequest{}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestGraphErrorsAndOriginGuard(t *testing.T) {
	srv, _, _ := newServer(t)
	c, _ := New(Credential{TenantID: "tenant-1", ClientID: "app", ClientSecret: "wrong"}, WithHosts(srv.URL, srv.URL))
	if _, err := c.ListDevices(context.Background()); err == nil || !strings.Contains(err.Error(), "token request") {
		t.Fatalf("expected token error, got %v", err)
	}
	c2, _ := New(Credential{TenantID: "tenant-1", ClientID: "app", ClientSecret: "s3cret"}, WithHosts(srv.URL, srv.URL))
	err := c2.do(context.Background(), http.MethodGet, "https://evil.example/v1.0/x", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "refusing to follow") {
		t.Fatalf("expected origin guard, got %v", err)
	}
	// Non-2xx is a GraphError.
	c2.token, c2.tokenExp = "bad", time.Now().Add(time.Hour)
	var ge *GraphError
	if _, err := c2.ListProfiles(context.Background()); !errors.As(err, &ge) || ge.Status != 401 || ge.Code != "InvalidAuthenticationToken" {
		t.Fatalf("expected GraphError 401, got %v", err)
	}
	if _, err := New(Credential{}); err == nil {
		t.Fatal("expected credential validation error")
	}
}
