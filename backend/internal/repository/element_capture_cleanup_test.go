package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestElementCaptureRepositoryCleanupExpiredData(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("delete from element_capture_candidates where expires_at <= $1")).WithArgs(now).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("update element_capture_sessions set token_hash=''").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err = NewElementCaptureRepository(db).CleanupExpiredData(context.Background(), now); err != nil {
		t.Fatalf("CleanupExpiredData 返回错误：%v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryCleanupExpiredDataRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("delete from element_capture_candidates where expires_at <= $1")).WithArgs(now).WillReturnError(context.DeadlineExceeded)
	mock.ExpectRollback()
	if err = NewElementCaptureRepository(db).CleanupExpiredData(context.Background(), now); err == nil {
		t.Fatal("期望清理失败")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
