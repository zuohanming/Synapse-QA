package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"synapseqa/backend/internal/model"
)

const captureRecoveryWindow = 60 * time.Second

// ElementCaptureRepository 负责页面元素采集会话的状态持久化。
type ElementCaptureRepository struct {
	db *sql.DB
}

func NewElementCaptureRepository(db *sql.DB) *ElementCaptureRepository {
	return &ElementCaptureRepository{db: db}
}

func (r *ElementCaptureRepository) PageExists(ctx context.Context, pageID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		select exists(select 1 from ui_assets where id = $1 and deleted_at is null)
	`, pageID).Scan(&exists)
	return exists, err
}

func (r *ElementCaptureRepository) HasActiveCapture(ctx context.Context, executorID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		select exists(select 1 from element_capture_sessions where executor_id = $1 and status in ('starting', 'active', 'interrupted'))
	`, executorID).Scan(&exists)
	return exists, err
}

func (r *ElementCaptureRepository) CreateSession(ctx context.Context, session model.ElementCaptureSession) error {
	_, err := r.db.ExecContext(ctx, `
		insert into element_capture_sessions(
			id, page_id, executor_id, browser_context_id, browser_channel, created_by,
			status, mode, current_url, token_hash, last_heartbeat_at, expires_at
		) values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, session.ID, session.PageID, session.ExecutorID, session.BrowserContextID, session.BrowserChannel, session.CreatedBy,
		session.Status, session.Mode, session.CurrentURL, session.TokenHash, session.LastHeartbeatAt, session.ExpiresAt)
	return err
}

func (r *ElementCaptureRepository) GetSession(ctx context.Context, userID int64, sessionID string) (model.ElementCaptureSessionDetail, error) {
	row := r.db.QueryRowContext(ctx, `
		select s.id, s.page_id, s.executor_id, s.browser_context_id, s.browser_channel,
		       s.created_by, s.status, s.mode, s.current_url, s.token_hash, s.candidate_count,
		       s.last_heartbeat_at, s.interrupted_at, s.recovery_expires_at, s.expires_at
		from element_capture_sessions s
		join users u on u.username = s.created_by
		where s.id = $1 and u.id = $2
	`, sessionID, userID)
	var detail model.ElementCaptureSessionDetail
	err := row.Scan(
		&detail.ID, &detail.PageID, &detail.ExecutorID, &detail.BrowserContextID, &detail.BrowserChannel,
		&detail.CreatedBy, &detail.Status, &detail.Mode, &detail.CurrentURL, &detail.TokenHash, &detail.CandidateCount,
		&detail.LastHeartbeatAt, &detail.InterruptedAt, &detail.RecoveryExpiresAt, &detail.ExpiresAt,
	)
	return detail, err
}

func (r *ElementCaptureRepository) SetMode(ctx context.Context, actor, sessionID, mode string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `
		update element_capture_sessions
		set mode = $1, updated_at = now()
		where id = $2 and created_by = $3 and status in ('starting', 'active', 'interrupted')
	`, mode, sessionID, actor)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (r *ElementCaptureRepository) StopSession(ctx context.Context, actor, sessionID string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `
		update element_capture_sessions
		set status = 'completed', updated_at = now()
		where id = $1 and created_by = $2 and status in ('starting', 'active', 'interrupted')
	`, sessionID, actor)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (r *ElementCaptureRepository) Heartbeat(ctx context.Context, sessionID, executorID, tokenHash, currentURL string) (bool, error) {
	var id string
	err := r.db.QueryRowContext(ctx, `
		update element_capture_sessions
		set status = 'active', last_heartbeat_at = now(), interrupted_at = null,
		    recovery_expires_at = null, current_url = $3, updated_at = now()
		where id = $1 and executor_id = $2 and token_hash = $4
		  and status in ('starting', 'active', 'interrupted')
		  and (status <> 'interrupted' or recovery_expires_at >= now())
		returning id
	`, sessionID, executorID, currentURL, tokenHash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (r *ElementCaptureRepository) ExpireSessions(ctx context.Context, now time.Time) error {
	if _, err := r.db.ExecContext(ctx, `
		update element_capture_sessions
		set status = 'expired', updated_at = now()
		where status in ('starting', 'active', 'interrupted') and expires_at <= $1
	`, now); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `
		update element_capture_sessions
		set status = 'interrupted', interrupted_at = $1, recovery_expires_at = $2, updated_at = now()
		where status = 'active' and expires_at > $1 and last_heartbeat_at < $3
	`, now, now.Add(captureRecoveryWindow), now.Add(-captureRecoveryWindow)); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `
		update element_capture_sessions
		set status = 'expired', updated_at = now()
		where status = 'interrupted' and recovery_expires_at <= $1
	`, now)
	return err
}
