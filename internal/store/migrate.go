package store

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pressly/goose/v3"
)

// backupPrefix names backups <data dir>/gwen.db.bak-<version>.
const backupPrefix = DatabaseFile + ".bak-"

// keepBackups is how many pre-migration backups are kept.
const keepBackups = 2

// migrate applies every pending migration in migrations and returns the
// resulting version. Before applying anything to a database that already has a
// schema, it writes a backup with VACUUM INTO and prunes older backups.
func migrate(ctx context.Context, db *sql.DB, dataDir string, migrations fs.FS) (int64, error) {
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, migrations)
	if err != nil {
		return 0, fmt.Errorf("load migrations: %w", err)
	}
	current, err := provider.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	sources := provider.ListSources()
	if len(sources) == 0 {
		return 0, fmt.Errorf("load migrations: none embedded")
	}
	target := sources[len(sources)-1].Version
	if current >= target {
		return current, nil
	}
	if current > 0 {
		if err := backup(ctx, db, dataDir, current); err != nil {
			return 0, err
		}
	}
	if _, err := provider.Up(ctx); err != nil {
		return 0, fmt.Errorf("migrate from version %d to %d: %w", current, target, err)
	}
	return target, nil
}

// backup writes <dataDir>/gwen.db.bak-<version> and keeps only the most recent
// keepBackups backups.
func backup(ctx context.Context, db *sql.DB, dataDir string, version int64) error {
	path := filepath.Join(dataDir, backupPrefix+strconv.FormatInt(version, 10))
	// VACUUM INTO refuses to overwrite; a leftover from a failed attempt is stale.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("back up database: %w", err)
	}
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return fmt.Errorf("back up database to %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("back up database: %w", err)
	}
	return pruneBackups(dataDir)
}

func pruneBackups(dataDir string) error {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return fmt.Errorf("prune backups: %w", err)
	}
	var versions []int64
	for _, e := range entries {
		rest, ok := strings.CutPrefix(e.Name(), backupPrefix)
		if !ok {
			continue
		}
		if v, err := strconv.ParseInt(rest, 10, 64); err == nil {
			versions = append(versions, v)
		}
	}
	slices.Sort(versions)
	for len(versions) > keepBackups {
		name := backupPrefix + strconv.FormatInt(versions[0], 10)
		if err := os.Remove(filepath.Join(dataDir, name)); err != nil {
			return fmt.Errorf("prune backups: %w", err)
		}
		versions = versions[1:]
	}
	return nil
}
