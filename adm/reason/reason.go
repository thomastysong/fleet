// Package reason turns plain language into an Intent. Two compilers exist:
// a deterministic, rule-based one that covers the common asks offline and
// acts as the validator and fallback, and a Claude-backed one that handles
// open-ended language, asks for clarification when the ask is ambiguous, and
// reports its own confidence, which the risk engine turns into caution.
//
// The language model never executes anything. It emits an Intent; the
// catalog validates it, the planner compiles it, and the risk engine decides
// how much autonomy it gets.
package reason

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fleetdm/fleet/v4/adm/capability"
	"github.com/fleetdm/fleet/v4/adm/intent"
)

// Request is what the operator said, plus tenant hints that ground scope.
type Request struct {
	Text   string
	Author string
	// Fleets and Labels are the tenant's known names so the compiler can map
	// "finance laptops" to real selectors.
	Fleets []string
	Labels []string
	// Platforms restricts the intent when the operator's tooling already
	// knows the platform (for example a per-platform console page).
	Platforms []intent.Platform
}

// Result is a compiled intent plus any clarifying questions.
type Result struct {
	Intent         *intent.Intent
	Clarifications []string
	Summary        string
}

// Compiler compiles a request.
type Compiler interface {
	Name() string
	Compile(ctx context.Context, req Request) (*Result, error)
}

// ErrNoMatch is returned when the deterministic compiler recognises nothing.
var ErrNoMatch = errors.New("no capability matched the request")

// Deterministic is the rule-based compiler.
type Deterministic struct {
	Catalog *capability.Catalog
	Now     func() time.Time
	NewID   func() string
}

// NewDeterministic returns a rule-based compiler over the catalog.
func NewDeterministic(cat *capability.Catalog) *Deterministic {
	return &Deterministic{Catalog: cat, Now: time.Now, NewID: NewIntentID}
}

// NewIntentID returns a time-prefixed, random intent ID that is unique across
// processes.
func NewIntentID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("intent-%d-%s", time.Now().Unix(), hex.EncodeToString(b))
}

func (d *Deterministic) Name() string { return "deterministic" }

var (
	reMinutes = regexp.MustCompile(`(\d+)\s*(?:min|minute|minutes)`)
	reDays    = regexp.MustCompile(`(\d+)\s*(?:day|days)`)
	reDomain  = regexp.MustCompile(`\b([a-z0-9-]+(?:\.[a-z0-9-]+)+)\b`)
	reVersion = regexp.MustCompile(`\b(\d+(?:\.\d+){1,3})\b`)
	reBehind  = regexp.MustCompile(`(\d+)\s*(?:versions?|releases?)\s*behind`)
)

// aiTools maps spoken names to ai_tools knowledge-base names and provider
// domains.
var aiTools = []struct {
	names    []string
	tool     string
	provider string
}{
	{[]string{"chatgpt", "openai", "codex"}, "chatgpt", "chatgpt.com"},
	{[]string{"claude", "anthropic", "claude code"}, "claude", "claude.ai"},
	{[]string{"gemini", "google ai"}, "gemini", "gemini.google.com"},
	{[]string{"copilot", "github copilot"}, "github-copilot", "api.githubcopilot.com"},
	{[]string{"cursor"}, "cursor", "api2.cursor.sh"},
	{[]string{"aider"}, "aider", ""},
	{[]string{"windsurf", "codeium"}, "windsurf", "server.codeium.com"},
	{[]string{"deepseek"}, "deepseek", "chat.deepseek.com"},
	{[]string{"perplexity"}, "perplexity", "www.perplexity.ai"},
}

var softwareTitles = map[string]string{
	"chrome": "Google Chrome", "google chrome": "Google Chrome", "firefox": "Firefox", "slack": "Slack",
	"zoom": "zoom.us", "edge": "Microsoft Edge", "vscode": "Visual Studio Code", "vs code": "Visual Studio Code",
	"1password": "1Password", "docker": "Docker", "notion": "Notion", "figma": "Figma",
}

