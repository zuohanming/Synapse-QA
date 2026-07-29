package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
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

type candidateLocator struct {
	Type  string `json:"type"`
	Value string `json:"value"`
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
		return candidate, errors.New("采集会话不存在、执行器或令牌无效，或会话未激活")
	}
	if err != nil {
		return candidate, err
	}
	if count >= 500 {
		return candidate, errors.New("单个采集会话最多 500 个候选项")
	}
	var duplicateID sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		select id from page_elements where page_id = $1 and fingerprint = $2 and deleted_at is null limit 1
	`, pageID, candidate.Fingerprint).Scan(&duplicateID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return candidate, err
	}
	if duplicateID.Valid {
		candidate.DuplicateElementID = duplicateID.Int64
		candidate.ConflictStatus = "duplicate"
	}
	candidate.ID = time.Now().UnixNano()
	candidate.ExpiresAt = time.Now().Add(30 * time.Minute)
	if _, err = tx.ExecContext(ctx, `
		insert into element_capture_candidates(id,session_id,name,fingerprint,capture_url,tag_name,accessible_name,locators,quality_score,duplicate_element_id,conflict_status,conflict_resolution,status,expires_at)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9,nullif($10,0),$11,$12,'pending',$13)
	`, strconv.FormatInt(candidate.ID, 10), candidate.SessionID, candidate.Name, candidate.Fingerprint, candidate.CaptureURL, candidate.TagName, candidate.AccessibleName, candidate.Locators, candidate.QualityScore, candidate.DuplicateElementID, candidate.ConflictStatus, candidate.ConflictResolution, candidate.ExpiresAt); err != nil {
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
	rows, err := r.db.QueryContext(ctx, `
		select c.id::bigint,c.session_id,c.name,c.fingerprint,c.capture_url,c.tag_name,c.accessible_name,c.locators,c.quality_score,
		       coalesce(c.duplicate_element_id,0),c.conflict_status,c.conflict_resolution,c.status,c.expires_at
		from element_capture_candidates c join element_capture_sessions s on s.id=c.session_id join users u on u.username=s.created_by
		where c.session_id=$1 and u.id=$2 and c.id::bigint>$3 order by c.id::bigint asc limit $4
	`, sessionID, userID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.ElementCaptureCandidate, 0)
	for rows.Next() {
		var item model.ElementCaptureCandidate
		if err := rows.Scan(&item.ID, &item.SessionID, &item.Name, &item.Fingerprint, &item.CaptureURL, &item.TagName, &item.AccessibleName, &item.Locators, &item.QualityScore, &item.DuplicateElementID, &item.ConflictStatus, &item.ConflictResolution, &item.Status, &item.ExpiresAt); err != nil {
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
		quality_score=case when $3=0 then c.quality_score else $3 end,
		conflict_resolution=case when $4='' then c.conflict_resolution else $4 end,
		updated_at=now()
		from element_capture_sessions s where c.session_id=s.id and c.id::bigint=$5 and c.session_id=$6 and s.created_by=$7
		and s.status in ('active','completed') and c.status='pending'
	`, strings.TrimSpace(req.Name), req.Locators, req.QualityScore, req.ConflictResolution, candidateID, sessionID, actor)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (r *ElementCaptureRepository) GetBatchSaveData(ctx context.Context, sessionID string, _ []int64) (model.CaptureBatchData, error) {
	var data model.CaptureBatchData
	err := r.db.QueryRowContext(ctx, `select id,page_id,status from element_capture_sessions where id=$1`, sessionID).Scan(&data.Session.ID, &data.Session.PageID, &data.Session.Status)
	if err != nil {
		return data, err
	}
	rows, err := r.db.QueryContext(ctx, `
		select id::bigint,session_id,name,fingerprint,capture_url,tag_name,accessible_name,locators,quality_score,coalesce(duplicate_element_id,0),conflict_status,conflict_resolution,status,expires_at
		from element_capture_candidates where session_id=$1 order by id::bigint
	`, sessionID)
	if err != nil {
		return data, err
	}
	defer rows.Close()
	for rows.Next() {
		var candidate model.ElementCaptureCandidate
		if err := rows.Scan(&candidate.ID, &candidate.SessionID, &candidate.Name, &candidate.Fingerprint, &candidate.CaptureURL, &candidate.TagName, &candidate.AccessibleName, &candidate.Locators, &candidate.QualityScore, &candidate.DuplicateElementID, &candidate.ConflictStatus, &candidate.ConflictResolution, &candidate.Status, &candidate.ExpiresAt); err != nil {
			return data, err
		}
		data.Candidates = append(data.Candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return data, err
	}
	nameRows, err := r.db.QueryContext(ctx, `select lower(name) from page_elements where page_id=$1 and deleted_at is null`, data.Session.PageID)
	if err != nil {
		return data, err
	}
	defer nameRows.Close()
	data.ExistingNames = map[string]bool{}
	for nameRows.Next() {
		var name string
		if err := nameRows.Scan(&name); err != nil {
			return data, err
		}
		data.ExistingNames[name] = true
	}
	return data, nameRows.Err()
}

func (r *ElementCaptureRepository) SaveCandidates(ctx context.Context, actor string, req model.CandidateBatchSaveRequest, candidates []model.ElementCaptureCandidate) (model.BatchSaveResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.BatchSaveResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var pageID int64
	if err = tx.QueryRowContext(ctx, `select page_id from element_capture_sessions where id=$1 and status in ('active','completed') for update`, req.SessionID).Scan(&pageID); err != nil {
		return model.BatchSaveResult{}, err
	}
	lockedCandidates, err := tx.QueryContext(ctx, `select id from element_capture_candidates where session_id=$1 for update`, req.SessionID)
	if err != nil {
		return model.BatchSaveResult{}, err
	}
	if err = lockedCandidates.Close(); err != nil {
		return model.BatchSaveResult{}, err
	}
	requested := make(map[int64]string, len(req.Items))
	for _, item := range req.Items {
		requested[item.CandidateID] = item.Resolution
	}
	result := model.BatchSaveResult{}
	for _, candidate := range candidates {
		resolution, ok := requested[candidate.ID]
		if !ok {
			continue
		}
		if resolution == "ignore" {
			if _, err = tx.ExecContext(ctx, `update element_capture_candidates set status='ignored',updated_at=now() where id::bigint=$1 and session_id=$2`, candidate.ID, req.SessionID); err != nil {
				return result, err
			}
			result.IgnoredCandidateIDs = append(result.IgnoredCandidateIDs, candidate.ID)
			continue
		}
		elementID, version, err := saveCapturedElement(ctx, tx, pageID, actor, candidate, resolution)
		if err != nil {
			return result, err
		}
		snapshot, err := capturedElementSnapshot(candidate, elementID, version)
		if err != nil {
			return result, err
		}
		if _, err = tx.ExecContext(ctx, `insert into page_element_versions(page_element_id,version,snapshot,change_summary,created_by) values($1,$2,$3,$4,$5)`, elementID, version, snapshot, "采集候选项审核入库", actor); err != nil {
			return result, err
		}
		if _, err = tx.ExecContext(ctx, `update element_capture_candidates set status='saved',conflict_resolution=$1,updated_at=now() where id::bigint=$2 and session_id=$3`, resolution, candidate.ID, req.SessionID); err != nil {
			return result, err
		}
		result.SavedCandidateIDs = append(result.SavedCandidateIDs, candidate.ID)
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
	if resolution == "update" {
		var id int64
		var currentVersion int
		err := tx.QueryRowContext(ctx, `
			update page_elements set name=$1,type1=$2,locator1=$3,fingerprint=$4,capture_source='element_capture',capture_url=$5,tag_name=$6,accessible_name=$7,quality_score=$8,captured_by=$9,captured_at=now(),current_version=current_version+1,updated_at=now()
			where id=$10 and page_id=$11 and deleted_at is null returning id,current_version
		`, candidate.Name, first.Type, first.Value, candidate.Fingerprint, candidate.CaptureURL, candidate.TagName, candidate.AccessibleName, candidate.QualityScore, actor, candidate.DuplicateElementID, pageID).Scan(&id, &currentVersion)
		return id, currentVersion, err
	}
	var id int64
	err := tx.QueryRowContext(ctx, `
		insert into page_elements(page_id,name,type1,locator1,fingerprint,capture_source,capture_url,tag_name,accessible_name,quality_score,captured_by,captured_at,current_version)
		values($1,$2,$3,$4,$5,'element_capture',$6,$7,$8,$9,$10,now(),1) returning id
	`, pageID, candidate.Name, first.Type, first.Value, candidate.Fingerprint, candidate.CaptureURL, candidate.TagName, candidate.AccessibleName, candidate.QualityScore, actor).Scan(&id)
	return id, 1, err
}

func capturedElementSnapshot(candidate model.ElementCaptureCandidate, elementID int64, version int) ([]byte, error) {
	return json.Marshal(map[string]any{"id": elementID, "version": version, "name": candidate.Name, "fingerprint": candidate.Fingerprint, "captureUrl": candidate.CaptureURL, "tagName": candidate.TagName, "accessibleName": candidate.AccessibleName, "locators": json.RawMessage(candidate.Locators), "qualityScore": candidate.QualityScore})
}
