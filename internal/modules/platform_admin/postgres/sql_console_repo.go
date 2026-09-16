package postgres

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// ExecuteSQL executes an arbitrary SQL query against PostgreSQL with safety and logs the execution.
// SQLConsoleRole is the database role console statements run as.
const SQLConsoleRole = "dawa24_sql_console"

func (r *Repository) ExecuteSQL(ctx context.Context, actorID *int64, actorName, query string) (*platformadmin.SQLQueryResult, error) {
	result := &platformadmin.SQLQueryResult{
		Columns: []string{},
		Rows:    [][]any{},
	}

	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		result.Error = i18n.TDefault("w4_mod.sql_238")
		return result, nil
	}

	cleaned := trimmed
	for {
		if strings.HasPrefix(cleaned, "--") {
			idx := strings.Index(cleaned, "\n")
			if idx == -1 {
				cleaned = ""
				break
			}
			cleaned = strings.TrimSpace(cleaned[idx+1:])
			continue
		}
		if strings.HasPrefix(cleaned, "/*") {
			idx := strings.Index(cleaned, "*/")
			if idx == -1 {
				cleaned = ""
				break
			}
			cleaned = strings.TrimSpace(cleaned[idx+2:])
			continue
		}
		break
	}
	cleaned = strings.TrimLeft(cleaned, "(\r\n\t ")
	upper := strings.ToUpper(cleaned)
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") && !strings.HasPrefix(upper, "EXPLAIN") && !strings.HasPrefix(upper, "SHOW") {
		result.Error = i18n.TDefault("w4_mod.select_explain_239")
		return result, nil
	}

	lower := strings.ToLower(cleaned)
	prohibitedPatterns := []string{
		"pg_read_file",
		"pg_read_binary_file",
		"pg_ls_dir",
		"pg_stat_file",
		"pg_shadow",
		"pg_authid",
		"password_hash",
		"session_token",
		"verification_token",
	}
	for _, p := range prohibitedPatterns {
		if strings.Contains(lower, p) {
			result.Error = "Security violation: Access to system files, system authentication catalogs, or sensitive credentials is prohibited."
			return result, nil
		}
	}

	start := time.Now()

	// Execute inside a read-only, timed-out, rolling-back transaction
	_ = r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// Set transaction read-only and statement timeout
		// SET LOCAL ROLE: the statement runs as dawa24_sql_console, which is not
		// a superuser and holds no credential tables or columns (migration 217).
		// The denylist above is a courtesy message; the role is the control.
		if _, err := tx.Exec(txCtx, "SET LOCAL ROLE "+SQLConsoleRole+"; SET LOCAL transaction_read_only = on; SET LOCAL statement_timeout = '10s';"); err != nil {
			result.Error = i18n.TDefault("w4_mod.w4str_240_240") + err.Error()
			return nil
		}

		rows, err := tx.Query(txCtx, trimmed)
		if err != nil {
			result.Error = err.Error()
			return nil
		}
		defer rows.Close()

		fieldDescs := rows.FieldDescriptions()
		for _, fd := range fieldDescs {
			result.Columns = append(result.Columns, fd.Name)
		}

		count := 0
		for rows.Next() {
			count++
			if count > 1000 {
				result.Truncated = true
				result.Message = i18n.TDefault("w4_mod.1000_241")
				break
			}

			vals, err := rows.Values()
			if err != nil {
				result.Error = err.Error()
				return nil
			}
			rowVals := make([]any, len(vals))
			for i, v := range vals {
				if v == nil {
					rowVals[i] = "NULL"
				} else {
					switch val := v.(type) {
					case []byte:
						if utf8.Valid(val) {
							rowVals[i] = string(val)
						} else {
							rowVals[i] = "\\x" + hex.EncodeToString(val)
						}
					case time.Time:
						rowVals[i] = val.Format("2006-01-02 15:04:05")
					case map[string]any, []any:
						if b, err := json.Marshal(val); err == nil {
							rowVals[i] = string(b)
						} else {
							rowVals[i] = fmt.Sprintf("%v", val)
						}
					default:
						rowVals[i] = fmt.Sprintf("%v", val)
					}
				}
			}
			result.Rows = append(result.Rows, rowVals)
		}
		if rows.Err() != nil && result.Error == "" {
			result.Error = rows.Err().Error()
		}
		result.RowsAffected = int64(len(result.Rows))
		return nil
	})

	result.DurationMS = time.Since(start).Milliseconds()

	// Log query execution into platform_admin.sql_logs table in separate transaction
	_ = r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			INSERT INTO platform_admin.sql_logs (query, executed_by, actor_name, duration_ms, rows_affected, error_message, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, now());
		`, trimmed, actorID, actorName, result.DurationMS, result.RowsAffected, result.Error)
		return err
	})

	return result, nil
}

// ListSQLLogs returns previous executed queries from the SQL Console.
func (r *Repository) ListSQLLogs(ctx context.Context, limit, offset int) ([]*platformadmin.SQLLog, error) {
	if limit <= 0 {
		limit = 30
	}
	var logs []*platformadmin.SQLLog

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, query, executed_by, COALESCE(actor_name, ''), duration_ms, rows_affected, COALESCE(error_message, ''), created_at
			FROM platform_admin.sql_logs
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2;
		`
		rows, err := tx.Query(txCtx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var l platformadmin.SQLLog
			if err := rows.Scan(&l.ID, &l.Query, &l.ExecutedBy, &l.ActorName, &l.DurationMS, &l.RowsAffected, &l.ErrorMessage, &l.CreatedAt); err != nil {
				return err
			}
			logs = append(logs, &l)
		}
		return nil
	})

	return logs, err
}

