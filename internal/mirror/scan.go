package mirror

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ScanFinding struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type ScanReport struct {
	Findings []ScanFinding `json:"findings,omitempty"`
}

func (r ScanReport) HasFindings() bool { return len(r.Findings) > 0 }

func (r ScanReport) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "mirror export failed blacklist scan with %d finding(s):", len(r.Findings))
	limit := len(r.Findings)
	if limit > 20 {
		limit = 20
	}
	for i := 0; i < limit; i++ {
		f := r.Findings[i]
		fmt.Fprintf(&b, "\n  %s: %s", f.Path, f.Reason)
	}
	if len(r.Findings) > limit {
		fmt.Fprintf(&b, "\n  ... %d more", len(r.Findings)-limit)
	}
	return b.String()
}

var allowedPathPatterns = []string{
	".env.example",
	"**/.env.example",
	".env.sample",
	"**/.env.sample",
}

var forbiddenPathPatterns = []string{
	".env",
	".env.*",
	"**/.env",
	"**/.env.*",
	"*.env",
	"**/*.env",
	"*.pem",
	"*.key",
	"*.p12",
	"*.pfx",
	"id_rsa",
	"id_ed25519",
	"**/id_rsa",
	"**/id_ed25519",
	"secrets.nix",
	"**/secrets.nix",
	"secrets/**",
	"**/secrets/**",
	".mcp.json",
	"**/.mcp.json",
}

var forbiddenContentPatterns = []struct {
	reason string
	re     *regexp.Regexp
}{
	{"private key block", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"GitHub token", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9_]{30,}`)},
	{"OpenAI-style API key", regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`)},
	{"quoted credential assignment", regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password)['\"]?\s*[:=]\s*['\"][A-Za-z0-9_./+=-]{20,}['\"]`)},
}

var envCredentialPattern = regexp.MustCompile(`(?i)^\s*(?:export\s+)?[A-Z0-9_]*(?:API[_-]?KEY|SECRET|TOKEN|PASSWORD)[A-Z0-9_]*\s*=\s*['\"]?[A-Za-z0-9_./+=-]{20,}`)

func ScanPlan(plan Plan) (ScanReport, error) {
	var report ScanReport
	for _, file := range plan.Files {
		rel := slashPath(file.Path)
		if matchesAny(rel, forbiddenPathPatterns) && !matchesAny(rel, allowedPathPatterns) {
			report.Findings = append(report.Findings, ScanFinding{Path: rel, Reason: "forbidden path pattern"})
			continue
		}
		src := filepath.Join(plan.Source, filepath.FromSlash(rel))
		for _, overlay := range plan.Overlays {
			if overlay.To == rel {
				src = filepath.Join(plan.Source, filepath.FromSlash(overlay.From))
				break
			}
		}
		findings, err := scanFileContent(rel, src)
		if err != nil {
			return report, err
		}
		report.Findings = append(report.Findings, findings...)
	}
	return report, nil
}

func isAllowedQuotedCredentialReference(rel, line string) bool {
	lowerRel := strings.ToLower(rel)
	if strings.Contains(lowerRel, ".test.") || strings.HasSuffix(lowerRel, "_test.go") {
		return true
	}
	lowerLine := strings.ToLower(line)
	return strings.Contains(lowerLine, "publishable") || strings.Contains(lowerLine, "public") || strings.Contains(lowerLine, "revenuecat")
}

func isEnvLikePath(rel string) bool {
	base := filepath.Base(slashPath(rel))
	return base == ".env" || strings.HasPrefix(base, ".env.") || strings.HasSuffix(base, ".env")
}

func scanFileContent(rel, path string) ([]ScanFinding, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > 2*1024*1024 {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var findings []ScanFinding
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if isEnvLikePath(rel) && !matchesAny(rel, allowedPathPatterns) && envCredentialPattern.MatchString(line) {
			findings = append(findings, ScanFinding{Path: rel, Reason: fmt.Sprintf("env credential assignment on line %d", lineNo)})
			continue
		}
		for _, pattern := range forbiddenContentPatterns {
			if pattern.re.MatchString(line) {
				if pattern.reason == "private key block" && strings.Contains(line, "TEST PRIVATE KEY") {
					continue
				}
				if pattern.reason == "quoted credential assignment" && isAllowedQuotedCredentialReference(rel, line) {
					continue
				}
				findings = append(findings, ScanFinding{Path: rel, Reason: fmt.Sprintf("%s on line %d", pattern.reason, lineNo)})
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return findings, err
	}
	return findings, nil
}
