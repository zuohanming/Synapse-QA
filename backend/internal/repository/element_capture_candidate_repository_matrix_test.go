package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"

	"synapseqa/backend/internal/model"
)

const (
	repositoryFingerprintA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	repositoryFingerprintB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

var candidateColumns = []string{
	"id", "cursor_id", "session_id", "name", "fingerprint", "capture_url", "tag_name", "accessible_name",
	"locators", "quality_score", "duplicate_element_id", "page_id", "conflict_status", "conflict_resolution", "status", "expires_at",
}

func newElementCaptureRepositoryMock(t *testing.T) (*ElementCaptureRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New：%v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewElementCaptureRepository(db), mock
}

func captureCandidateRows(rows ...[]driver.Value) *sqlmock.Rows {
	result := sqlmock.NewRows(candidateColumns)
	for _, row := range rows {
		result.AddRow(row...)
	}
	return result
}

func captureCandidateRow(id string, cursorID int64, name, fingerprint string, locators []byte, quality float64, duplicateID, duplicatePageID int64, status string) []driver.Value {
	return []driver.Value{
		id, cursorID, "session-1", name, fingerprint, "https://example.test/page", "button", strings.Title(name),
		locators, quality, duplicateID, duplicatePageID, "", "", status, time.Now().Add(time.Hour),
	}
}

func expectCandidateSaveLockPrefix(mock sqlmock.Sqlmock, rows *sqlmock.Rows, elements *sqlmock.Rows) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("select page_id,status from element_capture_sessions where id=$1 and status in ('active','completed') for update")).
		WithArgs("session-1").
		WillReturnRows(sqlmock.NewRows([]string{"page_id", "status"}).AddRow(8, "active"))
	mock.ExpectQuery(regexp.QuoteMeta("select id from ui_assets where id=$1 and deleted_at is null for update")).
		WithArgs(8).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(8))
	mock.ExpectQuery(`where c\.session_id=\$1 order by c\.cursor_id for update of c`).
		WithArgs("session-1").
		WillReturnRows(rows)
	mock.ExpectQuery(regexp.QuoteMeta("select id,lower(name),fingerprint from page_elements where page_id=$1 and deleted_at is null for update")).
		WithArgs(8).
		WillReturnRows(elements)
}

