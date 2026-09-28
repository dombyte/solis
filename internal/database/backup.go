package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	sqlite "modernc.org/sqlite"
)

// BackupConfig contains configuration for backup operations.
type BackupConfig struct {
	// Enabled determines if backup functionality is active.
	Enabled bool
	// MaxBackups is the maximum number of backups to keep (0 = unlimited).
	MaxBackups int
	// BackupInterval is the interval for periodic online backups.
	BackupInterval time.Duration
}

// BackupInfo contains information about a backup file.
type BackupInfo struct {
	Filename  string
	Timestamp time.Time
	Size      int64
}

// backupStampLayout is the second part of a backup timestamp; milliseconds follow as
// "_mmm" so two backups within one second never share a file name.
const backupStampLayout = "20060102_150405"

// backupDirPerm is the permission of created backup directories (owner rwx, group rx).
const backupDirPerm = 0o750

// msPerSecond scales the millisecond suffix.
const msPerSecond = int(time.Second / time.Millisecond)

// GenerateBackupFilename generates a backup filename for the instant now.
// Format: backups/{name}.{YYYYMMDD_HHMMSS_mmm}.backup
func GenerateBackupFilename(dbPath string, now time.Time) string {
	dbName := filepath.Base(dbPath)
	backupsDir := filepath.Join(filepath.Dir(dbPath), "backups")
	ms := now.Nanosecond() / int(time.Millisecond) % msPerSecond
	stamp := fmt.Sprintf("%s_%03d", now.Format(backupStampLayout), ms)
	return filepath.Join(backupsDir, fmt.Sprintf("%s.%s.backup", dbName, stamp))
}

// ExtractBackupInfo extracts information from a backup filename.
// Expected format: {name}.{timestamp}.backup, with or without the millisecond suffix.
func ExtractBackupInfo(filename string) (*BackupInfo, error) {
	base := filepath.Base(filename)
	if !strings.HasSuffix(base, ".backup") {
		return nil, fmt.Errorf("not a backup file: %s", filename)
	}
	nameWithoutExt := strings.TrimSuffix(base, ".backup")
	lastDotIndex := strings.LastIndex(nameWithoutExt, ".")
	if lastDotIndex <= 0 {
		return nil, fmt.Errorf("could not parse backup filename: %s", filename)
	}
	if t, ok := parseBackupStamp(nameWithoutExt[lastDotIndex+1:]); ok {
		return &BackupInfo{Filename: filename, Timestamp: t}, nil
	}
	return nil, fmt.Errorf("could not parse timestamp in backup filename: %s", filename)
}

// parseBackupStamp parses "YYYYMMDD_HHMMSS" with an optional "_mmm" suffix.
func parseBackupStamp(stamp string) (time.Time, bool) {
	secs, msPart := stamp, ""
	if len(stamp) > len(backupStampLayout) {
		secs, msPart = stamp[:len(backupStampLayout)], stamp[len(backupStampLayout):]
	}
	t, err := time.Parse(backupStampLayout, secs)
	if err != nil {
		return time.Time{}, false
	}
	if msPart == "" {
		return t, true
	}
	ms, ok := parseBackupMillis(msPart)
	if !ok {
		return time.Time{}, false
	}
	return t.Add(time.Duration(ms) * time.Millisecond), true
}

// parseBackupMillis parses the "_mmm" millisecond suffix of a backup stamp.
func parseBackupMillis(part string) (int, bool) {
	digits, found := strings.CutPrefix(part, "_")
	if !found {
		return 0, false
	}
	ms, err := strconv.Atoi(digits)
	if err != nil || ms < 0 || ms >= msPerSecond {
		return 0, false
	}
	return ms, true
}

// backuper interface for accessing SQLite backup functionality.
// This matches the interface provided by modernc.org/sqlite driver connections.
type backuper interface {
	NewBackup(string) (*sqlite.Backup, error)
}

