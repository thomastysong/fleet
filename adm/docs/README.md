# ADM design documents

| # | Document | What it answers |
|---|---|---|
| 00 | [Vision](00-vision.md) | Why device management needs to change and what ADM is. |
| 01 | [Architecture](01-architecture.md) | The planes, the loop, the components, how they fit on Fleet. |
| 02 | [Intent model](02-intent-model.md) | What an intent is, how plain language becomes one, how it stays persistent. |
| 03 | [Risk and autonomy](03-risk-and-autonomy.md) | How ADM decides what it may do without a human, and how that changes over time. |
| 04 | [Native action layer](04-native-action-layer.md) | How ADM changes devices through OS APIs instead of scripts, per platform. |
| 05 | [Enrollment and Autopilot](05-enrollment-and-autopilot.md) | Zero-touch on every platform, and Windows Autopilot in depth. |
| 06 | [Capability packs](06-capability-packs.md) | Feature parity with Fleet and Xavier, and what goes beyond both. |
| 07 | [Data and learning](07-data-and-learning.md) | The lakehouse, external sources, the outcome ledger, the feedback loop. |
| 08 | [Interfaces](08-interfaces.md) | Console, CLI, MCP, chat, GitOps, API. |
| 09 | [Security](09-security.md) | Threat model, guardrails for an LLM in the loop, identity, audit. |
| 10 | [Roadmap](10-roadmap.md) | What exists, what is next, open decisions. |

The architectural decision to build ADM this way is recorded in
[ADR-0013](../../docs/Contributing/adr/0013-agentic-device-management.md).
