package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestBackupFilename_PlainTimestamp(t *testing.T) {
	// Every backup (legacy, pre-migration, periodic) uses one filename format:
	// <db>.<timestamp>.backup, without version or "online" markers.
	dbPath := filepath.Join(t.TempDir(), "test.db")
	require.NoError(t, createTestDB(dbPath))
	cfg := &BackupConfig{Enabled: true, MaxBackups: 3, BackupInterval: 24 * time.Hour}

	backupPath, err := CreateBackup(context.Background(), dbPath, cfg, time.Now(), zerolog.Nop())
	require.NoError(t, err)

	assert.True(t, strings.HasSuffix(backupPath, ".backup"), backupPath)
	assert.NotContains(t, backupPath, "online")
	assert.NotContains(t, backupPath, ".v")
	_, err = ExtractBackupInfo(backupPath)
	assert.NoError(t, err)
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
	backupPath, err := CreateBackup(context.Background(), dbPath, configBackup, time.Now(), zerolog.Nop())
	if err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Verify backup was created with consistent naming
	if !strings.Contains(backupPath, ".backup") {
		t.Errorf("Expected backup to have .backup extension, got: %s", backupPath)
	}

	t.Logf("Backup created before migration: %s", backupPath)
}
