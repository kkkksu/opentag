package kagent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"

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

func TestTaskToBackend(t *testing.T) {
	task := &a2a.Task{
		ID:     "task-1",
		Status: a2a.TaskStatus{State: a2a.TaskStateCompleted},
		Artifacts: []*a2a.Artifact{
			{Parts: a2a.ContentParts{a2a.NewTextPart("the "), a2a.NewTextPart("result")}},
		},
	}
	got := taskToBackend(task)
	if got.ID != "task-1" || !got.Terminal || got.Text != "the result" {
		t.Errorf("taskToBackend = %+v", got)
	}
}

func TestTaskToBackend_FallsBackToStatusMessage(t *testing.T) {
	task := &a2a.Task{
		ID: "t2",
		Status: a2a.TaskStatus{
			State:   a2a.TaskStateWorking,
			Message: a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("still going")),
		},
	}
	got := taskToBackend(task)
	if got.Terminal || got.Text != "still going" {
		t.Errorf("taskToBackend = %+v", got)
	}
}

func TestAgentRefString(t *testing.T) {
	if got := agentRefString(config.AgentRef{Namespace: "ns", Name: "a"}); got != "ns/a" {
		t.Errorf("agentRefString = %q", got)
	}
}
