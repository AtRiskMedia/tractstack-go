package rendering

// VisualBreakData contains the configuration for a specific visual break shape, including the collection source and fill color.
type VisualBreakData struct {
	Collection string `json:"collection"`
	Image      string `json:"image"`
	SvgFill    string `json:"svgFill"`
}

// VisualBreakNode defines the responsive configuration for visual breaks across mobile, tablet, and desktop viewports.
type VisualBreakNode struct {
	BreakDesktop          *VisualBreakData `json:"breakDesktop,omitempty"`
	BreakTablet           *VisualBreakData `json:"breakTablet,omitempty"`
	BreakMobile           *VisualBreakData `json:"breakMobile,omitempty"`
	HiddenViewportMobile  bool             `json:"hiddenViewportMobile,omitempty"`
	HiddenViewportTablet  bool             `json:"hiddenViewportTablet,omitempty"`
	HiddenViewportDesktop bool             `json:"hiddenViewportDesktop,omitempty"`
}

// GetViewportData retrieves the specific visual break configuration for the requested viewport size.
func (vbn *VisualBreakNode) GetViewportData(viewport string) *VisualBreakData {
	if vbn == nil {
		return nil
	}

	switch viewport {
	case "mobile":
		if vbn.HiddenViewportMobile {
			return nil
		}
		return vbn.BreakMobile
	case "tablet":
		if vbn.HiddenViewportTablet {
			return nil
		}
		return vbn.BreakTablet
	case "desktop":
		if vbn.HiddenViewportDesktop {
			return nil
		}
		return vbn.BreakDesktop
	default:
		return nil
	}
}
