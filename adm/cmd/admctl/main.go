// Command admctl is the ADM command line: compile plain language into
// intents and plans, inspect and approve them, apply them, explain them,
// feed events to the reconciler, and serve the MCP write plane.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/fleetdm/fleet/v4/adm/bridge"
	"github.com/fleetdm/fleet/v4/adm/capability"
	"github.com/fleetdm/fleet/v4/adm/events"
	"github.com/fleetdm/fleet/v4/adm/intent"
	"github.com/fleetdm/fleet/v4/adm/learn"
	"github.com/fleetdm/fleet/v4/adm/mcp"
	"github.com/fleetdm/fleet/v4/adm/native"
	"github.com/fleetdm/fleet/v4/adm/native/darwin"
	"github.com/fleetdm/fleet/v4/adm/native/demo"
	"github.com/fleetdm/fleet/v4/adm/native/windows"
	"github.com/fleetdm/fleet/v4/adm/plan"
	"github.com/fleetdm/fleet/v4/adm/reason"
	"github.com/fleetdm/fleet/v4/adm/risk"
	"github.com/fleetdm/fleet/v4/adm/service"
	"github.com/fleetdm/fleet/v4/adm/substrate"
	fleetsub "github.com/fleetdm/fleet/v4/adm/substrate/fleet"
)

