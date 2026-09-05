package fleet

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/fleetdm/fleet/v4/adm/intent"
	"github.com/fleetdm/fleet/v4/adm/substrate"
)

func server(t *testing.T) (*httptest.Server, map[string]any) {
	t.Helper()
	state := map[string]any{}
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok" {
				http.Error(w, `{"message":"auth"}`, http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/api/latest/fleet/fleets", auth(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"fleets": []map[string]any{{"id": 7, "name": "Finance"}}})
	}))
	mux.HandleFunc("/api/latest/fleet/labels", auth(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"labels": []map[string]any{{"id": 3, "name": "laptops"}}})
	}))
	mux.HandleFunc("/api/latest/fleet/hosts/count", auth(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"count": 42})
	}))
	mux.HandleFunc("/api/latest/fleet/hosts", auth(func(w http.ResponseWriter, r *http.Request) {
		state["hosts_query"] = r.URL.Query()
		if r.URL.Query().Get("page") != "0" {
			_ = json.NewEncoder(w).Encode(map[string]any{"hosts": []any{}})
			return
		}
		fleet := "Finance"
		_ = json.NewEncoder(w).Encode(map[string]any{"hosts": []map[string]any{
			{"id": 1, "hostname": "win-carol", "uuid": "u1", "platform": "windows", "os_version": "11", "hardware_serial": "5CD1", "status": "online", "fleet_name": fleet},
			{"id": 2, "hostname": "ubu", "uuid": "u2", "platform": "ubuntu", "status": "offline", "team_name": fleet},
		}})
	}))
	mux.HandleFunc("/api/latest/fleet/reports", auth(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		state["report"] = body
		_ = json.NewEncoder(w).Encode(map[string]any{"report": map[string]any{"id": 99, "name": body["name"]}})
	}))
	mux.HandleFunc("/api/latest/fleet/reports/99/run", auth(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		state["run"] = body
		_ = json.NewEncoder(w).Encode(map[string]any{"query_id": 99, "targeted_host_count": 2, "responded_host_count": 1, "results": []map[string]any{
			{"host_id": 1, "rows": []map[string]string{{"1": "1"}}},
			{"host_id": 2, "rows": nil, "error": "offline"},
		}})
	}))
	mux.HandleFunc("/api/latest/fleet/reports/id/99", auth(func(w http.ResponseWriter, r *http.Request) {
		state["deleted"] = r.Method == http.MethodDelete
		w.WriteHeader(http.StatusOK)
	}))
	mux.HandleFunc("/api/latest/fleet/scripts/run/sync", auth(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"exit_code": 0, "output": "done"})
	}))
	mux.HandleFunc("/api/latest/fleet/commands/run", auth(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		state["command"] = body
		w.WriteHeader(http.StatusOK)
	}))
	mux.HandleFunc("/api/latest/fleet/mdm/profiles", auth(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f, hdr, err := r.FormFile("profile")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		b, _ := io.ReadAll(f)
		state["profile"] = map[string]any{"filename": hdr.Filename, "fleet_id": r.FormValue("fleet_id"), "labels": r.MultipartForm.Value["labels_include_all"], "body": string(b)}
		w.WriteHeader(http.StatusOK)
	}))
	mux.HandleFunc("/api/latest/fleet/fleets/7/policies", auth(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["name"] == "dup" {
			http.Error(w, `{"message":"exists"}`, http.StatusConflict)
			return
		}
		state["policy"] = body
		w.WriteHeader(http.StatusOK)
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, state
}

func TestClient(t *testing.T) {
	srv, state := server(t)
	c, err := New(srv.URL, "tok")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	hosts, err := c.ListHosts(ctx, intent.Scope{Fleets: []string{"finance"}, Labels: []string{"laptops"}, Platforms: []intent.Platform{intent.PlatformWindows, intent.PlatformLinux}})
	if err != nil || len(hosts) != 2 {
		t.Fatalf("%v %v", hosts, err)
	}
	if hosts[0].Platform != intent.PlatformWindows || !hosts[0].Online || hosts[1].Platform != intent.PlatformLinux || hosts[1].Online || hosts[0].Fleet != "Finance" {
		t.Fatalf("%+v", hosts)
	}
	q := state["hosts_query"].(url.Values)
	if q["fleet_id"][0] != "7" || q["label_id"][0] != "3" {
		t.Fatalf("query %v", q)
	}
	only, _ := c.ListHosts(ctx, intent.Scope{Fleets: []string{"Finance"}, Platforms: []intent.Platform{intent.PlatformWindows}})
	if len(only) != 1 {
		t.Fatalf("platform filter: %+v", only)
	}
	if n, _ := c.CountHosts(ctx); n != 42 {
		t.Fatalf("count %d", n)
	}
	res, err := c.LiveQuery(ctx, "SELECT 1", []uint{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if res.Responded != 1 || len(res.RowsFor(1)) != 1 || res.Errors[2] != "offline" || state["deleted"] != true {
		t.Fatalf("%+v deleted=%v", res, state["deleted"])
	}
	if !strings.HasPrefix(state["report"].(map[string]any)["name"].(string), "adm-temp-") {
		t.Fatalf("temp report %v", state["report"])
	}
	sr, err := c.RunScript(ctx, 1, "echo")
	if err != nil || sr.ExitCode != 0 || sr.Output != "done" {
		t.Fatalf("%+v %v", sr, err)
	}
	if err := c.RunCommand(ctx, substrate.Command{Platform: intent.PlatformWindows, Raw: []byte("<SyncBody/>"), HostUUIDs: []string{"u1"}}); err != nil {
		t.Fatal(err)
	}
	if cmd := state["command"].(map[string]any); cmd["command"] != base64.StdEncoding.EncodeToString([]byte("<SyncBody/>")) {
		t.Fatalf("%v", cmd)
	}
	if err := c.AddProfile(ctx, substrate.Profile{Name: "com.adm.screen", Platform: intent.PlatformDarwin, Contents: []byte(`{"Type":"x"}`), Fleet: "Finance", Labels: []string{"laptops"}}); err != nil {
		t.Fatal(err)
	}
	prof := state["profile"].(map[string]any)
	if prof["filename"] != "com.adm.screen.json" || prof["fleet_id"] != "7" || prof["labels"].([]string)[0] != "laptops" {
		t.Fatalf("%v", prof)
	}
	if err := c.UpsertPolicy(ctx, substrate.Policy{Name: "ADM x", Query: "SELECT 1", Platform: intent.PlatformWindows, Fleet: "Finance"}); err != nil {
		t.Fatal(err)
	}
	if state["policy"].(map[string]any)["platform"] != "windows" {
		t.Fatalf("%v", state["policy"])
	}
	if err := c.UpsertPolicy(ctx, substrate.Policy{Name: "dup", Query: "SELECT 1", Fleet: "Finance"}); err != nil {
		t.Fatalf("conflict should be tolerated: %v", err)
	}
	if err := c.Nudge(ctx, nil); err != substrate.ErrUnsupported {
		t.Fatal("nudge should be unsupported")
	}
	bad, _ := New(srv.URL, "wrong")
	if _, err := bad.CountHosts(ctx); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401, got %v", err)
	}
	if _, err := New("", ""); err == nil {
		t.Fatal("expected validation error")
	}
}
