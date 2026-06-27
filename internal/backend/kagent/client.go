// Package kagent implements backend.AgentBackend against a kagent server.
//
// It uses two of kagent's surfaces:
//   - A2A (github.com/a2aproject/a2a-go) at /api/a2a/{ns}/{name} to stream turns.
//   - REST (/api/sessions, /api/tasks) to create durable sessions and poll tasks.
//
// Identity flows via the X-User-ID header, matching kagent's principal handling
// (dev default "admin@kagent.dev"); OpenTag sends the per-channel service id or
// the per-user id derived in the identity package.
package kagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"
	"github.com/google/uuid"

	"github.com/kkkksu/opentag/internal/backend"
	"github.com/kkkksu/opentag/internal/config"
)

// protocolVersion mirrors the version kagent's A2A registrar negotiates so
// OpenTag stays compatible with both v1 and legacy (0.3) agent runtimes.
const protocolVersion = a2a.ProtocolVersion("0.3")

const userIDHeader = "X-User-ID"

// Client talks to a kagent server. It caches one A2A client per (agent, user).
type Client struct {
	baseURL string
	rest    *http.Client

	mu      sync.Mutex
	clients map[string]*a2aclient.Client
}

var _ backend.AgentBackend = (*Client)(nil)

// New returns a Client targeting a kagent server at baseURL (e.g.
// http://localhost:8083).
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		rest:    &http.Client{Timeout: 30 * time.Second},
		clients: make(map[string]*a2aclient.Client),
	}
}

// headerTransport injects X-User-ID on every request.
type headerTransport struct {
	base   http.RoundTripper
	userID string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set(userIDHeader, t.userID)
	return t.base.RoundTrip(req)
}

func (c *Client) a2aURL(agent config.AgentRef) string {
	return fmt.Sprintf("%s/api/a2a/%s/%s", c.baseURL, agent.Namespace, agent.Name)
}

// agentRefString is the "namespace/name" form kagent's REST API expects.
func agentRefString(a config.AgentRef) string { return a.Namespace + "/" + a.Name }

// a2aClientFor returns a cached A2A client for (agent, userID), creating one if
// needed. Each gets its own HTTP client that stamps the user's X-User-ID.
func (c *Client) a2aClientFor(ctx context.Context, agent config.AgentRef, userID string) (*a2aclient.Client, error) {
	key := agentRefString(agent) + "|" + userID

	c.mu.Lock()
	defer c.mu.Unlock()
	if cl, ok := c.clients[key]; ok {
		return cl, nil
	}

	httpClient := &http.Client{
		Timeout:   5 * time.Minute,
		Transport: &headerTransport{base: http.DefaultTransport, userID: userID},
	}
	endpoints := []*a2a.AgentInterface{{
		URL:             c.a2aURL(agent),
		ProtocolBinding: a2a.TransportProtocolJSONRPC,
		ProtocolVersion: protocolVersion,
	}}

	cl, err := a2aclient.NewFromEndpoints(
		ctx,
		endpoints,
		a2aclient.WithJSONRPCTransport(httpClient),
		// Legacy fallback so agents still on the 0.3 wire keep working,
		// matching kagent's own client construction.
		a2aclient.WithCompatTransport(
			protocolVersion,
			a2a.TransportProtocolJSONRPC,
			a2aclient.TransportFactoryFn(func(_ context.Context, _ *a2a.AgentCard, iface *a2a.AgentInterface) (a2aclient.Transport, error) {
				return a2av0.NewJSONRPCTransport(a2av0.JSONRPCTransportConfig{
					URL:    iface.URL,
					Client: httpClient,
				}), nil
			}),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create A2A client for %s: %w", key, err)
	}
	c.clients[key] = cl
	return cl, nil
}

// EnsureSession idempotently creates the kagent session backing a thread.
// A 409 (already exists) is treated as success.
func (c *Client) EnsureSession(ctx context.Context, in backend.EnsureSessionInput) error {
	source := in.Source
	if source == "" {
		source = "user"
	}
	agentRef := agentRefString(in.Agent)
	body := map[string]any{
		"id":        in.SessionID,
		"agent_ref": agentRef,
		"source":    source,
	}
	if in.Name != "" {
		body["name"] = in.Name
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal session request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/sessions", bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("build session request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(userIDHeader, in.UserID)

	resp, err := c.rest.Do(req)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusConflict:
		return nil // session already exists — idempotent success
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	default:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("create session for %s: kagent returned %s: %s", agentRef, resp.Status, strings.TrimSpace(string(msg)))
	}
}

// Stream sends one turn to the agent, invoking onChunk for each text fragment.
func (c *Client) Stream(ctx context.Context, in backend.StreamInput, onChunk func(string)) (backend.StreamResult, error) {
	cl, err := c.a2aClientFor(ctx, in.Agent, in.UserID)
	if err != nil {
		return backend.StreamResult{}, err
	}

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(in.Text))
	msg.ID = uuid.NewString()
	if in.SessionID != "" {
		msg.ContextID = in.SessionID
	}

	result := backend.StreamResult{SessionID: in.SessionID}
	for event, evErr := range cl.SendStreamingMessage(ctx, &a2a.SendMessageRequest{Message: msg}) {
		if evErr != nil {
			return result, fmt.Errorf("stream from agent %s: %w", agentRefString(in.Agent), evErr)
		}
		if id := contextIDOf(event); id != "" {
			result.SessionID = id
		}
		if tid, done := taskStateOf(event); tid != "" {
			result.TaskID = tid
			result.Done = done
		}
		if chunk := textOf(event); chunk != "" {
			onChunk(chunk)
		}
	}
	// A bare Message response (no Task) means the turn completed inline.
	if result.TaskID == "" {
		result.Done = true
	}
	return result, nil
}

