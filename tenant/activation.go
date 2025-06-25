// Package tenant provides tenant activation and status management.
package tenant

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// ActivateTenant creates tables and indexes for an inactive tenant
func ActivateTenant(ctx *Context) error {
	if ctx.Status == "active" {
		return nil // Already activated, trust it's correct
	}

	log.Printf("Activating tenant: %s", ctx.TenantID)
	start := time.Now()

	// Determine database type
	dbType := "sqlite3"
	if ctx.Database.UseTurso {
		dbType = "turso"
	}

	// Create tables idempotently
	if err := createTables(ctx.Database); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	if err := createIndexes(ctx.Database); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	// Verify tables actually exist before marking as active
	tablesExist, err := CheckTablesExist(ctx.Database)
	if err != nil {
		return fmt.Errorf("failed to verify tables: %w", err)
	}
	if !tablesExist {
		return fmt.Errorf("tables creation failed - not all tables exist")
	}

	// Only mark as active after confirming tables exist
	if err := updateTenantStatus(ctx.TenantID, "active", dbType); err != nil {
		return fmt.Errorf("failed to update tenant status: %w", err)
	}

	log.Printf("Tenant %s activated (%s) in %v", ctx.TenantID, dbType, time.Since(start))
	ctx.Status = "active"
	return nil
}

// CheckTablesExist verifies if all required tables exist
func CheckTablesExist(db *Database) (bool, error) {
	requiredTables := []string{
		"tractstacks", "menus", "resources", "files", "markdowns", "epinets",
		"leads", "fingerprints", "campaigns", "visits", "actions", "beliefs",
		"heldbeliefs", "stories", "transcript_overrides", "aai_tokens_used",
		"storyfragment_topics", "panes", "storyfragments", "files_resource",
		"storyfragment_panes", "file_panes", "storyfragment_has_topic",
		"storyfragment_details",
	}

	for _, tableName := range requiredTables {
		var name string
		query := "SELECT name FROM sqlite_master WHERE type='table' AND name=?"
		if db.UseTurso {
			// For Turso, use the same query (libsql supports sqlite_master)
			query = "SELECT name FROM sqlite_master WHERE type='table' AND name=?"
		}

		err := db.Conn.QueryRow(query, tableName).Scan(&name)
		if err == sql.ErrNoRows {
			return false, nil // Table doesn't exist
		} else if err != nil {
			return false, fmt.Errorf("failed to check table %s: %w", tableName, err)
		}
	}

	return true, nil
}

