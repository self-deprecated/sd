package mirror_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/self-deprecated/sd/internal/config"
	"github.com/self-deprecated/sd/internal/mirror"
)

func TestScanPlanFindsForbiddenPathsAndContent(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "project")
	dest := filepath.Join(root, "mirror")
	write(t, filepath.Join(source, "README.md"), "hello")
	write(t, filepath.Join(source, ".env"), "TOKEN=secret")
	write(t, filepath.Join(source, "leaky.env"), "TOKEN=abcdefghijklmnopqrstuvwxyz123456")
	write(t, filepath.Join(source, "config.ts"), `export const apiKey = "abcdefghijklmnopqrstuvwxyz123456"`)

	plan, err := mirror.BuildPlan("app", source, config.MirrorConfig{Source: source, Include: []string{"**"}}, dest)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	report, err := mirror.ScanPlan(plan)
	if err != nil {
		t.Fatalf("ScanPlan: %v", err)
	}
	if !report.HasFindings() {
		t.Fatalf("expected findings")
	}
	text := report.Error()
	for _, want := range []string{".env", "forbidden path pattern", "leaky.env", "config.ts", "quoted credential assignment"} {
		if !strings.Contains(text, want) {
			t.Fatalf("scan output missing %q: %s", want, text)
		}
	}
}

func TestScanPlanAllowsTestPrivateKeyFixtures(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "project")
	write(t, filepath.Join(source, "redaction.test.ts"), "-----BEGIN TEST PRIVATE KEY-----\nsecret-key\n-----END TEST PRIVATE KEY-----")
	plan, err := mirror.BuildPlan("app", source, config.MirrorConfig{Source: source, Include: []string{"**"}}, filepath.Join(root, "mirror"))
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	report, err := mirror.ScanPlan(plan)
	if err != nil {
		t.Fatalf("ScanPlan: %v", err)
	}
	if report.HasFindings() {
		t.Fatalf("unexpected findings: %s", report.Error())
	}
}

func TestScanPlanAllowsEnvExamplesAndCodeReferencesToCredentials(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "project")
	write(t, filepath.Join(source, ".env.example"), "# TOKEN=change-me\nDEMO_CLERK_SECRET_KEY=sk_...\n")
	write(t, filepath.Join(source, "src", "config.ts"), "adminToken: env.DEMO_ADMIN_TOKEN,\nsecret: optionalEnv(env.DEMO_SESSION_SECRET),\n")
	write(t, filepath.Join(source, "src", "App.svelte"), "adminToken = localStorage.getItem(adminTokenStorageKey) ?? '';\n")
	write(t, filepath.Join(source, "src", "purchases.ts"), "const defaultRevenueCatIOSAPIKey = \"appl_jjQlssmgYrPUZmFPbFuSyVOfPCV\";\n")
	write(t, filepath.Join(source, "src", "client.test.ts"), "const token = \"header.payload.signature\";\n")
	plan, err := mirror.BuildPlan("app", source, config.MirrorConfig{Source: source, Include: []string{"**"}}, filepath.Join(root, "mirror"))
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	report, err := mirror.ScanPlan(plan)
	if err != nil {
		t.Fatalf("ScanPlan: %v", err)
	}
	if report.HasFindings() {
		t.Fatalf("unexpected findings: %s", report.Error())
	}
}

func TestScanPlanAllowsOrdinaryFiles(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "project")
	write(t, filepath.Join(source, "README.md"), "ordinary docs")
	plan, err := mirror.BuildPlan("app", source, config.MirrorConfig{Source: source, Include: []string{"**"}}, filepath.Join(root, "mirror"))
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	report, err := mirror.ScanPlan(plan)
	if err != nil {
		t.Fatalf("ScanPlan: %v", err)
	}
	if report.HasFindings() {
		t.Fatalf("unexpected findings: %s", report.Error())
	}
}

func scanSource(t *testing.T, source string) mirror.ScanReport {
	t.Helper()
	plan, err := mirror.BuildPlan("app", source, config.MirrorConfig{Source: source}, filepath.Join(t.TempDir(), "mirror"))
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	report, err := mirror.ScanPlan(plan)
	if err != nil {
		t.Fatalf("ScanPlan: %v", err)
	}
	return report
}

const githubToken = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"