// LogError records a system diagnostic error or exception.
func (r *Repository) LogError(ctx context.Context, entry *platformadmin.ErrorLog) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		payloadJSON, _ := json.Marshal(entry.RequestPayload)

		query := `
			INSERT INTO platform_admin.error_logs (
				user_id, user_name, user_email, organization_name, error_level,
				error_message, exception_class, stack_trace, file_path, line_number,
				http_method, url_path, ip_address, user_agent, request_payload, status, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, now())
			RETURNING id, created_at;
		`
		return tx.QueryRow(txCtx, query,
			entry.UserID, entry.UserName, entry.UserEmail, entry.OrganizationName, entry.ErrorLevel,
			entry.ErrorMessage, entry.ExceptionClass, entry.StackTrace, entry.FilePath, entry.LineNumber,
			entry.HTTPMethod, entry.URLPath, entry.IPAddress, entry.UserAgent, payloadJSON, entry.Status,
		).Scan(&entry.ID, &entry.CreatedAt)
	})
}

// ListErrorLogs searches and retrieves diagnostic error logs.
func (r *Repository) ListErrorLogs(ctx context.Context, filter platformadmin.ErrorLogFilter) ([]*platformadmin.ErrorLog, int, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}

	var logs []*platformadmin.ErrorLog
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		countQuery := `
			SELECT COUNT(*) FROM platform_admin.error_logs
			WHERE ($1 = '' OR error_level = $1)
			  AND ($2 = '' OR status = $2)
			  AND ($3 = '' OR error_message ILIKE '%' || $3 || '%' OR exception_class ILIKE '%' || $3 || '%' OR user_name ILIKE '%' || $3 || '%')
			  AND ($4::BIGINT IS NULL OR user_id = $4);
		`
		if err := tx.QueryRow(txCtx, countQuery, filter.Level, filter.Status, filter.Search, filter.UserID).Scan(&total); err != nil {
			return err
		}

		query := `
			SELECT id, user_id, COALESCE(user_name, ''), COALESCE(user_email, ''), COALESCE(organization_name, ''),
			       COALESCE(error_level, 'ERROR'), error_message, COALESCE(exception_class, ''),
			       COALESCE(stack_trace, ''), COALESCE(file_path, ''), line_number,
			       COALESCE(http_method, 'GET'), COALESCE(url_path, ''), COALESCE(ip_address, ''),
			       COALESCE(user_agent, ''), request_payload, COALESCE(status, 'NEW'), created_at
			FROM platform_admin.error_logs
			WHERE ($1 = '' OR error_level = $1)
			  AND ($2 = '' OR status = $2)
			  AND ($3 = '' OR error_message ILIKE '%' || $3 || '%' OR exception_class ILIKE '%' || $3 || '%' OR user_name ILIKE '%' || $3 || '%')
			  AND ($4::BIGINT IS NULL OR user_id = $4)
			ORDER BY created_at DESC
			LIMIT $5 OFFSET $6;
		`
		rows, err := tx.Query(txCtx, query, filter.Level, filter.Status, filter.Search, filter.UserID, filter.Limit, filter.Offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var e platformadmin.ErrorLog
			var payloadRaw []byte
			if err := rows.Scan(
				&e.ID, &e.UserID, &e.UserName, &e.UserEmail, &e.OrganizationName,
				&e.ErrorLevel, &e.ErrorMessage, &e.ExceptionClass,
				&e.StackTrace, &e.FilePath, &e.LineNumber,
				&e.HTTPMethod, &e.URLPath, &e.IPAddress,
				&e.UserAgent, &payloadRaw, &e.Status, &e.CreatedAt,
			); err != nil {
				return err
			}
			if len(payloadRaw) > 0 {
				_ = json.Unmarshal(payloadRaw, &e.RequestPayload)
			}
			logs = append(logs, &e)
		}
		return rows.Err()
	})

	return logs, total, err
}

