package database

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dombyte/solis/internal/logging"
)

// backupLogger is the package-level logger for backup operations.
var backupLogger = logging.NewComponentLogger("database.backup")

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
	Filename    string
	Timestamp   time.Time
	Size        int64
	IsOnline    bool // Deprecated: all backups use the same format now
	Version     int  // Deprecated: all backups are version 0 in simplified scheme
}

// DefaultBackupConfig returns a BackupConfig with sensible defaults.
func DefaultBackupConfig() *BackupConfig {
	return &BackupConfig{
		Enabled:        true,
		MaxBackups:     3,
		BackupInterval: 24 * time.Hour,
	}
}

// GenerateBackupFilename generates a backup filename with timestamp.
// Format: {name}.{timestamp}.backup
func GenerateBackupFilename(dbPath string) string {
	dbName := filepath.Base(dbPath)
	dir := filepath.Dir(dbPath)
	timestamp := time.Now().Format("20060102_150405")

	prefix := fmt.Sprintf("%s.%s.backup", dbName, timestamp)

	return filepath.Join(dir, prefix)
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
		Filename:  filename,
		Version:   0, // All backups are version 0 in simplified scheme
		IsOnline:  false, // No distinction between online and migration backups
	}

	if t, err := time.Parse("20060102_150405", timestampStr); err == nil {
		info.Timestamp = t
		return info, nil
	}

	return nil, fmt.Errorf("could not parse timestamp in backup filename: %s", filename)
}

// copyFile copies a file from src to dst using simple file copy.
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer source.Close()

	// Check if source file exists and get its info
	sourceInfo, err := source.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}

	// Create destination file
	destination, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destination.Close()

	// Copy data
	copied, err := io.Copy(destination, source)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// Verify size
	if copied != sourceInfo.Size() {
		return fmt.Errorf("incomplete copy: expected %d bytes, copied %d bytes", sourceInfo.Size(), copied)
	}

	return nil
}

// CreateBackup creates a backup copy of the database file.
func CreateBackup(dbPath string, config *BackupConfig) (string, error) {
	if !config.Enabled {
		backupLogger.Info().Msg("Backup disabled, skipping backup creation")
		return "", nil
	}

	// Check if source file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return "", fmt.Errorf("database file does not exist: %s", dbPath)
	}

	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Generate backup filename
	backupPath := GenerateBackupFilename(dbPath)

	backupLogger.Info().Msgf("Creating backup (source: %s, destination: %s)", dbPath, backupPath)

	// Create the backup
	if err := copyFile(dbPath, backupPath); err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	// Verify backup file
	backupInfo, err := os.Stat(backupPath)
	if err != nil {
		return "", fmt.Errorf("failed to verify backup file: %w", err)
	}

	if backupInfo.Size() == 0 {
		return "", fmt.Errorf("backup file is empty")
	}

	backupLogger.Info().Msgf("Backup created successfully (file: %s, size: %d)", backupPath, backupInfo.Size())

	return backupPath, nil
}

// RestoreBackup restores a database from a backup file.
func RestoreBackup(backupPath string, targetPath string) error {
	// Check if backup file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file does not exist: %s", backupPath)
	}

	// Ensure target directory exists
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	backupLogger.Info().Msgf("Restoring backup (source: %s, target: %s)", backupPath, targetPath)

	// Create the restore
	if err := copyFile(backupPath, targetPath); err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	// Verify restored file
	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		return fmt.Errorf("failed to verify restored file: %w", err)
	}

	if targetInfo.Size() == 0 {
		return fmt.Errorf("restored file is empty")
	}

	backupLogger.Info().Msgf("Backup restored successfully (file: %s, size: %d)", targetPath, targetInfo.Size())

	return nil
}

// ListBackups lists all backup files in the same directory as the database.
// Returns both migration backups and online backups, sorted by timestamp (newest first).
func ListBackups(dbPath string) ([]BackupInfo, error) {
	dir := filepath.Dir(dbPath)
	if dir == "" {
		dir = "."
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	backups := make([]BackupInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		if !strings.HasSuffix(filename, ".backup") {
			continue
		}

		fullPath := filepath.Join(dir, filename)
		info, err := ExtractBackupInfo(fullPath)
		if err != nil {
			// Skip files that don't match our naming pattern
			continue
		}

		// Get file size
		fileInfo, err := os.Stat(fullPath)
		if err != nil {
			continue
		}
		info.Size = fileInfo.Size()

		backups = append(backups, *info)
	}

	// Sort by timestamp (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp.After(backups[j].Timestamp)
	})

	return backups, nil
}

// CleanupBackups removes old backup files, keeping only the most recent MaxBackups.
// If maxBackups <= 0, keeps all backups.
func CleanupBackups(dbPath string, maxBackups int) error {
	if maxBackups <= 0 {
		backupLogger.Debug().Msg("Backup cleanup skipped: maxBackups <= 0")
		return nil
	}

	backups, err := ListBackups(dbPath)
	if err != nil {
		return fmt.Errorf("failed to list backups: %w", err)
	}

	if len(backups) <= maxBackups {
		backupLogger.Debug().Msgf("No cleanup needed (backups_count: %d, max_backups: %d)", len(backups), maxBackups)
		return nil
	}

	// Calculate how many to remove
	toRemove := len(backups) - maxBackups
	backupsToRemove := backups[maxBackups:]

	backupLogger.Info().Msgf("Cleaning up old backups (to_remove: %d, keeping: %d)", toRemove, maxBackups)

	// Remove the oldest backups
	for _, backup := range backupsToRemove {
		if err := os.Remove(backup.Filename); err != nil {
			backupLogger.Error().Msgf("Failed to remove backup (file: %s, error: %v)", backup.Filename, err)
			// Continue with cleanup even if one file fails
			continue
		}
		backupLogger.Debug().Msgf("Removed old backup (file: %s)", backup.Filename)
	}

	return nil
}