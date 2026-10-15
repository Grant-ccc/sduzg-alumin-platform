package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestHistoryWikiMigrationDefinesCoreTablesAndAttachmentRules(t *testing.T) {
	migration, err := os.ReadFile("008_add_history_wiki.sql")
	if err != nil {
		t.Fatalf("read history wiki migration: %v", err)
	}
	content := string(migration)
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS history_entries",
		"CREATE TABLE IF NOT EXISTS history_entry_versions",
		"CREATE TABLE IF NOT EXISTS history_contributions",
		"CREATE TABLE IF NOT EXISTS history_attachments",
		"data_domain_id",
		"author_user_id",
		"review_comment",
		"consent_confirmed",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("history migration does not contain %q", want)
		}
	}
}

func TestMigrationTrackingIncludesHistoryWikiMigration(t *testing.T) {
	migration, err := os.ReadFile("009_add_migration_tracking.sql")
	if err != nil {
		t.Fatalf("read migration tracking: %v", err)
	}
	content := string(migration)
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS schema_migrations",
		"'008_add_history_wiki.sql'",
		"'009_add_migration_tracking.sql'",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("migration tracking does not contain %q", want)
		}
	}
}
