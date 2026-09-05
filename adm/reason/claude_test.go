package reason

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/fleetdm/fleet/v4/adm/capability"
	"github.com/fleetdm/fleet/v4/adm/intent"
)

type fakeAPI struct {
	params anthropic.BetaMessageNewParams
	raw    string
	err    error
}

func (f *fakeAPI) New(_ context.Context, params anthropic.BetaMessageNewParams, _ ...option.RequestOption) (*anthropic.BetaMessage, error) {
	f.params = params
	if f.err != nil {
		return nil, f.err
	}
	var msg anthropic.BetaMessage
	if err := json.Unmarshal([]byte(f.raw), &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

const toolUse = `{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5","stop_reason":"tool_use","stop_sequence":null,
"content":[{"type":"text","text":"Compiling."},{"type":"tool_use","id":"tu_1","name":"emit_intent","input":{
 "summary":"Block removable storage on Finance Windows laptops with approval.",
 "scope":{"fleets":["Finance"],"labels":["laptops"],"exclude_labels":[],"platforms":["windows"],"query":"","host_ids":[]},
 "desired":[{"capability":"storage.removable.block","params":{"mode":"block"},"rationale":"operator asked to block USB drives"}],
 "constraints":{"max_autonomy":"approve","canary_percent":0,"max_wave_hosts":0,"max_user_impact":"","allow_fleet_wide":false},
 "confidence":0.93,"clarifications":[]}}],
"usage":{"input_tokens":10,"output_tokens":20}}`

func TestClaudeCompilesToolOutput(t *testing.T) {
	api := &fakeAPI{raw: toolUse}
	c := NewClaudeWithAPI(capability.Default(), api)
	res, err := c.Compile(context.Background(), Request{Text: "Block USB drives on Finance Windows laptops, ask me first", Author: "alice", Fleets: []string{"Finance"}, Labels: []string{"laptops"}})
	if err != nil {
		t.Fatal(err)
	}
	in := res.Intent
	if in.Desired[0].Capability != "storage.removable.block" || in.Scope.Fleets[0] != "Finance" || in.Scope.Platforms[0] != intent.PlatformWindows || in.Constraints.MaxAutonomy != intent.AutonomyApprove {
		t.Fatalf("%+v", in)
	}
	if in.Provenance.Compiler != "claude" || in.Provenance.Model != "claude-opus-5" || in.Provenance.Confidence != 0.93 {
		t.Fatalf("provenance %+v", in.Provenance)
	}
	// Request shape: default model, cached system prompt, the tool, fallback beta.
	if api.params.Model != "claude-opus-5" || len(api.params.System) != 1 || api.params.System[0].CacheControl.Type != "ephemeral" {
		t.Fatalf("params %+v", api.params)
	}
	if len(api.params.Tools) != 1 || api.params.Tools[0].OfTool.Name != "emit_intent" {
		t.Fatal("emit_intent tool missing")
	}
	if len(api.params.Betas) != 1 || api.params.Betas[0] != anthropic.AnthropicBetaServerSideFallback2026_06_01 || len(api.params.Fallbacks.OfBetaFallbackArray) != 1 {
		t.Fatalf("fallback not configured: %+v", api.params)
	}
	if !strings.Contains(SystemPrompt(capability.Default()), "storage.removable.block") {
		t.Fatal("system prompt should list the catalog")
	}
}

func TestClaudeRejectsUnknownCapabilityAndRefusals(t *testing.T) {
	bad := strings.Replace(toolUse, "storage.removable.block", "storage.teleport", 1)
	c := NewClaudeWithAPI(capability.Default(), &fakeAPI{raw: bad})
	if _, err := c.Compile(context.Background(), Request{Text: "x"}); err == nil || !strings.Contains(err.Error(), "unknown capability") {
		t.Fatalf("expected catalog rejection, got %v", err)
	}
	refused := `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5","stop_reason":"refusal","stop_details":{"type":"refusal","category":"cyber","explanation":"nope"},"content":[],"usage":{"input_tokens":1,"output_tokens":1}}`
	c2 := NewClaudeWithAPI(capability.Default(), &fakeAPI{raw: refused})
	if _, err := c2.Compile(context.Background(), Request{Text: "x"}); err == nil || !strings.Contains(err.Error(), "declined") {
		t.Fatalf("expected refusal error, got %v", err)
	}
	noTool := `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5","stop_reason":"end_turn","content":[{"type":"text","text":"I need more details."}],"usage":{"input_tokens":1,"output_tokens":1}}`
	c3 := NewClaudeWithAPI(capability.Default(), &fakeAPI{raw: noTool})
	if _, err := c3.Compile(context.Background(), Request{Text: "x"}); err == nil || !strings.Contains(err.Error(), "did not call emit_intent") {
		t.Fatalf("expected missing tool error, got %v", err)
	}
	c4 := NewClaudeWithAPI(capability.Default(), &fakeAPI{err: errors.New("network down")})
	chain := &Chain{Compilers: []Compiler{c4, NewDeterministic(capability.Default())}}
	res, err := chain.Compile(context.Background(), Request{Text: "enable the firewall everywhere"})
	if err != nil || res.Intent.Provenance.Compiler != "deterministic" {
		t.Fatalf("chain should fall back to deterministic: %v %+v", err, res)
	}
}
