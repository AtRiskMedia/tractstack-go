package templates

import (
	"fmt"
	"regexp"
	"strings"
)

// UnsetButtonRenderer handles UNSET button HTML generation and injection
type UnsetButtonRenderer struct{}

// NewUnsetButtonRenderer creates a new unset button renderer
func NewUnsetButtonRenderer() *UnsetButtonRenderer {
	return &UnsetButtonRenderer{}
}

// RenderUnsetButton generates the UNSET button HTML with HTMX payload
func (r *UnsetButtonRenderer) RenderUnsetButton(
	paneID string,
	beliefIDs []string,
	gotoPaneID string,
) string {
	if len(beliefIDs) == 0 {
		return ""
	}

	unsetBeliefIds := strings.Join(beliefIDs, ",")

	var hxVals string
	if gotoPaneID != "" {
		hxVals = fmt.Sprintf(`{"unsetBeliefIds": %q, "paneId": %q, "gotoPaneID": %q}`,
			unsetBeliefIds, paneID, gotoPaneID)
	} else {
		hxVals = fmt.Sprintf(`{"unsetBeliefIds": %q, "paneId": %q}`,
			unsetBeliefIds, paneID)
	}

	return fmt.Sprintf(`
        <button
            type="button"
            class="z-10 absolute top-2 right-2 p-1.5 bg-white rounded-full hover:bg-black text-mydarkgrey hover:text-white"
            title="Go Back"
            hx-post="/api/v1/state"
            hx-trigger="click"
            hx-swap="none"
            hx-vals='%s'
            hx-preserve="true"
        >
            <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" d="M9 15L3 9m0 0l6-6M3 9h12a6 6 0 010 12h-3" />
            </svg>
        </button>
    `, hxVals)
}

// InjectButtonIntoHTML injects button HTML after opening pane div tag
func (r *UnsetButtonRenderer) InjectButtonIntoHTML(htmlContent, buttonHTML string) string {
	panePattern := `(<div[^>]*id="pane-[^"]*"[^>]*>)`
	re := regexp.MustCompile(panePattern)
	return re.ReplaceAllString(htmlContent, "$1"+buttonHTML)
}
