package bulk

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/admin"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
)

type DependencyScanner struct {
	db     *database.DB
	logger *logging.ChanneledLogger
}

func NewDependencyScanner(db *database.DB, logger *logging.ChanneledLogger) *DependencyScanner {
	return &DependencyScanner{
		db:     db,
		logger: logger,
	}
}

func (ds *DependencyScanner) ScanAllContentIDs(tenantID string) (*admin.ContentIDMap, error) {
	start := time.Now()
	ds.logger.Database().Debug("Starting content IDs scan", "tenantID", tenantID)

	contentMap := &admin.ContentIDMap{
		StoryFragments: make(map[string][]string),
		Panes:          make(map[string][]string),
		Menus:          make(map[string][]string),
		Files:          make(map[string][]string),
		Beliefs:        make(map[string][]string),
	}

	queries := map[string]*map[string][]string{
		"SELECT id FROM storyfragments": &contentMap.StoryFragments,
		"SELECT id FROM panes":          &contentMap.Panes,
		"SELECT id FROM menus":          &contentMap.Menus,
		"SELECT id FROM files":          &contentMap.Files,
		"SELECT id FROM beliefs":        &contentMap.Beliefs,
	}

	totalItems := 0
	for query, targetMap := range queries {
		queryStart := time.Now()
		ds.logger.Database().Debug("Executing content ID query", "query", query)

		rows, err := ds.db.Query(query)
		if err != nil {
			ds.logger.Database().Error("Content ID query failed", "error", err.Error(), "query", query)
			return nil, err
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				(*targetMap)[id] = []string{}
				count++
				totalItems++
			}
		}

		ds.logger.Database().Info("Content ID query completed", "query", query, "count", count, "duration", time.Since(queryStart))
	}

	ds.logger.Database().Info("Content IDs scan completed", "tenantID", tenantID, "totalItems", totalItems, "duration", time.Since(start))
	return contentMap, nil
}

func (ds *DependencyScanner) ScanStoryFragmentDependencies(tenantID string) (map[string][]string, error) {
	start := time.Now()
	ds.logger.Database().Debug("Starting story fragment dependencies scan", "tenantID", tenantID)

	dependencies := make(map[string][]string)

	query := "SELECT id FROM storyfragments"
	ds.logger.Database().Debug("Executing story fragment dependency query", "query", query)

	rows, err := ds.db.Query(query)
	if err != nil {
		ds.logger.Database().Error("Story fragment dependency query failed", "error", err.Error(), "query", query)
		return nil, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			dependencies[id] = []string{}
			count++
		}
	}

	ds.logger.Database().Info("Story fragment dependencies scan completed", "tenantID", tenantID, "sfCount", count, "duration", time.Since(start))
	return dependencies, nil
}

func (ds *DependencyScanner) ScanPaneDependencies(tenantID string) (map[string][]string, error) {
	start := time.Now()
	ds.logger.Database().Debug("Starting pane dependencies scan", "tenantID", tenantID)

	dependencies := make(map[string][]string)

	// First query: Get all pane IDs
	query1 := "SELECT id FROM panes"
	ds.logger.Database().Debug("Executing pane IDs query", "query", query1)

	rows, err := ds.db.Query(query1)
	if err != nil {
		ds.logger.Database().Error("Pane IDs query failed", "error", err.Error(), "query", query1)
		return nil, err
	}
	defer rows.Close()

	paneCount := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			dependencies[id] = []string{}
			paneCount++
		}
	}

	ds.logger.Database().Info("Pane IDs loaded", "paneCount", paneCount)

	// Second query: Get pane dependencies
	query2 := "SELECT pane_id, storyfragment_id FROM storyfragment_panes"
	ds.logger.Database().Debug("Executing pane dependencies query", "query", query2)

	sfPaneRows, err := ds.db.Query(query2)
	if err != nil {
		ds.logger.Database().Error("Pane dependencies query failed", "error", err.Error(), "query", query2)
		return nil, err
	}
	defer sfPaneRows.Close()

	depCount := 0
	for sfPaneRows.Next() {
		var paneID, sfID string
		if err := sfPaneRows.Scan(&paneID, &sfID); err == nil {
			if _, exists := dependencies[paneID]; exists {
				dependencies[paneID] = append(dependencies[paneID], sfID)
				depCount++
			}
		}
	}

	ds.logger.Database().Info("Pane dependencies scan completed", "tenantID", tenantID, "paneCount", paneCount, "depCount", depCount, "duration", time.Since(start))
	return dependencies, nil
}