func TestScanPlanScansLargeFilesAndLongLines(t *testing.T) {
	source := filepath.Join(t.TempDir(), "project")
	filler := strings.Repeat("ordinary line of generated output\n", 100_000)
	write(t, filepath.Join(source, "large.log"), filler+"token "+githubToken+"\n")
	write(t, filepath.Join(source, "minified.js"), strings.Repeat("a", 3<<20)+" "+githubToken)
	for _, offset := range []int{64<<10 - 15, 1<<20 - 15} {
		write(t, filepath.Join(source, fmt.Sprintf("straddle-%d.js", offset)), strings.Repeat("a", offset)+" "+githubToken)
	}
	text := scanSource(t, source).Error()
	for _, want := range []string{"large.log: GitHub token on line 100001", "minified.js: GitHub token on line 1", "straddle-65521.js", "straddle-1048561.js"} {
		if !strings.Contains(text, want) {
			t.Errorf("scan output missing %q: %s", want, text)
		}
	}
}

func TestScanPlanHasNoBroadExemptions(t *testing.T) {
	source := filepath.Join(t.TempDir(), "project")
	write(t, filepath.Join(source, "auth.test.ts"), `const password = "Zq8vN2kLp4Rt7Yw1Xc5Bm9Hd";`+"\n")
	write(t, filepath.Join(source, "auth_test.go"), `apiKey := "Zq8vN2kLp4Rt7Yw1Xc5Bm9Hd"`+"\n")
	write(t, filepath.Join(source, "client.ts"), `const apiKey = "Zq8vN2kLp4Rt7Yw1Xc5Bm9Hd"; // public API`+"\n")
	write(t, filepath.Join(source, "billing.ts"), `const secret = "Zq8vN2kLp4Rt7Yw1Xc5Bm9Hd"; // not the publishable or RevenueCat key`+"\n")
	write(t, filepath.Join(source, ".env.example"), "API_TOKEN=Zq8vN2kLp4Rt7Yw1Xc5Bm9Hd\n")
	write(t, filepath.Join(source, "web", ".env.sample"), "export DB_PASSWORD='Zq8vN2kLp4Rt7Yw1Xc5Bm9Hd'\n")
	// Word-like fixture values are only accepted in test files.
	write(t, filepath.Join(source, "db.ts"), `const password = "staging-database-password";`+"\n")
	write(t, filepath.Join(source, "fixture.test.ts"), `const token = "atk_live_0123456789abcdef";`+"\n")
	text := scanSource(t, source).Error()
	for _, want := range []string{"auth.test.ts: quoted credential", "auth_test.go: quoted credential", "client.ts: quoted credential", "billing.ts: quoted credential", ".env.example: env credential", "web/.env.sample: env credential", "db.ts: quoted credential", "fixture.test.ts: quoted credential"} {
		if !strings.Contains(text, want) {
			t.Errorf("scan output missing %q: %s", want, text)
		}
	}
}

func TestScanPlanAllowsPlaceholderAndPublishableValues(t *testing.T) {
	source := filepath.Join(t.TempDir(), "project")
	write(t, filepath.Join(source, ".env.example"), "API_TOKEN=change-me-to-your-own-token\nDB_PASSWORD=xxxxxxxxxxxxxxxxxxxxxxxx\nSECRET_KEY=${SECRET_KEY}\nCLERK_SECRET_KEY=<your clerk secret key>\n")
	write(t, filepath.Join(source, "config.ts"), `const publishableKey = "pk_live_abcdefghijklmnopqrstuvwxyz"; const apiKey = "placeholder-api-key-value";`+"\n")
	write(t, filepath.Join(source, "session.test.ts"), `const sessionToken = "provider_token_for_tests"; const password = 'fixture-not-a-secret';`+"\n")
	write(t, filepath.Join(source, "session_test.go"), `token := "provider-session-token-fixture"`+"\n")
	if report := scanSource(t, source); report.HasFindings() {
		t.Fatalf("unexpected findings: %s", report.Error())
	}
}

func TestScanPlanReportsFilesItCannotScan(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "project")
	write(t, filepath.Join(source, "notes.txt"), "hello")
	write(t, filepath.Join(root, "private.env"), "TOKEN="+githubToken)
	plan, err := mirror.BuildPlan("app", source, config.MirrorConfig{Source: source}, filepath.Join(t.TempDir(), "mirror"))
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	// The file is replaced by a symlink after planning.
	if err := os.Remove(filepath.Join(source, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	symlink(t, filepath.Join(root, "private.env"), filepath.Join(source, "notes.txt"))
	report, err := mirror.ScanPlan(plan)
	if err != nil {
		t.Fatalf("ScanPlan: %v", err)
	}
	if text := report.Error(); !strings.Contains(text, "notes.txt: not a regular file") {
		t.Fatalf("expected unscannable file finding, got %s", text)
	}
}
