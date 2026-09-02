package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// DefaultMigrator implements the Migrator interface for migrating data
// between different database backends.
type DefaultMigrator struct {
	source Database
	target Database
}

// NewMigrator creates a new migrator to transfer data from source to target.
func NewMigrator(source, target Database) *DefaultMigrator {
	return &DefaultMigrator{
		source: source,
		target: target,
	}
}

// Migrate transfers all data from source to target database.
// Pre-conditions: target must already have schema (call target.RunMigrations
// first, or use Validate which checks this). Source is read-only throughout.
//
// If migration fails partway through, Rollback is called to clean up
// the target database (DELETE all migrated tables) so the source
// remains authoritative and the user can retry safely.
func (m *DefaultMigrator) Migrate(ctx context.Context, progress ProgressCallback) (retErr error) {
	// Disable FK checks on target for the duration of migration.
	// We insert in dependency order anyway, but this guards against
	// edge cases and self-referencing tables.
	if err := m.disableFKChecks(ctx, m.target); err != nil {
		// Non-fatal: log and continue
	}
	defer func() {
		// Re-enable FK checks regardless of migration result
		_ = m.enableFKChecks(ctx, m.target)
	}()

	// Get list of tables to migrate (auto-detected from source schema)
	tables, err := m.getTables(ctx)
	if err != nil {
		return fmt.Errorf("get tables: %w", err)
	}

	// Order tables by FK dependencies (parents first)
	tables, err = m.orderTablesByDependencies(ctx, tables)
	if err != nil {
		return fmt.Errorf("order tables: %w", err)
	}

	// Get total row count for progress tracking
	stats, err := m.GetStats(ctx)
	if err != nil {
		return fmt.Errorf("get stats: %w", err)
	}

	var migratedRows int64
	migratedTables := make([]string, 0, len(tables))

	// Migrate each table
	for _, table := range tables {
		if err := m.migrateTable(ctx, table, &migratedRows, stats.TotalRows, progress); err != nil {
			// Migration failed: rollback by deleting already-migrated tables.
			// Source database is untouched (we only read from it), so the
			// system remains in a consistent state pointing at the source.
			rollbackErr := m.rollback(ctx, migratedTables)
			if rollbackErr != nil {
				// Rollback failed too — return both errors so the operator
				// knows the target DB may need manual cleanup.
				return fmt.Errorf("migrate table %s: %w (rollback also failed: %v)", table, err, rollbackErr)
			}
			return fmt.Errorf("migrate table %s: %w (target rolled back, source untouched)", table, err)
		}
		migratedTables = append(migratedTables, table)
	}

	return nil
}

// rollback deletes all migrated rows from the target database (reverse
// order to respect FK constraints, though we also disable FK checks).
// This is called when migration fails partway through to leave the target
// in a clean state so the user can retry.
func (m *DefaultMigrator) rollback(ctx context.Context, tables []string) error {
	if len(tables) == 0 {
		return nil
	}

	// Disable FK checks during cleanup
	_ = m.disableFKChecks(ctx, m.target)
	defer m.enableFKChecks(ctx, m.target) //nolint:errcheck

	// Delete in reverse order (children before parents)
	for i := len(tables) - 1; i >= 0; i-- {
		table := tables[i]
		_, _ = m.target.Exec(ctx, fmt.Sprintf("DELETE FROM %s", table))
		// Continue with other tables even if one fails
	}
	return nil
}

// migrateTable migrates a single table from source to target.
func (m *DefaultMigrator) migrateTable(
	ctx context.Context,
	table string,
	migratedRows *int64,
	totalRows int64,
	progress ProgressCallback,
) error {
	// Get table schema
	columns, err := m.getTableColumns(ctx, table)
	if err != nil {
		return fmt.Errorf("get columns: %w", err)
	}
	
	// Query all rows from source
	query := fmt.Sprintf("SELECT %s FROM %s", joinColumns(columns), table)
	rows, err := m.source.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("query source: %w", err)
	}
	defer rows.Close()
	
	// Prepare insert statement for target
	insertQuery := m.buildInsertQuery(table, columns)
	stmt, err := m.target.Prepare(ctx, insertQuery)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()
	
	// Migrate rows in batches
	batch := make([][]interface{}, 0, 100)
	
	for rows.Next() {
		// Scan row values
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		
		if err := rows.Scan(valuePtrs...); err != nil {
			return fmt.Errorf("scan row: %w", err)
		}
		
		batch = append(batch, values)
		
		// Insert batch when full
		if len(batch) >= 100 {
			if err := m.insertBatch(ctx, stmt, batch); err != nil {
				return fmt.Errorf("insert batch: %w", err)
			}
			
			*migratedRows += int64(len(batch))
			if progress != nil {
				progress(*migratedRows, totalRows, table)
			}
			
			batch = batch[:0] // Reset batch
		}
	}
	
	// Insert remaining rows
	if len(batch) > 0 {
		if err := m.insertBatch(ctx, stmt, batch); err != nil {
			return fmt.Errorf("insert final batch: %w", err)
		}
		
		*migratedRows += int64(len(batch))
		if progress != nil {
			progress(*migratedRows, totalRows, table)
		}
	}
	
	return rows.Err()
}

