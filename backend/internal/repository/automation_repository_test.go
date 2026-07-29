package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAutomationRepositoryListPageElementsReturnsCaptureMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("select count(*) from page_elements where page_id = $1 and deleted_at is null")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("select id, page_id, name").
		WithArgs(int64(7), 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "page_id", "name", "type1", "locator1", "index1", "type2", "locator2", "index2", "type3", "locator3", "index3", "ai_prompt", "wait_time",
			"fingerprint", "capture_source", "capture_url", "tag_name", "accessible_name", "quality_score", "captured_by", "captured_at", "last_verified_at", "verification_status", "current_version",
			"created_at", "updated_at",
		}).AddRow(
			1, 7, "提交", "css", "#submit", "", "", "", "", "", "", "", "", "",
			"sha256", "element_capture", "https://example.test/login", "button", "提交", 0.95, "admin", now, now, "verified", 2,
			now, now,
		))

	items, total, err := NewAutomationRepository(db).ListPageElements(context.Background(), 7, 1, 20)
	if err != nil {
		t.Fatalf("ListPageElements 返回错误：%v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("分页结果异常：total=%d items=%+v", total, items)
	}
	item := items[0]
	if item.Fingerprint != "sha256" || item.CaptureSource != "element_capture" || item.CaptureURL != "https://example.test/login" || item.TagName != "button" || item.AccessibleName != "提交" || item.QualityScore != 0.95 || item.CapturedBy != "admin" || item.VerificationStatus != "verified" || item.CurrentVersion != 2 {
		t.Fatalf("采集元数据未完整返回：%+v", item)
	}
	if item.CapturedAt == nil || item.LastVerifiedAt == nil {
		t.Fatalf("采集时间未返回：%+v", item)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
