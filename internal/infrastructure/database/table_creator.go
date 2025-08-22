// Package database provides tenant instantiation
package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
)

// TableCreator handles the creation of the database schema for a new tenant.
type TableCreator struct{}

// NewTableCreator creates a new TableCreator.
func NewTableCreator() *TableCreator {
	return &TableCreator{}
}

// CreateSchema executes all necessary queries to build the tenant's database tables and indexes.
func (tc *TableCreator) CreateSchema(db *sql.DB) error {
	for _, tableSQL := range tables {
		if _, err := db.Exec(tableSQL); err != nil {
			return fmt.Errorf("failed to create table for query [%s]: %w", tableSQL, err)
		}
	}

	for _, indexSQL := range indexes {
		if _, err := db.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create index for query [%s]: %w", indexSQL, err)
		}
	}
	return nil
}

// SeedInitialContent adds the default content required for a new tenant to function.
func (tc *TableCreator) SeedInitialContent(db *sql.DB) error {
	// Idempotently create the default "HELLO" TractStack.
	var tractStackID string
	err := db.QueryRow("SELECT id FROM tractstacks WHERE slug = 'HELLO'").Scan(&tractStackID)
	if err == sql.ErrNoRows {
		tractStackID = security.GenerateULID()
		_, err = db.Exec(`INSERT INTO tractstacks (id, title, slug) VALUES (?, ?, ?)`, tractStackID, "Tract Stack", "HELLO")
		if err != nil {
			return fmt.Errorf("failed to insert default tractstack: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to check for default tractstack: %w", err)
	}

	// Idempotently create the default "hello" StoryFragment.
	var storyFragmentExists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM storyfragments WHERE slug = 'hello')").Scan(&storyFragmentExists)
	if err != nil {
		return fmt.Errorf("failed to check for storyfragment existence: %w", err)
	}

	if !storyFragmentExists {
		storyFragmentID := security.GenerateULID()
		now := time.Now().UTC()
		_, err = db.Exec(`INSERT INTO storyfragments (id, title, slug, tractstack_id, created, changed) VALUES (?, ?, ?, ?, ?, ?)`,
			storyFragmentID, "Hello", "hello", tractStackID, now, now)
		if err != nil {
			return fmt.Errorf("failed to insert default storyfragment: %w", err)
		}
	}

	// Idempotently create the default epinet.
	var epinetExists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM epinets WHERE title = 'My Tract Stack')").Scan(&epinetExists)
	if err != nil {
		return fmt.Errorf("failed to check for epinet existence: %w", err)
	}

	if !epinetExists {
		epinetID := security.GenerateULID()
		optionsPayload := `{"promoted":true,"steps":[{"title":"Entered Site","gateType":"commitmentAction","objectType":"StoryFragment","values":["ENTERED"]},{"title":"Experienced Site","gateType":"commitmentAction","objectType":"StoryFragment","values":["PAGEVIEWED"]},{"title":"Experienced Content","gateType":"commitmentAction","objectType":"Pane","values":["READ","GLOSSED","CLICKED","WATCHED"]}]}`
		_, err = db.Exec(`INSERT INTO epinets (id, title, options_payload) VALUES (?, ?, ?)`, epinetID, "My Tract Stack", optionsPayload)
		if err != nil {
			return fmt.Errorf("failed to insert default epinet: %w", err)
		}
	}

	return nil
}

