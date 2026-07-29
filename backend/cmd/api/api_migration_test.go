package main

import (
	"strings"
	"testing"
)

func TestElementCaptureMigrationDefinesActiveElementUniqueness(t *testing.T) {
	statements := elementCaptureMigrationStatements()
	joined := strings.Join(statements, "\n")
	for _, want := range []string{
		"duplicate active page element names",
		"uq_page_elements_active_name",
		"drop index if exists uq_page_elements_active_fingerprint",
		"uq_element_capture_candidates_cursor_id",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing element capture migration invariant: %s", want)
		}
	}
}

func TestPermissionSeedsContainElementCapturePermissions(t *testing.T) {
	seedCodes := permissionSeedCodes()
	for _, want := range []string{
		"ui.element.read",
		"ui.element.capture",
		"ui.element.manage",
		"ui.element.rollback",
	} {
		if !seedCodes[want] {
			t.Fatalf("权限种子缺少 %q", want)
		}
	}
}

func TestElementCaptureMigrationStatementsSupportSessionRecovery(t *testing.T) {
	ddl := strings.Join(elementCaptureMigrationStatements(), "\n")
	for _, want := range []string{
		"token_hash text not null",
		"last_heartbeat_at timestamptz not null",
		"interrupted_at timestamptz",
		"recovery_expires_at timestamptz",
		"browser_channel text not null",
	} {
		if !strings.Contains(ddl, want) {
			t.Fatalf("会话迁移缺少 %q", want)
		}
	}
}

func TestElementCaptureMigrationStatementsRestrictAllActiveSessionStatuses(t *testing.T) {
	ddl := strings.Join(elementCaptureMigrationStatements(), "\n")
	if !strings.Contains(ddl, "where status in ('starting', 'active', 'interrupted')") {
		t.Fatal("活动会话唯一索引未覆盖 starting、active 与 interrupted 状态")
	}
}

func TestElementCaptureMigrationStatementsStoreCandidateReviewData(t *testing.T) {
	ddl := strings.Join(elementCaptureMigrationStatements(), "\n")
	for _, want := range []string{
		"name text not null default ''",
		"capture_url text not null default ''",
		"client_capture_id text",
		"uq_element_capture_candidates_session_client_capture",
		"duplicate_element_id bigint references page_elements(id)",
		"conflict_status text not null default ''",
		"conflict_resolution text not null default ''",
	} {
		if !strings.Contains(ddl, want) {
			t.Fatalf("候选迁移缺少 %q", want)
		}
	}
}
