# 03 — Risk and autonomy

## Why a deterministic engine

The language model is the best component ADM has for understanding what an
operator meant. It is the worst component to decide what may run on ten
thousand laptops without a person. So it does not decide. The risk engine
(`adm/risk`) is deterministic, inspectable, unit-tested and versioned, and it
is the only thing that assigns autonomy.

## Tiers

| Tier | Meaning | Who is in the loop |
|---|---|---|
| `observe` | Never act; report drift. | Nobody; it is a report. |
| `auto` | Execute now. | Nobody; the ledger and Fleet activities record it. |
| `canary` | Execute on a canary wave, verify, continue in waves. | Nobody unless verification fails; then it rolls back and a person is told. |
| `approve` | Wait for a human approval from someone other than the author. | One approver. |
| `cab` | Wait for an approval that references a change ticket. | A change board. |

## Score

`risk.Score` produces a 0–100 number from factors the planner extracts:

| Factor | Contribution | Why |
|---|---|---|
| Base risk of the capability (0–40, from the catalog) | + base | Turning on a firewall and wiping a disk are not the same kind of change. |
| Blast radius | + up to 20, from the fraction of the fleet and the log of the count | 5 hosts of 10,000 and 5,000 hosts of 10,000 differ; so do 5 and 500 of 500. |
| Irreversible | + 15 | Rollback is the safety net; without it the net is gone. |
| Destructive | + 30 | Data loss is a different category. |
| User impact | + 5 prompt, + 10 logout, + 15 restart | People notice. |
| Privileged | + 5 | System or MDM level. |
| Compiler confidence | + (1 − confidence) × 15 | A hesitant intent deserves a second look. |
| Pattern novelty | + novelty × 10 | Never-run patterns are riskier than proven ones. |
| Platforms | + 2 per extra platform | More native surfaces, more ways to be wrong. |
| Uses a script | + 10 | A script is the least observable, least reversible mechanism. |
| Inside a maintenance window | − 5 | The blast is where people expect it. |

The default thresholds: ≤ 25 auto, ≤ 45 canary, ≤ 70 approve, above that
CAB. The score is one input; three more rules apply afterwards.

## Rules that override the score

1. **Floors.** Policy sets a minimum tier per capability (prefix match):
   `host.wipe` → CAB, `host.lock` and `identity.*` → approve,
   `compliance.signal` and `os.update.enforce` → canary, `script.run` →
   approve. A floor can be raised per tenant, never lowered below the
   defaults for destructive capabilities.
2. **Blast-radius caps.** An `auto` plan may touch at most `MaxAutoHosts`
   (50) and `MaxAutoFraction` (5 %) of the fleet; beyond that it becomes a
   canary regardless of score.
3. **The intent's cap.** `constraints.max_autonomy` can only make the tier
   more conservative. "Automatically" in an operator's ask is a preference;
   the engine can still say canary.

## Hard invariants

Invariants are evaluated per step and per capability across the plan, and a
violated invariant blocks the plan: it cannot be approved, and `explain`
says why. The defaults:

- **never-disable-encryption**: no intent may turn disk encryption off.
- **wipe-requires-cab** and **wipe-single-host**: a wipe runs at the CAB
  tier and targets exactly one host per plan.
- **never-remove-agent**: no uninstall or script may stop fleetd, orbit,
  osquery or the ADM agent.
- **protected-accounts**: `root`, `Administrator` and break-glass accounts
  are never created, modified or removed.
- **no-fleet-wide-scripts**: a script on more than 100 hosts needs CAB.

Tenants add invariants in code (a function over `risk.Action`), never in a
prompt.

## The leash: autonomy that is earned

For every pattern (`intent.PatternKey`) the ledger yields a `LeashState`:
how many plans were accepted (approved by a person, or verified after an
unattended run), rejected, verified, failed, and how many incidents
(rollbacks and reported incidents) occurred, with the time of the last one.

- **Novelty** is `1 / (1 + (verified + accepted) / 3)`: 1.0 for an unseen
  pattern, under 0.1 after thirty good runs. It feeds the score.
- **Relaxation** is `min(accepted / 5, 2)` tiers, lowering the assigned tier
  (never below `auto`, never below a floor, never for destructive changes).
  Five accepted runs of "screen lock on macOS" turn the sixth from `approve`
  into `canary`; ten turn it into `auto`.
- **Suspension**: an incident within the cooldown (30 days), or a
  verification record worse than three verified per failed, sets relaxation
  to zero. Trust is revoked faster than it is earned.

Because the key is the pattern and not the scope, trust earned on five
laptops applies to the next five hundred in the sense that the *pattern* is
proven, while the blast-radius factor and caps still make the five hundred a
canary. Both things are true at once, which is what an experienced
administrator would say too.

## Approvals

- The author of an intent cannot approve their own plan at the approve tier
  or above; the service refuses it.
- The CAB tier requires a ticket reference in the approval; it is recorded
  in the plan and the ledger, and `explain` shows it.
- Approvals arrive from any interface (CLI, console, MCP with a principal,
  chat, ITSM webhook) and all land in the same `plan.Approval`.
- An approval is for a plan, not an intent. Recompiling an intent (because
  the fleet changed, or the intent was edited) produces a new plan that
  needs its own decision, unless the pattern has since earned `auto`.

## Where the model's judgement still enters

The model contributes three numbers the engine consumes as data: the
confidence in its compilation, the clarifications it asked (which cap
confidence at 0.7), and, in the reasoning agent (document 07), the
explanations it attaches to a proposal it made from telemetry rather than
from an operator. None of them can lower a tier below what the score, the
floors, the caps and the leash allow.
