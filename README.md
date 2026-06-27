# OpenTag

**A shared `@OpenTag` AI teammate for Slack, backed by [kagent](https://github.com/kagent-dev/kagent).**

Instead of a private chatbot per person, a Slack **channel** gets **one shared
teammate** that everyone can tag, watch, redirect, and hand work to. OpenTag
handles the chat-collaboration layer — identity, sessions, governance, audit —
and delegates the actual agent execution to a kagent cluster.

Status: alpha. Channel mentions, DMs, channel-scoped memory, background tasks,
and proactive routines work today.

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

Create an app with Socket Mode. Minimal manifest:

```yaml
display_information:
  name: OpenTag
features:
  bot_user:
    display_name: OpenTag
    always_online: true
oauth_config:
  scopes:
    bot: [app_mentions:read, chat:write, im:history, im:read]
settings:
  event_subscriptions:
    bot_events: [app_mention, message.im]
  socket_mode_enabled: true
```

Then:
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