func (ds *DependencyScanner) ScanMenuDependencies(tenantID string) (map[string][]string, error) {
	start := time.Now()
	ds.logger.Database().Debug("Starting menu dependencies scan", "tenantID", tenantID)

	dependencies := make(map[string][]string)

	// First query: Get all menu IDs
	query1 := "SELECT id FROM menus"
	ds.logger.Database().Debug("Executing menu IDs query", "query", query1)

	rows, err := ds.db.Query(query1)
	if err != nil {
		ds.logger.Database().Error("Menu IDs query failed", "error", err.Error(), "query", query1)
		return nil, err
	}
	defer rows.Close()

	menuCount := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			dependencies[id] = []string{}
			menuCount++
		}
	}

	ds.logger.Database().Info("Menu IDs loaded", "menuCount", menuCount)

	// Second query: Get menu dependencies
	query2 := "SELECT menu_id, id FROM storyfragments WHERE menu_id IS NOT NULL"
	ds.logger.Database().Debug("Executing menu dependencies query", "query", query2)

	sfMenuRows, err := ds.db.Query(query2)
	if err != nil {
		ds.logger.Database().Error("Menu dependencies query failed", "error", err.Error(), "query", query2)
		return nil, err
	}
	defer sfMenuRows.Close()

	depCount := 0
	for sfMenuRows.Next() {
		var menuID, sfID string
		if err := sfMenuRows.Scan(&menuID, &sfID); err == nil {
			if _, exists := dependencies[menuID]; exists {
				dependencies[menuID] = append(dependencies[menuID], sfID)
				depCount++
			}
		}
	}

	ds.logger.Database().Info("Menu dependencies scan completed", "tenantID", tenantID, "menuCount", menuCount, "depCount", depCount, "duration", time.Since(start))
	return dependencies, nil
}

func (ds *DependencyScanner) ScanFileDependencies(tenantID string) (map[string][]string, error) {
	start := time.Now()
	ds.logger.Database().Debug("Starting file dependencies scan", "tenantID", tenantID)

	dependencies := make(map[string][]string)

	// First query: Get all file IDs
	query1 := "SELECT id FROM files"
	ds.logger.Database().Debug("Executing file IDs query", "query", query1)

	rows, err := ds.db.Query(query1)
	if err != nil {
		ds.logger.Database().Error("File IDs query failed", "error", err.Error(), "query", query1)
		return nil, err
	}
	defer rows.Close()

	fileCount := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			dependencies[id] = []string{}
			fileCount++
		}
	}

	ds.logger.Database().Info("File IDs loaded", "fileCount", fileCount)

	// Second query: Get file dependencies
	query2 := "SELECT file_id, pane_id FROM file_panes"
	ds.logger.Database().Debug("Executing file dependencies query", "query", query2)

	filePaneRows, err := ds.db.Query(query2)
	if err != nil {
		ds.logger.Database().Error("File dependencies query failed", "error", err.Error(), "query", query2)
		return nil, err
	}
	defer filePaneRows.Close()

	depCount := 0
	for filePaneRows.Next() {
		var fileID, paneID string
		if err := filePaneRows.Scan(&fileID, &paneID); err == nil {
			if _, exists := dependencies[fileID]; exists {
				dependencies[fileID] = append(dependencies[fileID], paneID)
				depCount++
			}
		}
	}

	ds.logger.Database().Info("File dependencies scan completed", "tenantID", tenantID, "fileCount", fileCount, "depCount", depCount, "duration", time.Since(start))
	return dependencies, nil
}

func (ds *DependencyScanner) ScanBeliefDependencies(tenantID string) (map[string][]string, error) {
	start := time.Now()
	ds.logger.Database().Debug("Starting belief dependencies scan", "tenantID", tenantID)

	dependencies := make(map[string][]string)

	query := "SELECT id FROM beliefs"
	ds.logger.Database().Debug("Executing belief IDs query", "query", query)

	rows, err := ds.db.Query(query)
	if err != nil {
		ds.logger.Database().Error("Belief IDs query failed", "error", err.Error(), "query", query)
		return nil, err
	}
	defer rows.Close()

	beliefCount := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			dependencies[id] = []string{}
			beliefCount++
		}
	}

	ds.logger.Database().Info("Belief dependencies scan completed", "tenantID", tenantID, "beliefCount", beliefCount, "duration", time.Since(start))
	return dependencies, nil
}
