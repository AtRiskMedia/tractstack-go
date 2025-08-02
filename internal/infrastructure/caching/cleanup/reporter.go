// Package cleanup provides ascii reporter
package cleanup

import (
	"fmt"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
)

const (
	cyan       = "\033[38;2;86;182;194m"  // One Dark Cyan: #56B6C2
	cyanBright = "\033[38;2;97;228;240m"  // Brighter Cyan: #61E4F0
	dimCyan    = "\033[38;2;47;91;102m"   // Dim Cyan: #2F5B66
	grey       = "\033[38;2;110;118;129m" // Brighter Grey: #6E7681
	dimGrey    = "\033[38;2;75;82;99m"    // Darker Grey: #4B5263
	success    = "\033[38;2;62;130;144m"  // Dim Cyan: #3E8290
	warning    = "\033[38;2;229;192;123m" // One Dark Yellow: #E5C07B
	errorRed   = "\033[38;2;224;108;117m" // One Dark Red: #E06C75
	white      = "\033[38;2;171;178;191m" // One Dark Foreground: #ABB2BF
	purple     = "\033[38;2;198;120;221m" // One Dark Purple: #C678DD
	reset      = "\033[0m"
	bold       = "\033[1m"
)

type Reporter struct {
	cache interfaces.Cache
}

func NewReporter(cache interfaces.Cache) *Reporter {
	return &Reporter{cache: cache}
}

func (r *Reporter) LogHeader(title string) {
	fmt.Printf("%s%s✓ %s %s\n", bold, cyan, strings.ToUpper(title), reset)
}

func (r *Reporter) LogSubHeader(text string) {
	fmt.Printf("%s%s░▒▓ %s %s\n", bold, dimCyan, text, reset)
}

func (r *Reporter) LogStepSuccess(message string, args ...any) {
	formattedMsg := fmt.Sprintf(message, args...)
	fmt.Printf("%s⚡ %s%s...%s\n", dimGrey, grey, formattedMsg, reset)
}

func (r *Reporter) LogStage(message string, args ...any) {
	formattedMsg := fmt.Sprintf(message, args...)
	fmt.Printf("%s%s✦ %s%s%s\n", success, bold, grey, formattedMsg, reset)
}

func (r *Reporter) LogSuccess(message string, args ...any) {
	formattedMsg := fmt.Sprintf(message, args...)
	fmt.Printf("%s%s✦ %s%s%s\n", success, bold, white, formattedMsg, reset)
}

func (r *Reporter) LogError(message string, err error) {
	fmt.Printf("%s%s✖ ERROR: %s%s: %v%s\n", bold, errorRed, grey, message, err, reset)
}

func (r *Reporter) LogWarning(message string, args ...any) {
	formattedMsg := fmt.Sprintf(message, args...)
	fmt.Printf("%s%s⚠ WARNING: %s%s%s\n", bold, warning, grey, formattedMsg, reset)
}

func (r *Reporter) LogInfo(message string, args ...any) {
	formattedMsg := fmt.Sprintf(message, args...)
	fmt.Printf("%s▶ %s%s%s\n", dimGrey, grey, formattedMsg, reset)
}

func (r *Reporter) GenerateTenantReport(tenantID string) string {
	var report strings.Builder

	timestamp := time.Now().UTC().Format("2006-01-02 15:04:05 MST")

	report.WriteString(fmt.Sprintf("%s%s▓ %s | Tenant: %s %s\n", bold, cyan, timestamp, tenantID, reset))

	var statusLine strings.Builder
	if contentMap, exists := r.cache.GetFullContentMap(tenantID); exists {
		statusLine.WriteString(fmt.Sprintf("%s✦ %sContent Map: %s%d items%s",
			success, grey, cyanBright, len(contentMap), reset))
	} else {
		statusLine.WriteString(fmt.Sprintf("%s✖ %sContent Map: %sNOT LOADED%s",
			errorRed, grey, errorRed, reset))
	}

	statusLine.WriteString("  ")

	if _, _, exists := r.cache.GetOrphanAnalysis(tenantID); exists {
		statusLine.WriteString(fmt.Sprintf("%s✦ %sOrphan Analysis: %sREADY%s",
			success, grey, white, reset))
	} else {
		statusLine.WriteString(fmt.Sprintf("%s○ %sOrphan Analysis: %sPRIMED%s",
			dimGrey, grey, cyan, reset))
	}
	report.WriteString(statusLine.String() + "\n")

	var countsLine strings.Builder
	countsLine.WriteString(fmt.Sprintf("%s✦ Cached nodes:%s", dimGrey, reset))

	contentTypes := []struct {
		name   string
		getter func(string) ([]string, bool)
	}{
		{"TractStacks", r.cache.GetAllTractStackIDs},
		{"StoryFragments", r.cache.GetAllStoryFragmentIDs},
		{"Panes", r.cache.GetAllPaneIDs},
		{"Menus", r.cache.GetAllMenuIDs},
		{"Resources", r.cache.GetAllResourceIDs},
		{"Beliefs", r.cache.GetAllBeliefIDs},
		{"Epinets", r.cache.GetAllEpinetIDs},
		{"Files", r.cache.GetAllFileIDs},
	}

	for _, ct := range contentTypes {
		countsLine.WriteString(" ")
		if ids, exists := ct.getter(tenantID); exists {
			countsLine.WriteString(fmt.Sprintf("%s%s:%s%d", dimCyan, ct.name, cyan, len(ids)))
		} else {
			countsLine.WriteString(fmt.Sprintf("%s%s:%s--", dimGrey, ct.name, dimGrey))
		}
	}
	report.WriteString(countsLine.String() + "\n")

	var activityLine strings.Builder
	activityLine.WriteString(fmt.Sprintf("%s✦ Activity:%s", purple, reset))

	sessionIDs := r.cache.GetAllSessionIDs(tenantID)
	fingerprintIDs := r.cache.GetAllFingerprintIDs(tenantID)
	visitIDs := r.cache.GetAllVisitIDs(tenantID)
	beliefRegistryIDs := r.cache.GetAllStoryfragmentBeliefRegistryIDs(tenantID)
	htmlChunkIDs := r.cache.GetAllHTMLChunkIDs(tenantID)

	activityLine.WriteString(fmt.Sprintf(" %sSessions:%s%d", purple, white, len(sessionIDs)))
	activityLine.WriteString(fmt.Sprintf(" %sFingerprints:%s%d", purple, white, len(fingerprintIDs)))
	activityLine.WriteString(fmt.Sprintf(" %sVisits:%s%d", purple, white, len(visitIDs)))

	if len(beliefRegistryIDs) > 0 {
		activityLine.WriteString(fmt.Sprintf(" %sBelief-Maps:%s%d", purple, white, len(beliefRegistryIDs)))
	} else {
		activityLine.WriteString(fmt.Sprintf(" %sBelief-Maps:%s--", purple, dimGrey))
	}

	if len(htmlChunkIDs) > 0 {
		activityLine.WriteString(fmt.Sprintf(" %sHTML:%s%d", purple, white, len(htmlChunkIDs)))
	} else {
		activityLine.WriteString(fmt.Sprintf(" %sHTML:%s--", purple, dimGrey))
	}

	report.WriteString(activityLine.String() + "\n")

	return report.String()
}
