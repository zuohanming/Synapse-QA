package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"synapseqa/backend/internal/model"
)

func TestAIToolDefinitionsFilterExecutionTools(t *testing.T) {
	executor := &AIToolExecutor{}
	tests := []struct {
		name          string
		claims        model.Claims
		wantExecution bool
	}{
		{name: "no permission", claims: model.Claims{RoleCode: "viewer"}},
		{name: "execution permission", claims: model.Claims{RoleCode: "viewer", Permissions: []string{"menu.execution.read"}}, wantExecution: true},
		{name: "admin", claims: model.Claims{RoleCode: "admin"}, wantExecution: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := false
			for _, definition := range executor.ToolDefinitions(tt.claims) {
				function, _ := definition["function"].(map[string]any)
				name, _ := function["name"].(string)
				if isExecutionTool(name) {
					found = true
				}
			}
			if found != tt.wantExecution {
				t.Fatalf("execution tools present=%v, want %v", found, tt.wantExecution)
			}
		})
	}
}

func TestAIToolExecutorExecutionToolsUseClaimsScope(t *testing.T) {
	repo := &fakeExecutionRepo{run: model.ExecutionRun{ID: 8, RunType: "ui", Status: "running"}}
	execSvc := NewExecutionService(repo, nil, nil, &fakeExecutionOperationLogger{}, "")
	executor := &AIToolExecutor{execSvc: execSvc}
	ctx := context.Background()

	_, err := executor.Execute(ctx, model.Claims{UserID: 7, RoleCode: "viewer"}, "list_execution_runs", json.RawMessage(`{"limit":1}`))
	if err == nil || !strings.Contains(err.Error(), "无权使用执行工具") {
		t.Fatalf("forged execution tool call error=%v", err)
	}
	if repo.listRunsScopedCalls != 0 || repo.getRunScopedCalls != 0 || repo.updateScopedCalls != 0 {
		t.Fatal("unauthorized execution tool call reached repository")
	}

	member := model.Claims{UserID: 7, RoleCode: "viewer", Permissions: []string{"menu.execution.read"}}
	if _, err := executor.Execute(ctx, member, "list_execution_runs", json.RawMessage(`{"limit":1}`)); err != nil {
		t.Fatalf("member list execution runs: %v", err)
	}
	if _, err := executor.Execute(ctx, member, "get_execution_run", json.RawMessage(`{"id":8}`)); err != nil {
		t.Fatalf("member get execution run: %v", err)
	}
	if _, err := executor.Execute(ctx, member, "cancel_execution_run", json.RawMessage(`{"id":8}`)); err != nil {
		t.Fatalf("member cancel execution run: %v", err)
	}
	if repo.listRunsScopedCalls != 1 || repo.getRunScopedCalls != 2 || repo.updateScopedCalls != 1 {
		t.Fatalf("scoped repository calls = list:%d get:%d update:%d", repo.listRunsScopedCalls, repo.getRunScopedCalls, repo.updateScopedCalls)
	}

	admin := model.Claims{UserID: 1, RoleCode: "admin"}
	if _, err := executor.Execute(ctx, admin, "list_execution_runs", json.RawMessage(`{"limit":1}`)); err != nil {
		t.Fatalf("admin list execution runs: %v", err)
	}
}