const usage = `admctl — Agentic Device Management

Usage:
  admctl [flags] <command> [args]

Flags:
  --state-dir DIR       where intents, plans and the ledger live (default ~/.adm)
  --demo                use the built-in demo fleet and demo adapters (no devices touched)
  --fleet-url URL       Fleet server URL (with --fleet-token; an API-only user)
  --fleet-token TOKEN   Fleet API token (or FLEET_API_TOKEN)
  --claude              compile with Claude (ANTHROPIC_API_KEY); falls back to rules
  --principal NAME      who you are, for approvals and the ledger (default $USER)
  --json                print machine-readable output

Commands:
  capabilities                       list the catalog
  propose "<text>" [--author NAME]   compile plain language into an intent and a plan
  plans                              list plans
  plan <id>                          show a plan
  simulate <id>                      dry run: which hosts drift, what would change
  explain <id>                       what, where, how, why this tier, how to undo, history
  approve <id> [--note N] [--ticket T]
  reject <id> [--note N]
  apply <id> [--dry-run]             execute wave by wave with verification and rollback
  leash <pattern>                    adaptive-autonomy state of a pattern
  event --type T [--host N] [--platform P] [--subject S]
  mcp                                serve the MCP write plane on stdin/stdout
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "admctl:", err)
		os.Exit(1)
	}
}

type options struct {
	stateDir, fleetURL, fleetToken, principal string
	demo, claude, jsonOut                     bool
}

func run(args []string) error {
	fs := flag.NewFlagSet("admctl", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	var o options
	home, _ := os.UserHomeDir()
	fs.StringVar(&o.stateDir, "state-dir", filepath.Join(home, ".adm"), "")
	fs.BoolVar(&o.demo, "demo", false, "")
	fs.StringVar(&o.fleetURL, "fleet-url", os.Getenv("FLEET_URL"), "")
	fs.StringVar(&o.fleetToken, "fleet-token", os.Getenv("FLEET_API_TOKEN"), "")
	fs.BoolVar(&o.claude, "claude", false, "")
	fs.StringVar(&o.principal, "principal", os.Getenv("USER"), "")
	fs.BoolVar(&o.jsonOut, "json", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return errors.New("a command is required")
	}
	if !o.demo && o.fleetURL == "" {
		return errors.New("choose --demo or --fleet-url (with --fleet-token)")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	svc, err := build(o)
	if err != nil {
		return err
	}
	cmd, cargs := rest[0], rest[1:]
	switch cmd {
	case "capabilities":
		return capabilities(svc, o)
	case "propose":
		return propose(ctx, svc, o, cargs)
	case "plans":
		return plans(ctx, svc, o)
	case "plan":
		return showPlan(ctx, svc, o, cargs)
	case "simulate":
		return simulate(ctx, svc, o, cargs)
	case "explain":
		return explain(ctx, svc, o, cargs)
	case "approve":
		return approve(ctx, svc, o, cargs)
	case "reject":
		return reject(ctx, svc, o, cargs)
	case "apply":
		return apply(ctx, svc, o, cargs)
	case "leash":
		return leash(ctx, svc, o, cargs)
	case "event":
		return event(ctx, svc, o, cargs)
	case "mcp":
		return mcp.New(svc, o.principal).Serve(ctx, os.Stdin, os.Stdout)
	case "help", "-h", "--help":
		fs.Usage()
		return nil
	}
	fs.Usage()
	return fmt.Errorf("unknown command %q", cmd)
}

func build(o options) (*service.Service, error) {
	if err := os.MkdirAll(o.stateDir, 0o700); err != nil {
		return nil, err
	}
	store, err := service.NewFileStore(o.stateDir)
	if err != nil {
		return nil, err
	}
	ledger, err := learn.NewFileLedger(filepath.Join(o.stateDir, "outcomes.jsonl"))
	if err != nil {
		return nil, err
	}
	cat := capability.Default()
	reg := native.NewRegistry()
	var sub substrate.Substrate
	if o.demo {
		mem := substrate.NewMemory(substrate.DemoFleet()...)
		demo.Install(reg, mem, cat, demo.NewWorld())
		sub = mem
	} else {
		client, err := fleetsub.New(o.fleetURL, o.fleetToken)
		if err != nil {
			return nil, err
		}
		sub = client
		// Real adapters that reach devices through the substrate today:
		// Windows CSPs (reads via mdm_bridge, writes via the MDM command
		// queue) and Apple declarations (as Fleet profiles). Everything else
		// waits for the ADM agent extension; the planner reports it as
		// unsupported rather than falling back to a script.
		reg.MustRegister(windows.NewAdapters(bridge.NewWindowsMDM(client))...)
		reg.MustRegister(darwin.NewAdapters(bridge.NewApplePusher(client, "", nil))...)
	}
	var compiler reason.Compiler = reason.NewDeterministic(cat)
	if o.claude {
		compiler = &reason.Chain{Compilers: []reason.Compiler{reason.NewClaude(cat), reason.NewDeterministic(cat)}}
	}
	pol := risk.DefaultPolicy()
	return service.New(service.Config{Catalog: cat, Policy: &pol, Compiler: compiler, Substrate: sub, Adapters: reg, Ledger: ledger, Store: store, Bus: events.NewMemoryBus()}), nil
}

func emit(o options, v any, human func()) error {
	if o.jsonOut {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	human()
	return nil
}

func capabilities(svc *service.Service, o options) error {
	all := svc.Catalog.All()
	return emit(o, all, func() {
		for _, c := range all {
			mode := ""
			if c.ReadOnly {
				mode = " (read-only)"
			}
			fmt.Printf("%-32s %-11s risk %2d  %s%s\n", c.Name, c.Domain, c.BaseRisk, c.Summary, mode)
			for _, p := range c.Platforms() {
				b := c.Bindings[p]
				fmt.Printf("    %-8s %-18s %s\n", p, b.Mechanism, b.Native)
			}
		}
	})
}

func propose(ctx context.Context, svc *service.Service, o options, args []string) error {
	fs := flag.NewFlagSet("propose", flag.ContinueOnError)
	author := fs.String("author", o.principal, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	text := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if text == "" {
		return errors.New("propose needs the request text")
	}
	prop, err := svc.Propose(ctx, reason.Request{Text: text, Author: *author})
	if err != nil {
		return err
	}
	return emit(o, prop, func() {
		p := prop.Plan
		fmt.Printf("Intent %s (%s, confidence %.2f)\n  %s\n", prop.Intent.ID, prop.Intent.Provenance.Compiler, prop.Intent.Provenance.Confidence, prop.Summary)
		printScope(prop.Intent)
		fmt.Printf("Plan %s: %d host(s), %d step(s), %d wave(s), tier %s (score %d)\n", p.ID, p.Hosts, len(p.Steps), len(p.Waves), p.Risk.Tier, p.Risk.Score)
		for _, s := range p.Steps {
			fmt.Printf("  - %s on %s (%d host(s)) via %s: %s\n", s.Capability, s.Platform, len(s.Targets), s.Mechanism, s.Native)
		}
		for _, u := range p.Unsupported {
			fmt.Printf("  ! %s not compiled for %d %s host(s): %s\n", u.Capability, u.Hosts, u.Platform, u.Reason)
		}
		for _, v := range p.Violations {
			fmt.Printf("  BLOCKED: %s\n", v.Error())
		}
		for _, c := range prop.Clarifications {
			fmt.Printf("  ? %s\n", c)
		}
		fmt.Printf("Next: %s\n", prop.NextStep)
	})
}

func printScope(in *intent.Intent) {
	var parts []string
	if len(in.Scope.Fleets) > 0 {
		parts = append(parts, "fleets "+strings.Join(in.Scope.Fleets, ","))
	}
	if len(in.Scope.Labels) > 0 {
		parts = append(parts, "labels "+strings.Join(in.Scope.Labels, ","))
	}
	if len(in.Scope.Platforms) > 0 {
		ps := make([]string, len(in.Scope.Platforms))
		for i, p := range in.Scope.Platforms {
			ps[i] = string(p)
		}
		parts = append(parts, "platforms "+strings.Join(ps, ","))
	}
	if len(in.Scope.HostIDs) > 0 {
		parts = append(parts, fmt.Sprintf("hosts %v", in.Scope.HostIDs))
	}
	if in.Constraints.AllowFleetWide && in.Scope.IsEmpty() {
		parts = append(parts, "every device")
	}
	if in.Constraints.MaxAutonomy != "" {
		parts = append(parts, "max autonomy "+string(in.Constraints.MaxAutonomy))
	}
	fmt.Printf("  scope: %s\n", strings.Join(parts, "; "))
}

func plans(ctx context.Context, svc *service.Service, o options) error {
	ps, err := svc.ListPlans(ctx)
	if err != nil {
		return err
	}
	return emit(o, ps, func() {
		for _, p := range ps {
			fmt.Printf("%-28s %-11s tier %-8s score %3d  %3d host(s)  %s\n", p.ID, p.Status, p.Risk.Tier, p.Risk.Score, p.Hosts, oneLine(p.Intent.Text))
		}
	})
}

func showPlan(ctx context.Context, svc *service.Service, o options, args []string) error {
	if len(args) != 1 {
		return errors.New("plan needs an id")
	}
	p, err := svc.GetPlan(ctx, args[0])
	if err != nil {
		return err
	}
	o.jsonOut = true
	return emit(o, p, nil)
}

func simulate(ctx context.Context, svc *service.Service, o options, args []string) error {
	if len(args) != 1 {
		return errors.New("simulate needs a plan id")
	}
	p, err := svc.GetPlan(ctx, args[0])
	if err != nil {
		return err
	}
	rep, err := svc.Apply(ctx, p.ID, o.principal, true)
	if err != nil {
		return err
	}
	sim := plan.Simulate(p)
	return emit(o, map[string]any{"simulation": sim, "drift": rep.Drift, "summary": rep.Summary}, func() {
		for _, s := range sim.Summary {
			fmt.Println("  " + s)
		}
		fmt.Printf("Time to effect: %s\n%s\n", sim.Estimate, rep.Summary)
	})
}

func explain(ctx context.Context, svc *service.Service, o options, args []string) error {
	if len(args) != 1 {
		return errors.New("explain needs a plan id")
	}
	ex, err := svc.Explain(ctx, args[0])
	if err != nil {
		return err
	}
	return emit(o, ex, func() {
		for _, n := range ex.Narrative {
			fmt.Println(n)
		}
		fmt.Println("Rollback:")
		for _, r := range ex.RollbackHow {
			fmt.Println("  " + r)
		}
		if len(ex.History) > 0 {
			fmt.Println("History:")
			for _, h := range ex.History {
				fmt.Printf("  %s %-11s %-14s %s\n", h.At.Format("2006-01-02 15:04:05"), h.Kind, h.Actor, h.Message)
			}
		}
		fmt.Println("GitOps:")
		fmt.Println(indent(ex.Plan.GitOps))
	})
}

func approve(ctx context.Context, svc *service.Service, o options, args []string) error {
	fs := flag.NewFlagSet("approve", flag.ContinueOnError)
	note := fs.String("note", "", "")
	ticket := fs.String("ticket", "", "")
	id, rest := splitID(args)
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if id == "" || fs.NArg() != 0 {
		return errors.New("approve needs a plan id")
	}
	p, err := svc.Approve(ctx, id, o.principal, "cli", *note, *ticket)
	if err != nil {
		return err
	}
	return emit(o, p, func() { fmt.Printf("Plan %s approved by %s (tier %s)\n", p.ID, p.Approval.By, p.Risk.Tier) })
}

func reject(ctx context.Context, svc *service.Service, o options, args []string) error {
	fs := flag.NewFlagSet("reject", flag.ContinueOnError)
	note := fs.String("note", "", "")
	id, rest := splitID(args)
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if id == "" || fs.NArg() != 0 {
		return errors.New("reject needs a plan id")
	}
	p, err := svc.Reject(ctx, id, o.principal, *note)
	if err != nil {
		return err
	}
	return emit(o, p, func() { fmt.Printf("Plan %s rejected\n", p.ID) })
}

func apply(ctx context.Context, svc *service.Service, o options, args []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	dry := fs.Bool("dry-run", false, "")
	id, rest := splitID(args)
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if id == "" || fs.NArg() != 0 {
		return errors.New("apply needs a plan id")
	}
	rep, err := svc.Apply(ctx, id, o.principal, *dry)
	if err != nil {
		return err
	}
	return emit(o, rep, func() {
		for _, w := range rep.Waves {
			if rep.DryRun {
				fmt.Printf("%s: drifted %d, already satisfied %d, errors %d\n", w.Wave.Name, w.Drifted, w.Skipped, w.Failed)
				continue
			}
			fmt.Printf("%s: applied %d, skipped %d, failed %d, unverified %d%s\n", w.Wave.Name, w.Applied, w.Skipped, w.Failed, w.Unverified, rolledBack(w.RolledBack))
			for _, r := range w.Results {
				if r.Error != "" {
					fmt.Printf("  host %d: %s\n", r.HostID, r.Error)
				}
			}
		}
		if len(rep.Drift) > 0 {
			fmt.Printf("Drifted hosts: %v\n", rep.Drift)
		}
		fmt.Printf("%s: %s\n", rep.Status, rep.Summary)
	})
}

func rolledBack(b bool) string {
	if b {
		return " (rolled back)"
	}
	return ""
}

func leash(ctx context.Context, svc *service.Service, o options, args []string) error {
	if len(args) != 1 {
		return errors.New("leash needs a pattern, e.g. firewall.enable@darwin,windows,linux")
	}
	rep, err := svc.Leash(ctx, args[0])
	if err != nil {
		return err
	}
	return emit(o, rep, func() {
		fmt.Printf("%s: accepted %d, verified %d, rejected %d, failed %d, incidents %d; novelty %.2f; relaxation %d tier(s)\n", rep.Pattern, rep.State.Accepted, rep.State.Verified, rep.State.Rejected, rep.State.Failed, rep.State.Incidents, rep.Novelty, rep.Relaxation)
	})
}

func event(ctx context.Context, svc *service.Service, o options, args []string) error {
	fs := flag.NewFlagSet("event", flag.ContinueOnError)
	typ := fs.String("type", "", "")
	host := fs.String("host", "", "")
	platform := fs.String("platform", "", "")
	subject := fs.String("subject", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *typ == "" {
		return errors.New("event needs --type")
	}
	var hostID uint
	if *host != "" {
		n, err := strconv.ParseUint(*host, 10, 64)
		if err != nil {
			return fmt.Errorf("bad --host: %w", err)
		}
		hostID = uint(n)
	}
	plans, err := svc.Reconcile(ctx, events.Event{Type: events.Type(*typ), Source: "cli:" + o.principal, HostID: hostID, Platform: intent.Platform(*platform), Subject: *subject})
	if err != nil {
		return err
	}
	return emit(o, plans, func() {
		if len(plans) == 0 {
			fmt.Println("no intents affected")
		}
		for _, p := range plans {
			fmt.Printf("%s: intent %s, %d host(s), tier %s, status %s\n", p.ID, p.IntentID, p.Hosts, p.Risk.Tier, p.Status)
		}
	})
}

// splitID pulls the first positional argument (the id) out of args so that
// flags may appear before or after it.
func splitID(args []string) (string, []string) {
	var id string
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if id == "" && !strings.HasPrefix(a, "-") {
			id = a
			continue
		}
		rest = append(rest, a)
	}
	return id, rest
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n")
}
