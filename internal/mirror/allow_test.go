package mirror_test

import (
	"path/filepath"
	"testing"

	"github.com/self-deprecated/sd/internal/config"
	"github.com/self-deprecated/sd/internal/mirror"
)

func TestAllowedPathsAreReportedButDoNotFail(t *testing.T) {
	source := filepath.Join(t.TempDir(), "project")
	write(t, filepath.Join(source, "test", "fixtures.test.ts"), `const token = "Zq8vN2kLp4Rt7Yw1Xc5Bm9Hd";`+"\n")
	write(t, filepath.Join(source, "src", "client.ts"), `const token = "Zq8vN2kLp4Rt7Yw1Xc5Bm9Hd";`+"\n")
	plan, err := mirror.BuildPlan("app", source, config.MirrorConfig{Source: source, Allow: []string{"test/fixtures.test.ts"}}, filepath.Join(t.TempDir(), "mirror"))
	if err != nil {
		t.Fatal(err)
	}
	report, err := mirror.ScanPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Findings) != 1 || report.Findings[0].Path != "src/client.ts" {
		t.Fatalf("findings = %+v, want only src/client.ts", report.Findings)
	}
	if len(report.Allowed) != 1 || report.Allowed[0].Path != "test/fixtures.test.ts" {
		t.Fatalf("allowed = %+v, want test/fixtures.test.ts", report.Allowed)
	}

	destination := t.TempDir()
	write(t, filepath.Join(destination, "test", "fixtures.test.ts"), `const token = "Zq8vN2kLp4Rt7Yw1Xc5Bm9Hd";`+"\n")
	report, err = mirror.ScanFiles(destination, []string{"test/fixtures.test.ts"}, plan.Allow)
	if err != nil || report.HasFindings() || len(report.Allowed) != 1 {
		t.Fatalf("ScanFiles report = %+v err = %v", report, err)
	}
}

func TestMalformedAllowPatternsAreConfigErrors(t *testing.T) {
	source := filepath.Join(t.TempDir(), "project")
	write(t, filepath.Join(source, "README.md"), "hello")
	if _, err := mirror.BuildPlan("app", source, config.MirrorConfig{Source: source, Allow: []string{"[abc"}}, filepath.Join(t.TempDir(), "mirror")); err == nil {
		t.Fatal("expected a malformed allow pattern to be rejected")
	}
}

// allow only accepts reviewed content findings; forbidden paths such as
// .env files and files that cannot be scanned always block.
func TestAllowNeverWaivesForbiddenPathsOrUnscannableFiles(t *testing.T) {
	source := filepath.Join(t.TempDir(), "project")
	write(t, filepath.Join(source, "test", ".env"), "TOKEN=ghp_abcdefghijklmnopqrstuvwxyz0123456789\n")
	write(t, filepath.Join(source, "test", "id_rsa"), "key\n")
	plan, err := mirror.BuildPlan("app", source, config.MirrorConfig{Source: source, Allow: []string{"test/**"}}, filepath.Join(t.TempDir(), "mirror"))
	if err != nil {
		t.Fatal(err)
	}
	report, err := mirror.ScanPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Findings) != 2 || len(report.Allowed) != 0 {
		t.Fatalf("report = %+v, want both forbidden paths to block", report)
	}

	destination := t.TempDir()
	write(t, filepath.Join(destination, "test", ".env"), "TOKEN=x\n")
	report, err = mirror.ScanFiles(destination, []string{"test/.env"}, []string{"test/**"})
	if err != nil || len(report.Findings) != 1 {
		t.Fatalf("ScanFiles report = %+v err = %v, want the .env to block", report, err)
	}
}
