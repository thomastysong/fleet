// Package mcp exposes ADM to AI agents over the Model Context Protocol
// (stdio, newline-delimited JSON-RPC 2.0). Fleet's own MCP server
// (cmd/fleet-mcp) is the read plane: hosts, inventory, live queries. ADM's
// server is the write plane, and every write goes through the same intent,
// risk and approval machinery a human uses: an agent can propose, simulate
// and explain freely, but applying anything above the auto tier needs an
// approval the agent cannot grant itself.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/fleetdm/fleet/v4/adm/events"
	"github.com/fleetdm/fleet/v4/adm/intent"
	"github.com/fleetdm/fleet/v4/adm/plan"
	"github.com/fleetdm/fleet/v4/adm/reason"
	"github.com/fleetdm/fleet/v4/adm/service"
)

// ProtocolVersion is the MCP revision the server speaks.
const ProtocolVersion = "2025-06-18"

// Version is the server version reported to clients.
const Version = "0.1.0"

// Annotations are MCP tool hints.
type Annotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    bool   `json:"readOnlyHint"`
	DestructiveHint bool   `json:"destructiveHint"`
	IdempotentHint  bool   `json:"idempotentHint"`
	OpenWorldHint   bool   `json:"openWorldHint"`
}

// Tool is an MCP tool definition plus its handler.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations Annotations    `json:"annotations"`
	handler     func(ctx context.Context, args map[string]any) (any, error)
}

// Server serves ADM tools.
type Server struct {
	svc   *service.Service
	tools []Tool
	// Principal is the identity every write is attributed to (the MCP
	// client's operator), so an agent cannot approve as someone else.
	Principal string
}

// New builds a server over a service.
func New(svc *service.Service, principal string) *Server {
	s := &Server{svc: svc, Principal: principal}
	s.tools = s.buildTools()
	return s
}

// Tools returns the tool catalog.
func (s *Server) Tools() []Tool { return s.tools }

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Serve reads requests from r and writes responses to w until r ends.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	var mu sync.Mutex
	write := func(resp response) error {
		mu.Lock()
		defer mu.Unlock()
		b, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		_, err = w.Write(append(b, '\n'))
		return err
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			if err := write(response{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}}); err != nil {
				return err
			}
			continue
		}
		resp, respond := s.handle(ctx, req)
		if !respond {
			continue
		}
		if err := write(resp); err != nil {
			return err
		}
	}
	return sc.Err()
}

// Handle processes one request and reports whether a response is due
// (notifications get none).
func (s *Server) handle(ctx context.Context, req request) (response, bool) {
	resp := response{JSONRPC: "2.0", ID: req.ID}
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"
	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "adm-mcp", "version": Version},
			"instructions":    "ADM is an agentic device-management control plane. Propose an intent in plain language with adm_propose, inspect the plan with adm_explain or adm_simulate, and only then adm_apply. Plans above the auto tier need adm_approve by a second person; agents cannot approve their own proposals.",
		}
	case "notifications/initialized", "notifications/cancelled":
		return resp, false
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		list := make([]map[string]any, 0, len(s.tools))
		for _, t := range s.tools {
			list = append(list, map[string]any{"name": t.Name, "description": t.Description, "inputSchema": t.InputSchema, "annotations": t.Annotations})
		}
		resp.Result = map[string]any{"tools": list}
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			resp.Error = &rpcError{Code: -32602, Message: "invalid params"}
			break
		}
		resp.Result = s.call(ctx, p.Name, p.Arguments)
	default:
		if isNotification {
			return resp, false
		}
		resp.Error = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}
	return resp, !isNotification
}

// call runs a tool and formats the MCP result.
func (s *Server) call(ctx context.Context, name string, args map[string]any) map[string]any {
	for _, t := range s.tools {
		if t.Name != name {
			continue
		}
		out, err := t.handler(ctx, args)
		if err != nil {
			return map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": err.Error()}}}
		}
		text, ok := out.(string)
		if !ok {
			b, _ := json.MarshalIndent(out, "", "  ")
			text = string(b)
		}
		return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "structuredContent": structured(out)}
	}
	return map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": "unknown tool " + name}}}
}

func structured(v any) any {
	if _, isString := v.(string); isString {
		return nil
	}
	return v
}

func str(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return strings.TrimSpace(v)
}

func boolean(args map[string]any, key string) bool {
	v, _ := args[key].(bool)
	return v
}

