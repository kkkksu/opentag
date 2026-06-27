package kagent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kkkksu/opentag/internal/backend"
	"github.com/kkkksu/opentag/internal/config"
)

func TestEnsureSession_RequestShaping(t *testing.T) {
	var gotPath, gotUser string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUser = r.Header.Get("X-User-ID")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"error":false,"data":{"id":"thread-x"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.EnsureSession(context.Background(), backend.EnsureSessionInput{
		UserID:    "opentag:org:acme:T1:C1",
		SessionID: "thread-x",
		Agent:     config.AgentRef{Namespace: "kagent", Name: "k8s"},
		Name:      "1700.1",
	})
	if err != nil {
		t.Fatalf("EnsureSession() = %v", err)
	}
	if gotPath != "/api/sessions" {
		t.Errorf("path = %q", gotPath)
	}
	if gotUser != "opentag:org:acme:T1:C1" {
		t.Errorf("X-User-ID = %q", gotUser)
	}
	if gotBody["id"] != "thread-x" || gotBody["agent_ref"] != "kagent/k8s" || gotBody["source"] != "user" {
		t.Errorf("body = %+v", gotBody)
	}
}

func TestEnsureSession_ConflictIsIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()

	if err := New(srv.URL).EnsureSession(context.Background(), backend.EnsureSessionInput{
		SessionID: "s", Agent: config.AgentRef{Namespace: "n", Name: "m"},
	}); err != nil {
		t.Errorf("409 should be idempotent success, got %v", err)
	}
}

func TestEnsureSession_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer srv.Close()

	if err := New(srv.URL).EnsureSession(context.Background(), backend.EnsureSessionInput{
		SessionID: "s", Agent: config.AgentRef{Namespace: "n", Name: "m"},
	}); err == nil {
		t.Errorf("expected error on 500")
	}
}

func TestGetTask_ParsesEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-User-ID"); got != "u1" {
			t.Errorf("X-User-ID = %q", got)
		}
		_, _ = w.Write([]byte(`{"error":false,"data":{"id":"task-1","status":{"state":"TASK_STATE_COMPLETED"}}}`))
	}))
	defer srv.Close()

	task, err := New(srv.URL).GetTask(context.Background(), "u1", "task-1")
	if err != nil {
		t.Fatalf("GetTask() = %v", err)
	}
	if task.ID != "task-1" || task.State != "TASK_STATE_COMPLETED" {
		t.Errorf("task = %+v", task)
	}
}

func TestAgentRefString(t *testing.T) {
	if got := agentRefString(config.AgentRef{Namespace: "ns", Name: "a"}); got != "ns/a" {
		t.Errorf("agentRefString = %q", got)
	}
}
