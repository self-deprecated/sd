package mirror

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
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

// Path patterns match at any depth; see internal/glob.
var allowedPathPatterns = []string{
	".env.example",
	".env.sample",
}

var forbiddenPathPatterns = []string{
	".env",
	".env.*",
	"*.env",
	"*.pem",
	"*.key",
	"*.p12",
	"*.pfx",
	"id_rsa",
	"id_ed25519",
	"secrets.nix",
	"secrets/**",
	".mcp.json",
}

var forbiddenContentPatterns = []struct {
	reason string
	re     *regexp.Regexp
}{
	{"private key block", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"GitHub token", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9_]{30,}`)},
	{"OpenAI-style API key", regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`)},
	{"quoted credential assignment", regexp.MustCompile(`(?i)(?:api[_-]?key|secret|token|password)['"]?\s*(?::=|[:=])\s*['"]([A-Za-z0-9_./+=-]{20,})['"]`)},
}

var envCredentialPattern = regexp.MustCompile(`(?i)^\s*(?:export\s+)?[A-Z0-9_]*(?:API[_-]?KEY|SECRET|TOKEN|PASSWORD)[A-Z0-9_]*\s*=\s*['"]?([A-Za-z0-9_./+=-]{20,})`)

// Long lines are scanned in chunks of scanChunk bytes, each prefixed with the
// last scanOverlap bytes of the previous chunk so that a credential spanning
// a chunk boundary is still matched.
const (
	scanChunk   = 64 * 1024
	scanOverlap = 4 * 1024
)

func ScanPlan(plan Plan) (ScanReport, error) {
	var report ScanReport
	for _, file := range plan.Files {
		rel := slashPath(file.Path)
		src := filepath.Join(plan.Source, filepath.FromSlash(rel))
		for _, overlay := range plan.Overlays {
			if overlay.To == rel {
				src = filepath.Join(plan.Source, filepath.FromSlash(overlay.From))
				break
			}
		}
		findings, err := scanFile(rel, src)
		if err != nil {
			return report, err
		}
		report.Findings = append(report.Findings, findings...)
	}
	return report, nil
}

// ScanFiles scans the files at the given slash-separated paths below root,
// such as the changes a mirror commit would record. Paths that no longer
// exist are deletions and are skipped; VCS directories are never scanned.
func ScanFiles(root string, rels []string) (ScanReport, error) {
	var report ScanReport
	for _, rel := range rels {
		rel = slashPath(rel)
		if isProtected(rel) {
			continue
		}
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Lstat(abs); os.IsNotExist(err) {
			continue
		}
		findings, err := scanFile(rel, abs)
		if err != nil {
			return report, err
		}
		report.Findings = append(report.Findings, findings...)
	}
	return report, nil
}

// scanFile checks the exported path rel and the content of the file at path.
// Every file is either scanned completely or reported as a finding.
func scanFile(rel, path string) ([]ScanFinding, error) {
	if matchesAny(rel, forbiddenPathPatterns) && !matchesAny(rel, allowedPathPatterns) {
		return []ScanFinding{{Path: rel, Reason: "forbidden path pattern"}}, nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return []ScanFinding{{Path: rel, Reason: "not a regular file, so it cannot be scanned"}}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return scanContent(rel, file)
}

func scanContent(rel string, r io.Reader) ([]ScanFinding, error) {
	var findings []ScanFinding
	reader := bufio.NewReaderSize(r, scanChunk)
	envLike := isEnvLikePath(rel)
	testFile := isTestPath(rel)
	lineNo := 0
	continuing, reported := false, false
	var carry []byte
	for {
		fragment, isPrefix, err := reader.ReadLine()
		if err == io.EOF {
			return findings, nil
		}
		if err != nil {
			return findings, err
		}
		if !continuing {
			lineNo++
			reported = false
			carry = carry[:0]
		}
		text := string(carry) + string(fragment)
		if !reported {
			if reason := lineFinding(text, envLike && !continuing, testFile); reason != "" {
				findings = append(findings, ScanFinding{Path: rel, Reason: fmt.Sprintf("%s on line %d", reason, lineNo)})
				reported = true
			}
		}
		continuing = isPrefix
		if isPrefix {
			carry = append(carry[:0], text[max(0, len(text)-scanOverlap):]...)
		}
	}
}

// lineFinding returns the reason text contains a credential, or "". The env
// assignment pattern only applies at the start of a line in env-like files.
func lineFinding(text string, envLine, testFile bool) string {
	if envLine && hasNonPlaceholderValue(envCredentialPattern, text, false) {
		return "env credential assignment"
	}
	for _, pattern := range forbiddenContentPatterns {
		switch pattern.reason {
		case "private key block":
			if pattern.re.MatchString(text) && !strings.Contains(text, "TEST PRIVATE KEY") {
				return pattern.reason
			}
		case "quoted credential assignment":
			if hasCredentialWord(text) && hasNonPlaceholderValue(pattern.re, text, testFile) {
				return pattern.reason
			}
		default:
			if pattern.re.MatchString(text) {
				return pattern.reason
			}
		}
	}
	return ""
}

// hasCredentialWord is a fast ASCII case-insensitive prefilter for the
// quoted credential pattern, which is slow to run over large binary files.
func hasCredentialWord(text string) bool {
	lower := make([]byte, len(text))
	for i := 0; i < len(text); i++ {
		c := text[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		lower[i] = c
	}
	for _, word := range []string{"key", "secret", "token", "password"} {
		if bytes.Contains(lower, []byte(word)) {
			return true
		}
	}
	return false
}

func hasNonPlaceholderValue(re *regexp.Regexp, text string, testFile bool) bool {
	for _, match := range re.FindAllStringSubmatch(text, -1) {
		value := match[1]
		if isPlaceholderValue(value) || testFile && fixtureValuePattern.MatchString(value) {
			continue
		}
		return true
	}
	return false
}

// fixtureValuePattern matches readable fixture values such as
// provider_token_for_tests: lowercase words joined by separators, with no
// digits or capitals, which generated secrets always contain.
var fixtureValuePattern = regexp.MustCompile(`^[a-z]+(?:[_.-][a-z]+)+$`)

// isPlaceholderValue reports whether a credential-shaped value is an obvious
// placeholder or a publishable client key rather than a secret. Values such
// as <your key>, ${SECRET} and env.SECRET never match the value patterns.
func isPlaceholderValue(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{"change-me", "changeme", "replace-me", "placeholder", "xxxx", "..."} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	if lower == "header.payload.signature" {
		return true
	}
	// Publishable client keys: Stripe and Clerk pk_, RevenueCat platform keys.
	for _, prefix := range []string{"pk_live_", "pk_test_", "appl_", "goog_", "amzn_"} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func isTestPath(rel string) bool {
	base := path.Base(slashPath(rel))
	return strings.Contains(base, ".test.") || strings.HasSuffix(base, "_test.go")
}

func isEnvLikePath(rel string) bool {
	base := path.Base(slashPath(rel))
	return base == ".env" || strings.HasPrefix(base, ".env.") || strings.HasSuffix(base, ".env")
}
