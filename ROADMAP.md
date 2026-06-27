# Roadmap

OpenTag is **alpha** — intentionally. The core interaction model and the kagent
backend work end to end; the surface area is deliberately small while the design
settles. Here's an honest snapshot.

## Available today

- **Slack** channel mentions and DMs over Socket Mode (no public URL needed)
- **Thread-scoped sessions** — each Slack thread is its own conversation
- **Channel-scoped service identity** — one shared teammate per channel
- **Personal identity for DMs**
- **Channel-scoped memory** (via the bound kagent agent's memory)
- **Background tasks** — long work is tracked and posted back to the thread
- **Proactive routines** — scheduled prompts that post results unprompted
- **`triggers` / `stop`** commands to list and cancel standing work
- **Governance** — default-deny routing, DM policy, best-effort per-channel turn caps
- **JSONL audit trail** of every interaction
- **kagent (A2A) backend** behind a backend-agnostic interface
- **Cloud-native delivery** — container image, Helm chart, CI, release workflow

## Next

- **More chat providers** — Discord, Microsoft Teams (the `chat.Provider`
  interface is already the seam)
- **More agent backends** — additional runtimes behind `backend.AgentBackend`
- **Real spend caps** — replace the best-effort turn meter with token-cost
  accounting (`TODO(spend)`)
- **Ambient follow-ups** — proactively nudge threads that have gone quiet
- **Richer admin controls** — per-channel access bundles, finer governance
- **More audit sinks** — beyond JSONL (e.g. structured/remote sinks)
- **Demo assets** — screenshots / GIF captured from a live workspace
- **First tagged release** and published `ghcr.io` image

Have a use case or want to drive one of these? Open an issue — see
[CONTRIBUTING.md](CONTRIBUTING.md).