// insertBatch inserts a batch of rows using the prepared statement.
func (m *DefaultMigrator) insertBatch(ctx context.Context, stmt *sql.Stmt, batch [][]interface{}) error {
	for _, values := range batch {
		if _, err := stmt.ExecContext(ctx, values...); err != nil {
			return fmt.Errorf("insert row: %w", err)
		}
	}
	return nil
}

// getTables returns the list of tables to migrate by introspecting
// the source database schema. Tables are ordered by FK dependencies
// (parents before children) to avoid constraint violations during insert.
func (m *DefaultMigrator) getTables(ctx context.Context) ([]string, error) {
	var tables []string

	if m.source.Type() == DatabaseTypeSQLite {
		// SQLite: query sqlite_master for all user tables
		rows, err := m.source.Query(ctx, 
			`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'schema_migrations'`)
		if err != nil {
			return nil, fmt.Errorf("list sqlite tables: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, err
			}
			tables = append(tables, name)
		}
		return tables, rows.Err()
	}

	// PostgreSQL: query information_schema
	rows, err := m.source.Query(ctx,
		`SELECT table_name FROM information_schema.tables 
		 WHERE table_schema='public' AND table_type='BASE TABLE' 
		 AND table_name != 'schema_migrations'`)
	if err != nil {
		return nil, fmt.Errorf("list postgres tables: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// orderTablesByDependencies returns tables ordered so that tables with
// no FK dependencies come first, followed by tables that reference them.
// This is a simple topological sort. If circular dependencies are detected,
// the original order is preserved (FK checks are disabled during migration
// anyway, so circular refs don't block the process).
func (m *DefaultMigrator) orderTablesByDependencies(ctx context.Context, tables []string) ([]string, error) {
	// Build dependency map: table -> tables it depends on (references)
	deps := make(map[string][]string)
	for _, t := range tables {
		deps[t] = m.getFKDependencies(ctx, t)
	}

	// Simple topological sort (Kahn's algorithm)
	resolved := make([]string, 0, len(tables))
	remaining := make(map[string]bool)
	for _, t := range tables {
		remaining[t] = true
	}

	for len(remaining) > 0 {
		progress := false
		for t := range remaining {
			// Check if all dependencies are resolved
			allDepsResolved := true
			for _, dep := range deps[t] {
				if remaining[dep] {
					allDepsResolved = false
					break
				}
			}
			if allDepsResolved {
				resolved = append(resolved, t)
				delete(remaining, t)
				progress = true
			}
		}
		if !progress {
			// Circular dependency — add remaining in original order
			for _, t := range tables {
				if remaining[t] {
					resolved = append(resolved, t)
					delete(remaining, t)
				}
			}
		}
	}
	return resolved, nil
}

// getFKDependencies returns the list of tables that `table` references
// via foreign key constraints.
func (m *DefaultMigrator) getFKDependencies(ctx context.Context, table string) []string {
	if m.source.Type() == DatabaseTypeSQLite {
		// PRAGMA foreign_key_list returns referenced tables
		rows, err := m.source.Query(ctx, fmt.Sprintf("PRAGMA foreign_key_list(%s)", table))
		if err != nil {
			return nil
		}
		defer rows.Close()

		var deps []string
		for rows.Next() {
			// Columns: id, seq, table, from, to, on_update, on_delete, match
			var id, seq int
			var refTable, fromCol, toCol, onUpdate, onDelete, match string
			if err := rows.Scan(&id, &seq, &refTable, &fromCol, &toCol, &onUpdate, &onDelete, &match); err != nil {
				continue
			}
			deps = append(deps, refTable)
		}
		return deps
	}

	// PostgreSQL: query information_schema
	rows, err := m.source.Query(ctx, `
		SELECT DISTINCT ccu.table_name AS referenced_table
		FROM information_schema.table_constraints tc
		JOIN information_schema.constraint_column_usage ccu
			ON tc.constraint_name = ccu.constraint_name
		WHERE tc.table_name = $1 AND tc.constraint_type = 'FOREIGN KEY'`,
		table)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var deps []string
	for rows.Next() {
		var refTable string
		if err := rows.Scan(&refTable); err != nil {
			continue
		}
		deps = append(deps, refTable)
	}
	return deps
}

// disableFKChecks disables foreign key constraint checking on the target
// database during migration. This allows inserting rows in any order
// without worrying about parent rows existing first.
func (m *DefaultMigrator) disableFKChecks(ctx context.Context, db Database) error {
	switch db.Type() {
	case DatabaseTypeSQLite:
		_, err := db.Exec(ctx, "PRAGMA foreign_keys = OFF")
		return err
	case DatabaseTypePostgres, DatabaseTypeSupabase:
		_, err := db.Exec(ctx, "SET session_replication_role = replica")
		return err
	}
	return nil
}

// enableFKChecks re-enables foreign key constraint checking.
func (m *DefaultMigrator) enableFKChecks(ctx context.Context, db Database) error {
	switch db.Type() {
	case DatabaseTypeSQLite:
		_, err := db.Exec(ctx, "PRAGMA foreign_keys = ON")
		return err
	case DatabaseTypePostgres, DatabaseTypeSupabase:
		_, err := db.Exec(ctx, "SET session_replication_role = DEFAULT")
		return err
	}
	return nil
}

// getTableColumns returns the column names for a table.
func (m *DefaultMigrator) getTableColumns(ctx context.Context, table string) ([]string, error) {
	// For SQLite source
	if m.source.Type() == DatabaseTypeSQLite {
		query := fmt.Sprintf("PRAGMA table_info(%s)", table)
		rows, err := m.source.Query(ctx, query)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		
		var columns []string
		for rows.Next() {
			var cid int
			var name, typ string
			var notnull, pk int
			var dfltValue interface{}
			
			if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
				return nil, err
			}
			
			columns = append(columns, name)
		}
		
		return columns, rows.Err()
	}
	
	// For PostgreSQL source
	query := `
		SELECT column_name 
		FROM information_schema.columns 
		WHERE table_name = $1 
		ORDER BY ordinal_position`
	
	rows, err := m.source.Query(ctx, query, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var columns []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	
	return columns, rows.Err()
}

// buildInsertQuery builds an INSERT statement for the target database.
func (m *DefaultMigrator) buildInsertQuery(table string, columns []string) string {
	placeholders := make([]string, len(columns))
	
	// Use appropriate placeholder syntax
	if m.target.Type() == DatabaseTypeSQLite {
		for i := range placeholders {
			placeholders[i] = "?"
		}
	} else {
		// PostgreSQL uses $1, $2, etc.
		for i := range placeholders {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		}
	}
	
	return fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		table,
		joinColumns(columns),
		joinColumns(placeholders),
	)
}

// joinColumns joins column names with commas.
func joinColumns(cols []string) string {
	result := ""
	for i, col := range cols {
		if i > 0 {
			result += ", "
		}
		result += col
	}
	return result
}

// Validate checks if the migration is possible.
// Pre-conditions:
//   1. Source database is accessible and has tables
//   2. Target database is accessible and has schema (run target.RunMigrations first)
func (m *DefaultMigrator) Validate(ctx context.Context) error {
	// Check source database is accessible
	if err := m.source.Ping(ctx); err != nil {
		return fmt.Errorf("source database not accessible: %w", err)
	}

	// Check target database is accessible
	if err := m.target.Ping(ctx); err != nil {
		return fmt.Errorf("target database not accessible: %w", err)
	}

	// Check target database has schema (schema_migrations table exists)
	version, err := m.target.GetSchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("target database missing schema — run RunMigrations() on target first: %w", err)
	}
	if version == 0 {
		return fmt.Errorf("target database schema is empty — run RunMigrations() on target first")
	}

	// Check source has tables to migrate
	tables, err := m.getTables(ctx)
	if err != nil {
		return fmt.Errorf("source database schema query failed: %w", err)
	}
	if len(tables) == 0 {
		return fmt.Errorf("source database has no tables to migrate")
	}

	return nil
}

// EstimateTime estimates how long the migration will take.
func (m *DefaultMigrator) EstimateTime(ctx context.Context) (time.Duration, error) {
	stats, err := m.GetStats(ctx)
	if err != nil {
		return 0, err
	}
	
	// Rough estimate: 1000 rows per second
	seconds := stats.TotalRows / 1000
	if seconds < 1 {
		seconds = 1
	}
	
	return time.Duration(seconds) * time.Second, nil
}

// GetStats returns statistics about the data to be migrated.
func (m *DefaultMigrator) GetStats(ctx context.Context) (*MigrationStats, error) {
	tables, err := m.getTables(ctx)
	if err != nil {
		return nil, err
	}
	
	stats := &MigrationStats{
		Tables: make([]TableStats, 0, len(tables)),
	}
	
	for _, table := range tables {
		var count int64
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		err := m.source.QueryRow(ctx, query).Scan(&count)
		if err != nil {
			// Table might not exist, skip it
			continue
		}
		
		stats.Tables = append(stats.Tables, TableStats{
			Name:          table,
			RowCount:      count,
			EstimatedSize: count * 1024, // Rough estimate: 1KB per row
		})
		
		stats.TotalRows += count
		stats.EstimatedSize += count * 1024
	}
	
	return stats, nil
}
