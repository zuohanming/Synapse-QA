package repository

import (
	"context"
	"database/sql"

	"synapseqa/backend/internal/model"
)

type NotificationRepository struct{ db *sql.DB }

func NewNotificationRepository(db *sql.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) Create(ctx context.Context, req model.NotificationCreate) (model.Notification, error) {
	row := r.db.QueryRowContext(ctx, `insert into notifications(user_id,type,level,title,content,target_type,target_id,target_url)
		select u.id,$2,$3,$4,$5,$6,$7,$8 from users u
		left join notification_preferences p on p.user_id=u.id
		left join system_setting_groups s on s.group_key='notification'
		where u.username=$1 and
		(case
		 when $2='execution.success' then case when coalesce(p.use_system_defaults,true) then coalesce((s.value->>'executionSuccess')::boolean,true) else p.execution_success end
		 when $2='execution.failure' then case when coalesce(p.use_system_defaults,true) then coalesce((s.value->>'executionFailure')::boolean,true) else p.execution_failure end
		 when $2 like 'executor.%' then case when coalesce(p.use_system_defaults,true) then coalesce((s.value->>'executorOffline')::boolean,true) else p.executor_alert end
		 when $2='system' then true
		 else true end)
		on conflict (user_id,type,target_type,target_id) where type='perf.degradation' do nothing
		returning id,user_id,type,level,title,content,target_type,target_id,target_url,is_read,read_at,created_at`, req.Username, req.Type, req.Level, req.Title, req.Content, req.TargetType, req.TargetID, req.TargetURL)
	notification, err := scanNotification(row)
	if err == sql.ErrNoRows && req.Type == "perf.degradation" {
		return model.Notification{}, nil
	}
	return notification, err
}

func (r *NotificationRepository) CreateForAll(ctx context.Context, req model.NotificationCreate) error {
	_, err := r.db.ExecContext(ctx, `insert into notifications(user_id,type,level,title,content,target_type,target_id,target_url)
		select u.id,$1,$2,$3,$4,$5,$6,$7 from users u
		left join notification_preferences p on p.user_id=u.id
		left join system_setting_groups s on s.group_key='notification'
		where u.status='active' and
		(case when $1 like 'executor.%' then case when coalesce(p.use_system_defaults,true) then coalesce((s.value->>'executorOffline')::boolean,true) else p.executor_alert end when $1='system' then true else true end)`, req.Type, req.Level, req.Title, req.Content, req.TargetType, req.TargetID, req.TargetURL)
	return err
}

func (r *NotificationRepository) List(ctx context.Context, userID int64, unreadOnly bool, category string, page, size int) ([]model.Notification, int64, error) {
	rows, err := r.db.QueryContext(ctx, `select id,user_id,type,level,title,content,target_type,target_id,target_url,is_read,read_at,created_at from notifications
		where user_id=$1 and ($2=false or is_read=false) and ($3='' or type like $3 || '%') order by created_at desc limit $4 offset $5`, userID, unreadOnly, category, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []model.Notification{}
	for rows.Next() {
		item, err := scanNotification(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	var total int64
	err = r.db.QueryRowContext(ctx, `select count(*) from notifications where user_id=$1 and ($2=false or is_read=false) and ($3='' or type like $3 || '%')`, userID, unreadOnly, category).Scan(&total)
	return items, total, err
}

func (r *NotificationRepository) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	var n int64
	err := r.db.QueryRowContext(ctx, `select count(*) from notifications where user_id=$1 and is_read=false`, userID).Scan(&n)
	return n, err
}
func (r *NotificationRepository) MarkRead(ctx context.Context, userID, id int64) error {
	_, err := r.db.ExecContext(ctx, `update notifications set is_read=true,read_at=now() where id=$1 and user_id=$2`, id, userID)
	return err
}
func (r *NotificationRepository) MarkAllRead(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `update notifications set is_read=true,read_at=now() where user_id=$1 and is_read=false`, userID)
	return err
}
func (r *NotificationRepository) Delete(ctx context.Context, userID, id int64) error {
	_, err := r.db.ExecContext(ctx, `delete from notifications where id=$1 and user_id=$2`, id, userID)
	return err
}

func (r *NotificationRepository) GetPreferences(ctx context.Context, userID int64) (model.NotificationPreference, error) {
	var p model.NotificationPreference
	err := r.db.QueryRowContext(ctx, `
		with pref as (
			insert into notification_preferences(user_id) values($1)
			on conflict(user_id) do update set user_id=excluded.user_id
			returning use_system_defaults,execution_success,execution_failure,executor_alert,system_notice
		)
		select pref.use_system_defaults,
		  case when pref.use_system_defaults then coalesce((s.value->>'executionSuccess')::boolean,true) else pref.execution_success end,
		  case when pref.use_system_defaults then coalesce((s.value->>'executionFailure')::boolean,true) else pref.execution_failure end,
		  case when pref.use_system_defaults then coalesce((s.value->>'executorOffline')::boolean,true) else pref.executor_alert end,
		  case when pref.use_system_defaults then true else pref.system_notice end
		from pref left join system_setting_groups s on s.group_key='notification'
	`, userID).Scan(&p.UseSystemDefaults, &p.ExecutionSuccess, &p.ExecutionFailure, &p.ExecutorAlert, &p.SystemNotice)
	return p, err
}
func (r *NotificationRepository) UpdatePreferences(ctx context.Context, userID int64, p model.NotificationPreference) error {
	_, err := r.db.ExecContext(ctx, `insert into notification_preferences(user_id,use_system_defaults,execution_success,execution_failure,executor_alert,system_notice) values($1,$2,$3,$4,$5,$6) on conflict(user_id) do update set use_system_defaults=$2,execution_success=$3,execution_failure=$4,executor_alert=$5,system_notice=$6,updated_at=now()`, userID, p.UseSystemDefaults, p.ExecutionSuccess, p.ExecutionFailure, p.ExecutorAlert, p.SystemNotice)
	return err
}

type notificationScanner interface{ Scan(...any) error }

func scanNotification(s notificationScanner) (model.Notification, error) {
	var n model.Notification
	err := s.Scan(&n.ID, &n.UserID, &n.Type, &n.Level, &n.Title, &n.Content, &n.TargetType, &n.TargetID, &n.TargetURL, &n.IsRead, &n.ReadAt, &n.CreatedAt)
	return n, err
}
