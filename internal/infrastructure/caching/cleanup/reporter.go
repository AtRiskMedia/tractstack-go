// Package cleanup provides ASCII reporting for cache contents
package cleanup

import (
	"fmt"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
)

const (
	// OneDark colors (foreground only)
	headerColor  = "\033[38;2;224;108;117m" // Red
	successColor = "\033[38;2;152;195;121m" // Green
	warningColor = "\033[38;2;229;192;123m" // Yellow
	infoColor    = "\033[38;2;97;175;239m"  // Blue
	textColor    = "\033[38;2;171;178;191m" // Light gray
	accentColor  = "\033[38;2;198;120;221m" // Purple
	resetColor   = "\033[0m"                // Reset
)

// Reporter generates ASCII reports of cache contents
type Reporter struct {
	cache interfaces.Cache
}

// NewReporter creates a new cache reporter
func NewReporter(cache interfaces.Cache) *Reporter {
	return &Reporter{cache: cache}
}

func (r *Reporter) LogHeader(title string) {
	fmt.Printf("\n%s=== %s ===%s\n", headerColor, strings.ToUpper(title), resetColor)
}

func (r *Reporter) LogSubHeader(text string) {
	fmt.Printf("%s--- %s ---%s\n", accentColor, text, resetColor)
}

func (r *Reporter) LogStage(message string, args ...interface{}) {
	formattedMsg := fmt.Sprintf(message, args...)
	fmt.Printf("%s  - %s...%s\n", textColor, formattedMsg, resetColor)
}

func (r *Reporter) LogSuccess(message string, args ...interface{}) {
	formattedMsg := fmt.Sprintf(message, args...)
	fmt.Printf("%s  - ✓ %s%s\n", successColor, formattedMsg, resetColor)
}

func (r *Reporter) LogError(message string, err error) {
	fmt.Printf("%s  - ✗ ERROR: %s: %v%s\n", headerColor, message, err, resetColor)
}

func (r *Reporter) LogWarning(message string, args ...interface{}) {
	formattedMsg := fmt.Sprintf(message, args...)
	fmt.Printf("%s  - ! WARNING: %s%s\n", warningColor, formattedMsg, resetColor)
}

func (r *Reporter) LogInfo(message string, args ...interface{}) {
	formattedMsg := fmt.Sprintf(message, args...)
	fmt.Printf("%s    > %s%s\n", infoColor, formattedMsg, resetColor)
}

// GenerateTenantReport creates ASCII visualization of tenant cache contents
func (r *Reporter) GenerateTenantReport(tenantID string) string {
	var report strings.Builder

	report.WriteString(r.generateHeader(tenantID))
	report.WriteString(r.generateContentCacheReport(tenantID))
	report.WriteString(r.generateAnalyticsCacheReport(tenantID))
	report.WriteString(r.generateSessionCacheReport(tenantID))
	report.WriteString(r.generateFragmentCacheReport(tenantID))
	report.WriteString(r.generateFooter())

	return report.String()
}

// generateHeader creates the report header
func (r *Reporter) generateHeader(tenantID string) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	return fmt.Sprintf(`
%s╔══════════════════════════════════════════════════════════════════════════════╗%s
%s║                     %sCache Report: %s%-12s%s                      ║%s
%s║                        %s%s%s                        ║%s
%s╠══════════════════════════════════════════════════════════════════════════════╣%s
`, headerColor, resetColor,
		headerColor, accentColor, successColor, padCenter(tenantID, 12), headerColor, resetColor,
		headerColor, infoColor, timestamp, headerColor, resetColor,
		headerColor, resetColor)
}

// generateContentCacheReport shows content cache status
func (r *Reporter) generateContentCacheReport(tenantID string) string {
	var report strings.Builder
	report.WriteString(fmt.Sprintf("%s║ %sCONTENT CACHE:%s\n", textColor, accentColor, resetColor))

	// Content Map
	if contentMap, exists := r.cache.GetFullContentMap(tenantID); exists {
		report.WriteString(fmt.Sprintf("%s║   %s✓ %sContent Map: %s%d items%s\n", textColor, successColor, textColor, infoColor, len(contentMap), resetColor))
	} else {
		report.WriteString(fmt.Sprintf("%s║   %s✗ %sContent Map: %sMISSING%s\n", textColor, warningColor, textColor, warningColor, resetColor))
	}

	// Orphan Analysis
	if _, _, exists := r.cache.GetOrphanAnalysis(tenantID); exists {
		report.WriteString(fmt.Sprintf("%s║   %s✓ %sOrphan Analysis: %sCACHED%s\n", textColor, successColor, textColor, successColor, resetColor))
	} else {
		report.WriteString(fmt.Sprintf("%s║   %s✗ %sOrphan Analysis: %sMISSING%s\n", textColor, warningColor, textColor, warningColor, resetColor))
	}

	report.WriteString(fmt.Sprintf("%s║   %s• %sTractStacks: TODO%s\n", textColor, infoColor, textColor, resetColor))
	report.WriteString(fmt.Sprintf("%s║   %s• %sStoryFragments: TODO%s\n", textColor, infoColor, textColor, resetColor))
	report.WriteString(fmt.Sprintf("%s║   %s• %sPanes: TODO%s\n", textColor, infoColor, textColor, resetColor))
	report.WriteString(fmt.Sprintf("%s║   %s• %sMenus: TODO%s\n", textColor, infoColor, textColor, resetColor))
	report.WriteString(fmt.Sprintf("%s║   %s• %sResources: TODO%s\n", textColor, infoColor, textColor, resetColor))
	report.WriteString(fmt.Sprintf("%s║   %s• %sBeliefs: TODO%s\n", textColor, infoColor, textColor, resetColor))
	report.WriteString(fmt.Sprintf("%s║   %s• %sEpinets: TODO%s\n", textColor, infoColor, textColor, resetColor))
	report.WriteString(fmt.Sprintf("%s║   %s• %sImageFiles: TODO%s\n", textColor, infoColor, textColor, resetColor))

	report.WriteString(fmt.Sprintf("%s║%s\n", textColor, resetColor))
	return report.String()
}