func TestElementCaptureRepositoryRejects501stCandidateUnderTokenAndActiveSessionLock(t *testing.T) {
	repo, mock := newElementCaptureRepositoryMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`select page_id, candidate_count from element_capture_sessions[\s\S]*executor_id = \$2[\s\S]*token_hash = \$3[\s\S]*status = 'active'[\s\S]*for update`).
		WithArgs("session-1", "executor-1", "token-hash").
		WillReturnRows(sqlmock.NewRows([]string{"page_id", "candidate_count"}).AddRow(8, 500))
	mock.ExpectRollback()

	_, err := repo.AddCandidate(context.Background(), model.ElementCaptureCandidate{
		SessionID: "session-1", Name: "submit", Fingerprint: repositoryFingerprintA,
	}, "executor-1", "token-hash")
	if err == nil || !strings.Contains(err.Error(), "最多 500") {
		t.Fatalf("第 501 个候选未被拒绝：%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryReturnsWarningWhenCandidateCountReaches400(t *testing.T) {
	repo, mock := newElementCaptureRepositoryMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`select page_id, candidate_count from element_capture_sessions[\s\S]*status = 'active'[\s\S]*for update`).
		WithArgs("session-1", "executor-1", "token-hash").
		WillReturnRows(sqlmock.NewRows([]string{"page_id", "candidate_count"}).AddRow(8, 399))
	mock.ExpectQuery(`select id from page_elements where page_id = \$1 and fingerprint = \$2 and deleted_at is null order by id limit 1`).
		WithArgs(8, repositoryFingerprintA).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`insert into element_capture_candidates`).
		WithArgs(sqlmock.AnyArg(), "session-1", "submit", repositoryFingerprintA, "https://example.test", "button", "Submit", []byte(`[{"type":"testid","value":"submit","score":95,"unique":true}]`), 95.0, int64(0), "", "", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"cursor_id"}).AddRow(400))
	mock.ExpectExec(`update element_capture_sessions set candidate_count=\$1`).
		WithArgs(400, "session-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	added, err := repo.AddCandidate(context.Background(), model.ElementCaptureCandidate{
		SessionID: "session-1", Name: "submit", Fingerprint: repositoryFingerprintA,
		CaptureURL: "https://example.test", TagName: "button", AccessibleName: "Submit",
		Locators: []byte(`[{"type":"testid","value":"submit","score":95,"unique":true}]`), QualityScore: 95,
	}, "executor-1", "token-hash")
	if err != nil {
		t.Fatalf("AddCandidate 返回错误：%v", err)
	}
	if added.CandidateCount != 400 || !strings.Contains(added.Warning, "400") {
		t.Fatalf("400 项预警错误：%+v", added)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryRequiresMatchingTokenAndActiveSession(t *testing.T) {
	repo, mock := newElementCaptureRepositoryMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`select page_id, candidate_count from element_capture_sessions[\s\S]*token_hash = \$3[\s\S]*status = 'active'`).
		WithArgs("session-1", "executor-1", "wrong-token-hash").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	_, err := repo.AddCandidate(context.Background(), model.ElementCaptureCandidate{SessionID: "session-1"}, "executor-1", "wrong-token-hash")
	if err == nil || !strings.Contains(err.Error(), "令牌无效") || !strings.Contains(err.Error(), "未激活") {
		t.Fatalf("token/active 校验错误：%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryListsVisibleCandidatesByCursorAndLimit(t *testing.T) {
	repo, mock := newElementCaptureRepositoryMock(t)
	mock.ExpectQuery(`from element_capture_candidates c join element_capture_sessions s on s\.id=c\.session_id join users u on u\.username=s\.created_by[\s\S]*u\.id=\$2[\s\S]*c\.cursor_id>\$3 order by c\.cursor_id asc limit \$4`).
		WithArgs("session-1", int64(7), int64(5), 20).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "cursor_id", "session_id", "name", "fingerprint", "capture_url", "tag_name", "accessible_name",
			"locators", "quality_score", "duplicate_element_id", "conflict_status", "conflict_resolution", "status", "expires_at",
		}).AddRow("candidate-6", 6, "session-1", "submit", repositoryFingerprintA, "https://example.test", "button", "Submit", []byte(`[]`), 95, 0, "", "", "pending", time.Now()))

	items, err := repo.ListCandidates(context.Background(), 7, "session-1", 5, 20)
	if err != nil {
		t.Fatalf("ListCandidates 返回错误：%v", err)
	}
	if len(items) != 1 || items[0].CursorID != 6 {
		t.Fatalf("游标结果错误：%+v", items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryLoadsCurrentBatchCandidatesNamesAndFingerprints(t *testing.T) {
	repo, mock := newElementCaptureRepositoryMock(t)
	mock.ExpectQuery(`select id,page_id,status from element_capture_sessions where id=\$1`).
		WithArgs("session-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "page_id", "status"}).AddRow("session-1", 8, "completed"))
	mock.ExpectQuery(`from element_capture_candidates c left join page_elements p[\s\S]*where c\.session_id=\$1 order by c\.cursor_id`).
		WithArgs("session-1").
		WillReturnRows(captureCandidateRows(
			captureCandidateRow("candidate-1", 1, "submit", repositoryFingerprintA, []byte(`[]`), 95, 42, 8, "pending"),
			captureCandidateRow("candidate-2", 2, "cancel", repositoryFingerprintB, []byte(`[]`), 95, 0, 0, "saved"),
		))
	mock.ExpectQuery(`select id,lower\(name\),fingerprint from page_elements where page_id=\$1 and deleted_at is null`).
		WithArgs(8).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "fingerprint"}).
			AddRow(42, "submit", repositoryFingerprintA).
			AddRow(43, "copy", repositoryFingerprintA))

	data, err := repo.GetBatchSaveData(context.Background(), "session-1", []int64{1})
	if err != nil {
		t.Fatalf("GetBatchSaveData 返回错误：%v", err)
	}
	if data.Session.Status != "completed" || len(data.Candidates) != 2 {
		t.Fatalf("未读取会话全部当前候选：%+v", data)
	}
	if len(data.ExistingNames["submit"]) != 1 || len(data.ExistingFingerprints[repositoryFingerprintA]) != 2 {
		t.Fatalf("当前名称/指纹索引错误：names=%+v fingerprints=%+v", data.ExistingNames, data.ExistingFingerprints)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryBatchSaveCommitsCreateUpdateIgnoreVersionsAndAudit(t *testing.T) {
	repo, mock := newElementCaptureRepositoryMock(t)
	createLocators := []byte(`[{"type":"testid","value":"create","index":"0","score":95,"unique":true}]`)
	updateLocators := []byte(`[{"type":"css","value":"#updated","index":"1","score":90,"unique":true}]`)
	rows := captureCandidateRows(
		captureCandidateRow("candidate-create", 1, "create", repositoryFingerprintA, createLocators, 95, 0, 0, "pending"),
		captureCandidateRow("candidate-update", 2, "updated", repositoryFingerprintB, updateLocators, 90, 0, 0, "pending"),
		captureCandidateRow("candidate-ignore", 3, "", "", []byte(`[]`), 0, 0, 0, "pending"),
		captureCandidateRow("candidate-already-saved", 4, "old", repositoryFingerprintA, createLocators, 95, 0, 0, "saved"),
	)
	elements := sqlmock.NewRows([]string{"id", "name", "fingerprint"}).AddRow(42, "existing", repositoryFingerprintB)
	expectCandidateSaveLockPrefix(mock, rows, elements)

	mock.ExpectQuery(`insert into page_elements`).
		WithArgs(8, "create", "testid", "create", "0", "", "", "", "", "", "", repositoryFingerprintA, "https://example.test/page", "button", "Create", 95.0, "admin").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(101))
	mock.ExpectExec(`insert into page_element_versions`).
		WithArgs(int64(101), 1, sqlmock.AnyArg(), "采集候选项审核入库", "admin").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`update element_capture_candidates set status='saved',conflict_resolution=\$1`).
		WithArgs("create", int64(1), "session-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectQuery(`update page_elements set name=\$1`).
		WithArgs("updated", "css", "#updated", "1", "", "", "", "", "", "", repositoryFingerprintB, "https://example.test/page", "button", "Updated", 90.0, "admin", int64(42), int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "current_version"}).AddRow(42, 3))
	mock.ExpectExec(`insert into page_element_versions`).
		WithArgs(int64(42), 3, sqlmock.AnyArg(), "采集候选项审核入库", "admin").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`update element_capture_candidates set status='saved',conflict_resolution=\$1`).
		WithArgs("update", int64(2), "session-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(`update element_capture_candidates set status='ignored'`).
		WithArgs(int64(3), "session-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`insert into operation_logs`).
		WithArgs("admin", "session-1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	validated := false
	result, err := repo.SaveCandidates(context.Background(), "admin", model.CandidateBatchSaveRequest{
		SessionID: "session-1",
		Items: []model.CandidateSaveItem{
			{CandidateID: 1, Resolution: "create"},
			{CandidateID: 2, Resolution: "update", TargetElementID: 42},
			{CandidateID: 3, Resolution: "ignore"},
		},
	}, func(data model.CaptureBatchData) error {
		validated = true
		if len(data.Candidates) != 3 {
			t.Fatalf("回调应仅接收请求候选，但锁 SQL 必须读取全部候选：%+v", data.Candidates)
		}
		if data.Candidates[1].DuplicateElementID != 42 {
			t.Fatalf("update 未使用显式目标：%+v", data.Candidates[1])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("SaveCandidates 返回错误：%v", err)
	}
	if !validated || len(result.SavedCandidateIDs) != 2 || result.SavedCandidateIDs[0] != 1 || result.SavedCandidateIDs[1] != 2 || len(result.IgnoredCandidateIDs) != 1 || result.IgnoredCandidateIDs[0] != 3 {
		t.Fatalf("事务结果错误：validated=%v result=%+v", validated, result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryBatchSaveConstraintMappingMatrix(t *testing.T) {
	tests := []struct {
		name            string
		constraintName  string
		wantDomain      string
		wantPassthrough bool
	}{
		{name: "maps exact active name constraint", constraintName: "uq_page_elements_active_name", wantDomain: "候选项 1：页面元素名称冲突"},
		{name: "passes through unrelated unique violation", constraintName: "page_elements_pkey", wantPassthrough: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newElementCaptureRepositoryMock(t)
			locators := []byte(`[{"type":"testid","value":"create","score":95,"unique":true}]`)
			expectCandidateSaveLockPrefix(
				mock,
				captureCandidateRows(captureCandidateRow("candidate-create", 1, "create", repositoryFingerprintA, locators, 95, 0, 0, "pending")),
				sqlmock.NewRows([]string{"id", "name", "fingerprint"}),
			)
			pgErr := &pgconn.PgError{Code: "23505", ConstraintName: tt.constraintName}
			mock.ExpectQuery(`insert into page_elements`).WillReturnError(pgErr)
			mock.ExpectRollback()

			_, err := repo.SaveCandidates(context.Background(), "admin", model.CandidateBatchSaveRequest{
				SessionID: "session-1",
				Items:     []model.CandidateSaveItem{{CandidateID: 1, Resolution: "create"}},
			}, func(model.CaptureBatchData) error { return nil })
			if tt.wantPassthrough {
				var actual *pgconn.PgError
				if !errors.As(err, &actual) || actual.ConstraintName != tt.constraintName {
					t.Fatalf("其它 23505 未原样透传：%T %v", err, err)
				}
			} else if err == nil || err.Error() != tt.wantDomain {
				t.Fatalf("命名约束映射错误：%v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestElementCaptureRepositoryUpdateCandidateConstraintMappingMatrix(t *testing.T) {
	tests := []struct {
		name            string
		constraintName  string
		wantDomain      string
		wantPassthrough bool
	}{
		{name: "maps exact active name constraint", constraintName: "uq_page_elements_active_name", wantDomain: "页面元素名称冲突"},
		{name: "passes through unrelated unique violation", constraintName: "other_unique_constraint", wantPassthrough: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newElementCaptureRepositoryMock(t)
			pgErr := &pgconn.PgError{Code: "23505", ConstraintName: tt.constraintName}
			mock.ExpectExec(`update element_capture_candidates c set`).
				WithArgs("renamed", sqlmock.AnyArg(), nil, "", int64(1), "session-1").
				WillReturnError(pgErr)

			_, err := repo.UpdateCandidate(context.Background(), "admin", "session-1", 1, model.CaptureCandidateUpdateRequest{Name: "renamed"})
			if tt.wantPassthrough {
				var actual *pgconn.PgError
				if !errors.As(err, &actual) || actual.ConstraintName != tt.constraintName {
					t.Fatalf("其它 23505 未原样透传：%T %v", err, err)
				}
			} else if err == nil || err.Error() != tt.wantDomain {
				t.Fatalf("命名约束映射错误：%v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
