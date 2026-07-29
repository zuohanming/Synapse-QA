package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"synapseqa/backend/internal/model"

	"github.com/jackc/pgx/v5/pgconn"
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
		select exists(select 1 from ui_assets where id = $1 and asset_type = 'page' and deleted_at is null)
	`, pageID).Scan(&exists)
	return exists, err
}

// PageAccessible 限定为页面创建者或管理员。页面目前没有 project_id，不能伪造 project_members 授权。
func (r *ElementCaptureRepository) PageAccessible(ctx context.Context, pageID int64, actor string) (bool, error) {
	var allowed bool
	err := r.db.QueryRowContext(ctx, `
		select exists(select 1 from ui_assets p where p.id=$1 and p.asset_type='page' and p.deleted_at is null and (
			p.created_by=$2 or exists(select 1 from users u join roles ro on ro.id=u.role_id where u.username=$2 and u.deleted_at is null and ro.code='admin')
		))
	`, pageID, actor).Scan(&allowed)
	return allowed, err
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

func (r *ElementCaptureRepository) CreateSessionWithStartCommand(ctx context.Context, actor string, session model.ElementCaptureSession, command model.ElementCaptureCommand) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var lockedPageID int64
	if err = tx.QueryRowContext(ctx, `select p.id from ui_assets p where p.id=$1 and p.asset_type='page' and p.deleted_at is null and (p.created_by=$2 or exists(select 1 from users u join roles ro on ro.id=u.role_id where u.username=$2 and u.deleted_at is null and ro.code='admin')) for update`, session.PageID, actor).Scan(&lockedPageID); errors.Is(err, sql.ErrNoRows) {
		return model.NewDomainError(model.ErrNotFound, "页面不存在")
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `insert into element_capture_sessions(id,page_id,executor_id,browser_context_id,browser_channel,created_by,status,mode,current_url,token_hash,last_heartbeat_at,expires_at) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, session.ID, session.PageID, session.ExecutorID, session.BrowserContextID, session.BrowserChannel, session.CreatedBy, session.Status, session.Mode, session.CurrentURL, session.TokenHash, session.LastHeartbeatAt, session.ExpiresAt); err != nil {
		return err
	}
	if err = insertCaptureCommand(ctx, tx, command, session.ExecutorID, session.ExpiresAt); err != nil {
		return err
	}
	return tx.Commit()
}

func insertCaptureCommand(ctx context.Context, tx *sql.Tx, command model.ElementCaptureCommand, executorID string, expiresAt time.Time) error {
	payload, err := json.Marshal(command)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `insert into element_capture_commands(session_id,executor_id,command_type,payload,status,expires_at) values($1,$2,$3,$4,'pending',$5)`, command.SessionID, executorID, command.Type, payload, expiresAt)
	return err
}

func (r *ElementCaptureRepository) SetModeWithCommand(ctx context.Context, actor, sessionID, mode string) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var executorID string
	err = tx.QueryRowContext(ctx, `select executor_id from element_capture_sessions where id=$1 and status in ('starting','active','interrupted') and (created_by=$2 or exists(select 1 from users u join roles ro on ro.id=u.role_id where u.username=$2 and u.deleted_at is null and ro.code='admin')) for update`, sessionID, actor).Scan(&executorID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, r.captureSessionAccessError(ctx, sessionID, actor)
	}
	if err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `update element_capture_sessions set mode=$1,updated_at=now() where id=$2`, mode, sessionID); err != nil {
		return false, err
	}
	if err = insertCaptureCommand(ctx, tx, model.ElementCaptureCommand{SessionID: sessionID, Type: "set_mode", Mode: mode}, executorID, time.Now().Add(30*time.Minute)); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (r *ElementCaptureRepository) StopSessionWithCommand(ctx context.Context, actor, sessionID string) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var executorID, status string
	err = tx.QueryRowContext(ctx, `select executor_id,status from element_capture_sessions where id=$1 and (created_by=$2 or exists(select 1 from users u join roles ro on ro.id=u.role_id where u.username=$2 and u.deleted_at is null and ro.code='admin')) for update`, sessionID, actor).Scan(&executorID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, model.NewDomainError(model.ErrNotFound, "采集会话不存在")
	}
	if err != nil {
		return false, err
	}
	if status == "completed" || status == "expired" || status == "failed" {
		return true, tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, `update element_capture_sessions set status='completed',updated_at=now() where id=$1`, sessionID); err != nil {
		return false, err
	}
	if err = insertCaptureCommand(ctx, tx, model.ElementCaptureCommand{SessionID: sessionID, Type: "stop"}, executorID, time.Now().Add(30*time.Minute)); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (r *ElementCaptureRepository) captureSessionAccessError(ctx context.Context, sessionID, actor string) error {
	var exists bool
	err := r.db.QueryRowContext(ctx, `select exists(select 1 from element_capture_sessions where id=$1 and (created_by=$2 or exists(select 1 from users u join roles ro on ro.id=u.role_id where u.username=$2 and u.deleted_at is null and ro.code='admin')))`, sessionID, actor).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return model.NewDomainError(model.ErrConflict, "采集会话已结束")
	}
	return model.NewDomainError(model.ErrNotFound, "采集会话不存在")
}