// createSQLiteBackup creates a backup of a SQLite database using the native SQLite backup API
// and verifies the result with PRAGMA integrity_check.
func createSQLiteBackup(ctx context.Context, sourcePath, destPath string,
	log zerolog.Logger,
) (err error) {
	srcDB, err := openSourceDatabase(ctx, sourcePath)
	if err != nil {
		return err
	}
	defer closeInto(&err, srcDB, "source database")
	if err := ensureDestinationDirectory(destPath); err != nil {
		return err
	}
	conn, err := getDatabaseConnection(ctx, srcDB)
	if err != nil {
		return err
	}
	defer closeInto(&err, conn, "connection")
	if err := performBackupCopy(conn, destPath); err != nil {
		return err
	}
	return verifyBackupFile(ctx, destPath, log)
}

// closeInto closes c and records its error in *err unless an earlier error is set.
func closeInto(err *error, c io.Closer, what string) {
	if closeErr := c.Close(); closeErr != nil && *err == nil {
		*err = fmt.Errorf("failed to close %s: %w", what, closeErr)
	}
}

// openSourceDatabase opens the source database for backup
func openSourceDatabase(ctx context.Context, sourcePath string) (*sql.DB, error) {
	srcDB, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source database for backup: %w", err)
	}

	if err := srcDB.PingContext(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("failed to ping source database: %w", err),
			srcDB.Close())
	}

	return srcDB, nil
}

// ensureDestinationDirectory ensures the destination directory exists
func ensureDestinationDirectory(destPath string) error {
	destDir := filepath.Dir(destPath)
	if destDir != "" && destDir != "." {
		if err := os.MkdirAll(destDir, backupDirPerm); err != nil {
			return fmt.Errorf("failed to create destination directory: %w", err)
		}
	}
	return nil
}

// getDatabaseConnection gets a connection from the database
func getDatabaseConnection(ctx context.Context, db *sql.DB) (*sql.Conn, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}
	return conn, nil
}

// performBackupCopy performs the actual backup copy using SQLite backup API
func performBackupCopy(conn *sql.Conn, destPath string) error {
	err := conn.Raw(func(driverConn any) error { return runBackup(driverConn, destPath) })
	if err != nil {
		return fmt.Errorf("SQLite backup failed: %w", err)
	}
	return nil
}

// runBackup copies the whole database behind driverConn to destPath.
func runBackup(driverConn any, destPath string) error {
	b, ok := driverConn.(backuper)
	if !ok {
		return errors.New("driver connection does not support backups")
	}
	bkp, err := b.NewBackup(destPath)
	if err != nil {
		return fmt.Errorf("failed to create backup object: %w", err)
	}
	for more := true; more; {
		if more, err = bkp.Step(-1); err != nil {
			return fmt.Errorf("failed during backup step: %w", err)
		}
	}
	if err := bkp.Finish(); err != nil {
		return fmt.Errorf("failed to finish backup: %w", err)
	}
	return nil
}

// verifyBackupFile opens the finished backup read-only and runs PRAGMA integrity_check;
// a backup that is empty or not a sound SQLite database is removed and reported.
func verifyBackupFile(ctx context.Context, destPath string, log zerolog.Logger) error {
	err := checkBackupIntegrity(ctx, destPath)
	if err == nil {
		return nil
	}
	if removeErr := os.Remove(destPath); removeErr != nil && !os.IsNotExist(removeErr) {
		log.Warn().Err(removeErr).Msg("failed to remove invalid backup file")
	}
	return fmt.Errorf("backup verification failed: %w", err)
}

func checkBackupIntegrity(ctx context.Context, path string) (err error) {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("backup file not found after creation: %w", err)
	}
	if info.Size() == 0 {
		return errors.New("backup file is empty")
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer closeInto(&err, db, "backup database")
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("integrity check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity check: %s", result)
	}
	return nil
}

