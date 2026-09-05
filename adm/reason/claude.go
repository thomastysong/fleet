package reason

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/fleetdm/fleet/v4/adm/capability"
	"github.com/fleetdm/fleet/v4/adm/intent"
)

// DefaultModel is the model the Claude compiler uses unless configured.
const DefaultModel = "claude-opus-5"

// DefaultFallbackModel serves a request the primary model declines.
const DefaultFallbackModel = "claude-opus-4-8"

// MessagesAPI is the slice of the Anthropic SDK the compiler needs; the
// SDK's client.Beta.Messages satisfies it and tests supply a fake.
type MessagesAPI interface {
	New(ctx context.Context, params anthropic.BetaMessageNewParams, opts ...option.RequestOption) (*anthropic.BetaMessage, error)
}

// Claude compiles natural language with the Claude API. It asks the model to
// call a single tool, emit_intent, whose input is the Intent IR; the tool
// input is validated against the catalog before anything else happens.
type Claude struct {
	API      MessagesAPI
	Model    string
	Fallback string
	Effort   anthropic.BetaOutputConfigEffort
	Catalog  *capability.Catalog
	Now      func() time.Time
	NewID    func() string
	// MaxTokens bounds the response; the IR is small, so 16k is generous.
	MaxTokens int64
}

// NewClaude returns a compiler using the SDK's default credential resolution
// (ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN or an `ant auth login` profile).
func NewClaude(cat *capability.Catalog, opts ...option.RequestOption) *Claude {
	client := anthropic.NewClient(opts...)
	return NewClaudeWithAPI(cat, &client.Beta.Messages)
}

// NewClaudeWithAPI returns a compiler over an explicit API (tests).
func NewClaudeWithAPI(cat *capability.Catalog, api MessagesAPI) *Claude {
	return &Claude{
		API: api, Model: DefaultModel, Fallback: DefaultFallbackModel, Effort: anthropic.BetaOutputConfigEffortHigh,
		Catalog: cat, Now: time.Now, MaxTokens: 16000, NewID: NewIntentID,
	}
}

func (c *Claude) Name() string { return "claude" }

// emitted is the tool input schema, mirrored as a Go struct.
type emitted struct {
	Summary string `json:"summary"`
	Scope   struct {
		Fleets        []string `json:"fleets"`
		Labels        []string `json:"labels"`
		ExcludeLabels []string `json:"exclude_labels"`
		Platforms     []string `json:"platforms"`
		Query         string   `json:"query"`
		HostIDs       []uint   `json:"host_ids"`
	} `json:"scope"`
	Desired []struct {
		Capability string         `json:"capability"`
		Params     map[string]any `json:"params"`
		Rationale  string         `json:"rationale"`
	} `json:"desired"`
	Constraints struct {
		MaxAutonomy    string `json:"max_autonomy"`
		CanaryPercent  int    `json:"canary_percent"`
		MaxWaveHosts   int    `json:"max_wave_hosts"`
		MaxUserImpact  string `json:"max_user_impact"`
		AllowFleetWide bool   `json:"allow_fleet_wide"`
	} `json:"constraints"`
	Confidence     float64  `json:"confidence"`
	Clarifications []string `json:"clarifications"`
}

// SystemPrompt renders the stable system prompt: what ADM is, the catalog,
// and the rules. It contains nothing request-specific so it caches.
func SystemPrompt(cat *capability.Catalog) string {
	var b strings.Builder
	b.WriteString(`You are the intent compiler for ADM (Agentic Device Management), a cross-platform device management control plane built on Fleet (osquery) and native OS APIs. An operator states what they want their fleet to look like in plain language. You translate it into an Intent: a scope (which devices), desired predicates (capabilities from the catalog below with parameters), constraints (autonomy, rollout) and honest confidence.

You never execute anything. A deterministic planner compiles the Intent to native, reversible, verifiable actions per platform, and a risk engine decides how much autonomy it gets. Your job is fidelity to what the operator meant.

Rules:
- Use only capabilities from the catalog, with the documented parameters. Never invent capabilities or parameters. If the ask needs something not in the catalog, leave it out and say so in clarifications.
- Scope must reflect the operator's words. Map named teams to fleets and named groups to labels using the tenant hints in the message. Only set allow_fleet_wide when the operator clearly means every device ("everywhere", "the whole fleet", "all devices"). If no scope can be inferred, keep scope empty, set allow_fleet_wide=false, and ask which devices in clarifications.
- max_autonomy: "ask me first"/"approval" -> approve; "gradually"/"canary" -> canary; "just watch"/"report only" -> observe; "automatically" -> auto; otherwise omit.
- Prefer the most specific capability. A generic "keep it secure" means disk.encryption.enforce, firewall.enable and screen.lock.enforce (idle_minutes 10) unless the operator says otherwise.
- Destructive asks (host.wipe) only when explicitly requested, and always with a clarification confirming the exact device.
- confidence is your probability that a careful IT engineer would agree this Intent captures the ask: 0.9+ only for unambiguous requests, below 0.6 when you had to guess.
- Respond by calling the emit_intent tool exactly once. Do not answer in prose.

Capability catalog:
`)
	for _, cap := range cat.All() {
		fmt.Fprintf(&b, "- %s [%s]: %s", cap.Name, cap.Domain, cap.Summary)
		if len(cap.Params) > 0 {
			var ps []string
			for _, p := range cap.Params {
				req := ""
				if p.Required {
					req = ", required"
				}
				ps = append(ps, fmt.Sprintf("%s (%s%s): %s", p.Name, p.Type, req, p.Description))
			}
			fmt.Fprintf(&b, " Params: %s.", strings.Join(ps, "; "))
		}
		plats := make([]string, 0, len(cap.Bindings))
		for _, p := range cap.Platforms() {
			plats = append(plats, string(p))
		}
		fmt.Fprintf(&b, " Platforms: %s.", strings.Join(plats, ", "))
		if cap.Destructive {
			b.WriteString(" DESTRUCTIVE.")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (c *Claude) tool() anthropic.BetaToolParam {
	names := c.Catalog.Names()
	plats := make([]string, 0, len(intent.AllPlatforms))
	for _, p := range intent.AllPlatforms {
		plats = append(plats, string(p))
	}
	strList := map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	return anthropic.BetaToolParam{
		Name:        "emit_intent",
		Description: anthropic.String("Emit the compiled Intent for the operator's request."),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Properties: map[string]any{
				"summary": map[string]any{"type": "string", "description": "One sentence restating the ask in ADM terms."},
				"scope": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"fleets": strList, "labels": strList, "exclude_labels": strList,
						"platforms": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": plats}},
						"query":     map[string]any{"type": "string", "description": "Optional osquery SQL predicate refining membership."},
						"host_ids":  map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
					},
				},
				"desired": map[string]any{
					"type": "array", "minItems": 1,
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"capability": map[string]any{"type": "string", "enum": names},
							"params":     map[string]any{"type": "object"},
							"rationale":  map[string]any{"type": "string"},
						},
						"required": []string{"capability", "params", "rationale"},
					},
				},
				"constraints": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"max_autonomy":     map[string]any{"type": "string", "enum": []string{"", "observe", "auto", "canary", "approve", "cab"}},
						"canary_percent":   map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
						"max_wave_hosts":   map[string]any{"type": "integer", "minimum": 0},
						"max_user_impact":  map[string]any{"type": "string", "enum": []string{"", "none", "prompt", "logout", "restart"}},
						"allow_fleet_wide": map[string]any{"type": "boolean"},
					},
				},
				"confidence":     map[string]any{"type": "number", "minimum": 0, "maximum": 1},
				"clarifications": strList,
			},
			Required: []string{"summary", "scope", "desired", "constraints", "confidence", "clarifications"},
		},
	}
}

// Compile calls the model and validates its output.
func (c *Claude) Compile(ctx context.Context, req Request) (*Result, error) {
	if c.API == nil {
		return nil, errors.New("claude compiler: no API configured")
	}
	var user strings.Builder
	fmt.Fprintf(&user, "Tenant hints:\n- fleets: %s\n- labels: %s\n", orNone(req.Fleets), orNone(req.Labels))
	if len(req.Platforms) > 0 {
		ps := make([]string, len(req.Platforms))
		for i, p := range req.Platforms {
			ps[i] = string(p)
		}
		fmt.Fprintf(&user, "- platform context: %s\n", strings.Join(ps, ", "))
	}
	fmt.Fprintf(&user, "\nOperator (%s) says:\n%s\n\nCall emit_intent.", orDash(req.Author), strings.TrimSpace(req.Text))

	tool := c.tool()
	params := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(c.Model),
		MaxTokens: c.MaxTokens,
		System: []anthropic.BetaTextBlockParam{{
			Text:         SystemPrompt(c.Catalog),
			CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
		}},
		Messages:     []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(user.String()))},
		Tools:        []anthropic.BetaToolUnionParam{{OfTool: &tool}},
		OutputConfig: anthropic.BetaOutputConfigParam{Effort: c.Effort},
		// Server-side refusal fallback: a policy decline on the primary model
		// is re-served by the fallback model inside the same call.
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_06_01},
		Fallbacks: anthropic.BetaFallbacksParamUnion{OfBetaFallbackArray: []anthropic.BetaFallbackParam{{Model: anthropic.Model(c.Fallback)}}},
	}
	resp, err := c.API.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("claude compiler: %w", err)
	}
	if resp.StopReason == anthropic.BetaStopReasonRefusal {
		return nil, fmt.Errorf("claude compiler: request declined (%s): %s", resp.StopDetails.Category, resp.StopDetails.Explanation)
	}
	var out *emitted
	var notes []string
	for _, block := range resp.Content {
		switch v := block.AsAny().(type) {
		case anthropic.BetaToolUseBlock:
			if v.Name != "emit_intent" {
				continue
			}
			var e emitted
			if err := json.Unmarshal([]byte(v.JSON.Input.Raw()), &e); err != nil {
				return nil, fmt.Errorf("claude compiler: malformed tool input: %w", err)
			}
			out = &e
		case anthropic.BetaTextBlock:
			if strings.TrimSpace(v.Text) != "" {
				notes = append(notes, strings.TrimSpace(v.Text))
			}
		}
	}
	if out == nil {
		return nil, fmt.Errorf("claude compiler: model did not call emit_intent (stop_reason=%s; %s)", resp.StopReason, strings.Join(notes, " "))
	}
	in, err := c.toIntent(req, out, string(resp.Model))
	if err != nil {
		return nil, err
	}
	return &Result{Intent: in, Clarifications: out.Clarifications, Summary: out.Summary}, nil
}

func (c *Claude) toIntent(req Request, e *emitted, model string) (*intent.Intent, error) {
	now := c.Now()
	in := &intent.Intent{ID: c.NewID(), Text: req.Text, Author: req.Author, Source: intent.SourceNaturalLanguage, State: intent.StateDraft, Version: 1, CreatedAt: now, UpdatedAt: now}
	in.Scope = intent.Scope{Fleets: e.Scope.Fleets, Labels: e.Scope.Labels, ExcludeLabels: e.Scope.ExcludeLabels, Query: e.Scope.Query, HostIDs: e.Scope.HostIDs}
	for _, p := range e.Scope.Platforms {
		in.Scope.Platforms = append(in.Scope.Platforms, intent.Platform(p))
	}
	sort.Slice(in.Scope.Platforms, func(i, j int) bool { return in.Scope.Platforms[i] < in.Scope.Platforms[j] })
	for _, d := range e.Desired {
		in.Desired = append(in.Desired, intent.Predicate{Capability: d.Capability, Params: d.Params, Rationale: d.Rationale})
	}
	in.Constraints = intent.Constraints{
		MaxAutonomy:    intent.Autonomy(e.Constraints.MaxAutonomy),
		CanaryPercent:  e.Constraints.CanaryPercent,
		MaxWaveHosts:   e.Constraints.MaxWaveHosts,
		MaxUserImpact:  intent.UserImpact(e.Constraints.MaxUserImpact),
		AllowFleetWide: e.Constraints.AllowFleetWide,
	}
	conf := e.Confidence
	if len(e.Clarifications) > 0 && conf > 0.7 {
		conf = 0.7
	}
	in.Provenance = intent.Provenance{Compiler: c.Name(), Model: model, PromptHash: intent.PromptHash(req.Text), Confidence: conf}
	if err := ValidateAgainstCatalog(in, c.Catalog); err != nil {
		return nil, fmt.Errorf("claude compiler: model output rejected: %w", err)
	}
	return in, nil
}

func orNone(list []string) string {
	if len(list) == 0 {
		return "(none)"
	}
	return strings.Join(list, ", ")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