// createTables creates all required database tables
func createTables(db *Database) error {
	tables := []struct {
		name string
		sql  string
	}{
		{"tractstacks", "CREATE TABLE IF NOT EXISTS tractstacks (id TEXT PRIMARY KEY, title TEXT NOT NULL, slug TEXT NOT NULL UNIQUE, social_image_path TEXT)"},
		{"menus", "CREATE TABLE IF NOT EXISTS menus (id TEXT PRIMARY KEY, title TEXT NOT NULL, theme TEXT NOT NULL, options_payload TEXT NOT NULL)"},
		{"resources", "CREATE TABLE IF NOT EXISTS resources (id TEXT PRIMARY KEY, title TEXT NOT NULL, slug TEXT NOT NULL UNIQUE, category_slug TEXT, oneliner TEXT NOT NULL, options_payload TEXT NOT NULL, action_lisp TEXT)"},
		{"files", "CREATE TABLE IF NOT EXISTS files (id TEXT PRIMARY KEY, filename TEXT NOT NULL, alt_description TEXT NOT NULL, url TEXT NOT NULL, src_set TEXT)"},
		{"markdowns", "CREATE TABLE IF NOT EXISTS markdowns (id TEXT PRIMARY KEY, body TEXT NOT NULL)"},
		{"epinets", "CREATE TABLE IF NOT EXISTS epinets (id TEXT PRIMARY KEY, title TEXT NOT NULL, options_payload TEXT NOT NULL)"},
		{"leads", "CREATE TABLE IF NOT EXISTS leads (id TEXT PRIMARY KEY, first_name TEXT NOT NULL, email TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL, contact_persona TEXT NOT NULL, short_bio TEXT, encrypted_code TEXT, encrypted_email TEXT, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, changed TIMESTAMP)"},
		{"fingerprints", "CREATE TABLE IF NOT EXISTS fingerprints (id TEXT PRIMARY KEY, lead_id TEXT REFERENCES leads(id), created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)"},
		{"campaigns", "CREATE TABLE IF NOT EXISTS campaigns (id TEXT PRIMARY KEY, name TEXT NOT NULL, source TEXT, medium TEXT, term TEXT, content TEXT, http_referrer TEXT, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)"},
		{"visits", "CREATE TABLE IF NOT EXISTS visits (id TEXT PRIMARY KEY, fingerprint_id TEXT NOT NULL REFERENCES fingerprints(id), campaign_id TEXT REFERENCES campaigns(id), created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)"},
		{"actions", "CREATE TABLE IF NOT EXISTS actions (id TEXT PRIMARY KEY, object_id TEXT NOT NULL, object_type TEXT NOT NULL, duration INTEGER, visit_id TEXT NOT NULL REFERENCES visits(id), fingerprint_id TEXT NOT NULL REFERENCES fingerprints(id), verb TEXT NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)"},
		{"beliefs", "CREATE TABLE IF NOT EXISTS beliefs (id TEXT PRIMARY KEY, title TEXT NOT NULL, slug TEXT NOT NULL UNIQUE, scale TEXT NOT NULL, custom_values TEXT)"},
		{"heldbeliefs", "CREATE TABLE IF NOT EXISTS heldbeliefs (id TEXT PRIMARY KEY, belief_id TEXT NOT NULL REFERENCES beliefs(id), fingerprint_id TEXT NOT NULL REFERENCES fingerprints(id), verb TEXT NOT NULL, object TEXT, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)"},
		{"stories", "CREATE TABLE IF NOT EXISTS stories (id INTEGER PRIMARY KEY AUTOINCREMENT, transcript_id TEXT, uuid TEXT, data TEXT)"},
		{"transcript_overrides", "CREATE TABLE IF NOT EXISTS transcript_overrides (id INTEGER PRIMARY KEY AUTOINCREMENT, transcript_id TEXT, data TEXT)"},
		{"aai_tokens_used", "CREATE TABLE IF NOT EXISTS aai_tokens_used (id INTEGER PRIMARY KEY, timestamp DATETIME NOT NULL, tokens_used INTEGER NOT NULL)"},
		{"storyfragment_topics", "CREATE TABLE IF NOT EXISTS storyfragment_topics (id NUMERIC PRIMARY KEY, title TEXT NOT NULL)"},
		{"panes", "CREATE TABLE IF NOT EXISTS panes (id TEXT PRIMARY KEY, title TEXT NOT NULL, slug TEXT NOT NULL UNIQUE, created TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, changed TIMESTAMP, markdown_id TEXT REFERENCES markdowns(id), options_payload TEXT NOT NULL, is_context_pane BOOLEAN DEFAULT 0, pane_type TEXT NOT NULL)"},
		{"storyfragments", "CREATE TABLE IF NOT EXISTS storyfragments (id TEXT PRIMARY KEY, title TEXT NOT NULL, slug TEXT NOT NULL UNIQUE, social_image_path TEXT, tailwind_background_colour TEXT, created TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, changed TIMESTAMP, menu_id TEXT REFERENCES menus(id), tractstack_id TEXT NOT NULL REFERENCES tractstacks(id))"},
		{"files_resource", "CREATE TABLE IF NOT EXISTS files_resource (id TEXT PRIMARY KEY, resource_id TEXT NOT NULL REFERENCES resources(id), file_id TEXT NOT NULL REFERENCES files(id), UNIQUE(resource_id, file_id))"},
		{"storyfragment_panes", "CREATE TABLE IF NOT EXISTS storyfragment_panes (id TEXT PRIMARY KEY, storyfragment_id TEXT NOT NULL REFERENCES storyfragments(id), pane_id TEXT NOT NULL REFERENCES panes(id), weight INTEGER NOT NULL, UNIQUE(storyfragment_id, pane_id))"},
		{"file_panes", "CREATE TABLE IF NOT EXISTS file_panes (id TEXT PRIMARY KEY, file_id TEXT NOT NULL REFERENCES files(id), pane_id TEXT NOT NULL REFERENCES panes(id), UNIQUE(file_id, pane_id))"},
		{"storyfragment_has_topic", "CREATE TABLE IF NOT EXISTS storyfragment_has_topic (id NUMERIC PRIMARY KEY, storyfragment_id TEXT NOT NULL REFERENCES storyfragments(id), topic_id NUMERIC NOT NULL REFERENCES storyfragment_topics(id))"},
		{"storyfragment_details", "CREATE TABLE IF NOT EXISTS storyfragment_details (id NUMERIC PRIMARY KEY, storyfragment_id TEXT NOT NULL REFERENCES storyfragments(id), description TEXT NOT NULL)"},
	}

	for _, t := range tables {
		var name string
		err := db.Conn.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, t.name).Scan(&name)
		if err == sql.ErrNoRows {
			if _, err := db.Conn.Exec(t.sql); err != nil {
				return fmt.Errorf("failed to create table %s: %w", t.name, err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to check table %s existence: %w", t.name, err)
		}
	}

	return nil
}

// createIndexes creates all required database indexes
func createIndexes(db *Database) error {
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_storyfragments_slug ON storyfragments(slug)",
		"CREATE INDEX IF NOT EXISTS idx_storyfragment_tractstack_id ON storyfragments(tractstack_id)",
		"CREATE INDEX IF NOT EXISTS idx_storyfragment_menu_id ON storyfragments(menu_id)",
		"CREATE INDEX IF NOT EXISTS idx_storyfragment_pane_storyfragment_id ON storyfragment_panes(storyfragment_id)",
		"CREATE INDEX IF NOT EXISTS idx_storyfragment_pane_pane_id ON storyfragment_panes(pane_id)",
		"CREATE INDEX IF NOT EXISTS idx_file_pane_file_id ON file_panes(file_id)",
		"CREATE INDEX IF NOT EXISTS idx_file_pane_pane_id ON file_panes(pane_id)",
		"CREATE INDEX IF NOT EXISTS idx_pane_markdown_id ON panes(markdown_id)",
		"CREATE INDEX IF NOT EXISTS idx_visits_fingerprint_id ON visits(fingerprint_id)",
		"CREATE INDEX IF NOT EXISTS idx_visits_campaign_id ON visits(campaign_id)",
		"CREATE INDEX IF NOT EXISTS idx_fingerprints_lead_id ON fingerprints(lead_id)",
		"CREATE INDEX IF NOT EXISTS idx_leads_email ON leads(email)",
		"CREATE INDEX IF NOT EXISTS idx_actions_object ON actions(object_id, object_type)",
		"CREATE INDEX IF NOT EXISTS idx_actions_visit_id ON actions(visit_id)",
		"CREATE INDEX IF NOT EXISTS idx_actions_fingerprint_id ON actions(fingerprint_id)",
		"CREATE INDEX IF NOT EXISTS idx_actions_verb ON actions(verb)",
		"CREATE INDEX IF NOT EXISTS idx_beliefs_slug ON beliefs(slug)",
		"CREATE INDEX IF NOT EXISTS idx_heldbeliefs_belief_id ON heldbeliefs(belief_id)",
		"CREATE INDEX IF NOT EXISTS idx_heldbeliefs_fingerprint_id ON heldbeliefs(fingerprint_id)",
		"CREATE INDEX IF NOT EXISTS idx_heldbeliefs_composite ON heldbeliefs(fingerprint_id, belief_id)",
		"CREATE INDEX IF NOT EXISTS idx_panes_slug ON panes(slug)",
		"CREATE INDEX IF NOT EXISTS idx_resources_slug ON resources(slug)",
		"CREATE INDEX IF NOT EXISTS idx_resources_category ON resources(category_slug)",
		"CREATE INDEX IF NOT EXISTS idx_storyfragment_panes_weight ON storyfragment_panes(weight)",
		"CREATE INDEX IF NOT EXISTS idx_aai_tokens_used_timestamp ON aai_tokens_used(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_storyfragment_has_topic_storyfragment_id ON storyfragment_has_topic(storyfragment_id)",
		"CREATE INDEX IF NOT EXISTS idx_storyfragment_has_topic_topic_id ON storyfragment_has_topic(topic_id)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_storyfragment_has_topic_unique ON storyfragment_has_topic(storyfragment_id, topic_id)",
		"CREATE INDEX IF NOT EXISTS idx_storyfragment_details_storyfragment_id ON storyfragment_details(storyfragment_id)",
		"CREATE INDEX IF NOT EXISTS idx_files_resource_resource_id ON files_resource(resource_id)",
		"CREATE INDEX IF NOT EXISTS idx_files_resource_file_id ON files_resource(file_id)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_files_resource_unique ON files_resource(resource_id, file_id)",
	}

	for _, indexSQL := range indexes {
		if _, err := db.Conn.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// updateTenantStatus updates the tenant status in the registry
func updateTenantStatus(tenantID, status, dbType string) error {
	registryPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", "t8k", "tenants.json")

	// Load current registry
	registry, err := LoadTenantRegistry()
	if err != nil {
		return err
	}

	// Update status and database type
	if tenantInfo, exists := registry.Tenants[tenantID]; exists {
		tenantInfo.Status = status
		if dbType != "" {
			tenantInfo.DatabaseType = dbType
		}
		registry.Tenants[tenantID] = tenantInfo
	}

	// Ensure directory exists
	registryDir := filepath.Dir(registryPath)
	if err := os.MkdirAll(registryDir, 0755); err != nil {
		return fmt.Errorf("failed to create registry directory: %w", err)
	}

	// Write back to file
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal registry: %w", err)
	}

	if err := os.WriteFile(registryPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write registry: %w", err)
	}

	return nil
}
