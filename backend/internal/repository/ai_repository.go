package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"synapseqa/backend/internal/model"
)

type AIRepository struct {
	db *sql.DB
}

func NewAIRepository(db *sql.DB) *AIRepository {
	return &AIRepository{db: db}
}

func aiConfigKey(userID int64) string {
	return fmt.Sprintf("ai.config.%d", userID)
}

func (r *AIRepository) GetAIConfig(ctx context.Context, userID int64) (*model.AIConfig, error) {
	var raw string
	err := r.db.QueryRowContext(ctx, `select value from platform_settings where key = $1`, aiConfigKey(userID)).Scan(&raw)
	if err == sql.ErrNoRows || raw == "" {
		return model.DefaultAIConfig(), nil
	}
	if err != nil {
		return model.DefaultAIConfig(), nil
	}
	cfg := model.DefaultAIConfig()
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return model.DefaultAIConfig(), nil
	}
	return cfg, nil
}

func (r *AIRepository) SaveAIConfig(ctx context.Context, userID int64, value string) error {
	_, err := r.db.ExecContext(ctx,
		`insert into platform_settings(key, value) values($1, $2) on conflict(key) do update set value = excluded.value`,
		aiConfigKey(userID), value)
	return err
}