func (r *ElementCaptureRepository) AuthorizeCommandExecutor(ctx context.Context, executorID, token string) (bool, error) {
	var allowed bool
	err := r.db.QueryRowContext(ctx, `select exists(select 1 from executors e where e.executor_id=$1 and (e.executor_token=$2 or (e.executor_token='' and exists(select 1 from platform_settings where key='executor_shared_token' and value=$2))))`, executorID, token).Scan(&allowed)
	return allowed, err
}

func (r *ElementCaptureRepository) ClaimCommands(ctx context.Context, executorID string, limit int) ([]model.ElementCaptureCommand, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `update element_capture_commands set status='expired' where status='pending' and expires_at <= now()`); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `delete from element_capture_commands where status in ('claimed','expired') and created_at < now()-interval '24 hours'`); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `with picked as (select id from element_capture_commands where executor_id=$1 and status='pending' and expires_at>now() order by id for update skip locked limit $2) update element_capture_commands c set status='claimed',claimed_at=now() from picked where c.id=picked.id returning c.id,c.session_id,c.command_type,c.payload`, executorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.ElementCaptureCommand{}
	for rows.Next() {
		var item model.ElementCaptureCommand
		var payload []byte
		if err = rows.Scan(&item.ID, &item.SessionID, &item.Type, &payload); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(payload, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *ElementCaptureRepository) GetSession(ctx context.Context, userID int64, sessionID string) (model.ElementCaptureSessionDetail, error) {
	row := r.db.QueryRowContext(ctx, `
		select s.id, s.page_id, s.executor_id, s.browser_context_id, s.browser_channel,
		       s.created_by, s.status, s.mode, s.current_url, s.token_hash, s.candidate_count,
		       s.last_heartbeat_at, s.interrupted_at, s.recovery_expires_at, s.expires_at
		from element_capture_sessions s
		where s.id = $1 and (
			s.created_by=(select username from users where id=$2 and deleted_at is null)
			or exists(select 1 from users u join roles ro on ro.id=u.role_id where u.id=$2 and u.deleted_at is null and ro.code='admin')
		)
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
		where id = $2 and status in ('starting', 'active', 'interrupted') and (
			created_by=$3 or exists(select 1 from users u join roles ro on ro.id=u.role_id where u.username=$3 and u.deleted_at is null and ro.code='admin')
		)
	`, mode, sessionID, actor)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err == nil && rows == 0 {
		var exists bool
		if checkErr := r.db.QueryRowContext(ctx, `
			select exists(select 1 from element_capture_sessions where id=$1 and (
				created_by=$2 or exists(select 1 from users u join roles ro on ro.id=u.role_id where u.username=$2 and u.deleted_at is null and ro.code='admin')
			))
		`, sessionID, actor).Scan(&exists); checkErr != nil {
			return false, checkErr
		} else if exists {
			return false, model.NewDomainError(model.ErrConflict, "采集会话已结束")
		}
		return false, model.NewDomainError(model.ErrNotFound, "采集会话不存在")
	}
	return rows > 0, err
}

func (r *ElementCaptureRepository) StopSession(ctx context.Context, actor, sessionID string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `
		update element_capture_sessions
		set status = 'completed', updated_at = now()
		where id = $1 and status in ('starting', 'active', 'interrupted') and (
			created_by=$2 or exists(select 1 from users u join roles ro on ro.id=u.role_id where u.username=$2 and u.deleted_at is null and ro.code='admin')
		)
	`, sessionID, actor)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err == nil && rows == 0 {
		var exists bool
		if checkErr := r.db.QueryRowContext(ctx, `
			select exists(select 1 from element_capture_sessions where id=$1 and (
				created_by=$2 or exists(select 1 from users u join roles ro on ro.id=u.role_id where u.username=$2 and u.deleted_at is null and ro.code='admin')
			))
		`, sessionID, actor).Scan(&exists); checkErr != nil {
			return false, checkErr
		} else if exists {
			return false, model.NewDomainError(model.ErrConflict, "采集会话已结束")
		}
		return false, model.NewDomainError(model.ErrNotFound, "采集会话不存在")
	}
	return rows > 0, err
}

