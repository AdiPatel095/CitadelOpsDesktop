package Reports

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const battleResearchTrialLoadLimit = 500

func (store *SQLiteStore) initializeBattleResearch(ctx context.Context) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("report analytics database is unavailable")
	}
	if _, err := store.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS battle_research_trials (
			trial_id TEXT PRIMARY KEY,
			account_uid INTEGER NOT NULL DEFAULT 0,
			world_id TEXT NOT NULL DEFAULT '',
			player_id INTEGER NOT NULL DEFAULT 0,
			movement_id INTEGER NOT NULL DEFAULT 0,
			phase TEXT NOT NULL,
			upload_state TEXT NOT NULL DEFAULT '',
			trial_json BLOB NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS battle_research_trials_updated
			ON battle_research_trials(updated_at DESC, trial_id);
		CREATE INDEX IF NOT EXISTS battle_research_trials_upload
			ON battle_research_trials(upload_state, updated_at, trial_id);
		CREATE UNIQUE INDEX IF NOT EXISTS battle_research_trials_movement
			ON battle_research_trials(account_uid, world_id, player_id, movement_id)
			WHERE movement_id > 0;
	`); err != nil {
		return fmt.Errorf("initialize battle research trials: %w", err)
	}
	return nil
}

func (store *SQLiteStore) SaveBattleResearchTrial(ctx context.Context, trial BattleResearchTrial) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("report analytics database is unavailable")
	}
	if trial.ID == "" || trial.Version <= 0 || trial.Phase == "" {
		return fmt.Errorf("battle research trial identity, version, and phase are required")
	}
	if trial.CreatedAt.IsZero() {
		return fmt.Errorf("battle research trial creation time is required")
	}
	if trial.UpdatedAt.IsZero() {
		trial.UpdatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(trial)
	if err != nil {
		return fmt.Errorf("encode battle research trial: %w", err)
	}
	movementID := int64(0)
	if trial.Movement != nil {
		movementID = int64(trial.Movement.ID)
	}
	_, err = store.db.ExecContext(ctx, `
		INSERT INTO battle_research_trials (
			trial_id, account_uid, world_id, player_id, movement_id, phase,
			upload_state, trial_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(trial_id) DO UPDATE SET
			account_uid=excluded.account_uid,
			world_id=excluded.world_id,
			player_id=excluded.player_id,
			movement_id=excluded.movement_id,
			phase=excluded.phase,
			upload_state=excluded.upload_state,
			trial_json=excluded.trial_json,
			updated_at=excluded.updated_at`,
		trial.ID, trial.AccountUID, trial.WorldID, int64(trial.PlayerID), movementID,
		trial.Phase, trial.UploadState, payload, trial.CreatedAt.UTC().Format(time.RFC3339Nano),
		trial.UpdatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		store.setLastError(err)
		return fmt.Errorf("save battle research trial: %w", err)
	}
	store.setLastError(nil)
	return nil
}

func (store *SQLiteStore) ListBattleResearchTrials(ctx context.Context, limit int) ([]BattleResearchTrial, error) {
	if store == nil || store.db == nil {
		return nil, fmt.Errorf("report analytics database is unavailable")
	}
	if limit <= 0 || limit > battleResearchTrialLoadLimit {
		limit = battleResearchTrialLoadLimit
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT trial_json
		FROM battle_research_trials
		ORDER BY updated_at DESC, trial_id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list battle research trials: %w", err)
	}
	defer rows.Close()
	trials := make([]BattleResearchTrial, 0, limit)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var trial BattleResearchTrial
		if json.Unmarshal(payload, &trial) == nil && trial.ID != "" {
			trials = append(trials, trial)
		}
	}
	return trials, rows.Err()
}
