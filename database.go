package db2

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const defaultAdminTimeout = 60 * time.Second

// CreateDb creates a new database on the IBM Db2 server specified in connStr.
// Optional options can be provided in "key=value" format (e.g. "codeset=UTF-8", "territory=US", "mode=...").
func CreateDb(dbname string, connStr string, options ...string) (bool, error) {
	if strings.TrimSpace(dbname) == "" {
		return false, fmt.Errorf("db2: database name cannot be empty")
	}
	if strings.TrimSpace(connStr) == "" {
		return false, fmt.Errorf("db2: connection string cannot be empty")
	}

	var codeset, territory, mode string
	for _, opt := range options {
		parts := strings.SplitN(opt, "=", 2)
		if len(parts) != 2 {
			return false, fmt.Errorf("db2: invalid option format %q (expected key=value)", opt)
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])
		switch key {
		case "codeset":
			codeset = val
		case "territory":
			territory = val
		case "mode":
			mode = val
		default:
			return false, fmt.Errorf("db2: unsupported option %q", key)
		}
	}

	db, err := sql.Open("db2", connStr)
	if err != nil {
		return false, fmt.Errorf("db2: failed to open connection: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), defaultAdminTimeout)
	defer cancel()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("CREATE DATABASE %s", dbname))
	if codeset != "" {
		sb.WriteString(fmt.Sprintf(" CODESET %s", codeset))
	}
	if territory != "" {
		sb.WriteString(fmt.Sprintf(" TERRITORY %s", territory))
	}
	if mode != "" {
		sb.WriteString(fmt.Sprintf(" %s", mode))
	}

	createSQL := sb.String()
	_, err = db.ExecContext(ctx, createSQL)
	if err != nil {
		// Also try via SYSPROC.ADMIN_CMD if direct DDL requires administrative routing
		adminSQL := fmt.Sprintf("CALL SYSPROC.ADMIN_CMD('%s')", strings.ReplaceAll(createSQL, "'", "''"))
		if _, adminErr := db.ExecContext(ctx, adminSQL); adminErr == nil {
			return true, nil
		}
		return false, fmt.Errorf("db2: failed to create database %q: %w", dbname, err)
	}

	return true, nil
}

// DropDb drops the specified database from the IBM Db2 server.
func DropDb(dbname string, connStr string) (bool, error) {
	if strings.TrimSpace(dbname) == "" {
		return false, fmt.Errorf("db2: database name cannot be empty")
	}
	if strings.TrimSpace(connStr) == "" {
		return false, fmt.Errorf("db2: connection string cannot be empty")
	}

	db, err := sql.Open("db2", connStr)
	if err != nil {
		return false, fmt.Errorf("db2: failed to open connection: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), defaultAdminTimeout)
	defer cancel()

	dropSQL := fmt.Sprintf("DROP DATABASE %s", dbname)
	_, err = db.ExecContext(ctx, dropSQL)
	if err != nil {
		adminSQL := fmt.Sprintf("CALL SYSPROC.ADMIN_CMD('%s')", strings.ReplaceAll(dropSQL, "'", "''"))
		if _, adminErr := db.ExecContext(ctx, adminSQL); adminErr == nil {
			return true, nil
		}
		return false, fmt.Errorf("db2: failed to drop database %q: %w", dbname, err)
	}

	return true, nil
}

// ExecAdminCmd executes an administrative command via Db2's SYSPROC.ADMIN_CMD stored procedure.
// It returns sql.Result with execution status.
func ExecAdminCmd(ctx context.Context, db *sql.DB, command string) (sql.Result, error) {
	if strings.TrimSpace(command) == "" {
		return nil, fmt.Errorf("db2: admin command cannot be empty")
	}
	stmt := fmt.Sprintf("CALL SYSPROC.ADMIN_CMD('%s')", strings.ReplaceAll(command, "'", "''"))
	return db.ExecContext(ctx, stmt)
}
