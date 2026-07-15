package daemon

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/GoCodeAlone/workflow/secrets"
)

const providerCleanupWorkers = 2

func queueProviderSecretCleanupTx(ctx context.Context, tx *sql.Tx, secretName string) error {
	if secretName == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO provider_secret_cleanup
		(secret_name, attempt_count, failure, created_at, updated_at, next_attempt_at)
		VALUES (?, 0, '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(secret_name) DO NOTHING`, secretName)
	return err
}

func (m *providerOperationManager) WakeCleanup() {
	if m == nil {
		return
	}
	select {
	case m.cleanupWake <- struct{}{}:
	default:
	}
}

func (m *providerOperationManager) cleanupLoop() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.cleanupWake:
			m.dispatchCleanup()
		case <-ticker.C:
			m.dispatchCleanup()
		}
	}
}

func (m *providerOperationManager) dispatchCleanup() {
	m.cleanupMu.Lock()
	available := providerCleanupWorkers - len(m.cleaning)
	m.cleanupMu.Unlock()
	if available <= 0 {
		return
	}
	rows, err := m.engine.DB.QueryContext(m.ctx, `SELECT secret_name FROM provider_secret_cleanup
		WHERE unixepoch(next_attempt_at) <= unixepoch()
		ORDER BY unixepoch(next_attempt_at), unixepoch(created_at) LIMIT ?`, available*4)
	if err != nil {
		return
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Printf("provider cleanup: scan candidate: %v", err)
			_ = rows.Close()
			return
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		log.Printf("provider cleanup: iterate candidates: %v", err)
		_ = rows.Close()
		return
	}
	if err := rows.Close(); err != nil {
		log.Printf("provider cleanup: close candidates: %v", err)
		return
	}
	for _, name := range names {
		m.cleanupMu.Lock()
		if len(m.cleaning) >= providerCleanupWorkers {
			m.cleanupMu.Unlock()
			return
		}
		if m.cleaning[name] {
			m.cleanupMu.Unlock()
			continue
		}
		m.cleaning[name] = true
		m.cleanupMu.Unlock()
		m.background.Add(1)
		go func() {
			defer m.background.Done()
			m.cleanupSecret(name)
		}()
	}
}

func (m *providerOperationManager) cleanupSecret(secretName string) {
	defer func() {
		if recover() != nil {
			m.recordCleanupFailure(secretName, "internal")
		}
		m.cleanupMu.Lock()
		delete(m.cleaning, secretName)
		m.cleanupMu.Unlock()
		m.WakeCleanup()
	}()

	var references int
	err := m.engine.DB.QueryRowContext(m.ctx, `SELECT
		(SELECT count(*) FROM llm_providers WHERE secret_name = ?) +
		(SELECT count(*) FROM provider_operations WHERE secret_name = ? AND state IN (?, ?))`,
		secretName, secretName, providerOperationPending, providerOperationApplied).Scan(&references)
	if err != nil {
		m.recordCleanupFailure(secretName, "database")
		return
	}
	if references > 0 {
		_, _ = m.engine.DB.ExecContext(m.ctx, `DELETE FROM provider_secret_cleanup WHERE secret_name = ?`, secretName)
		return
	}
	if err := m.engine.SecretsProvider.Delete(m.ctx, secretName); err != nil && !errors.Is(err, secrets.ErrNotFound) {
		m.recordCleanupFailure(secretName, "delete")
		return
	}
	_, _ = m.engine.DB.ExecContext(m.ctx, `DELETE FROM provider_secret_cleanup WHERE secret_name = ?`, secretName)
}

func (m *providerOperationManager) recordCleanupFailure(secretName, failure string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(m.ctx), providerOperationFinalizerLimit)
	defer cancel()
	var attempts int
	_ = m.engine.DB.QueryRowContext(ctx,
		`SELECT attempt_count FROM provider_secret_cleanup WHERE secret_name = ?`, secretName).Scan(&attempts)
	delay := min(time.Duration(1<<min(attempts, 6))*time.Second, time.Minute)
	_, _ = m.engine.DB.ExecContext(ctx, `UPDATE provider_secret_cleanup
		SET attempt_count = attempt_count + 1, failure = ?, updated_at = CURRENT_TIMESTAMP, next_attempt_at = ?
		WHERE secret_name = ?`, failure, time.Now().UTC().Add(delay).Format(time.RFC3339Nano), secretName)
}
