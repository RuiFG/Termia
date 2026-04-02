package agentapp

import (
	"context"
	"testing"

	runtimeagent "github.com/termia/termia/internal/agent"
)

func TestRegistryRejectsScopeMismatch(t *testing.T) {
	registry := NewRegistry(DefaultMiddlewareSpecs()...)

	_, err := registry.Build(MiddlewareActivation{Name: "ralph-loop", Scope: MiddlewareScopeSession})
	if err == nil {
		t.Fatalf("expected scope mismatch error, got nil")
	}
}

func TestResolveSharedSlashCommandBuildsRunScopedActivation(t *testing.T) {
	got, ok := ResolveSharedSlashCommand("/ralph-loop", DefaultSharedSlashCommands())
	if !ok {
		t.Fatalf("expected shared slash command to resolve")
	}

	if got.Name != "ralph-loop" {
		t.Fatalf("unexpected slash command name: %+v", got)
	}

	activation, err := got.BuildActivation("")
	if err != nil {
		t.Fatalf("BuildActivation returned error: %v", err)
	}
	if activation.Name != "ralph-loop" {
		t.Fatalf("unexpected activation name: %+v", activation)
	}
	if activation.Scope != MiddlewareScopeRun {
		t.Fatalf("expected run-scoped activation, got %+v", activation)
	}
}

func TestRalphLoopRequestsContinueAfterCommandRun(t *testing.T) {
	registry := NewRegistry(DefaultMiddlewareSpecs()...)

	instance, err := registry.Build(MiddlewareActivation{Name: "ralph-loop", Scope: MiddlewareScopeRun})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	ctx := &RunContext{
		SessionID:        "session-1",
		Query:            "follow up",
		Cwd:              "/workdir",
		State:            DefaultSessionState(),
		SelectedCommands: []runtimeagent.Command{{ID: "cmd-1"}},
	}

	directive, err := instance.AfterRun(context.Background(), ctx, RunSummary{SawCommand: true})
	if err != nil {
		t.Fatalf("AfterRun returned error: %v", err)
	}
	if !directive.Continue {
		t.Fatalf("expected continue directive, got %+v", directive)
	}
	if directive.NextQuery == "" {
		t.Fatalf("expected non-empty next query, got %+v", directive)
	}
	if directive.EmitText != "" {
		t.Fatalf("expected no emit text when continuing, got %+v", directive)
	}

	directive, err = instance.AfterRun(context.Background(), ctx, RunSummary{SawCommand: false})
	if err != nil {
		t.Fatalf("AfterRun returned error: %v", err)
	}
	if directive.Continue {
		t.Fatalf("expected non-continuing directive, got %+v", directive)
	}
	if directive.EmitText != "已完成" {
		t.Fatalf("expected completion text, got %+v", directive)
	}
}
