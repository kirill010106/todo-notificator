package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/kirill010106/todo-notificator/internal/domain"
)

type transactionKey struct{}
type outcomeKey struct{}
type userTransaction struct {
	tx     *sql.Tx
	userID int64
}

func (s *Storage) executor(ctx context.Context) sqlExecutor {
	if current, ok := ctx.Value(transactionKey{}).(userTransaction); ok {
		return current.tx
	}
	return s.DB
}
func actionOutcome(ctx context.Context) *domain.ActionOutcome {
	value, _ := ctx.Value(outcomeKey{}).(*domain.ActionOutcome)
	return value
}
func (s *Storage) withReadSnapshot(ctx context.Context, userID int64, read func(context.Context) error) error {
	if _, ok := ctx.Value(transactionKey{}).(userTransaction); ok {
		return read(ctx)
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := read(context.WithValue(ctx, transactionKey{}, userTransaction{tx, userID})); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Storage) ExecuteOperation(ctx context.Context, userID int64, key, route string, body []byte, invoke func(context.Context) domain.OperationResponse) (domain.OperationResponse, error) {
	var response domain.OperationResponse
	var callbacks []func()
	var decoded any
	canonical := body
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if json.Valid(body) && decoder.Decode(&decoded) == nil {
		canonical, _ = json.Marshal(decoded)
	}
	digest := sha256.Sum256(append([]byte(route+"\n"), canonical...))
	fingerprint := hex.EncodeToString(digest[:])
	failed := errors.New("handler rejected operation")
	err := s.withUserTx(ctx, userID, func(tx *sql.Tx) error {
		if key != "" {
			var storedFingerprint string
			err := tx.QueryRowContext(ctx, `SELECT fingerprint, response, status_code FROM api_operations WHERE user_id=$1 AND idempotency_key=$2`, userID, key).Scan(&storedFingerprint, &response.Body, &response.Status)
			if err == nil {
				if storedFingerprint != fingerprint {
					return &domain.ContractError{Code: "IDEMPOTENCY_KEY_REUSED", Message: "Ключ уже использован для другого запроса", Status: 409}
				}
				return nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		if err := ensureStatsTx(ctx, tx, userID); err != nil {
			return err
		}
		txctx := context.WithValue(ctx, transactionKey{}, userTransaction{tx, userID})
		txctx = domain.WithAfterCommit(txctx, &callbacks)
		outcome := &domain.ActionOutcome{Result: domain.ActionResult{ChangedTaskIDs: []int64{}, SkippedTaskIDs: []int64{}}}
		txctx = context.WithValue(txctx, outcomeKey{}, outcome)
		before, err := s.GetUserStats(txctx, userID)
		if err != nil {
			return err
		}
		previous, err := achievementCodes(txctx, tx, userID)
		if err != nil {
			return err
		}
		response = invoke(txctx)
		if response.Status >= 400 {
			return failed
		}
		after, err := s.GetUserStats(txctx, userID)
		if err != nil {
			return err
		}
		current, err := achievementCodes(txctx, tx, userID)
		if err != nil {
			return err
		}
		unlocked := []domain.Achievement{}
		for _, definition := range domain.AvailableAchievements {
			if current[definition.Code] && !previous[definition.Code] {
				definition.Unlocked = true
				unlocked = append(unlocked, definition)
				outcome.Rewards.AchievementXP += definition.Points
			}
		}
		if before.Points != nil && after.Points != nil && outcome.Rewards.PenaltyXP > 0 {
			outcome.Rewards.PenaltyXP = int(*before.Points-*after.Points) + outcome.Rewards.AchievementXP
		}
		payload := map[string]any{}
		if len(response.Body) > 0 {
			decoder := json.NewDecoder(bytes.NewReader(response.Body))
			decoder.UseNumber()
			if err := decoder.Decode(&payload); err != nil {
				return err
			}
		}
		payload["operation_id"] = uuid.NewString()
		payload["result"] = outcome.Result
		payload["rewards"] = outcome.Rewards
		payload["stats"] = after
		payload["unlocked_achievements"] = unlocked
		focus, err := s.GetFocusState(txctx, userID)
		if err != nil {
			return err
		}
		payload["focus"] = focus
		if len(outcome.Result.ChangedTaskIDs) == 1 {
			task, err := s.GetTask(txctx, userID, outcome.Result.ChangedTaskIDs[0])
			if err == nil {
				payload["task"] = task
			}
		}
		response.Body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
		if key != "" {
			_, err = tx.ExecContext(txctx, `INSERT INTO api_operations(user_id,idempotency_key,fingerprint,response,status_code) VALUES($1,$2,$3,$4,$5)`, userID, key, fingerprint, string(response.Body), response.Status)
		}
		return err
	})
	if errors.Is(err, failed) {
		return response, nil
	}
	if err == nil {
		for _, callback := range callbacks {
			callback()
		}
	}
	return response, err
}
func achievementCodes(ctx context.Context, tx *sql.Tx, userID int64) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT code FROM achievements WHERE user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]bool{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		result[code] = true
	}
	return result, rows.Err()
}
