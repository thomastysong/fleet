# ADM — Agentic Device Management

ADM is a device-management control plane that starts where traditional MDM
ends. It is built on the Fleet substrate (osquery inventory and live query,
Apple / Windows / Android MDM protocols, GitOps) and on the lessons of
Xavier Device Management (developer package hygiene, removable-storage
control, AI-tool governance, digital signage, a read-only MCP surface), and
it adds the three things neither has:

1. **Intents instead of profiles.** An operator says what the fleet should
   look like, in plain language. ADM compiles that into a platform-agnostic
   Intent, then into native, reversible, verifiable actions per OS.
2. **An agentic loop with graduated autonomy.** ADM perceives (osquery, OS
   events, the lakehouse, ITSM, Git), reasons (a language model proposes; a
   deterministic risk engine decides), acts (native OS APIs, not scripts),
   verifies on the same devices it touched, and learns (every outcome is a
   ledger row that loosens or tightens the leash for that pattern).
3. **Event speed, not cron speed.** Drift is corrected when the event that
   caused it happens, on the device that changed, not on the next 30-second
   or one-hour sweep.

The design documents live in [`docs/`](docs/README.md). This directory also
contains a working Go scaffold of the control plane.

## What is here

| Package | What it is |
|---|---|
| `intent` | The Intent IR: scope, desired predicates, constraints, verification, provenance. |
| `capability` | The catalog: every capability ADM knows, with its native binding on every platform. |
| `reason` | Compilers from plain language to intents: deterministic rules and a Claude-backed compiler. |
| `plan` | Compiler from intent to plan: native steps per platform, waves, rollback, GitOps rendering. |
| `risk` | The autonomy engine: scoring, tiers (observe / auto / canary / approve / CAB), invariants, the adaptive leash. |
| `native` | Adapter contract plus Windows (CSP over the local MDM stack), Apple (DDM declarations) and Linux (udev, dconf, D-Bus) adapters. |
| `substrate` | The Fleet REST substrate and an in-memory substrate. |
| `execute` | The executor: check before apply, wave by wave, verify, roll back. |
| `events` | The event bus and the trigger table that makes reconciliation event-driven. |
| `learn` | The outcome ledger (the lakehouse schema) and leash calibration. |
| `enroll/autopilot` | Windows Autopilot through Microsoft Graph: import hardware hashes, tag, assign profiles, sync. |
| `service` | The control-plane facade: propose, simulate, approve, apply, explain, reconcile. |
| `mcp` | The ADM MCP server: the write plane for AI agents, with the same approval gates as humans. |
| `cmd/admctl` | The CLI. |

## Try it

```bash
cd adm
go test ./...

# Compile a plain-language ask against the built-in demo fleet.
go run ./cmd/admctl --demo propose "Block USB drives on Finance Windows laptops, ask me first"

# See the plan, the native steps, the risk reasons and the GitOps rendering.
go run ./cmd/admctl --demo explain <plan-id>

# A second person approves, then it runs: canary wave, verification, rollback on failure.
go run ./cmd/admctl --demo --principal bob approve <plan-id> --note "reviewed"
go run ./cmd/admctl --demo apply <plan-id>

# Serve the MCP write plane over stdio for Claude Code, Claude Desktop or Cursor.
go run ./cmd/admctl --demo --principal alice mcp
```

Point it at a real Fleet server with `--fleet-url` and `--fleet-token` (an
API-only user). Point the compiler at Claude with `--claude` and an
`ANTHROPIC_API_KEY`; without it the deterministic compiler handles the
common asks offline.

## Status

This is the design plus a scaffold, not a product. Everything in `docs/` is
the target; the roadmap in [`docs/10-roadmap.md`](docs/10-roadmap.md) says
what exists today, what is next, and what still needs a decision.