var platformWords = map[string]intent.Platform{
	"mac": intent.PlatformDarwin, "macs": intent.PlatformDarwin, "macos": intent.PlatformDarwin, "macbook": intent.PlatformDarwin, "macbooks": intent.PlatformDarwin, "darwin": intent.PlatformDarwin,
	"windows": intent.PlatformWindows, "pc": intent.PlatformWindows, "pcs": intent.PlatformWindows,
	"linux": intent.PlatformLinux, "ubuntu": intent.PlatformLinux, "fedora": intent.PlatformLinux,
	"iphone": intent.PlatformIOS, "iphones": intent.PlatformIOS, "ios": intent.PlatformIOS,
	"ipad": intent.PlatformIPadOS, "ipads": intent.PlatformIPadOS,
	"apple tv": intent.PlatformTVOS, "tvos": intent.PlatformTVOS,
	"android": intent.PlatformAndroid, "pixel": intent.PlatformAndroid, "samsung": intent.PlatformAndroid,
}

// Compile applies the rules.
func (d *Deterministic) Compile(_ context.Context, req Request) (*Result, error) {
	text := strings.ToLower(strings.TrimSpace(req.Text))
	if text == "" {
		return nil, fmt.Errorf("%w: empty request", ErrNoMatch)
	}
	in := &intent.Intent{ID: d.NewID(), Text: req.Text, Author: req.Author, Source: intent.SourceNaturalLanguage, State: intent.StateDraft, Version: 1, CreatedAt: d.Now(), UpdatedAt: d.Now()}
	var matched []string
	var clar []string
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(text, w) {
				return true
			}
		}
		return false
	}
	add := func(cap string, params map[string]any, why string) {
		in.Desired = append(in.Desired, intent.Predicate{Capability: cap, Params: params, Rationale: why})
		matched = append(matched, cap)
	}
	blocking := has("block", "disable", "prevent", "no ", "ban", "deny", "stop", "forbid", "not allowed", "read-only", "read only")

	// ---- security baseline
	if has("secure", "security baseline", "harden", "lock down", "lockdown") && !has("screen") {
		add("disk.encryption.enforce", map[string]any{"escrow": true}, "security baseline")
		add("firewall.enable", nil, "security baseline")
		add("screen.lock.enforce", map[string]any{"idle_minutes": 10}, "security baseline")
	}
	if has("encrypt", "filevault", "bitlocker", "luks") && !has("disk.encryption.enforce") && !containsCap(in, "disk.encryption.enforce") {
		add("disk.encryption.enforce", map[string]any{"escrow": true}, "encryption requested")
	}
	if has("firewall") && !containsCap(in, "firewall.enable") {
		add("firewall.enable", nil, "firewall requested")
	}
	if has("screen lock", "lock screen", "screensaver", "screen saver", "idle") && !containsCap(in, "screen.lock.enforce") {
		minutes := 10
		if m := reMinutes.FindStringSubmatch(text); m != nil {
			minutes, _ = strconv.Atoi(m[1])
		}
		add("screen.lock.enforce", map[string]any{"idle_minutes": minutes}, "screen lock requested")
	}
	// ---- storage
	if has("usb", "removable", "thumb drive", "flash drive", "external drive", "external storage", "sd card") {
		if blocking {
			mode := "block"
			if has("read-only", "read only") {
				mode = "read_only"
			}
			add("storage.removable.block", map[string]any{"mode": mode}, "removable storage restricted")
		} else {
			add("storage.removable.audit", nil, "removable storage audit")
		}
	}
	// ---- AI governance
	var tools, providers []string
	for _, t := range aiTools {
		for _, n := range t.names {
			if strings.Contains(text, n) {
				tools = append(tools, t.tool)
				if t.provider != "" {
					providers = append(providers, t.provider)
				}
				break
			}
		}
	}
	if has("ai tool", "ai tools", "ai agent", "ai agents", "coding agent", "coding agents", "llm", "generative ai") && len(tools) == 0 {
		// Govern the category now; the exact block list is a clarification.
		add("ai.tools.govern", map[string]any{}, "AI tool governance (tools to be confirmed)")
		if blocking {
			clar = append(clar, "Which AI tools should be blocked? (e.g. chatgpt, claude, gemini, copilot, cursor)")
		}
	}
	if len(tools) > 0 {
		params := map[string]any{}
		if blocking {
			params["block"] = uniq(tools)
			params["block_providers"] = uniq(providers)
		} else {
			params["allow"] = uniq(tools)
		}
		add("ai.tools.govern", params, "AI tool governance")
	}
	// ---- web blocking (domains not already attributed to an AI provider)
	if blocking && !has("usb", "removable") {
		var domains []string
		for _, m := range reDomain.FindAllStringSubmatch(text, -1) {
			dom := m[1]
			if strings.HasSuffix(dom, ".exe") || contains(providers, dom) || isVersion(dom) {
				continue
			}
			domains = append(domains, dom)
		}
		if len(domains) > 0 {
			add("web.block", map[string]any{"domains": uniq(domains)}, "web access restricted")
		}
	}
	// ---- software
	for word, title := range softwareTitles {
		if strings.Contains(text, word) && (has("update", "upgrade", "latest", "current", "behind", "outdated", "patched")) {
			params := map[string]any{"title": title, "max_versions_behind": 2}
			if m := reBehind.FindStringSubmatch(text); m != nil {
				params["max_versions_behind"], _ = strconv.Atoi(m[1])
			}
			add("software.version.floor", params, "keep "+title+" current")
			break
		}
	}
	if has("brew", "homebrew", "npm", "pip", "gem", "rubygems", "package manager", "packages") && has("outdated", "vulnerab", "cve", "upgrade", "update") {
		add("package.manager.upgrade", map[string]any{"only_vulnerable": !has("all packages")}, "developer package hygiene")
	}
	if has("end of life", "end-of-life", "eol") {
		add("runtime.eol.flag", nil, "runtime end-of-life audit")
	}
	if has("os update", "operating system", "macos 1", "windows 11", "kernel") && has("update", "upgrade", "minimum", "at least") {
		params := map[string]any{"deadline_days": 7}
		if m := reVersion.FindStringSubmatch(text); m != nil {
			params["min_version"] = m[1]
		} else {
			clar = append(clar, "Which minimum OS version should be enforced?")
		}
		if m := reDays.FindStringSubmatch(text); m != nil {
			params["deadline_days"], _ = strconv.Atoi(m[1])
		}
		if _, ok := params["min_version"]; ok {
			add("os.update.enforce", params, "OS update enforcement")
		}
	}
	// ---- signage / lifecycle
	if has("kiosk", "signage", "single app") {
		add("signage.kiosk", map[string]any{"app": "com.adm.signage"}, "digital signage")
	}
	if has("wipe", "erase", "factory reset") {
		add("host.wipe", nil, "wipe requested")
	} else if has("lock the device", "lock device", "lock host", "remote lock", "lock it") {
		add("host.lock", map[string]any{"message": "This device is managed by ADM. Contact IT."}, "remote lock requested")
	}
	if len(in.Desired) == 0 {
		return nil, fmt.Errorf("%w: %q", ErrNoMatch, req.Text)
	}

	// ---- scope
	for word, p := range platformWords {
		if containsWord(text, word) && !containsPlatform(in.Scope.Platforms, p) {
			in.Scope.Platforms = append(in.Scope.Platforms, p)
		}
	}
	sort.Slice(in.Scope.Platforms, func(i, j int) bool { return in.Scope.Platforms[i] < in.Scope.Platforms[j] })
	if len(req.Platforms) > 0 && len(in.Scope.Platforms) == 0 {
		in.Scope.Platforms = req.Platforms
	}
	for _, f := range req.Fleets {
		if containsWord(text, strings.ToLower(f)) {
			in.Scope.Fleets = append(in.Scope.Fleets, f)
		}
	}
	for _, l := range req.Labels {
		if containsWord(text, strings.ToLower(l)) {
			in.Scope.Labels = append(in.Scope.Labels, l)
		}
	}
	if has("everyone", "everywhere", "all devices", "every device", "fleet-wide", "fleet wide", "whole fleet", "entire fleet", "my fleet", "the fleet", "all hosts", "all laptops", "all machines") || (in.Scope.IsEmpty() && has("all ")) {
		in.Constraints.AllowFleetWide = true
	}
	if in.Scope.IsEmpty() && !in.Constraints.AllowFleetWide {
		clar = append(clar, "Which devices should this apply to? Name a fleet, a label or a platform, or say \"everywhere\".")
		in.Constraints.AllowFleetWide = true // draft stays valid; the question is surfaced
	}

	// ---- autonomy and rollout
	switch {
	case has("ask me", "ask first", "approval", "approve", "check with me", "before you"):
		in.Constraints.MaxAutonomy = intent.AutonomyApprove
	case has("canary", "gradually", "slowly", "in waves", "staged"):
		in.Constraints.MaxAutonomy = intent.AutonomyCanary
	case has("just watch", "observe", "report only", "don't change", "do not change", "audit only"):
		in.Constraints.MaxAutonomy = intent.AutonomyObserve
	case has("automatically", "auto-", "without asking", "no approval"):
		in.Constraints.MaxAutonomy = intent.AutonomyAuto
	}
	if has("no restart", "without restarting", "no reboot") {
		in.Constraints.MaxUserImpact = intent.ImpactPrompt
	}

	conf := 0.6 + 0.08*float64(len(matched))
	if len(clar) > 0 {
		conf -= 0.15
	}
	if conf > 0.92 {
		conf = 0.92
	}
	in.Provenance = intent.Provenance{Compiler: d.Name(), PromptHash: intent.PromptHash(req.Text), Confidence: round2(conf)}
	if err := in.Validate(); err != nil {
		return nil, err
	}
	return &Result{Intent: in, Clarifications: clar, Summary: "Matched " + strings.Join(matched, ", ")}, nil
}

