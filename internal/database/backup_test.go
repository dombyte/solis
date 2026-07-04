package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestGenerateBackupFilename(t *testing.T) {
	// Test backup filename generation (simplified - no version or online distinction)
	got := GenerateBackupFilename("/path/to/db/solis.db")
	if !strings.HasSuffix(got, ".backup") {
		t.Errorf("Expected filename to end with .backup, got: %s", got)
	}
	if !strings.Contains(got, filepath.Join("/path/to/db", "backups")) {
		t.Errorf("Expected filename to be in backups subdirectory, got: %s", got)
	}
	if !strings.Contains(got, "solis.db.") {
		t.Errorf("Expected filename to contain database name, got: %s", got)
	}
	// Should not contain version markers
	if strings.Contains(got, ".v") {
		t.Errorf("Expected filename NOT to contain version marker, got: %s", got)
	}
	// Should not contain online markers
	if strings.Contains(got, "online") {
		t.Errorf("Expected filename NOT to contain 'online', got: %s", got)
	}
}

func TestExtractBackupInfo(t *testing.T) {
	// Test new simplified backup filename parsing
	filename := "/path/to/backups/solis.db.20260627_143022.backup"
	info, err := ExtractBackupInfo(filename)
	if err != nil {
		t.Fatalf("Failed to extract backup info: %v", err)
	}

	// Version is always 0 in simplified scheme
	if info.Version != 0 {
		t.Errorf("Expected version 0, got: %d", info.Version)
	}
	// IsOnline is always false in simplified scheme
	if info.IsOnline {
		t.Error("Expected IsOnline to be false")
	}
	if !info.Timestamp.Equal(time.Date(2026, 6, 27, 14, 30, 22, 0, time.UTC)) {
		t.Errorf("Expected timestamp 2026-06-27 14:30:22, got: %v", info.Timestamp)
	}

	// Test invalid filename (no timestamp)
	filename = "/path/to/backups/solis.db.backup"
	_, err = ExtractBackupInfo(filename)
	if err == nil {
		t.Error("Expected error for invalid filename")
	}

	// Test invalid filename (no dot separator)
	filename = "/path/to/backups/solisdb20260627_143022.backup"
	_, err = ExtractBackupInfo(filename)
	if err == nil {
		t.Error("Expected error for invalid filename")
	}
}

func TestCopyFile(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "backup_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create source file
	sourcePath := filepath.Join(tmpDir, "source.db")
	sourceContent := []byte("test database content")
	if err := os.WriteFile(sourcePath, sourceContent, 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Test copy
	destPath := filepath.Join(tmpDir, "dest.db")
	if err := copyFile(sourcePath, destPath); err != nil {
		t.Fatalf("Failed to copy file: %v", err)
	}

	// Verify destination file exists and has same content
	destContent, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}

	if string(destContent) != string(sourceContent) {
		t.Errorf("Destination content doesn't match source. Expected: %s, Got: %s", sourceContent, destContent)
	}

	// Test copy to non-existent directory
	destPath = filepath.Join(tmpDir, "nonexistent", "dest.db")
	if err := copyFile(sourcePath, destPath); err == nil {
		t.Error("Expected error when copying to non-existent directory")
	}
}

func TestCreateBackup(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "backup_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create database file
	dbPath := filepath.Join(tmpDir, "test.db")
	dbContent := []byte("test database content")
	if err := os.WriteFile(dbPath, dbContent, 0644); err != nil {
		t.Fatalf("Failed to create database file: %v", err)
	}

	// Test backup creation
	config := DefaultBackupConfig()
	config.Enabled = true

	backupPath, err := CreateBackup(dbPath, config)
	if err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Verify backup file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Fatal("Backup file does not exist")
	}

	// Verify backup file has same content
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read backup file: %v", err)
	}

	if string(backupContent) != string(dbContent) {
		t.Errorf("Backup content doesn't match database. Expected: %s, Got: %s", dbContent, backupContent)
	}

	// Test backup with disabled config
	config.Enabled = false
	backupPath, err = CreateBackup(dbPath, config)
	if err != nil {
		t.Fatalf("Expected no error when backup disabled, got: %v", err)
	}
	if backupPath != "" {
		t.Errorf("Expected empty backup path when disabled, got: %s", backupPath)
	}
}

func TestRestoreBackup(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "backup_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create backup file
	backupPath := filepath.Join(tmpDir, "backup.db")
	backupContent := []byte("backup database content")
	if err := os.WriteFile(backupPath, backupContent, 0644); err != nil {
		t.Fatalf("Failed to create backup file: %v", err)
	}

	// Test restore
	targetPath := filepath.Join(tmpDir, "restored.db")
	if err := RestoreBackup(backupPath, targetPath); err != nil {
		t.Fatalf("Failed to restore backup: %v", err)
	}

	// Verify restored file exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		t.Fatal("Restored file does not exist")
	}

	// Verify restored file has same content
	restoredContent, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("Failed to read restored file: %v", err)
	}

	if string(restoredContent) != string(backupContent) {
		t.Errorf("Restored content doesn't match backup. Expected: %s, Got: %s", backupContent, restoredContent)
	}
}