func (r *ElementCaptureRepository) Heartbeat(ctx context.Context, sessionID, executorID, tokenHash, browserContextID, currentURL string) (bool, error) {
	var id string
	err := r.db.QueryRowContext(ctx, `
		update element_capture_sessions
		set status = 'active', last_heartbeat_at = now(), interrupted_at = null,
		    recovery_expires_at = null, browser_context_id = case when status = 'starting' then $3 else browser_context_id end,
		    current_url = $4, updated_at = now()
		where id = $1 and executor_id = $2 and token_hash = $5 and expires_at > now()
		  and status in ('starting', 'active', 'interrupted')
		  and (status <> 'interrupted' or recovery_expires_at >= now())
		  and ((status = 'starting' and browser_context_id = '' and $3 <> '') or
		       (status in ('active', 'interrupted') and browser_context_id = $3))
		returning id
	`, sessionID, executorID, browserContextID, currentURL, tokenHash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (r *ElementCaptureRepository) ExpireSessions(ctx context.Context, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = r.expireSessionsAndEnqueue(ctx, tx, `
		update element_capture_sessions set status='expired',updated_at=now()
		where status in ('starting','active','interrupted') and expires_at <= $1 returning id,executor_id
	`, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		update element_capture_sessions
		set status = 'interrupted', interrupted_at = $1, recovery_expires_at = $2, updated_at = now()
		where status = 'active' and expires_at > $1 and last_heartbeat_at < $3
	`, now, now.Add(captureRecoveryWindow), now.Add(-captureRecoveryWindow)); err != nil {
		return err
	}
	if err = r.expireSessionsAndEnqueue(ctx, tx, `
		update element_capture_sessions set status='expired',updated_at=now()
		where status='interrupted' and recovery_expires_at <= $1 returning id,executor_id
	`, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ElementCaptureRepository) expireSessionsAndEnqueue(ctx context.Context, tx *sql.Tx, query string, now time.Time) error {
	rows, err := tx.QueryContext(ctx, query, now)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var sessionID, executorID string
		if err = rows.Scan(&sessionID, &executorID); err != nil {
			return err
		}
		if err = insertCaptureCommand(ctx, tx, model.ElementCaptureCommand{SessionID: sessionID, Type: "expire"}, executorID, now.Add(30*time.Minute)); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r *ElementCaptureRepository) FailSession(ctx context.Context, sessionID, executorID, tokenHash, reason string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `
		update element_capture_sessions set status='failed',updated_at=now()
		where id=$1 and executor_id=$2 and token_hash=$3 and status in ('starting','active','interrupted')
	`, sessionID, executorID, tokenHash)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 0 {
		var terminal bool
		if err = r.db.QueryRowContext(ctx, `select exists(select 1 from element_capture_sessions where id=$1 and executor_id=$2 and token_hash=$3 and status in ('completed','expired','failed'))`, sessionID, executorID, tokenHash).Scan(&terminal); err != nil {
			return false, err
		}
		if terminal {
			return false, model.NewDomainError(model.ErrConflict, "采集会话已结束")
		}
		return false, nil
	}
	_, err = r.db.ExecContext(ctx, `insert into operation_logs(actor,action,target) values($1,'element_capture.failed',$2)`, executorID, sessionID+":"+reason)
	return true, err
}

func (r *ElementCaptureRepository) AuthorizeExecutor(ctx context.Context, sessionID, executorID, tokenHash string) (bool, error) {
	var allowed bool
	err := r.db.QueryRowContext(ctx, `
		select exists(select 1 from element_capture_sessions where id=$1 and executor_id=$2 and token_hash=$3)
	`, sessionID, executorID, tokenHash).Scan(&allowed)
	return allowed, err
}

func (r *ElementCaptureRepository) ListVersions(ctx context.Context, userID, elementID int64) ([]model.PageElementVersion, error) {
	var authorized bool
	if err := r.db.QueryRowContext(ctx, `
		select exists(
			select 1 from page_elements e join ui_assets p on p.id=e.page_id
			where e.id=$1 and e.deleted_at is null and p.deleted_at is null and p.asset_type='page' and (
				p.created_by=(select username from users where id=$2 and deleted_at is null)
				or exists(select 1 from users u join roles ro on ro.id=u.role_id where u.id=$2 and u.deleted_at is null and ro.code='admin')
			)
		)
	`, elementID, userID).Scan(&authorized); err != nil {
		return nil, err
	}
	if !authorized {
		return nil, sql.ErrNoRows
	}
	rows, err := r.db.QueryContext(ctx, `
		select id,page_element_id,version,snapshot,change_summary,created_by,created_at
		from page_element_versions where page_element_id=$1 order by version desc
	`, elementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.PageElementVersion, 0)
	for rows.Next() {
		var item model.PageElementVersion
		if err := rows.Scan(&item.ID, &item.PageElementID, &item.Version, &item.Snapshot, &item.ChangeSummary, &item.CreatedBy, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type pageElementVersionSnapshot struct {
	ID             int64              `json:"id"`
	Version        int                `json:"version"`
	Source         string             `json:"source"`
	Name           string             `json:"name"`
	Fingerprint    string             `json:"fingerprint"`
	CaptureURL     string             `json:"captureUrl"`
	TagName        string             `json:"tagName"`
	AccessibleName string             `json:"accessibleName"`
	Locators       []candidateLocator `json:"locators"`
	QualityScore   float64            `json:"qualityScore"`
}

func (r *ElementCaptureRepository) RollbackVersion(ctx context.Context, actor string, elementID int64, version int) (model.PageElementVersion, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.PageElementVersion{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var currentVersion int
	if err = tx.QueryRowContext(ctx, `
		select e.current_version from page_elements e join ui_assets p on p.id=e.page_id
		where e.id=$1 and e.deleted_at is null and p.deleted_at is null and p.asset_type='page' and (
			p.created_by=$2 or exists(select 1 from users u join roles ro on ro.id=u.role_id where u.username=$2 and u.deleted_at is null and ro.code='admin')
		) for update of e,p
	`, elementID, actor).Scan(&currentVersion); err != nil {
		return model.PageElementVersion{}, err
	}
	var raw json.RawMessage
	if err = tx.QueryRowContext(ctx, `select snapshot from page_element_versions where page_element_id=$1 and version=$2`, elementID, version).Scan(&raw); err != nil {
		return model.PageElementVersion{}, err
	}
	var snapshot pageElementVersionSnapshot
	if err = json.Unmarshal(raw, &snapshot); err != nil || snapshot.Name == "" || len(snapshot.Locators) == 0 {
		if err == nil {
			err = errors.New("版本快照无效")
		}
		return model.PageElementVersion{}, err
	}
	first := snapshot.Locators[0]
	var second, third candidateLocator
	if len(snapshot.Locators) > 1 {
		second = snapshot.Locators[1]
	}
	if len(snapshot.Locators) > 2 {
		third = snapshot.Locators[2]
	}
	nextVersion := currentVersion + 1
	result, err := tx.ExecContext(ctx, `
		update page_elements set name=$1,type1=$2,locator1=$3,index1=$4,type2=$5,locator2=$6,index2=$7,type3=$8,locator3=$9,index3=$10,
		fingerprint=$11,capture_source=$12,capture_url=$13,tag_name=$14,accessible_name=$15,quality_score=$16,captured_by=$17,captured_at=now(),current_version=$18,updated_at=now()
		where id=$19 and deleted_at is null
	`, snapshot.Name, first.Type, first.Value, first.Index, second.Type, second.Value, second.Index, third.Type, third.Value, third.Index,
		snapshot.Fingerprint, "rollback", snapshot.CaptureURL, snapshot.TagName, snapshot.AccessibleName, snapshot.QualityScore, actor, nextVersion, elementID)
	if err != nil {
		return model.PageElementVersion{}, err
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		if rowsErr != nil {
			return model.PageElementVersion{}, rowsErr
		}
		return model.PageElementVersion{}, sql.ErrNoRows
	}
	newSnapshot := map[string]any{}
	if err = json.Unmarshal(raw, &newSnapshot); err != nil {
		return model.PageElementVersion{}, err
	}
	newSnapshot["id"] = elementID
	newSnapshot["version"] = nextVersion
	newSnapshot["source"] = "rollback"
	encodedSnapshot, err := json.Marshal(newSnapshot)
	if err != nil {
		return model.PageElementVersion{}, err
	}
	item := model.PageElementVersion{PageElementID: elementID, Version: nextVersion, Snapshot: encodedSnapshot, ChangeSummary: fmt.Sprintf("回滚至版本 %d", version), CreatedBy: actor}
	if err = tx.QueryRowContext(ctx, `insert into page_element_versions(page_element_id,version,snapshot,change_summary,created_by) values($1,$2,$3,$4,$5) returning id,created_at`, elementID, nextVersion, encodedSnapshot, item.ChangeSummary, actor).Scan(&item.ID, &item.CreatedAt); err != nil {
		return model.PageElementVersion{}, err
	}
	if _, err = tx.ExecContext(ctx, `insert into operation_logs(actor,action,target) values($1,'element_capture.rollback',$2)`, actor, fmt.Sprintf("%d:%d", elementID, version)); err != nil {
		return model.PageElementVersion{}, err
	}
	if err = tx.Commit(); err != nil {
		return model.PageElementVersion{}, err
	}
	return item, nil
}

type candidateLocator struct {
	Type   string  `json:"type"`
	Value  string  `json:"value"`
	Index  string  `json:"index"`
	Score  float64 `json:"score"`
	Unique bool    `json:"unique"`
}

func newCandidateID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func (r *ElementCaptureRepository) AddCandidate(ctx context.Context, candidate model.ElementCaptureCandidate, executorID, tokenHash string) (model.ElementCaptureCandidate, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return candidate, err
	}
	defer func() { _ = tx.Rollback() }()
	var pageID int64
	var count int
	err = tx.QueryRowContext(ctx, `
		select page_id, candidate_count from element_capture_sessions
		where id = $1 and executor_id = $2 and token_hash = $3 and status = 'active' and expires_at > now()
		for update
	`, candidate.SessionID, executorID, tokenHash).Scan(&pageID, &count)
	if errors.Is(err, sql.ErrNoRows) {
		return candidate, model.NewDomainError(model.ErrUnauthorized, "执行器或会话令牌无效")
	}
	if err != nil {
		return candidate, err
	}
	if count >= 500 {
		return candidate, model.NewDomainError(model.ErrConflict, "单个采集会话最多 500 个候选项")
	}
	var duplicateID sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		select id from page_elements where page_id = $1 and fingerprint = $2 and deleted_at is null order by id limit 1
	`, pageID, candidate.Fingerprint).Scan(&duplicateID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return candidate, err
	}
	if duplicateID.Valid {
		candidate.DuplicateElementID = duplicateID.Int64
		candidate.ConflictStatus = "duplicate"
	}
	candidate.ID, err = newCandidateID()
	if err != nil {
		return candidate, errors.New("生成候选项 ID 失败")
	}
	candidate.ExpiresAt = time.Now().Add(30 * time.Minute)
	if err = tx.QueryRowContext(ctx, `
		insert into element_capture_candidates(id,session_id,name,fingerprint,capture_url,tag_name,accessible_name,locators,quality_score,duplicate_element_id,conflict_status,conflict_resolution,status,expires_at)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9,nullif($10,0),$11,$12,'pending',$13)
		returning cursor_id
	`, candidate.ID, candidate.SessionID, candidate.Name, candidate.Fingerprint, candidate.CaptureURL, candidate.TagName, candidate.AccessibleName, candidate.Locators, candidate.QualityScore, candidate.DuplicateElementID, candidate.ConflictStatus, candidate.ConflictResolution, candidate.ExpiresAt).Scan(&candidate.CursorID); err != nil {
		return candidate, err
	}
	count++
	if _, err = tx.ExecContext(ctx, `update element_capture_sessions set candidate_count=$1,updated_at=now() where id=$2`, count, candidate.SessionID); err != nil {
		return candidate, err
	}
	if err = tx.Commit(); err != nil {
		return candidate, err
	}
	candidate.CandidateCount = count
	if count >= 400 {
		candidate.Warning = "候选项数量已达到 400，请及时审核"
	}
	return candidate, nil
}

func (r *ElementCaptureRepository) ListCandidates(ctx context.Context, userID int64, sessionID string, afterID int64, limit int) ([]model.ElementCaptureCandidate, error) {
	var accessible bool
	if err := r.db.QueryRowContext(ctx, `select exists(select 1 from element_capture_sessions s where s.id=$1 and (s.created_by=(select username from users where id=$2 and deleted_at is null) or exists(select 1 from users u join roles ro on ro.id=u.role_id where u.id=$2 and u.deleted_at is null and ro.code='admin')))`, sessionID, userID).Scan(&accessible); err != nil {
		return nil, err
	}
	if !accessible {
		return nil, model.NewDomainError(model.ErrNotFound, "采集会话不存在")
	}
	rows, err := r.db.QueryContext(ctx, `
		select c.id,c.cursor_id,c.session_id,c.name,c.fingerprint,c.capture_url,c.tag_name,c.accessible_name,c.locators,c.quality_score,
		       coalesce(c.duplicate_element_id,0),c.conflict_status,c.conflict_resolution,c.status,c.expires_at
		from element_capture_candidates c join element_capture_sessions s on s.id=c.session_id
		where c.session_id=$1 and c.cursor_id>$3 and (
			s.created_by=(select username from users where id=$2 and deleted_at is null)
			or exists(select 1 from users u join roles ro on ro.id=u.role_id where u.id=$2 and u.deleted_at is null and ro.code='admin')
		) order by c.cursor_id asc limit $4
	`, sessionID, userID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.ElementCaptureCandidate, 0)
	for rows.Next() {
		var item model.ElementCaptureCandidate
		if err := rows.Scan(&item.ID, &item.CursorID, &item.SessionID, &item.Name, &item.Fingerprint, &item.CaptureURL, &item.TagName, &item.AccessibleName, &item.Locators, &item.QualityScore, &item.DuplicateElementID, &item.ConflictStatus, &item.ConflictResolution, &item.Status, &item.ExpiresAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *ElementCaptureRepository) UpdateCandidate(ctx context.Context, actor, sessionID string, candidateID int64, req model.CaptureCandidateUpdateRequest) (bool, error) {
	result, err := r.db.ExecContext(ctx, `
		update element_capture_candidates c set
		name=case when $1='' then c.name else $1 end,
		locators=coalesce($2,c.locators),
		quality_score=coalesce($3,c.quality_score),
		conflict_resolution=case when $4='' then c.conflict_resolution else $4 end,
		updated_at=now()
		from element_capture_sessions s where c.session_id=s.id and c.cursor_id=$5 and c.session_id=$6 and (
			s.created_by=$7 or exists(select 1 from users u join roles ro on ro.id=u.role_id where u.username=$7 and u.deleted_at is null and ro.code='admin')
		)
		and s.status in ('active','completed') and c.status='pending'
		and not exists(select 1 from page_elements p where p.page_id=s.page_id and p.deleted_at is null and lower(p.name)=lower($1) and p.id <> coalesce(c.duplicate_element_id,0))
	`, strings.TrimSpace(req.Name), req.Locators, req.QualityScore, req.ConflictResolution, candidateID, sessionID, actor)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_page_elements_active_name" {
			return false, model.NewDomainError(model.ErrConflict, "页面元素名称冲突")
		}
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (r *ElementCaptureRepository) GetBatchSaveData(ctx context.Context, actor, sessionID string, _ []int64) (model.CaptureBatchData, error) {
	var data model.CaptureBatchData
	err := r.db.QueryRowContext(ctx, `
		select id,page_id,status from element_capture_sessions where id=$1 and (
			created_by=$2 or exists(select 1 from users u join roles ro on ro.id=u.role_id where u.username=$2 and u.deleted_at is null and ro.code='admin')
		)
	`, sessionID, actor).Scan(&data.Session.ID, &data.Session.PageID, &data.Session.Status)
	if err != nil {
		return data, err
	}
	rows, err := r.db.QueryContext(ctx, `
		select c.id,c.cursor_id,c.session_id,c.name,c.fingerprint,c.capture_url,c.tag_name,c.accessible_name,c.locators,c.quality_score,coalesce(c.duplicate_element_id,0),coalesce(p.page_id,0),c.conflict_status,c.conflict_resolution,c.status,c.expires_at
		from element_capture_candidates c left join page_elements p on p.id=c.duplicate_element_id and p.deleted_at is null where c.session_id=$1 order by c.cursor_id
	`, sessionID)
	if err != nil {
		return data, err
	}
	defer rows.Close()
	for rows.Next() {
		var candidate model.ElementCaptureCandidate
		if err := rows.Scan(&candidate.ID, &candidate.CursorID, &candidate.SessionID, &candidate.Name, &candidate.Fingerprint, &candidate.CaptureURL, &candidate.TagName, &candidate.AccessibleName, &candidate.Locators, &candidate.QualityScore, &candidate.DuplicateElementID, &candidate.DuplicateElementPageID, &candidate.ConflictStatus, &candidate.ConflictResolution, &candidate.Status, &candidate.ExpiresAt); err != nil {
			return data, err
		}
		data.Candidates = append(data.Candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return data, err
	}
	nameRows, err := r.db.QueryContext(ctx, `select id,lower(name),fingerprint from page_elements where page_id=$1 and deleted_at is null`, data.Session.PageID)
	if err != nil {
		return data, err
	}
	defer nameRows.Close()
	data.ExistingNames = map[string][]int64{}
	data.ExistingFingerprints = map[string][]int64{}
	for nameRows.Next() {
		var id int64
		var name, fingerprint string
		if err := nameRows.Scan(&id, &name, &fingerprint); err != nil {
			return data, err
		}
		data.ExistingNames[name] = append(data.ExistingNames[name], id)
		if fingerprint != "" {
			data.ExistingFingerprints[fingerprint] = append(data.ExistingFingerprints[fingerprint], id)
		}
	}
	return data, nameRows.Err()
}

func (r *ElementCaptureRepository) SaveCandidates(ctx context.Context, actor string, req model.CandidateBatchSaveRequest, validate func(model.CaptureBatchData) error) (model.BatchSaveResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.BatchSaveResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var pageID int64
	var sessionStatus string
	if err = tx.QueryRowContext(ctx, `
		select page_id,status from element_capture_sessions where id=$1 and status in ('active','completed') and (
			created_by=$2 or exists(select 1 from users u join roles ro on ro.id=u.role_id where u.username=$2 and u.deleted_at is null and ro.code='admin')
		) for update
	`, req.SessionID, actor).Scan(&pageID, &sessionStatus); err != nil {
		return model.BatchSaveResult{}, err
	}
	var lockedPageID int64
	if err = tx.QueryRowContext(ctx, `select id from ui_assets where id=$1 and deleted_at is null for update`, pageID).Scan(&lockedPageID); err != nil {
		return model.BatchSaveResult{}, err
	}
	currentRows, err := tx.QueryContext(ctx, `
		select c.id,c.cursor_id,c.session_id,c.name,c.fingerprint,c.capture_url,c.tag_name,c.accessible_name,c.locators,c.quality_score,coalesce(c.duplicate_element_id,0),coalesce(p.page_id,0),c.conflict_status,c.conflict_resolution,c.status,c.expires_at
		from element_capture_candidates c left join page_elements p on p.id=c.duplicate_element_id and p.deleted_at is null
		where c.session_id=$1 order by c.cursor_id for update of c`, req.SessionID)
	if err != nil {
		return model.BatchSaveResult{}, err
	}
	locked := model.CaptureBatchData{Session: model.ElementCaptureSession{ID: req.SessionID, PageID: pageID, Status: sessionStatus}, ExistingNames: map[string][]int64{}, ExistingFingerprints: map[string][]int64{}}
	for currentRows.Next() {
		var candidate model.ElementCaptureCandidate
		if err = currentRows.Scan(&candidate.ID, &candidate.CursorID, &candidate.SessionID, &candidate.Name, &candidate.Fingerprint, &candidate.CaptureURL, &candidate.TagName, &candidate.AccessibleName, &candidate.Locators, &candidate.QualityScore, &candidate.DuplicateElementID, &candidate.DuplicateElementPageID, &candidate.ConflictStatus, &candidate.ConflictResolution, &candidate.Status, &candidate.ExpiresAt); err != nil {
			currentRows.Close()
			return model.BatchSaveResult{}, err
		}
		locked.Candidates = append(locked.Candidates, candidate)
	}
	if err = currentRows.Close(); err != nil {
		return model.BatchSaveResult{}, err
	}
	byCursor := make(map[int64]model.ElementCaptureCandidate, len(locked.Candidates))
	for _, candidate := range locked.Candidates {
		byCursor[candidate.CursorID] = candidate
	}
	selected := make([]model.ElementCaptureCandidate, 0, len(req.Items))
	for _, item := range req.Items {
		candidate, ok := byCursor[item.CandidateID]
		if !ok {
			return model.BatchSaveResult{}, fmt.Errorf("候选项 %d 不存在或不属于该会话", item.CandidateID)
		}
		selected = append(selected, candidate)
	}
	locked.Candidates = selected
	nameRows, err := tx.QueryContext(ctx, `select id,lower(name),fingerprint from page_elements where page_id=$1 and deleted_at is null for update`, pageID)
	if err != nil {
		return model.BatchSaveResult{}, err
	}
	for nameRows.Next() {
		var id int64
		var name, fingerprint string
		if err = nameRows.Scan(&id, &name, &fingerprint); err != nil {
			nameRows.Close()
			return model.BatchSaveResult{}, err
		}
		locked.ExistingNames[name] = append(locked.ExistingNames[name], id)
		if fingerprint != "" {
			locked.ExistingFingerprints[fingerprint] = append(locked.ExistingFingerprints[fingerprint], id)
		}
	}
	if err = nameRows.Close(); err != nil {
		return model.BatchSaveResult{}, err
	}
	for _, item := range req.Items {
		if item.Resolution != "update" {
			continue
		}
		for candidateIndex := range locked.Candidates {
			candidate := &locked.Candidates[candidateIndex]
			if candidate.CursorID == item.CandidateID && item.TargetElementID != 0 {
				candidate.DuplicateElementID = item.TargetElementID
				candidate.DuplicateElementPageID = pageID
			} else if candidate.CursorID == item.CandidateID && len(locked.ExistingFingerprints[candidate.Fingerprint]) == 1 {
				candidate.DuplicateElementID = locked.ExistingFingerprints[candidate.Fingerprint][0]
				candidate.DuplicateElementPageID = pageID
			}
		}
	}
	if err = validate(locked); err != nil {
		return model.BatchSaveResult{}, err
	}
	requested := make(map[int64]string, len(req.Items))
	for _, item := range req.Items {
		requested[item.CandidateID] = item.Resolution
	}
	result := model.BatchSaveResult{}
	for _, candidate := range locked.Candidates {
		resolution, ok := requested[candidate.CursorID]
		if !ok {
			continue
		}
		if resolution == "ignore" {
			updated, updateErr := tx.ExecContext(ctx, `update element_capture_candidates set status='ignored',updated_at=now() where cursor_id=$1 and session_id=$2 and status='pending'`, candidate.CursorID, req.SessionID)
			if updateErr != nil {
				return result, updateErr
			}
			rows, updateErr := updated.RowsAffected()
			if updateErr != nil || rows != 1 {
				if updateErr != nil {
					return result, updateErr
				}
				return result, model.NewDomainError(model.ErrConflict, "候选项状态已被并发修改")
			}
			result.IgnoredCandidateIDs = append(result.IgnoredCandidateIDs, candidate.CursorID)
			continue
		}
		elementID, version, err := saveCapturedElement(ctx, tx, pageID, actor, candidate, resolution)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_page_elements_active_name" {
				return result, model.NewDomainError(model.ErrConflict, fmt.Sprintf("候选项 %d：页面元素名称冲突", candidate.CursorID))
			}
			return result, err
		}
		snapshot, err := capturedElementSnapshot(candidate, elementID, version)
		if err != nil {
			return result, err
		}
		if _, err = tx.ExecContext(ctx, `insert into page_element_versions(page_element_id,version,snapshot,change_summary,created_by) values($1,$2,$3,$4,$5)`, elementID, version, snapshot, "采集候选项审核入库", actor); err != nil {
			return result, err
		}
		updated, updateErr := tx.ExecContext(ctx, `update element_capture_candidates set status='saved',conflict_resolution=$1,updated_at=now() where cursor_id=$2 and session_id=$3 and status='pending'`, resolution, candidate.CursorID, req.SessionID)
		if updateErr != nil {
			return result, updateErr
		}
		rows, updateErr := updated.RowsAffected()
		if updateErr != nil || rows != 1 {
			if updateErr != nil {
				return result, updateErr
			}
			return result, model.NewDomainError(model.ErrConflict, "候选项状态已被并发修改")
		}
		result.SavedCandidateIDs = append(result.SavedCandidateIDs, candidate.CursorID)
	}
	if _, err = tx.ExecContext(ctx, `insert into operation_logs(actor,action,target) values($1,'element_capture.batch_save',$2)`, actor, req.SessionID); err != nil {
		return result, err
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func saveCapturedElement(ctx context.Context, tx *sql.Tx, pageID int64, actor string, candidate model.ElementCaptureCandidate, resolution string) (int64, int, error) {
	locators := make([]candidateLocator, 0)
	if err := json.Unmarshal(candidate.Locators, &locators); err != nil {
		return 0, 0, err
	}
	first := locators[0]
	var second, third candidateLocator
	if len(locators) > 1 {
		second = locators[1]
	}
	if len(locators) > 2 {
		third = locators[2]
	}
	if resolution == "update" {
		var id int64
		var currentVersion int
		err := tx.QueryRowContext(ctx, `
			update page_elements set name=$1,type1=$2,locator1=$3,index1=$4,type2=$5,locator2=$6,index2=$7,type3=$8,locator3=$9,index3=$10,fingerprint=$11,capture_source='element_capture',capture_url=$12,tag_name=$13,accessible_name=$14,quality_score=$15,captured_by=$16,captured_at=now(),current_version=current_version+1,updated_at=now()
			where id=$17 and page_id=$18 and deleted_at is null returning id,current_version
		`, candidate.Name, first.Type, first.Value, first.Index, second.Type, second.Value, second.Index, third.Type, third.Value, third.Index, candidate.Fingerprint, candidate.CaptureURL, candidate.TagName, candidate.AccessibleName, candidate.QualityScore, actor, candidate.DuplicateElementID, pageID).Scan(&id, &currentVersion)
		return id, currentVersion, err
	}
	var id int64
	err := tx.QueryRowContext(ctx, `
		insert into page_elements(page_id,name,type1,locator1,index1,type2,locator2,index2,type3,locator3,index3,fingerprint,capture_source,capture_url,tag_name,accessible_name,quality_score,captured_by,captured_at,current_version)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'element_capture',$13,$14,$15,$16,$17,now(),1) returning id
	`, pageID, candidate.Name, first.Type, first.Value, first.Index, second.Type, second.Value, second.Index, third.Type, third.Value, third.Index, candidate.Fingerprint, candidate.CaptureURL, candidate.TagName, candidate.AccessibleName, candidate.QualityScore, actor).Scan(&id)
	return id, 1, err
}

func capturedElementSnapshot(candidate model.ElementCaptureCandidate, elementID int64, version int) ([]byte, error) {
	return json.Marshal(map[string]any{"id": elementID, "version": version, "name": candidate.Name, "fingerprint": candidate.Fingerprint, "captureUrl": candidate.CaptureURL, "tagName": candidate.TagName, "accessibleName": candidate.AccessibleName, "locators": json.RawMessage(candidate.Locators), "qualityScore": candidate.QualityScore})
}
