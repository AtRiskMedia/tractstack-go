package services

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/lisp"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

// HookUsageRow is one pane that targets a hook id (§7).
type HookUsageRow struct {
	PaneID             string  `json:"paneId"`
	PaneTitle          string  `json:"paneTitle"`
	IsContextPane      bool    `json:"isContextPane"`
	StoryFragmentID    *string `json:"storyFragmentId,omitempty"`
	StoryFragmentSlug  *string `json:"storyFragmentSlug,omitempty"`
}

// RouteUsageRow is one §7 hit for a route name.
type RouteUsageRow struct {
	Kind       string  `json:"kind"` // menu | resource | conflict | pane
	ID         string  `json:"id,omitempty"`
	Title      string  `json:"title,omitempty"`
	Slug       string  `json:"slug,omitempty"`
	ActionLisp string  `json:"actionLisp,omitempty"`
	PaneID     string  `json:"paneId,omitempty"`
	PaneTitle  string  `json:"paneTitle,omitempty"`
}

// ErrHookInUse is returned when DELETE /hooks/:id finds usages.
type ErrHookInUse struct {
	Usages []HookUsageRow
}

func (e *ErrHookInUse) Error() string { return "hook is in use" }

// ErrRouteInUse is returned when DELETE /routes/:name finds usages.
type ErrRouteInUse struct {
	Usages []RouteUsageRow
}

func (e *ErrRouteInUse) Error() string { return "route is in use" }

// ExtendUsageService scans content for hook/route usages (§7).
type ExtendUsageService struct {
	tenantManager *tenant.Manager
	logger        *logging.ChanneledLogger
	perfTracker   *performance.Tracker
}

// NewExtendUsageService constructs the usage scanner.
func NewExtendUsageService(
	tenantManager *tenant.Manager,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *ExtendUsageService {
	return &ExtendUsageService{
		tenantManager: tenantManager,
		logger:        logger,
		perfTracker:   perfTracker,
	}
}

func (s *ExtendUsageService) tenantIDsForScan(requestTenantID string) ([]string, error) {
	if !config.EnableMultiTenant {
		if requestTenantID == "" {
			requestTenantID = "default"
		}
		return []string{requestTenantID}, nil
	}
	registry := s.tenantManager.GetDetector().GetRegistry()
	if registry == nil {
		return nil, fmt.Errorf("tenant registry unavailable")
	}
	var ids []string
	for id, info := range registry.Tenants {
		if info.Status == "active" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// FindHookUsages returns panes targeting id across the §7 tenant set.
func (s *ExtendUsageService) FindHookUsages(requestTenantID, id string) ([]HookUsageRow, error) {
	tenantIDs, err := s.tenantIDsForScan(requestTenantID)
	if err != nil {
		return nil, err
	}
	var rows []HookUsageRow
	for _, tid := range tenantIDs {
		ctx, err := s.tenantManager.NewContextFromID(tid)
		if err != nil {
			return nil, err
		}
		panes, err := ctx.PaneRepo().FindAll(tid)
		if err != nil {
			return nil, err
		}
		fragments, err := ctx.StoryFragmentRepo().FindAll(tid)
		if err != nil {
			return nil, err
		}
		for _, pane := range panes {
			if pane == nil || pane.CodeHookTarget == nil || *pane.CodeHookTarget != id {
				continue
			}
			row := HookUsageRow{
				PaneID:        pane.ID,
				PaneTitle:     pane.Title,
				IsContextPane: pane.IsContextPane,
			}
			for _, sf := range fragments {
				if sf == nil {
					continue
				}
				for _, pid := range sf.PaneIDs {
					if pid == pane.ID {
						sid := sf.ID
						sslug := sf.Slug
						row.StoryFragmentID = &sid
						row.StoryFragmentSlug = &sslug
						break
					}
				}
				if row.StoryFragmentID != nil {
					break
				}
			}
			rows = append(rows, row)
		}
	}
	return rows, nil
}

// FindRouteUsages returns §7 hits for route name.
func (s *ExtendUsageService) FindRouteUsages(requestTenantID, name string) ([]RouteUsageRow, error) {
	tenantIDs, err := s.tenantIDsForScan(requestTenantID)
	if err != nil {
		return nil, err
	}
	prefix := "/" + name + "/"
	var rows []RouteUsageRow
	for _, tid := range tenantIDs {
		ctx, err := s.tenantManager.NewContextFromID(tid)
		if err != nil {
			return nil, err
		}

		menus, err := ctx.MenuRepo().FindAll(tid)
		if err != nil {
			return nil, err
		}
		for _, menu := range menus {
			if menu == nil {
				continue
			}
			for _, link := range menu.OptionsPayload {
				if link == nil {
					continue
				}
				if actionLispHitsRoute(link.ActionLisp, prefix) {
					rows = append(rows, RouteUsageRow{
						Kind:       "menu",
						ID:         menu.ID,
						Title:      menu.Title,
						ActionLisp: link.ActionLisp,
					})
				}
			}
		}

		resources, err := ctx.ResourceRepo().FindAll(tid)
		if err != nil {
			return nil, err
		}
		for _, res := range resources {
			if res == nil {
				continue
			}
			if actionLispHitsRoute(res.ActionLisp, prefix) {
				rows = append(rows, RouteUsageRow{
					Kind:       "resource",
					ID:         res.ID,
					Title:      res.Title,
					Slug:       res.Slug,
					ActionLisp: res.ActionLisp,
				})
			}
		}

		fragments, err := ctx.StoryFragmentRepo().FindAll(tid)
		if err != nil {
			return nil, err
		}
		for _, sf := range fragments {
			if sf == nil {
				continue
			}
			if sf.Slug == name {
				rows = append(rows, RouteUsageRow{
					Kind:  "conflict",
					ID:    sf.ID,
					Title: sf.Title,
					Slug:  sf.Slug,
				})
			}
		}

		panes, err := ctx.PaneRepo().FindAll(tid)
		if err != nil {
			return nil, err
		}
		for _, pane := range panes {
			if pane == nil || pane.OptionsPayload == nil {
				continue
			}
			b, err := json.Marshal(pane.OptionsPayload)
			if err != nil {
				continue
			}
			if strings.Contains(string(b), prefix) {
				rows = append(rows, RouteUsageRow{
					Kind:      "pane",
					PaneID:    pane.ID,
					PaneTitle: pane.Title,
				})
			}
		}
	}
	return rows, nil
}

// HasFragmentSlug reports whether any scanned tenant has a story fragment slug equal to name.
func (s *ExtendUsageService) HasFragmentSlug(requestTenantID, name string) (bool, error) {
	tenantIDs, err := s.tenantIDsForScan(requestTenantID)
	if err != nil {
		return false, err
	}
	for _, tid := range tenantIDs {
		ctx, err := s.tenantManager.NewContextFromID(tid)
		if err != nil {
			return false, err
		}
		fragments, err := ctx.StoryFragmentRepo().FindAll(tid)
		if err != nil {
			return false, err
		}
		for _, sf := range fragments {
			if sf != nil && sf.Slug == name {
				return true, nil
			}
		}
	}
	return false, nil
}

func actionLispHitsRoute(actionLisp, prefix string) bool {
	if actionLisp == "" {
		return false
	}
	// Plain string match fallback (§7).
	if strings.Contains(actionLisp, prefix) {
		return true
	}
	parsed := lisp.Parse(actionLisp, "", "")
	if parsed != nil && parsed.IsValid && parsed.Href != "" {
		return strings.HasPrefix(parsed.Href, prefix)
	}
	return false
}
