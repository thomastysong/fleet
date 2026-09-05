package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fleetdm/fleet/v4/adm/service"
)

func rpc(t *testing.T, srv *Server, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := srv.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var resps []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("bad response %q: %v", l, err)
		}
		resps = append(resps, m)
	}
	return resps
}

func result(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	if e, ok := resp["error"]; ok && e != nil {
		t.Fatalf("rpc error: %v", e)
	}
	r, _ := resp["result"].(map[string]any)
	return r
}

func TestInitializeAndListTools(t *testing.T) {
	srv := New(service.New(service.Config{}), "alice")
	resps := rpc(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"nope"}`,
	)
	if len(resps) != 3 {
		t.Fatalf("expected 3 responses (notification gets none), got %d", len(resps))
	}
	init := result(t, resps[0])
	if init["protocolVersion"] != ProtocolVersion {
		t.Fatalf("%v", init)
	}
	tools := result(t, resps[1])["tools"].([]any)
	if len(tools) != 11 {
		t.Fatalf("tools %d", len(tools))
	}
	for _, tl := range tools {
		m := tl.(map[string]any)
		ann := m["annotations"].(map[string]any)
		if m["name"] == "adm_apply" && ann["destructiveHint"] != true {
			t.Fatal("adm_apply must be destructive")
		}
		if m["name"] == "adm_explain" && ann["readOnlyHint"] != true {
			t.Fatal("adm_explain must be read-only")
		}
	}
	if resps[2]["error"] == nil {
		t.Fatal("unknown method should error")
	}
}

func TestProposeApproveApplyViaMCP(t *testing.T) {
	srv := New(service.New(service.Config{}), "bob")
	resps := rpc(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"adm_propose","arguments":{"text":"lock the device mbp-alice in Engineering","author":"alice"}}}`)
	res := result(t, resps[0])
	if res["isError"] == true {
		t.Fatalf("%v", res)
	}
	sc := res["structuredContent"].(map[string]any)
	planID, _ := sc["plan_id"].(string)
	if planID == "" || sc["tier"] != "approve" {
		t.Fatalf("%v", sc)
	}
	// Applying before approval is refused with a clean tool error.
	resps = rpc(t, srv, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"adm_apply","arguments":{"plan_id":"`+planID+`"}}}`)
	if r := result(t, resps[0]); r["isError"] != true {
		t.Fatalf("expected refusal: %v", r)
	}
	// bob (the principal) approves alice's plan, then simulates and explains.
	resps = rpc(t, srv,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"adm_approve","arguments":{"plan_id":"`+planID+`","note":"ok"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"adm_explain","arguments":{"plan_id":"`+planID+`"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"adm_leash","arguments":{"pattern":"host.lock@any"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"adm_list_plans","arguments":{}}}`,
	)
	if r := result(t, resps[0]); r["isError"] == true || r["structuredContent"].(map[string]any)["approved_by"] != "bob" {
		t.Fatalf("%v", r)
	}
	if r := result(t, resps[1]); r["isError"] == true || len(r["structuredContent"].(map[string]any)["narrative"].([]any)) < 3 {
		t.Fatalf("%v", r)
	}
	if r := result(t, resps[2]); r["isError"] == true {
		t.Fatalf("%v", r)
	}
	if r := result(t, resps[3]); len(r["structuredContent"].([]any)) != 1 {
		t.Fatalf("%v", r)
	}
	// Bad input and unknown tools are reported as tool errors, not crashes.
	resps = rpc(t, srv, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"adm_get_plan","arguments":{"plan_id":"missing"}}}`, `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"nope","arguments":{}}}`, `not json`)
	if result(t, resps[0])["isError"] != true || result(t, resps[1])["isError"] != true || resps[2]["error"] == nil {
		t.Fatalf("%v", resps)
	}
}

func TestPrincipalRequiredForApproval(t *testing.T) {
	srv := New(service.New(service.Config{}), "")
	resps := rpc(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"adm_approve","arguments":{"plan_id":"x"}}}`)
	if r := result(t, resps[0]); r["isError"] != true || !strings.Contains(r["content"].([]any)[0].(map[string]any)["text"].(string), "principal") {
		t.Fatalf("%v", r)
	}
	if !strings.Contains(srv.Describe(), "adm_propose") {
		t.Fatal("describe should list tools")
	}
}