// GetErrorLogByID retrieves a single error log record.
func (r *Repository) GetErrorLogByID(ctx context.Context, id int64) (*platformadmin.ErrorLog, error) {
	var e platformadmin.ErrorLog
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, user_id, COALESCE(user_name, ''), COALESCE(user_email, ''), COALESCE(organization_name, ''),
			       COALESCE(error_level, 'ERROR'), error_message, COALESCE(exception_class, ''),
			       COALESCE(stack_trace, ''), COALESCE(file_path, ''), line_number,
			       COALESCE(http_method, 'GET'), COALESCE(url_path, ''), COALESCE(ip_address, ''),
			       COALESCE(user_agent, ''), request_payload, COALESCE(status, 'NEW'), created_at
			FROM platform_admin.error_logs
			WHERE id = $1;
		`
		var payloadRaw []byte
		if err := tx.QueryRow(txCtx, query, id).Scan(
			&e.ID, &e.UserID, &e.UserName, &e.UserEmail, &e.OrganizationName,
			&e.ErrorLevel, &e.ErrorMessage, &e.ExceptionClass,
			&e.StackTrace, &e.FilePath, &e.LineNumber,
			&e.HTTPMethod, &e.URLPath, &e.IPAddress,
			&e.UserAgent, &payloadRaw, &e.Status, &e.CreatedAt,
		); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("error_log")
			}
			return err
		}
		if len(payloadRaw) > 0 {
			_ = json.Unmarshal(payloadRaw, &e.RequestPayload)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return &e, nil
}

// UpdateErrorLogStatus updates the investigation/resolution status of an error.
func (r *Repository) UpdateErrorLogStatus(ctx context.Context, id int64, status string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE platform_admin.error_logs SET status = $2 WHERE id = $1;`
		_, err := tx.Exec(txCtx, query, id, status)
		return err
	})
}

// GetErrorDiagnosticsMetrics computes high-level error metrics for the dashboard.
func (r *Repository) GetErrorDiagnosticsMetrics(ctx context.Context) (total, critical24h, unresolved, affectedUsers int, err error) {
	err = r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM platform_admin.error_logs;`).Scan(&total)
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM platform_admin.error_logs WHERE error_level IN ('CRITICAL', 'FATAL') AND created_at >= NOW() - INTERVAL '24 HOURS';`).Scan(&critical24h)
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM platform_admin.error_logs WHERE status IN ('NEW', 'INVESTIGATING');`).Scan(&unresolved)
		_ = tx.QueryRow(txCtx, `SELECT COUNT(DISTINCT user_id) FROM platform_admin.error_logs WHERE user_id IS NOT NULL;`).Scan(&affectedUsers)
		return nil
	})
	return
}
