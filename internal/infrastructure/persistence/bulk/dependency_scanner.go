package bulk

import (
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/admin"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
)

type DependencyScanner struct {
	db *database.DB
}

func NewDependencyScanner(db *database.DB) *DependencyScanner {
	return &DependencyScanner{db: db}
}

func (ds *DependencyScanner) ScanAllContentIDs(tenantID string) (*admin.ContentIDMap, error) {
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

	for query, targetMap := range queries {
		rows, err := ds.db.Query(query)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				(*targetMap)[id] = []string{}
			}
		}
	}

	return contentMap, nil
}

func (ds *DependencyScanner) ScanStoryFragmentDependencies(tenantID string) (map[string][]string, error) {
	dependencies := make(map[string][]string)

	rows, err := ds.db.Query("SELECT id FROM storyfragments")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			dependencies[id] = []string{}
		}
	}

	return dependencies, nil
}

func (ds *DependencyScanner) ScanPaneDependencies(tenantID string) (map[string][]string, error) {
	dependencies := make(map[string][]string)

	rows, err := ds.db.Query("SELECT id FROM panes")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			dependencies[id] = []string{}
		}
	}

	sfPaneRows, err := ds.db.Query("SELECT pane_id, storyfragment_id FROM storyfragment_panes")
	if err != nil {
		return nil, err
	}
	defer sfPaneRows.Close()

	for sfPaneRows.Next() {
		var paneID, sfID string
		if err := sfPaneRows.Scan(&paneID, &sfID); err == nil {
			if _, exists := dependencies[paneID]; exists {
				dependencies[paneID] = append(dependencies[paneID], sfID)
			}
		}
	}

	return dependencies, nil
}

func (ds *DependencyScanner) ScanMenuDependencies(tenantID string) (map[string][]string, error) {
	dependencies := make(map[string][]string)

	rows, err := ds.db.Query("SELECT id FROM menus")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			dependencies[id] = []string{}
		}
	}

	sfMenuRows, err := ds.db.Query("SELECT menu_id, id FROM storyfragments WHERE menu_id IS NOT NULL")
	if err != nil {
		return nil, err
	}
	defer sfMenuRows.Close()

	for sfMenuRows.Next() {
		var menuID, sfID string
		if err := sfMenuRows.Scan(&menuID, &sfID); err == nil {
			if _, exists := dependencies[menuID]; exists {
				dependencies[menuID] = append(dependencies[menuID], sfID)
			}
		}
	}

	return dependencies, nil
}

func (ds *DependencyScanner) ScanFileDependencies(tenantID string) (map[string][]string, error) {
	dependencies := make(map[string][]string)

	rows, err := ds.db.Query("SELECT id FROM files")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			dependencies[id] = []string{}
		}
	}

	filePaneRows, err := ds.db.Query("SELECT file_id, pane_id FROM file_panes")
	if err != nil {
		return nil, err
	}
	defer filePaneRows.Close()

	for filePaneRows.Next() {
		var fileID, paneID string
		if err := filePaneRows.Scan(&fileID, &paneID); err == nil {
			if _, exists := dependencies[fileID]; exists {
				dependencies[fileID] = append(dependencies[fileID], paneID)
			}
		}
	}

	return dependencies, nil
}

func (ds *DependencyScanner) ScanBeliefDependencies(tenantID string) (map[string][]string, error) {
	dependencies := make(map[string][]string)

	rows, err := ds.db.Query("SELECT id FROM beliefs")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			dependencies[id] = []string{}
		}
	}

	return dependencies, nil
}
