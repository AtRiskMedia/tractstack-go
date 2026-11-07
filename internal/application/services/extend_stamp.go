package services

import (
	"os"
	"path/filepath"
	"regexp"
)

var (
	buildStampJobIDRe   = regexp.MustCompile(`jobId:\s*'([^']*)'`)
	buildStampBuiltAtRe = regexp.MustCompile(`builtAt:\s*'([^']*)'`)
)

// BuildStamp is the live frontend stamp written by concierge into
// SourceRoot/src/generated/buildStamp.ts (same artifact Extend badges use).
type BuildStamp struct {
	JobID   string
	BuiltAt string
}

// ReadBuildStamp parses SourceRoot/src/generated/buildStamp.ts.
// Missing file, empty jobId, or parse failure → jobId "dev", builtAt "".
func ReadBuildStamp(backendPath string) BuildStamp {
	path := filepath.Join(SourceRoot(backendPath), "src", "generated", "buildStamp.ts")
	b, err := os.ReadFile(path)
	if err != nil {
		return BuildStamp{JobID: "dev", BuiltAt: ""}
	}
	jobM := buildStampJobIDRe.FindSubmatch(b)
	builtM := buildStampBuiltAtRe.FindSubmatch(b)
	jobID := ""
	builtAt := ""
	if jobM != nil {
		jobID = string(jobM[1])
	}
	if builtM != nil {
		builtAt = string(builtM[1])
	}
	if jobID == "" {
		return BuildStamp{JobID: "dev", BuiltAt: ""}
	}
	return BuildStamp{JobID: jobID, BuiltAt: builtAt}
}
