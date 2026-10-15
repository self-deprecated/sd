package mirror_test

import (
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

	plan, err := mirror.BuildPlan("app", root, config.MirrorConfig{Source: source, Include: []string{"**"}}, dest)
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
	plan, err := mirror.BuildPlan("app", root, config.MirrorConfig{Source: source, Include: []string{"**"}}, filepath.Join(root, "mirror"))
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
	plan, err := mirror.BuildPlan("app", root, config.MirrorConfig{Source: source, Include: []string{"**"}}, filepath.Join(root, "mirror"))
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
	plan, err := mirror.BuildPlan("app", root, config.MirrorConfig{Source: source, Include: []string{"**"}}, filepath.Join(root, "mirror"))
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
