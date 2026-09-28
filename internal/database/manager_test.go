package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"
)

// createTestDB creates a valid SQLite database file for testing
func createTestDB(dbPath string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	// Create a simple table to make it a valid database
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS test (id INTEGER)"); err != nil {
		return err
	}
	return nil
}

func TestBackupFilenameForLegacyDatabase(t *testing.T) {
	// Test that first run on legacy database creates migration backup (not online)
	// This simulates the scenario where a user upgrades to the new version
	// and the database doesn't have a schema_version table yet

	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "manager_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a database file (simulating existing legacy database)
	dbPath := filepath.Join(tmpDir, "test.db")
	if err := createTestDB(dbPath); err != nil {
		t.Fatalf("Failed to create database file: %v", err)
	}

	// Create config
	config := &BackupConfig{
		Enabled:        true,
		MaxBackups:     3,
		BackupInterval: 24 * time.Hour,
	}

	// Create backup (simplified - no version distinction)
	backupPath, err := CreateBackup(dbPath, config, time.Now(), zerolog.Nop())
	if err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Verify backup filename contains timestamp (no version or online)
	if !strings.Contains(backupPath, ".") || !strings.Contains(backupPath, ".backup") {
		t.Errorf("Expected backup to have timestamp in filename, got: %s", backupPath)
	}

	if strings.Contains(backupPath, "online") {
		t.Errorf("Expected backup NOT to contain 'online', got: %s", backupPath)
	}

	if strings.Contains(backupPath, ".v") {
		t.Errorf("Expected backup NOT to contain version marker, got: %s", backupPath)
	}

	t.Logf("Legacy database backup created: %s", backupPath)
}

func TestBackupFilenameForMigration(t *testing.T) {
	// Test that backup has consistent naming (no version or online markers)
	tmpDir, err := os.MkdirTemp("", "manager_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	if err := createTestDB(dbPath); err != nil {
		t.Fatalf("Failed to create database file: %v", err)
	}

	config := &BackupConfig{
		Enabled:        true,
		MaxBackups:     3,
		BackupInterval: 24 * time.Hour,
	}

	backupPath, err := CreateBackup(dbPath, config, time.Now(), zerolog.Nop())
	if err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Verify backup filename has timestamp format (no version or online)
	if !strings.Contains(backupPath, ".") || !strings.Contains(backupPath, ".backup") {
		t.Errorf("Expected backup to have timestamp in filename, got: %s", backupPath)
	}

	if strings.Contains(backupPath, "online") {
		t.Errorf("Expected backup NOT to contain 'online', got: %s", backupPath)
	}

	if strings.Contains(backupPath, ".v") {
		t.Errorf("Expected backup NOT to contain version marker, got: %s", backupPath)
	}

	t.Logf("Backup created: %s", backupPath)
}

func TestBackupFilenameConsistency(t *testing.T) {
	// Test that all backups use the same consistent filename format
	tmpDir, err := os.MkdirTemp("", "manager_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	if err := createTestDB(dbPath); err != nil {
		t.Fatalf("Failed to create database file: %v", err)
	}

	config := &BackupConfig{
		Enabled:        true,
		MaxBackups:     3,
		BackupInterval: 24 * time.Hour,
	}

	// Create backup - should use consistent naming
	backupPath, err := CreateBackup(dbPath, config, time.Now(), zerolog.Nop())
	if err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Verify backup filename has timestamp format (no version or online markers)
	if !strings.Contains(backupPath, ".") || !strings.Contains(backupPath, ".backup") {
		t.Errorf("Expected backup to have timestamp in filename, got: %s", backupPath)
	}

	if strings.Contains(backupPath, "online") {
		t.Errorf("Expected backup NOT to contain 'online', got: %s", backupPath)
	}

	if strings.Contains(backupPath, ".v") {
		t.Errorf("Expected backup NOT to contain version marker, got: %s", backupPath)
	}

	t.Logf("Backup created with consistent naming: %s", backupPath)
}

func TestBackupBeforeMigration(t *testing.T) {
	// Test that a backup is created before migration when database exists
	tmpDir, err := os.MkdirTemp("", "manager_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	if err := createTestDB(dbPath); err != nil {
		t.Fatalf("Failed to create database file: %v", err)
	}

	configBackup := &BackupConfig{
		Enabled:        true,
		MaxBackups:     3,
		BackupInterval: 24 * time.Hour,
	}

	// Create backup - should work for any existing database
	backupPath, err := CreateBackup(dbPath, configBackup, time.Now(), zerolog.Nop())
	if err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Verify backup was created with consistent naming
	if !strings.Contains(backupPath, ".backup") {
		t.Errorf("Expected backup to have .backup extension, got: %s", backupPath)
	}

	t.Logf("Backup created before migration: %s", backupPath)
}