// Chain tries compilers in order and falls back on error.
type Chain struct {
	Compilers []Compiler
}

func (c *Chain) Name() string { return "chain" }

// Compile returns the first successful result; errors are joined otherwise.
func (c *Chain) Compile(ctx context.Context, req Request) (*Result, error) {
	var errs []error
	for _, comp := range c.Compilers {
		res, err := comp.Compile(ctx, req)
		if err == nil {
			return res, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", comp.Name(), err))
	}
	return nil, errors.Join(errs...)
}

// ValidateAgainstCatalog checks every predicate against the catalog.
func ValidateAgainstCatalog(in *intent.Intent, cat *capability.Catalog) error {
	for _, d := range in.Desired {
		cap, ok := cat.Get(d.Capability)
		if !ok {
			return fmt.Errorf("unknown capability %q", d.Capability)
		}
		if err := cap.ValidateParams(d.Params); err != nil {
			return err
		}
	}
	return in.Validate()
}

func containsCap(in *intent.Intent, name string) bool {
	for _, d := range in.Desired {
		if d.Capability == name {
			return true
		}
	}
	return false
}

func containsPlatform(list []intent.Platform, p intent.Platform) bool {
	for _, x := range list {
		if x == p {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func containsWord(text, word string) bool {
	if word == "" {
		return false
	}
	re := regexp.MustCompile(`(^|[^a-z0-9])` + regexp.QuoteMeta(word) + `($|[^a-z0-9])`)
	return re.MatchString(text)
}

func isVersion(s string) bool {
	return reVersion.MatchString(s) && !strings.ContainsAny(s, "abcdefghijklmnopqrstuvwxyz")
}

func uniq(list []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range list {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }
