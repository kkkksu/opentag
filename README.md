# OpenTag

**Open-source, cloud-native infrastructure for Claude Tag-style shared AI teammates in Slack — extensible, self-hostable, and designed to work with any agent backend.**

[![CI](https://github.com/kkkksu/opentag/actions/workflows/ci.yml/badge.svg)](https://github.com/kkkksu/opentag/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/kkkksu/opentag.svg)](https://pkg.go.dev/github.com/kkkksu/opentag)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

Inspired by the Claude Tag pattern: instead of a private chatbot per person, a
Slack **channel** gets **one shared teammate** that everyone can tag, watch,
redirect, and hand work to. OpenTag owns the collaboration layer — **identity,
thread sessions, governance, audit, and routing** — and **delegates execution to
[kagent](https://github.com/kagent-dev/kagent) today, and other agent backends
later** through a small, backend-agnostic interface.

Status: alpha (intentionally — see the [roadmap](ROADMAP.md)). Channel mentions,
DMs, channel-scoped memory, background tasks, and proactive routines work today.

## Why OpenTag?

Claude Tag popularized the "shared AI teammate in a Slack channel" model. OpenTag
brings that interaction model to **open, cloud-native, self-hostable
infrastructure** with **pluggable agent backends** and full control over
**identity, governance, and audit** — run it in your own cluster, point it at
your own agents, and keep your data and audit trail in-house.

## Features

- **Shared Slack teammate** — one `@OpenTag` per channel, not one bot per person
- **Thread-scoped sessions** — each thread is its own coherent conversation
- **Channel-scoped service identity** — anyone can pick up a teammate's thread
- **Personal identity for DMs** — direct messages run as the individual user
- **Channel-scoped memory** — the teammate remembers a channel over time
- **Background tasks** — delegate long work; results post back to the thread
- **Proactive routines** — scheduled prompts that post results unprompted
- **Backend-agnostic agent interface** — kagent today, more backends later
- **Governance controls** — default-deny routing, DM policy, per-channel turn caps
- **JSONL audit trail** — who asked, which agent ran, the outcome
- **Cloud-native deployment** — container image + Helm chart
- **Extensible** — clean `chat.Provider` and `backend.AgentBackend` abstractions

## OpenTag vs. managed Claude Tag-style products

| | OpenTag | Managed Claude Tag-style |
| --- | :---: | :---: |
| Shared Slack teammate | ✓ | ✓ |
| Open source | ✓ | ✗ |
| Self-hostable | ✓ | ✗ |
| Bring-your-own AI backend | ✓ | limited |
| Cloud-native self-hosting | ✓ | managed only |
| Extensible runtime | ✓ | limited |
| Governance + audit | ✓ | ✓ |

*Comparison reflects OpenTag's current and near-term scope; see the
[roadmap](ROADMAP.md) for what's shipped vs. planned.*

## Demo

<!-- demo gif: capture during the first live run against a real Slack workspace -->
_A short walkthrough GIF is coming once OpenTag has been run against a live
workspace. Until then, the flow is: `@OpenTag <request>` → it opens a thread,
runs the bound agent, and streams the reply; anyone can continue the thread._

## Design in one picture

```
Slack (Socket Mode)
   │  @OpenTag mention (in a thread) / DM
   ▼
ChatProvider ──► core ──► AgentBackend (kagent)
                 ├─ routing     EnsureSession + A2A Stream
                 ├─ identity     (X-User-ID + sessionID/contextID)
                 ├─ governance
                 └─ audit
```

**Identity is per *channel*, work is per *thread*:**

| Behavior | How it works |
| --- | --- |
| One teammate per channel | a `ChannelBinding` → one kagent agent + one **org service identity** |
| Work happens in a thread | one kagent **session** per Slack thread (`contextID = hash(team,channel,threadTS)`) |
| Channel mention vs DM | channel → org service identity; DM → the user's personal identity |
| Delegate & walk away | long tasks are tracked in the background and posted back to the thread when done |
| Proactive teammate | scheduled routines run a prompt and post results without being asked |
| Spend caps + audit | best-effort per-channel turn meter + JSONL audit trail |

A channel mention runs under a shared service identity, so anyone in the channel
can pick up a thread someone else started, while each thread stays its own
coherent session.

## Memory

OpenTag gets **channel-scoped memory for free** from this identity model. kagent
keys an agent's memory by `(agent, user_id)`, and every mention in a channel runs
under the *same* `user_id` (the channel service identity) against the *same*
agent — so facts the agent learns in one thread are available in others, and
persist across restarts (kagent stores them).

Two things to know:

- **The bound agent must have memory enabled** — i.e. configured with kagent's
  memory tools and an embedding config. kagent generates the embeddings and does
  recall/save itself; OpenTag does not run an embedder.
- **`sharedMemory` (per binding, default `true`)** controls the lever OpenTag
  owns: `true` shares one identity across the whole channel (cross-thread
  memory); `false` isolates each thread.

## Background work & proactive routines

- **Delegate and walk away.** If a request doesn't finish inline, OpenTag tracks
  the task and posts the result back into the thread when it completes.
- **Routines** run a prompt on an interval and post the result to a channel
  without anyone asking (see `routines` in the config).
- **Manage standing work from Slack:**
  - `@OpenTag triggers` (or `status`) — list the channel's running tasks and routines.
  - `@OpenTag stop <id>` — cancel a task or routine by id.

## Requirements

- Go 1.26+
- A reachable **kagent** server (its `/api/a2a`, `/api/sessions` endpoints)
- A Slack app with **Socket Mode** enabled

## kagent setup

Point OpenTag at any kagent server. For local dev, port-forward it:

```sh
kubectl port-forward -n kagent svc/kagent 8083:8083
# sanity check an agent's A2A card:
curl http://localhost:8083/api/a2a/<namespace>/<name>/.well-known/agent-card.json
```

## Slack app setup

Create an app from the ready-made manifest at
[`examples/slack-app-manifest.yaml`](examples/slack-app-manifest.yaml)
(https://api.slack.com/apps → "Create New App" → "From a manifest"). Then:

1. Install the app to your workspace; copy the **Bot User OAuth Token** (`xoxb-…`).
2. Generate an **App-Level Token** with `connections:write` (`xapp-…`).
3. Invite the bot to a channel and note the channel id.

## Configure & run

```sh
cp config.example.yaml config.yaml   # edit bindings + org id
export SLACK_APP_TOKEN=xapp-...
export SLACK_BOT_TOKEN=xoxb-...
make run    # or: go run ./cmd/opentag -config config.yaml
```

In Slack: `@OpenTag summarize the open incidents` — OpenTag opens a thread, runs
the bound kagent agent, and streams the reply. Anyone in the channel can reply
in that thread to continue the same session.

## Deploy to Kubernetes

A Helm chart lives in `deploy/helm/opentag`. It renders your config into a
ConfigMap and injects the Slack tokens from a Secret:

```sh
helm install opentag deploy/helm/opentag \
  --set slack.appToken=$SLACK_APP_TOKEN \
  --set slack.botToken=$SLACK_BOT_TOKEN \
  --set config.kagent.baseURL=http://kagent.kagent.svc.cluster.local:8083 \
  --set config.org.id=acme
```

Set `existingSecret` to use a Secret you manage (keys `appToken`, `botToken`),
and put bindings/routines under `config.*` (see `values.yaml`).

## Development

```sh
make test
make vet
make build
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the contribution workflow.

## License

Apache-2.0. See [LICENSE](LICENSE).

---

OpenTag is an independent open-source project. It is **not affiliated with,
sponsored by, or endorsed by Anthropic**. "Claude Tag" is referenced only to
describe the shared-AI-teammate interaction pattern OpenTag is inspired by; any
trademarks are the property of their respective owners.
