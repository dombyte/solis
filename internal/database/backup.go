package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

// GenerateBackupFilename generates a backup filename with timestamp.
// Format: backups/{name}.{timestamp}.backup
func GenerateBackupFilename(dbPath string) string {
	dbName := filepath.Base(dbPath)
	dir := filepath.Dir(dbPath)
	backupsDir := filepath.Join(dir, "backups")
	timestamp := time.Now().Format("20060102_150405")

	prefix := fmt.Sprintf("%s.%s.backup", dbName, timestamp)

	return filepath.Join(backupsDir, prefix)
}

// ExtractBackupInfo extracts information from a backup filename.
// Expected format: {name}.{timestamp}.backup
func ExtractBackupInfo(filename string) (*BackupInfo, error) {
	base := filepath.Base(filename)
	if !strings.HasSuffix(base, ".backup") {
		return nil, fmt.Errorf("not a backup file: %s", filename)
	}

	// Remove .backup extension
	nameWithoutExt := strings.TrimSuffix(base, ".backup")

	// Find the last dot to separate db name from timestamp
	lastDotIndex := strings.LastIndex(nameWithoutExt, ".")
	if lastDotIndex <= 0 {
		return nil, fmt.Errorf("could not parse backup filename: %s", filename)
	}

	timestampStr := nameWithoutExt[lastDotIndex+1:]

	info := &BackupInfo{
		Filename: filename,
	}

	if t, err := time.Parse("20060102_150405", timestampStr); err == nil {
		info.Timestamp = t
		return info, nil
	}

	return nil, fmt.Errorf("could not parse timestamp in backup filename: %s", filename)
}

// backuper interface for accessing SQLite backup functionality.
// This matches the interface provided by modernc.org/sqlite driver connections.
type backuper interface {
	NewBackup(string) (*sqlite.Backup, error)
}

// calculateSHA256 calculates the SHA256 checksum of a file.
func calculateSHA256(filePath string) (string, error) {
	// #nosec G304 -- filePath comes from GenerateBackupFilename/CreateBackup, not user input
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file for checksum: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close file: %w", closeErr)
		}
	}()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("failed to read file for checksum: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// createSQLiteBackup creates a backup of a SQLite database using the native SQLite backup API.
// This provides better consistency and reliability compared to simple file copying.
// It also verifies the backup integrity using SHA256 checksums.
func createSQLiteBackup(sourcePath, destPath string, log zerolog.Logger) (err error) {
	srcDB, err := openSourceDatabase(sourcePath)
	if err != nil {
		return err
	}
	defer closeInto(&err, srcDB, "source database")
	if err := ensureDestinationDirectory(destPath); err != nil {
		return err
	}
	conn, err := getDatabaseConnection(srcDB)
	if err != nil {
		return err
	}
	defer closeInto(&err, conn, "connection")
	if err := performBackupCopy(conn, destPath); err != nil {
		return err
	}
	return verifyBackupFile(destPath, log)
}

// closeInto closes c and records its error in *err unless an earlier error is set.
func closeInto(err *error, c io.Closer, what string) {
	if closeErr := c.Close(); closeErr != nil && *err == nil {
		*err = fmt.Errorf("failed to close %s: %w", what, closeErr)
	}
}

// openSourceDatabase opens the source database for backup
func openSourceDatabase(sourcePath string) (*sql.DB, error) {
	srcDB, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source database for backup: %w", err)
	}

	if err := srcDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping source database: %w", err)
	}

	return srcDB, nil
}

// ensureDestinationDirectory ensures the destination directory exists
func ensureDestinationDirectory(destPath string) error {
	destDir := filepath.Dir(destPath)
	if destDir != "" && destDir != "." {
		if err := os.MkdirAll(destDir, 0750); err != nil {
			return fmt.Errorf("failed to create destination directory: %w", err)
		}
	}
	return nil
}

// getDatabaseConnection gets a connection from the database
func getDatabaseConnection(db *sql.DB) (*sql.Conn, error) {
	conn, err := db.Conn(context.Background())
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

// verifyBackupFile verifies the backup file exists, has content, and has a valid checksum
func verifyBackupFile(destPath string, log zerolog.Logger) error {
	backupChecksum, err := calculateSHA256(destPath)
	if err != nil {
		if removeErr := os.Remove(destPath); removeErr != nil {
			log.Warn().Msgf("Failed to remove incomplete backup file: %v", removeErr)
		}
		return fmt.Errorf("failed to verify backup file: %w", err)
	}

	log.Debug().Msgf("Backup created with checksum: %s", backupChecksum)

	backupInfo, err := os.Stat(destPath)
	if err != nil {
		return fmt.Errorf("backup file not found after creation: %w", err)
	}

	if backupInfo.Size() == 0 {
		if removeErr := os.Remove(destPath); removeErr != nil {
			log.Warn().Msgf("Failed to remove empty backup file: %v", removeErr)
		}
		return errors.New("backup file is empty")
	}

	return nil
}

// CreateBackup creates a backup copy of the database file.
func CreateBackup(dbPath string, config *BackupConfig, log zerolog.Logger) (string, error) {
	if !config.Enabled {
		log.Info().Msg("Backup disabled, skipping backup creation")
		return "", nil
	}

	// Check if source file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return "", fmt.Errorf("database file does not exist: %s", dbPath)
	}

	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Ensure backups subdirectory exists
	backupsDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupsDir, 0750); err != nil {
		return "", fmt.Errorf("failed to create backups directory: %w", err)
	}

	// Generate backup filename
	backupPath := GenerateBackupFilename(dbPath)

	log.Info().Msgf("Creating backup (source: %s, destination: %s)", dbPath, backupPath)

	// Create the backup using SQLite native backup API
	if err := createSQLiteBackup(dbPath, backupPath, log); err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	// Get backup file info for logging
	backupInfo, err := os.Stat(backupPath)
	if err != nil {
		return "", fmt.Errorf("failed to get backup file info: %w", err)
	}

	log.Info().Msgf("Backup created successfully (file: %s, size: %d)", backupPath, backupInfo.Size())

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

	log.Info().Msgf("Cleaning up old backups (to_remove: %d, keeping: %d)", toRemove, maxBackups)

	// Remove the oldest backups
	for _, backup := range backupsToRemove {
		if err := os.Remove(backup.Filename); err != nil {
			log.Error().Msgf("Failed to remove backup (file: %s, error: %v)", backup.Filename, err)
			// Continue with cleanup even if one file fails
			continue
		}
		log.Debug().Msgf("Removed old backup (file: %s)", backup.Filename)
	}

	return nil
}
