package plan

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fleetdm/fleet/v4/adm/capability"
)

// RenderGitOps renders the persistent, reviewable form of a plan: the ADM
// intent block plus the Fleet GitOps constructs it maps onto (policies for
// every verification, controls for capabilities Fleet manages declaratively).
// The output is what ADM commits to the GitOps repository as a change; a
// human reviews a diff, never a transcript.
func RenderGitOps(p *Plan, cat *capability.Catalog) string {
	var b strings.Builder
	in := p.Intent
	fmt.Fprintf(&b, "# ADM intent %s (v%d) — %s\n", in.ID, in.Version, oneLine(in.Text))
	fmt.Fprintf(&b, "# fingerprint: %s  pattern: %s  tier: %s  score: %d\n", in.Fingerprint(), in.PatternKey(), p.Risk.Tier, p.Risk.Score)
	b.WriteString("adm:\n  intents:\n")
	fmt.Fprintf(&b, "    - id: %s\n", in.ID)
	fmt.Fprintf(&b, "      text: %s\n", yamlString(in.Text))
	fmt.Fprintf(&b, "      author: %s\n", yamlString(in.Author))
	b.WriteString("      scope:\n")
	writeList(&b, "fleets", in.Scope.Fleets, 8)
	writeList(&b, "labels", in.Scope.Labels, 8)
	writeList(&b, "exclude_labels", in.Scope.ExcludeLabels, 8)
	if len(in.Scope.Platforms) > 0 {
		ps := make([]string, len(in.Scope.Platforms))
		for i, p := range in.Scope.Platforms {
			ps[i] = string(p)
		}
		writeList(&b, "platforms", ps, 8)
	}
	if in.Scope.Query != "" {
		fmt.Fprintf(&b, "        query: %s\n", yamlString(in.Scope.Query))
	}
	b.WriteString("      desired:\n")
	for _, d := range in.Desired {
		fmt.Fprintf(&b, "        - capability: %s\n", d.Capability)
		if len(d.Params) > 0 {
			b.WriteString("          params:\n")
			keys := make([]string, 0, len(d.Params))
			for k := range d.Params {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(&b, "            %s: %s\n", k, yamlScalar(d.Params[k]))
			}
		}
		if d.Rationale != "" {
			fmt.Fprintf(&b, "          rationale: %s\n", yamlString(d.Rationale))
		}
	}
	b.WriteString("      constraints:\n")
	if in.Constraints.MaxAutonomy != "" {
		fmt.Fprintf(&b, "        max_autonomy: %s\n", in.Constraints.MaxAutonomy)
	}
	if in.Constraints.CanaryPercent > 0 {
		fmt.Fprintf(&b, "        canary_percent: %d\n", in.Constraints.CanaryPercent)
	}
	if in.Constraints.MaxWaveHosts > 0 {
		fmt.Fprintf(&b, "        max_wave_hosts: %d\n", in.Constraints.MaxWaveHosts)
	}
	if in.Constraints.MaxUserImpact != "" {
		fmt.Fprintf(&b, "        max_user_impact: %s\n", in.Constraints.MaxUserImpact)
	}
	if in.Constraints.AllowFleetWide {
		b.WriteString("        allow_fleet_wide: true\n")
	}
	if w := in.Constraints.MaintenanceWindow; w != nil {
		fmt.Fprintf(&b, "        maintenance_window: {start: %q, end: %q}\n", w.Start, w.End)
	}

	// Fleet-native projections: every verification becomes a Fleet policy so
	// drift is visible in Fleet's own UI and can drive Fleet automations.
	var policies []Step
	for _, s := range p.Steps {
		if s.Verify != nil {
			policies = append(policies, s)
		}
	}
	controls := fleetControls(p)
	if len(policies) > 0 || len(controls) > 0 {
		b.WriteString("# Fleet GitOps projection (generated; do not edit by hand)\n")
	}
	if len(controls) > 0 {
		b.WriteString("controls:\n")
		for _, c := range controls {
			b.WriteString("  " + c + "\n")
		}
	}
	if len(policies) > 0 {
		b.WriteString("policies:\n")
		for _, s := range policies {
			fmt.Fprintf(&b, "  - name: %s\n", yamlString(fmt.Sprintf("ADM %s (%s)", s.Capability, s.Platform)))
			fmt.Fprintf(&b, "    platform: %s\n", fleetPlatform(string(s.Platform)))
			fmt.Fprintf(&b, "    description: %s\n", yamlString("Verifies ADM intent "+in.ID+": "+oneLine(in.Text)))
			fmt.Fprintf(&b, "    resolution: %s\n", yamlString("ADM reconciles this automatically; see adm explain "+p.ID))
			query := s.Verify.Query
			if s.Verify.Expect == "rows==0" {
				query = "SELECT 1 WHERE NOT EXISTS (" + query + ")"
			}
			fmt.Fprintf(&b, "    query: %s\n", yamlString(query))
		}
	}
	return b.String()
}

// fleetControls maps capabilities onto Fleet's declarative controls where a
// direct projection exists.
func fleetControls(p *Plan) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range p.Steps {
		var line string
		switch s.Capability {
		case "disk.encryption.enforce":
			line = "enable_disk_encryption: true"
		case "os.update.enforce":
			if v, ok := s.Params["min_version"].(string); ok {
				switch s.Platform {
				case "darwin":
					line = fmt.Sprintf("macos_updates: {minimum_version: %q, deadline: \"<computed>\"}", v)
				case "windows":
					line = "windows_updates: {deadline_days: 7, grace_period_days: 2}"
				case "ios":
					line = fmt.Sprintf("ios_updates: {minimum_version: %q, deadline: \"<computed>\"}", v)
				}
			}
		}
		if line != "" && !seen[line] {
			seen[line] = true
			out = append(out, line)
		}
	}
	sort.Strings(out)
	return out
}

func fleetPlatform(p string) string {
	switch p {
	case "darwin", "windows", "linux", "chrome":
		return p
	case "chromeos":
		return "chrome"
	}
	return "darwin"
}

func writeList(b *strings.Builder, key string, vals []string, indent int) {
	if len(vals) == 0 {
		return
	}
	pad := strings.Repeat(" ", indent)
	fmt.Fprintf(b, "%s%s:\n", pad, key)
	for _, v := range vals {
		fmt.Fprintf(b, "%s  - %s\n", pad, yamlString(v))
	}
}

func yamlString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return "\"" + s + "\""
}

func yamlScalar(v any) string {
	switch vv := v.(type) {
	case string:
		return yamlString(vv)
	case []string:
		q := make([]string, len(vv))
		for i, s := range vv {
			q[i] = yamlString(s)
		}
		return "[" + strings.Join(q, ", ") + "]"
	case []any:
		q := make([]string, 0, len(vv))
		for _, e := range vv {
			q = append(q, yamlScalar(e))
		}
		return "[" + strings.Join(q, ", ") + "]"
	case bool, int, int64, float64:
		return fmt.Sprint(vv)
	}
	return yamlString(fmt.Sprint(v))
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