func schema(props map[string]any, required ...string) map[string]any {
	out := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func (s *Server) buildTools() []Tool {
	read := Annotations{ReadOnlyHint: true, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false}
	planID := map[string]any{"plan_id": map[string]any{"type": "string", "description": "Plan ID returned by adm_propose"}}
	return []Tool{
		{
			Name: "adm_list_capabilities", Description: "List the ADM capability catalog: what can be observed or changed, with parameters and the native mechanism per platform.",
			InputSchema: schema(map[string]any{"domain": map[string]any{"type": "string", "description": "Optional domain filter (security, software, network, storage, identity, ai, signage, os, enrollment, telemetry, lifecycle)"}}),
			Annotations: read,
			handler: func(_ context.Context, args map[string]any) (any, error) {
				var out []map[string]any
				for _, c := range s.svc.Catalog.All() {
					if d := str(args, "domain"); d != "" && string(c.Domain) != d {
						continue
					}
					plats := map[string]string{}
					for p, b := range c.Bindings {
						plats[string(p)] = string(b.Mechanism) + ": " + b.Native
					}
					out = append(out, map[string]any{"name": c.Name, "domain": c.Domain, "summary": c.Summary, "params": c.Params, "reversible": c.Reversible, "destructive": c.Destructive, "base_risk": c.BaseRisk, "platforms": plats})
				}
				return out, nil
			},
		},
		{
			Name: "adm_propose", Description: "Compile a plain-language request into an ADM intent and a risk-scored plan. Nothing is changed on any device. Returns the plan, its simulation, the autonomy tier and the next step.",
			InputSchema: schema(map[string]any{
				"text":   map[string]any{"type": "string", "description": "What the operator wants, in plain language"},
				"author": map[string]any{"type": "string", "description": "Who is asking (defaults to the MCP principal)"},
			}, "text"),
			Annotations: Annotations{ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: false, OpenWorldHint: true},
			handler: func(ctx context.Context, args map[string]any) (any, error) {
				author := str(args, "author")
				if author == "" {
					author = s.Principal
				}
				prop, err := s.svc.Propose(ctx, reason.Request{Text: str(args, "text"), Author: author})
				if err != nil {
					return nil, err
				}
				return map[string]any{
					"intent_id": prop.Intent.ID, "plan_id": prop.Plan.ID, "summary": prop.Summary,
					"tier": prop.Plan.Risk.Tier.String(), "score": prop.Plan.Risk.Score, "reasons": prop.Plan.Risk.Reasons,
					"hosts": prop.Plan.Hosts, "steps": stepsView(prop.Plan), "waves": len(prop.Plan.Waves),
					"violations": prop.Plan.Violations, "unsupported": prop.Plan.Unsupported,
					"clarifications": prop.Clarifications, "next_step": prop.NextStep, "gitops": prop.Plan.GitOps,
				}, nil
			},
		},
		{
			Name: "adm_list_plans", Description: "List plans with status, tier and host counts.",
			InputSchema: schema(map[string]any{}), Annotations: read,
			handler: func(ctx context.Context, _ map[string]any) (any, error) {
				plans, err := s.svc.ListPlans(ctx)
				if err != nil {
					return nil, err
				}
				var out []map[string]any
				for _, p := range plans {
					out = append(out, map[string]any{"plan_id": p.ID, "intent_id": p.IntentID, "text": p.Intent.Text, "status": p.Status, "tier": p.Risk.Tier.String(), "score": p.Risk.Score, "hosts": p.Hosts, "created_at": p.CreatedAt})
				}
				return out, nil
			},
		},
		{
			Name: "adm_get_plan", Description: "Get a plan in full: steps, waves, rollback, risk assessment and GitOps rendering.",
			InputSchema: schema(planID, "plan_id"), Annotations: read,
			handler: func(ctx context.Context, args map[string]any) (any, error) {
				return s.svc.GetPlan(ctx, str(args, "plan_id"))
			},
		},
		{
			Name: "adm_simulate", Description: "Dry-run a plan: which hosts currently drift from the intent and what would change, without applying anything.",
			InputSchema: schema(planID, "plan_id"), Annotations: read,
			handler: func(ctx context.Context, args map[string]any) (any, error) {
				p, err := s.svc.GetPlan(ctx, str(args, "plan_id"))
				if err != nil {
					return nil, err
				}
				rep, err := s.svc.Apply(ctx, p.ID, s.Principal, true)
				if err != nil {
					return nil, err
				}
				return map[string]any{"simulation": plan.Simulate(p), "drift_hosts": rep.Drift, "summary": rep.Summary}, nil
			},
		},
		{
			Name: "adm_explain", Description: "Explain a plan: what it does natively on each platform, why it got its autonomy tier, how it rolls back, and its history.",
			InputSchema: schema(planID, "plan_id"), Annotations: read,
			handler: func(ctx context.Context, args map[string]any) (any, error) {
				ex, err := s.svc.Explain(ctx, str(args, "plan_id"))
				if err != nil {
					return nil, err
				}
				return map[string]any{"narrative": ex.Narrative, "rollback": ex.RollbackHow, "leash": ex.Leash, "history": ex.History, "simulation": ex.Simulation}, nil
			},
		},
		{
			Name: "adm_approve", Description: "Approve a plan on behalf of the MCP principal. The author of an intent cannot approve their own approve-tier plan; CAB-tier plans need a change ticket.",
			InputSchema: schema(map[string]any{
				"plan_id": planID["plan_id"],
				"note":    map[string]any{"type": "string"},
				"ticket":  map[string]any{"type": "string", "description": "Change ticket reference (required for CAB tier)"},
			}, "plan_id"),
			Annotations: Annotations{ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
			handler: func(ctx context.Context, args map[string]any) (any, error) {
				if s.Principal == "" {
					return nil, errors.New("this MCP server has no principal; approvals must come from an identified human")
				}
				p, err := s.svc.Approve(ctx, str(args, "plan_id"), s.Principal, "mcp", str(args, "note"), str(args, "ticket"))
				if err != nil {
					return nil, err
				}
				return map[string]any{"plan_id": p.ID, "status": p.Status, "approved_by": p.Approval.By}, nil
			},
		},
		{
			Name: "adm_reject", Description: "Reject a plan; the pattern's adaptive autonomy learns from the rejection.",
			InputSchema: schema(map[string]any{"plan_id": planID["plan_id"], "note": map[string]any{"type": "string"}}, "plan_id"),
			Annotations: Annotations{ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
			handler: func(ctx context.Context, args map[string]any) (any, error) {
				p, err := s.svc.Reject(ctx, str(args, "plan_id"), s.Principal, str(args, "note"))
				if err != nil {
					return nil, err
				}
				return map[string]any{"plan_id": p.ID, "status": p.Status}, nil
			},
		},
		{
			Name: "adm_apply", Description: "Execute a plan wave by wave with verification and automatic rollback. Refuses plans that need an approval they do not have. Use dry_run=true to only report drift.",
			InputSchema: schema(map[string]any{"plan_id": planID["plan_id"], "dry_run": map[string]any{"type": "boolean"}}, "plan_id"),
			Annotations: Annotations{ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: true, OpenWorldHint: true},
			handler: func(ctx context.Context, args map[string]any) (any, error) {
				rep, err := s.svc.Apply(ctx, str(args, "plan_id"), s.Principal, boolean(args, "dry_run"))
				if err != nil {
					return nil, err
				}
				return rep, nil
			},
		},
		{
			Name: "adm_leash", Description: "Show the adaptive-autonomy (leash) state of a pattern of change: how often it was accepted, verified, rejected, and whether incidents suspended its relaxation.",
			InputSchema: schema(map[string]any{"pattern": map[string]any{"type": "string", "description": "Pattern key, e.g. firewall.enable@darwin,windows"}}, "pattern"),
			Annotations: read,
			handler: func(ctx context.Context, args map[string]any) (any, error) {
				return s.svc.Leash(ctx, str(args, "pattern"))
			},
		},
		{
			Name: "adm_event", Description: "Feed an external event into the reconciler (for example a lakehouse detector saw drift, or a vulnerability was published). Affected intents are re-evaluated immediately.",
			InputSchema: schema(map[string]any{
				"type":     map[string]any{"type": "string", "description": "Event type, e.g. config.drift, storage.attached, vuln.published, host.enrolled"},
				"host_id":  map[string]any{"type": "integer"},
				"platform": map[string]any{"type": "string"},
				"subject":  map[string]any{"type": "string"},
			}, "type"),
			Annotations: Annotations{ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: false, OpenWorldHint: true},
			handler: func(ctx context.Context, args map[string]any) (any, error) {
				var hostID uint
				if f, ok := args["host_id"].(float64); ok {
					hostID = uint(f)
				}
				plans, err := s.svc.Reconcile(ctx, events.Event{Type: events.Type(str(args, "type")), Source: "mcp:" + s.Principal, HostID: hostID, Platform: intent.Platform(str(args, "platform")), Subject: str(args, "subject")})
				if err != nil {
					return nil, err
				}
				var out []map[string]any
				for _, p := range plans {
					out = append(out, map[string]any{"plan_id": p.ID, "intent_id": p.IntentID, "status": p.Status, "tier": p.Risk.Tier.String(), "hosts": p.Hosts})
				}
				return map[string]any{"plans": out, "count": len(out)}, nil
			},
		},
	}
}

func stepsView(p *plan.Plan) []map[string]any {
	var out []map[string]any
	for _, st := range p.Steps {
		out = append(out, map[string]any{"capability": st.Capability, "platform": st.Platform, "mechanism": st.Mechanism, "native": st.Native, "hosts": len(st.Targets), "reversible": st.Reversible})
	}
	return out
}

// Describe renders the tool list for humans.
func (s *Server) Describe() string {
	var b strings.Builder
	for _, t := range s.tools {
		fmt.Fprintf(&b, "%-22s %s\n", t.Name, t.Description)
	}
	return b.String()
}
