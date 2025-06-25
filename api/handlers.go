// Package api provides HTTP handlers and database connectivity for the application's API.
package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/utils"
	"github.com/gin-gonic/gin"
)

// DBStatusHandler checks if all required tables exist in the database and creates them if missing.
func DBStatusHandler(c *gin.Context) {
	start := time.Now()
	db, err := NewDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to connect to database: %v", err)})
		return
	}
	defer db.Conn.Close()
	log.Printf("DB connection took %v", time.Since(start))

	// Tables in dependency order
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

	tableStart := time.Now()
	allTablesExist := true
	for _, t := range tables {
		var name string
		err := db.Conn.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, t.name).Scan(&name)
		if err == sql.ErrNoRows {
			if _, err := db.Conn.Exec(t.sql); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create table %s: %v", t.name, err)})
				return
			}
			allTablesExist = false
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to check table %s existence: %v", t.name, err)})
			return
		}
	}
	log.Printf("Table checks/creation took %v", time.Since(tableStart))

	indexStart := time.Now()
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create index: %v", err)})
			return
		}
	}
	log.Printf("Index checks/creation took %v", time.Since(indexStart))
	log.Printf("Total handler execution took %v", time.Since(start))

	c.JSON(http.StatusOK, gin.H{"allTablesExist": allTablesExist})
}

func VisitHandler(c *gin.Context) {
	var req models.VisitRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	fpID, _ := c.Cookie("fp_id")
	visitID, _ := c.Cookie("visit_id")
	consent, _ := c.Cookie("consent")
	profileToken, _ := c.Cookie("profile_token")
	var profile *models.Profile
	if profileToken != "" {
		claims, err := utils.ValidateJWT(profileToken)
		if err == nil {
			profile = utils.GetProfileFromClaims(claims)
		}
	}
	if profile == nil && req.EncryptedEmail != nil && req.EncryptedCode != nil {
		profile = validateEncryptedCredentials(*req.EncryptedEmail, *req.EncryptedCode)
	}
	hasProfile := profile != nil
	consentValue := consent
	if hasProfile {
		consentValue = "1"
	} else if req.Consent != nil {
		consentValue = *req.Consent
	}
	if fpID == "" || (hasProfile || consentValue == "1") {
		if req.Fingerprint != nil && *req.Fingerprint != "" {
			fpID = *req.Fingerprint
		} else {
			fpID = utils.GenerateULID()
		}
	}
	fpExpiry := time.Hour
	if hasProfile || consentValue == "1" {
		fpExpiry = 30 * 24 * time.Hour
	}
	c.SetCookie("fp_id", fpID, int(fpExpiry.Seconds()), "/", "", false, true)
	if visitID == "" {
		if req.VisitID != nil && *req.VisitID != "" {
			visitID = *req.VisitID
		} else {
			visitID = utils.GenerateULID()
		}
	}
	c.SetCookie("visit_id", visitID, 24*3600, "/", "", false, true)
	if hasProfile || consentValue == "1" {
		c.SetCookie("consent", "1", 30*24*3600, "/", "", false, true)
	}
	if hasProfile {
		token, err := utils.GenerateProfileToken(profile)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate profile token"})
			return
		}
		c.SetCookie("profile_token", token, 30*24*3600, "/", "", false, true)
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func SseHandler(c *gin.Context) {
	w := c.Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := models.Broadcaster.AddClient()
	defer models.Broadcaster.RemoveClient(ch)
	flusher, ok := w.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}
	for {
		select {
		case msg := <-ch:
			fmt.Fprint(w, msg)
			flusher.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}

func StateHandler(c *gin.Context) {
	var req struct {
		Events []models.Event `json:"events"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	for _, event := range req.Events {
		if event.Type == "Pane" && event.Verb == "CLICKED" {
			paneIDs := []string{"pane-123", "pane-456"}
			data, _ := json.Marshal(paneIDs)
			models.Broadcaster.Broadcast("reload_panes", string(data))
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func DecodeProfileHandler(c *gin.Context) {
	profileToken, err := c.Cookie("profile_token")
	if err != nil || profileToken == "" {
		c.JSON(http.StatusOK, gin.H{"profile": nil})
		return
	}
	claims, err := utils.ValidateJWT(profileToken)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"profile": nil})
		return
	}
	profile := claims["profile"]
	c.JSON(http.StatusOK, gin.H{"profile": profile})
}

func LoginHandler(c *gin.Context) {
	var req models.LoginRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if validateAdminLogin(req.TenantID, req.Password) {
		c.SetCookie("auth_token", "admin", 24*3600, "/", "", false, true)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
	}
}

func validateEncryptedCredentials(email, code string) *models.Profile {
	return nil
}

func validateAdminLogin(tenantID, password string) bool {
	return password == "admin" && tenantID == "default"
}