// Schema definitions extracted from schema.json
var tables = []string{
	`CREATE TABLE IF NOT EXISTS tractstacks (id TEXT PRIMARY KEY, title TEXT NOT NULL, slug TEXT NOT NULL UNIQUE, social_image_path TEXT)`,
	`CREATE TABLE IF NOT EXISTS menus (id TEXT PRIMARY KEY, title TEXT NOT NULL, theme TEXT NOT NULL, options_payload TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS resources (id TEXT PRIMARY KEY, title TEXT NOT NULL, slug TEXT NOT NULL UNIQUE, category_slug TEXT, oneliner TEXT NOT NULL, options_payload TEXT NOT NULL, action_lisp TEXT)`,
	`CREATE TABLE IF NOT EXISTS files_resource (id TEXT PRIMARY KEY, resource_id TEXT NOT NULL REFERENCES resources(id), file_id TEXT NOT NULL REFERENCES files(id), UNIQUE(resource_id, file_id))`,
	`CREATE TABLE IF NOT EXISTS epinets (id TEXT PRIMARY KEY, title TEXT NOT NULL, options_payload TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS files (id TEXT PRIMARY KEY, filename TEXT NOT NULL, alt_description TEXT NOT NULL, url TEXT NOT NULL, src_set TEXT)`,
	`CREATE TABLE IF NOT EXISTS markdowns (id TEXT PRIMARY KEY, body TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS storyfragments (id TEXT PRIMARY KEY, title TEXT NOT NULL, slug TEXT NOT NULL UNIQUE, social_image_path TEXT, tailwind_background_colour TEXT, created TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, changed TIMESTAMP, menu_id TEXT REFERENCES menus(id), tractstack_id TEXT NOT NULL REFERENCES tractstacks(id))`,
	`CREATE TABLE IF NOT EXISTS panes (id TEXT PRIMARY KEY, title TEXT NOT NULL, slug TEXT NOT NULL UNIQUE, created TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, changed TIMESTAMP, markdown_id TEXT REFERENCES markdowns(id), options_payload TEXT NOT NULL, is_context_pane BOOLEAN DEFAULT 0, pane_type TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS storyfragment_panes (id TEXT PRIMARY KEY, storyfragment_id TEXT NOT NULL REFERENCES storyfragments(id), pane_id TEXT NOT NULL REFERENCES panes(id), weight INTEGER NOT NULL, UNIQUE(storyfragment_id, pane_id))`,
	`CREATE TABLE IF NOT EXISTS file_panes (id TEXT PRIMARY KEY, file_id TEXT NOT NULL REFERENCES files(id), pane_id TEXT NOT NULL REFERENCES panes(id), UNIQUE(file_id, pane_id))`,
	`CREATE TABLE IF NOT EXISTS visits (id TEXT PRIMARY KEY, fingerprint_id TEXT NOT NULL REFERENCES fingerprints(id), campaign_id TEXT REFERENCES campaigns(id), created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
	`CREATE TABLE IF NOT EXISTS fingerprints (id TEXT PRIMARY KEY, lead_id TEXT REFERENCES leads(id), created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
	`CREATE TABLE IF NOT EXISTS leads (id TEXT PRIMARY KEY, first_name TEXT NOT NULL, email TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL, contact_persona TEXT NOT NULL, short_bio TEXT, encrypted_code TEXT, encrypted_email TEXT, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, changed TIMESTAMP)`,
	`CREATE TABLE IF NOT EXISTS campaigns (id TEXT PRIMARY KEY, name TEXT NOT NULL, source TEXT, medium TEXT, term TEXT, content TEXT, http_referrer TEXT, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
	`CREATE TABLE IF NOT EXISTS actions (id TEXT PRIMARY KEY, object_id TEXT NOT NULL, object_type TEXT NOT NULL, duration INTEGER, visit_id TEXT NOT NULL REFERENCES visits(id), fingerprint_id TEXT NOT NULL REFERENCES fingerprints(id), verb TEXT NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
	`CREATE TABLE IF NOT EXISTS beliefs (id TEXT PRIMARY KEY, title TEXT NOT NULL, slug TEXT NOT NULL UNIQUE, scale TEXT NOT NULL, custom_values TEXT)`,
	`CREATE TABLE IF NOT EXISTS heldbeliefs (id TEXT PRIMARY KEY, belief_id TEXT NOT NULL REFERENCES beliefs(id), fingerprint_id TEXT NOT NULL REFERENCES fingerprints(id), verb TEXT NOT NULL, object TEXT, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
	`CREATE TABLE IF NOT EXISTS stories (id INTEGER PRIMARY KEY AUTOINCREMENT, transcript_id TEXT, uuid TEXT, data TEXT)`,
	`CREATE TABLE IF NOT EXISTS transcript_overrides (id INTEGER PRIMARY KEY AUTOINCREMENT, transcript_id TEXT, data TEXT)`,
	`CREATE TABLE IF NOT EXISTS aai_tokens_used (id INTEGER PRIMARY KEY, timestamp DATETIME NOT NULL, tokens_used INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS storyfragment_topics (id NUMERIC PRIMARY KEY, title TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS storyfragment_has_topic (id NUMERIC PRIMARY KEY, storyfragment_id TEXT NOT NULL REFERENCES storyfragments(id), topic_id NUMERIC NOT NULL REFERENCES storyfragment_topics(id))`,
	`CREATE TABLE IF NOT EXISTS storyfragment_details (id NUMERIC PRIMARY KEY, storyfragment_id TEXT NOT NULL REFERENCES storyfragments(id), description TEXT NOT NULL)`,
}

var indexes = []string{
	`CREATE INDEX IF NOT EXISTS idx_storyfragments_slug ON storyfragments(slug)`,
	`CREATE INDEX IF NOT EXISTS idx_storyfragment_tractstack_id ON storyfragments(tractstack_id)`,
	`CREATE INDEX IF NOT EXISTS idx_storyfragment_menu_id ON storyfragments(menu_id)`,
	`CREATE INDEX IF NOT EXISTS idx_storyfragment_pane_storyfragment_id ON storyfragment_panes(storyfragment_id)`,
	`CREATE INDEX IF NOT EXISTS idx_storyfragment_pane_pane_id ON storyfragment_panes(pane_id)`,
	`CREATE INDEX IF NOT EXISTS idx_file_pane_file_id ON file_panes(file_id)`,
	`CREATE INDEX IF NOT EXISTS idx_file_pane_pane_id ON file_panes(pane_id)`,
	`CREATE INDEX IF NOT EXISTS idx_pane_markdown_id ON panes(markdown_id)`,
	`CREATE INDEX IF NOT EXISTS idx_visits_fingerprint_id ON visits(fingerprint_id)`,
	`CREATE INDEX IF NOT EXISTS idx_visits_campaign_id ON visits(campaign_id)`,
	`CREATE INDEX IF NOT EXISTS idx_fingerprints_lead_id ON fingerprints(lead_id)`,
	`CREATE INDEX IF NOT EXISTS idx_leads_email ON leads(email)`,
	`CREATE INDEX IF NOT EXISTS idx_actions_object ON actions(object_id, object_type)`,
	`CREATE INDEX IF NOT EXISTS idx_actions_visit_id ON actions(visit_id)`,
	`CREATE INDEX IF NOT EXISTS idx_actions_fingerprint_id ON actions(fingerprint_id)`,
	`CREATE INDEX IF NOT EXISTS idx_actions_verb ON actions(verb)`,
	`CREATE INDEX IF NOT EXISTS idx_beliefs_slug ON beliefs(slug)`,
	`CREATE INDEX IF NOT EXISTS idx_heldbeliefs_belief_id ON heldbeliefs(belief_id)`,
	`CREATE INDEX IF NOT EXISTS idx_heldbeliefs_fingerprint_id ON heldbeliefs(fingerprint_id)`,
	`CREATE INDEX IF NOT EXISTS idx_heldbeliefs_composite ON heldbeliefs(fingerprint_id, belief_id)`,
	`CREATE INDEX IF NOT EXISTS idx_panes_slug ON panes(slug)`,
	`CREATE INDEX IF NOT EXISTS idx_resources_slug ON resources(slug)`,
	`CREATE INDEX IF NOT EXISTS idx_resources_category ON resources(category_slug)`,
	`CREATE INDEX IF NOT EXISTS idx_storyfragment_panes_weight ON storyfragment_panes(weight)`,
	`CREATE INDEX IF NOT EXISTS idx_aai_tokens_used_timestamp ON aai_tokens_used(timestamp)`,
	`CREATE INDEX IF NOT EXISTS idx_storyfragment_has_topic_storyfragment_id ON storyfragment_has_topic(storyfragment_id)`,
	`CREATE INDEX IF NOT EXISTS idx_storyfragment_has_topic_topic_id ON storyfragment_has_topic(topic_id)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_storyfragment_has_topic_unique ON storyfragment_has_topic(storyfragment_id, topic_id)`,
	`CREATE INDEX IF NOT EXISTS idx_storyfragment_details_storyfragment_id ON storyfragment_details(storyfragment_id)`,
	`CREATE INDEX IF NOT EXISTS idx_files_resource_resource_id ON files_resource(resource_id)`,
	`CREATE INDEX IF NOT EXISTS idx_files_resource_file_id ON files_resource(file_id)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_files_resource_unique ON files_resource(resource_id, file_id)`,
}