// CreateBackup creates an integrity-checked backup copy of the database file; now names
// the file.
func CreateBackup(ctx context.Context, dbPath string, config *BackupConfig, now time.Time,
	log zerolog.Logger,
) (string, error) {
	if !config.Enabled {
		log.Info().Msg("Backup disabled, skipping backup creation")
		return "", nil
	}

	// Check if source file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return "", fmt.Errorf("database file does not exist: %s", dbPath)
	}

	// Ensure the backups subdirectory (and its parent) exists
	backupsDir := filepath.Join(filepath.Dir(dbPath), "backups")
	if err := os.MkdirAll(backupsDir, backupDirPerm); err != nil {
		return "", fmt.Errorf("failed to create backups directory: %w", err)
	}

	// Generate backup filename
	backupPath := GenerateBackupFilename(dbPath, now)

	log.Info().Str("source", dbPath).Str("destination", backupPath).Msg("creating backup")

	// Create the backup using SQLite native backup API
	if err := createSQLiteBackup(ctx, dbPath, backupPath, log); err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	// Get backup file info for logging
	backupInfo, err := os.Stat(backupPath)
	if err != nil {
		return "", fmt.Errorf("failed to get backup file info: %w", err)
	}

	log.Info().Str("file", backupPath).Int64("size", backupInfo.Size()).
		Msg("backup created successfully")

	return backupPath, nil
}

// ListBackups lists all backup files in the backups subdirectory of the database directory.
// Returns both migration backups and online backups, sorted by timestamp (newest first).
func ListBackups(dbPath string) ([]BackupInfo, error) {
	dir := filepath.Dir(dbPath)
	if dir == "" {
		dir = "."
	}
	backupsDir := filepath.Join(dir, "backups")

	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		// If backups directory doesn't exist, return empty list
		if os.IsNotExist(err) {
			return []BackupInfo{}, nil
		}
		return nil, fmt.Errorf("failed to read backups directory: %w", err)
	}

	backups := make([]BackupInfo, 0)
	for _, entry := range entries {
		if info, ok := backupEntry(backupsDir, entry); ok {
			backups = append(backups, *info)
		}
	}

	// Sort by timestamp (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp.After(backups[j].Timestamp)
	})

	return backups, nil
}

// backupEntry returns the backup info for entry, or false when it is not a backup file
// following our naming pattern.
func backupEntry(backupsDir string, entry os.DirEntry) (*BackupInfo, bool) {
	if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".backup") {
		return nil, false
	}
	fullPath := filepath.Join(backupsDir, entry.Name())
	info, err := ExtractBackupInfo(fullPath)
	if err != nil {
		return nil, false
	}
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		return nil, false
	}
	info.Size = fileInfo.Size()
	return info, true
}

// CleanupBackups removes old backup files, keeping only the most recent MaxBackups.
// If maxBackups <= 0, keeps all backups.
func CleanupBackups(dbPath string, maxBackups int, log zerolog.Logger) error {
	if maxBackups <= 0 {
		log.Debug().Msg("Backup cleanup skipped: maxBackups <= 0")
		return nil
	}

	backups, err := ListBackups(dbPath)
	if err != nil {
		return fmt.Errorf("failed to list backups: %w", err)
	}

	if len(backups) <= maxBackups {
		log.Debug().Int("backups_count", len(backups)).Int("max_backups", maxBackups).
			Msg("no backup cleanup needed")
		return nil
	}

	// Calculate how many to remove
	toRemove := len(backups) - maxBackups
	backupsToRemove := backups[maxBackups:]

	log.Info().Int("to_remove", toRemove).Int("keeping", maxBackups).Msg("cleaning up old backups")

	// Remove the oldest backups
	for _, backup := range backupsToRemove {
		if err := os.Remove(backup.Filename); err != nil {
			log.Error().Err(err).Str("file", backup.Filename).Msg("failed to remove backup")
			// Continue with cleanup even if one file fails
			continue
		}
		log.Debug().Str("file", backup.Filename).Msg("removed old backup")
	}

	return nil
}