// GetTask fetches async task state. Best-effort decode of the kagent task
// envelope; refined when the async tracker lands.
func (c *Client) GetTask(ctx context.Context, userID, taskID string) (backend.Task, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tasks/"+taskID, nil)
	if err != nil {
		return backend.Task{}, fmt.Errorf("build task request: %w", err)
	}
	req.Header.Set(userIDHeader, userID)

	resp, err := c.rest.Do(req)
	if err != nil {
		return backend.Task{}, fmt.Errorf("get task %s: %w", taskID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return backend.Task{}, fmt.Errorf("get task %s: kagent returned %s: %s", taskID, resp.Status, strings.TrimSpace(string(msg)))
	}

	var env struct {
		Data struct {
			ID     string `json:"id"`
			Status struct {
				State string `json:"state"`
			} `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return backend.Task{}, fmt.Errorf("decode task %s: %w", taskID, err)
	}
	return backend.Task{ID: env.Data.ID, State: env.Data.Status.State}, nil
}

// contextIDOf extracts the conversation context id carried by an event, if any.
func contextIDOf(event a2a.Event) string {
	switch e := event.(type) {
	case *a2a.Message:
		return e.ContextID
	case *a2a.Task:
		return e.ContextID
	case *a2a.TaskStatusUpdateEvent:
		return e.ContextID
	case *a2a.TaskArtifactUpdateEvent:
		return e.ContextID
	}
	return ""
}

// taskStateOf returns the task id and whether it has reached a terminal state.
func taskStateOf(event a2a.Event) (taskID string, done bool) {
	switch e := event.(type) {
	case *a2a.Task:
		return string(e.ID), e.Status.State.Terminal()
	case *a2a.TaskStatusUpdateEvent:
		return string(e.TaskID), e.Status.State.Terminal()
	case *a2a.TaskArtifactUpdateEvent:
		return string(e.TaskID), false
	}
	return "", false
}

// textOf concatenates the text parts of an event into a single string.
func textOf(event a2a.Event) string {
	switch e := event.(type) {
	case *a2a.Message:
		return partsText(e.Parts)
	case *a2a.TaskStatusUpdateEvent:
		if e.Status.Message != nil {
			return partsText(e.Status.Message.Parts)
		}
	case *a2a.TaskArtifactUpdateEvent:
		if e.Artifact != nil {
			return partsText(e.Artifact.Parts)
		}
	}
	return ""
}

func partsText(parts a2a.ContentParts) string {
	var b strings.Builder
	for _, p := range parts {
		if p == nil {
			continue
		}
		if t, ok := p.Content.(a2a.Text); ok {
			b.WriteString(string(t))
		}
	}
	return b.String()
}
