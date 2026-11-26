package directorymock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

var (
	errOperationAbsent   = errors.New("mock operation receipt absent")
	errOperationConflict = errors.New("mock operation content conflict")
	errScopeAbsent       = errors.New("mock operation scope absent")
)

type operationKey struct {
	scope string
	id    string
}

// recordOperation coordinates receipt writes and IAM facts using the same
// transaction. It resolves replays before examining current facts.
func recordOperation[T any](ctx context.Context, pool *pgxpool.Pool, key operationKey, input any,
	execute func(context.Context, repository.IAMDBTX, string) (T, error)) (T, error) {
	var zero T
	encoded, err := json.Marshal(input)
	if err != nil {
		return zero, fmt.Errorf("encode mock operation: %w", err)
	}
	digest := sha256.Sum256(encoded)
	hash := hex.EncodeToString(digest[:])
	return repository.WithIAMGuardValue(ctx, pool, func(ctx context.Context) (T, error) {
		db := repository.IAMDB(ctx, pool)
		stored, storedHash, err := readOperationReceipt[T](ctx, db, key)
		switch {
		case err == nil && storedHash == hash:
			return stored, nil
		case err == nil:
			return zero, errOperationConflict
		case !errors.Is(err, errOperationAbsent):
			return zero, err
		}

		var knownScope bool
		if err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM directory_mock_scopes WHERE scope_ref=$1)", key.scope).
			Scan(&knownScope); err != nil {
			return zero, fmt.Errorf("resolve mock scope: %w", err)
		}
		if !knownScope {
			return zero, errScopeAbsent
		}
		runID := uuid.NewString()
		receipt, err := execute(ctx, db, runID)
		if err != nil {
			return zero, err
		}
		if _, err := db.Exec(ctx, `INSERT INTO directory_mock_transfer_runs(scope_ref,operation_id,run_id,input_hash,receipt)
			VALUES ($1,$2,$3,$4,$5)`, key.scope, key.id, runID, hash, receipt); err != nil {
			return zero, fmt.Errorf("save mock operation receipt: %w", err)
		}
		return receipt, nil
	})
}

func readOperationReceipt[T any](ctx context.Context, db repository.IAMDBTX, key operationKey) (T, string, error) {
	var receipt, zero T
	var hash string
	err := db.QueryRow(ctx, `SELECT receipt,input_hash FROM directory_mock_transfer_runs WHERE scope_ref=$1 AND operation_id=$2`,
		key.scope, key.id).Scan(&receipt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, "", errOperationAbsent
	}
	if err != nil {
		return zero, "", fmt.Errorf("read mock operation receipt: %w", err)
	}
	return receipt, hash, nil
}