// generateAnalyticsCacheReport shows analytics cache status
func (r *Reporter) generateAnalyticsCacheReport(tenantID string) string {
	var report strings.Builder
	report.WriteString(fmt.Sprintf("%s║ %sANALYTICS CACHE:%s\n", textColor, accentColor, resetColor))

	report.WriteString(fmt.Sprintf("%s║   %s• %sHourly Epinet Bins: TODO%s\n", textColor, infoColor, textColor, resetColor))
	report.WriteString(fmt.Sprintf("%s║   %s• %sContent Bins: TODO%s\n", textColor, infoColor, textColor, resetColor))
	report.WriteString(fmt.Sprintf("%s║   %s• %sSite Bins: TODO%s\n", textColor, infoColor, textColor, resetColor))
	report.WriteString(fmt.Sprintf("%s║   %s• %sLead Metrics: TODO%s\n", textColor, infoColor, textColor, resetColor))
	report.WriteString(fmt.Sprintf("%s║   %s• %sDashboard Data: TODO%s\n", textColor, infoColor, textColor, resetColor))

	report.WriteString(fmt.Sprintf("%s║%s\n", textColor, resetColor))
	return report.String()
}

// generateSessionCacheReport shows session cache status
func (r *Reporter) generateSessionCacheReport(tenantID string) string {
	var report strings.Builder
	report.WriteString(fmt.Sprintf("%s║ %sSESSION CACHE:%s\n", textColor, accentColor, resetColor))

	report.WriteString(fmt.Sprintf("%s║   %s• %sFingerprint States: TODO%s\n", textColor, infoColor, textColor, resetColor))
	report.WriteString(fmt.Sprintf("%s║   %s• %sVisit States: TODO%s\n", textColor, infoColor, textColor, resetColor))
	report.WriteString(fmt.Sprintf("%s║   %s• %sSession Data: TODO%s\n", textColor, infoColor, textColor, resetColor))
	report.WriteString(fmt.Sprintf("%s║   %s• %sBelief Registries: TODO%s\n", textColor, infoColor, textColor, resetColor))
	report.WriteString(fmt.Sprintf("%s║   %s• %sSession Belief Contexts: TODO%s\n", textColor, infoColor, textColor, resetColor))

	report.WriteString(fmt.Sprintf("%s║%s\n", textColor, resetColor))
	return report.String()
}

// generateFragmentCacheReport shows HTML fragment cache status
func (r *Reporter) generateFragmentCacheReport(tenantID string) string {
	var report strings.Builder
	report.WriteString(fmt.Sprintf("%s║ %sHTML FRAGMENT CACHE:%s\n", textColor, accentColor, resetColor))

	report.WriteString(fmt.Sprintf("%s║   %s• %sCached Chunks: TODO%s\n", textColor, infoColor, textColor, resetColor))
	report.WriteString(fmt.Sprintf("%s║   %s• %sDependencies: TODO%s\n", textColor, infoColor, textColor, resetColor))

	report.WriteString(fmt.Sprintf("%s║%s\n", textColor, resetColor))
	return report.String()
}

// generateFooter creates the report footer
func (r *Reporter) generateFooter() string {
	return fmt.Sprintf("%s╚══════════════════════════════════════════════════════════════════════════════╝%s\n", headerColor, resetColor)
}

// padCenter centers text within specified width
func padCenter(text string, width int) string {
	if len(text) >= width {
		return text[:width]
	}

	padding := width - len(text)
	leftPad := padding / 2
	rightPad := padding - leftPad

	return strings.Repeat(" ", leftPad) + text + strings.Repeat(" ", rightPad)
}
