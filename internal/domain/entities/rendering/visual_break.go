package rendering

type VisualBreakData struct {
	Collection string `json:"collection"`
	Image      string `json:"image"`
	SvgFill    string `json:"svgFill"`
}

type VisualBreakNode struct {
	BreakDesktop          *VisualBreakData `json:"breakDesktop,omitempty"`
	BreakTablet           *VisualBreakData `json:"breakTablet,omitempty"`
	BreakMobile           *VisualBreakData `json:"breakMobile,omitempty"`
	HiddenViewportMobile  bool             `json:"hiddenViewportMobile,omitempty"`
	HiddenViewportTablet  bool             `json:"hiddenViewportTablet,omitempty"`
	HiddenViewportDesktop bool             `json:"hiddenViewportDesktop,omitempty"`
}

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