func TestCleanupBackups(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "backup_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create database file
	dbPath := filepath.Join(tmpDir, "test.db")
	if err := os.WriteFile(dbPath, []byte("db"), 0644); err != nil {
		t.Fatalf("Failed to create database file: %v", err)
	}

	// Create backups directory
	backupsDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupsDir, 0755); err != nil {
		t.Fatalf("Failed to create backups directory: %v", err)
	}

	// Create multiple backup files with different timestamps
	backupFiles := []string{
		filepath.Join(backupsDir, "test.db.v1.20260627_100000.backup"),
		filepath.Join(backupsDir, "test.db.v1.20260627_110000.backup"),
		filepath.Join(backupsDir, "test.db.v1.20260627_120000.backup"),
		filepath.Join(backupsDir, "test.db.v1.20260627_130000.backup"),
	}

	for _, file := range backupFiles {
		if err := os.WriteFile(file, []byte("backup"), 0644); err != nil {
			t.Fatalf("Failed to create backup file %s: %v", file, err)
		}
	}

	// Test cleanup with maxBackups = 2
	if err := CleanupBackups(dbPath, 2); err != nil {
		t.Fatalf("Failed to cleanup backups: %v", err)
	}

	// Check that only the 2 most recent backups remain
	remaining, err := ListBackups(dbPath)
	if err != nil {
		t.Fatalf("Failed to list backups: %v", err)
	}

	if len(remaining) != 2 {
		t.Errorf("Expected 2 remaining backups, got: %d", len(remaining))
	}

	// The 2 most recent should be the ones with latest timestamps
	// Note: ListBackups returns them sorted by timestamp (newest first)
	if !strings.Contains(remaining[0].Filename, "130000") {
		t.Errorf("Expected most recent backup to be 130000, got: %s", remaining[0].Filename)
	}
	if !strings.Contains(remaining[1].Filename, "120000") {
		t.Errorf("Expected second most recent backup to be 120000, got: %s", remaining[1].Filename)
	}
}

func TestListBackups(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "backup_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create database file
	dbPath := filepath.Join(tmpDir, "test.db")
	if err := os.WriteFile(dbPath, []byte("db"), 0644); err != nil {
		t.Fatalf("Failed to create database file: %v", err)
	}

	// Create backups directory
	backupsDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupsDir, 0755); err != nil {
		t.Fatalf("Failed to create backups directory: %v", err)
	}

	// Create backup files
	backupFiles := []string{
		filepath.Join(backupsDir, "test.db.v1.20260627_143022.backup"),
		filepath.Join(backupsDir, "test.db.online.20260627_153022.backup"),
		filepath.Join(backupsDir, "other_file.backup"), // Should be ignored
		filepath.Join(backupsDir, "test.db.v2.20260627_163022.backup"),
	}

	for _, file := range backupFiles {
		if err := os.WriteFile(file, []byte("backup"), 0644); err != nil {
			t.Fatalf("Failed to create backup file %s: %v", file, err)
		}
	}

	// Test list backups
	backups, err := ListBackups(dbPath)
	if err != nil {
		t.Fatalf("Failed to list backups: %v", err)
	}

	// Should return 3 backups (ignoring other_file.backup)
	if len(backups) != 3 {
		t.Errorf("Expected 3 backups, got: %d", len(backups))
	}

	// Check that backups are sorted by timestamp (newest first)
	for i := 1; i < len(backups); i++ {
		if backups[i-1].Timestamp.Before(backups[i].Timestamp) {
			t.Errorf("Backups not sorted by timestamp. Index %d: %v, Index %d: %v", i-1, backups[i-1].Timestamp, i, backups[i].Timestamp)
		}
	}
}

func TestDefaultBackupConfig(t *testing.T) {
	config := DefaultBackupConfig()
	if !config.Enabled {
		t.Error("Expected backup to be enabled by default")
	}
	if config.MaxBackups != 3 {
		t.Errorf("Expected max backups to be 3 by default, got: %d", config.MaxBackups)
	}
	if config.BackupInterval != 24*time.Hour {
		t.Errorf("Expected backup interval to be 24h by default, got: %v", config.BackupInterval)
	}
}

func TestSQLiteNativeBackup(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "sqlite_backup_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create source SQLite database with some content
	sourcePath := filepath.Join(tmpDir, "source.db")
	srcDB, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		t.Fatalf("Failed to open source database: %v", err)
	}
	defer srcDB.Close()

	// Create a table and insert some data
	if _, err := srcDB.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}
	if _, err := srcDB.Exec("INSERT INTO test (name) VALUES (?)", "test data"); err != nil {
		t.Fatalf("Failed to insert data: %v", err)
	}

	// Test SQLite native backup
	config := DefaultBackupConfig()
	config.Enabled = true

	backupResult, err := CreateBackup(sourcePath, config)
	if err != nil {
		t.Fatalf("Failed to create SQLite backup: %v", err)
	}
	if backupResult == "" {
		t.Fatal("Expected backup path to be returned")
	}

	// Verify backup file exists
	if _, err := os.Stat(backupResult); os.IsNotExist(err) {
		t.Fatalf("Backup file does not exist: %s", backupResult)
	}

	// Test SQLite native restore
	targetPath := filepath.Join(tmpDir, "restored.db")
	if err := RestoreBackup(backupResult, targetPath); err != nil {
		t.Fatalf("Failed to restore SQLite backup: %v", err)
	}

	// Verify restored file exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		t.Fatal("Restored file does not exist")
	}

	// Open the restored database and verify the data
	restoredDB, err := sql.Open("sqlite", targetPath)
	if err != nil {
		t.Fatalf("Failed to open restored database: %v", err)
	}
	defer restoredDB.Close()

	// Check that the table exists and has the correct data
	var count int
	if err := restoredDB.QueryRow("SELECT COUNT(*) FROM test WHERE name = ?", "test data").Scan(&count); err != nil {
		t.Fatalf("Failed to query restored database: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 row with 'test data', got: %d", count)
	}
}
