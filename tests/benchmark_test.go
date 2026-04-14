package tests

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Dharshan2208/git-scanner/internal/aggregator"
	"github.com/Dharshan2208/git-scanner/internal/detector"
	"github.com/Dharshan2208/git-scanner/internal/walker"
	"github.com/Dharshan2208/git-scanner/internal/worker"
)

// ─────────────────────────────────────────────────────────────────────────────
// Real GitHub repos used for benchmarking
// ─────────────────────────────────────────────────────────────────────────────
//
//
// Medium repos — real OSS projects; benchmarks measure scanner throughput on
// realistic codebases rather than synthetic noise.
//
// All repos are shallow-cloned (--depth 1) and cached under os.TempDir().
// Re-running benchmarks skips the clone if the cache dir already exists.

type repoSpec struct {
	// Short label used in test/bench names and log output.
	label string
	// Full HTTPS clone URL.
	url string
	// Expected minimum number of findings. Set to 0 for clean/unknown repos.
	minFindings int
}

// mediumRepos are real OSS projects used for throughput / false-positive benchmarks.
var mediumRepos = []repoSpec{
	{
		label:       "gin-gonic/gin",
		url:         "https://github.com/gin-gonic/gin.git",
		minFindings: 0,
	},
	{
		label:       "gofiber/fiber",
		url:         "https://github.com/gofiber/fiber.git",
		minFindings: 0,
	},
	{
		label:       "labstack/echo",
		url:         "https://github.com/labstack/echo.git",
		minFindings: 0,
	},
}

// ─────────────────────────────────────────────────────────────────────────────
// Repo cache helpers
// ─────────────────────────────────────────────────────────────────────────────

// cacheRoot returns the base directory where all cloned repos live.
// Uses a fixed path under os.TempDir() so it survives across test runs.
func cacheRoot() string {
	return filepath.Join(os.TempDir(), "git-scanner-bench-cache")
}

// cloneOrUseCache returns a path to a shallow clone of repoURL.
// If the clone already exists under cacheRoot(), the clone is skipped.
// label is used as the directory name (slashes replaced with underscores).
func cloneOrUseCache(tb testing.TB, spec repoSpec) string {
	tb.Helper()

	safeName := strings.ReplaceAll(spec.label, "/", "_")
	dest := filepath.Join(cacheRoot(), safeName)

	// Already cached — verify it looks like a git repo.
	if info, err := os.Stat(filepath.Join(dest, ".git")); err == nil && info.IsDir() {
		tb.Logf("  [cache hit]  %s → %s", spec.label, dest)
		return dest
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		tb.Fatalf("failed to create cache dir %s: %v", dest, err)
	}

	tb.Logf("  [cloning]    %s …", spec.url)
	start := time.Now()

	cmd := exec.Command(
		"git", "clone",
		"--depth", "1",
		"--single-branch",
		"--no-tags",
		spec.url,
		dest,
	)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if out, err := cmd.CombinedOutput(); err != nil {
		// Clean up partial clone so the next run retries.
		os.RemoveAll(dest)
		tb.Fatalf("git clone failed for %s: %v\n%s", spec.url, err, string(out))
	}

	tb.Logf("  [cloned]     %s in %v", spec.label, time.Since(start).Round(time.Millisecond))
	return dest
}

// ─────────────────────────────────────────────────────────────────────────────
// Synthetic test data (preserved from original)
// ─────────────────────────────────────────────────────────────────────────────

// NOTE: All "secrets" below are intentionally malformed/fake test fixtures.
// They exist solely to exercise the scanner's pattern-matching logic and
// will not pass validation against any real service.
var plantedSecrets = []struct {
	filename string
	content  string
}{
	{
		"config/database.yml",
		`database:
  host: db.production.internal
  port: 5432
  username: admin
  password: <TEST_PLACEHOLDER_PASSWORD>
  connection_string: postgresql://admin:<TEST_PLACEHOLDER_PASSWORD>@db.prod:5432/mydb
`,
	},
	{
		"src/api/client.py",
		`import requests

API_BASE = "https://api.example.com/v2"
api_key = "sk-proj-XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"

def get_data():
    headers = {"Authorization": f"Bearer {api_key}"}
    return requests.get(API_BASE, headers=headers)
`,
	},
	{
		"src/services/aws.go",
		`package services

const (
	awsAccessKeyID     = "AKIAXXXXXXXXXXXXXXXX"
	awsSecretAccessKey = "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
	region             = "us-east-1"
)
`,
	},
	{
		"src/services/stripe.go",
		`package services

var stripeKey = "sk_live_XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
var webhookSecret = "whsec_XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
`,
	},
	{
		".env",
		`DATABASE_URL=postgresql://user:<TEST_PLACEHOLDER_PASSWORD>@localhost:5432/myapp
REDIS_URL=redis://:<TEST_PLACEHOLDER_PASSWORD>@redis.internal:6379
SECRET_KEY=XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
API_KEY=sk-ant-api03-XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
GITHUB_TOKEN=ghp_XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
`,
	},
	{
		"src/bot/discord.js",
		`const { Client } = require('discord.js');
const TOKEN = "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX";
const WEBHOOK = "https://discord.com/api/webhooks/000000000000000000/XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX";
`,
	},
	{
		"src/integrations/slack.ts",
		`const slackBotToken = "xoxb-0000000000-0000000000000-XXXXXXXXXXXXXXXXXXXXXXXX";
const slackWebhook  = "https://hooks.slack.com/services/XXXXXXXXX/XXXXXXXXX/XXXXXXXXXXXXXXXXXXXXXXXX";
`,
	},
	{
		"deploy/terraform.tf",
		`variable "db_password" {
  default = "mongodb+srv://admin:<TEST_PLACEHOLDER_PASSWORD>@cluster0.mongodb.net"
}
`,
	},
	{
		"config/firebase.json",
		`{
  "apiKey": "AIzaSyXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
  "databaseURL": "https://myproject-12345.firebaseio.com",
  "serverKey": "XXXXXXXX:XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
}
`,
	},
	{
		"src/auth/jwt.go",
		`package auth

var supabaseAnonKey = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJvbGUiOiJ0ZXN0In0.XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
`,
	},
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func init() {
	detector.LoadSignatures()
}

func generateTestRepo(tb testing.TB, numCopies int) string {
	tb.Helper()

	dir, err := os.MkdirTemp("", "git-scanner-bench-*")
	if err != nil {
		tb.Fatal("failed to create temp dir:", err)
	}

	for copy := 0; copy < numCopies; copy++ {
		for _, secret := range plantedSecrets {
			var name string
			if numCopies == 1 {
				name = secret.filename
			} else {
				ext := filepath.Ext(secret.filename)
				base := strings.TrimSuffix(secret.filename, ext)
				name = fmt.Sprintf("%s_%d%s", base, copy, ext)
			}

			fullPath := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				tb.Fatal("failed to create dir:", err)
			}
			if err := os.WriteFile(fullPath, []byte(secret.content), 0o644); err != nil {
				tb.Fatal("failed to write file:", err)
			}
		}
	}

	return dir
}

func generateLargeRepo(tb testing.TB, totalFiles int, secretRatio float64) string {
	tb.Helper()

	dir, err := os.MkdirTemp("", "git-scanner-large-bench-*")
	if err != nil {
		tb.Fatal("failed to create temp dir:", err)
	}

	r := rand.New(rand.NewSource(42))

	extensions := []string{".go", ".py", ".js", ".ts", ".json", ".yaml", ".env", ".txt"}
	cleanLines := []string{
		"package main",
		"import \"fmt\"",
		"func main() {",
		"    fmt.Println(\"hello world\")",
		"}",
		"// This is a normal comment",
		"var x = 42",
		"const name = \"myapp\"",
		"type Config struct { Host string }",
		"func (c Config) String() string { return c.Host }",
	}

	secretLines := []string{
		`api_key = "sk-proj-XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"`,
		`password: "<TEST_PLACEHOLDER_PASSWORD>"`,
		`AKIAXXXXXXXXXXXXXXXX`,
		`secret_key = "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"`,
		`token = "ghp_XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"`,
		`sk_live_XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX`,
	}

	for i := 0; i < totalFiles; i++ {
		ext := extensions[r.Intn(len(extensions))]
		subdir := fmt.Sprintf("pkg/module_%d", i%20)
		filename := fmt.Sprintf("file_%d%s", i, ext)
		fullPath := filepath.Join(dir, subdir, filename)

		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			tb.Fatal(err)
		}

		var lines []string
		numLines := 50 + r.Intn(200)

		for j := 0; j < numLines; j++ {
			if r.Float64() < secretRatio {
				lines = append(lines, secretLines[r.Intn(len(secretLines))])
			} else {
				lines = append(lines, cleanLines[r.Intn(len(cleanLines))])
			}
		}

		if err := os.WriteFile(fullPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			tb.Fatal(err)
		}
	}

	return dir
}

// countFiles returns the total number of regular files under root.
func countFiles(root string) int {
	count := 0
	_ = filepath.WalkDir(root, func(_ string, d os.DirEntry, _ error) error {
		if !d.IsDir() {
			count++
		}
		return nil
	})
	return count
}

// ─────────────────────────────────────────────────────────────────────────────
// Memory / timing helpers
// ─────────────────────────────────────────────────────────────────────────────

type memStats struct {
	HeapAllocMB    float64
	HeapObjectsK   float64
	TotalAllocMB   float64
	NumGC          uint32
	GoroutineCount int
}

func captureMemStats() memStats {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return memStats{
		HeapAllocMB:    float64(m.HeapAlloc) / 1024 / 1024,
		HeapObjectsK:   float64(m.HeapObjects) / 1000,
		TotalAllocMB:   float64(m.TotalAlloc) / 1024 / 1024,
		NumGC:          m.NumGC,
		GoroutineCount: runtime.NumGoroutine(),
	}
}

func runFullPipeline(repoPath string) ([]worker.Finding, time.Duration, memStats, memStats) {
	before := captureMemStats()
	start := time.Now()

	jobs := make(chan worker.Job, 200)
	results := worker.StartWorkerPool(jobs)

	go func() {
		_ = walker.Walk(repoPath, jobs)
	}()

	findings := aggregator.Aggregate(results)

	elapsed := time.Since(start)
	after := captureMemStats()

	return findings, elapsed, before, after
}

// printStats is a shared pretty-printer for test log output.
func printStats(t *testing.T, label string, fileCount, findingCount int, elapsed time.Duration, before, after memStats) {
	t.Helper()
	t.Logf("═══════════════════════════════════════════════════════════════")
	t.Logf("  %s", label)
	t.Logf("═══════════════════════════════════════════════════════════════")
	t.Logf("  Files scanned  : %d", fileCount)
	t.Logf("  Findings       : %d", findingCount)
	t.Logf("  Wall time      : %v", elapsed)
	if fileCount > 0 {
		t.Logf("  Throughput     : %.0f files/s", float64(fileCount)/elapsed.Seconds())
	}
	t.Logf("  ───────────────────────────────────────────────────────────────")
	t.Logf("  MEMORY")
	t.Logf("  Heap before    : %.2f MB  (%.1fK objects)", before.HeapAllocMB, before.HeapObjectsK)
	t.Logf("  Heap after     : %.2f MB  (%.1fK objects)", after.HeapAllocMB, after.HeapObjectsK)
	t.Logf("  Heap delta     : %.2f MB", after.HeapAllocMB-before.HeapAllocMB)
	t.Logf("  Total alloc Δ  : %.2f MB", after.TotalAllocMB-before.TotalAllocMB)
	t.Logf("  GC runs Δ      : %d", after.NumGC-before.NumGC)
	t.Logf("  ───────────────────────────────────────────────────────────────")
	t.Logf("  CONCURRENCY")
	t.Logf("  Goroutines     : %d before → %d after", before.GoroutineCount, after.GoroutineCount)
	t.Logf("  Worker count   : %d (NumCPU×4 = %d×4)", runtime.NumCPU()*4, runtime.NumCPU())
	t.Logf("═══════════════════════════════════════════════════════════════")
}

// ─────────────────────────────────────────────────────────────────────────────
// Synthetic tests (original suite — preserved)
// ─────────────────────────────────────────────────────────────────────────────

func TestFullPipelineSmall(t *testing.T) {
	dir := generateTestRepo(t, 1)
	defer os.RemoveAll(dir)

	findings, elapsed, before, after := runFullPipeline(dir)
	printStats(t, "FULL PIPELINE — SMALL REPO (10 secret files)", countFiles(dir), len(findings), elapsed, before, after)

	if len(findings) == 0 {
		t.Error("expected findings but got 0 — signatures may not be loaded")
	}
	for i, f := range findings {
		rel, _ := filepath.Rel(dir, f.File)
		t.Logf("  [%2d] %-30s | Line %3d | %s", i+1, rel, f.Line, f.Type)
	}
}

func TestFullPipelineMedium(t *testing.T) {
	dir := generateLargeRepo(t, 100, 0.05)
	defer os.RemoveAll(dir)

	findings, elapsed, before, after := runFullPipeline(dir)
	printStats(t, "FULL PIPELINE — MEDIUM REPO (100 files, ~5% secret lines)", 100, len(findings), elapsed, before, after)
}

func TestFullPipelineLarge(t *testing.T) {
	dir := generateLargeRepo(t, 1000, 0.02)
	defer os.RemoveAll(dir)

	findings, elapsed, before, after := runFullPipeline(dir)
	printStats(t, "FULL PIPELINE — LARGE REPO (1000 files, ~2% secret lines)", 1000, len(findings), elapsed, before, after)

	if elapsed > 30*time.Second {
		t.Errorf("scan took too long: %v (threshold: 30s)", elapsed)
	}
}

func TestFullPipelineCleanRepo(t *testing.T) {
	dir, err := os.MkdirTemp("", "git-scanner-clean-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	cleanContent := `package main

import "fmt"

func main() {
    fmt.Println("hello world")
    x := 42
    name := "myapp"
    fmt.Printf("running %s version %d\n", name, x)
}
`
	for i := 0; i < 50; i++ {
		path := filepath.Join(dir, fmt.Sprintf("pkg/module_%d/main.go", i))
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(cleanContent), 0o644)
	}

	findings, elapsed, before, after := runFullPipeline(dir)
	printStats(t, "FULL PIPELINE — CLEAN REPO (50 files, 0 secrets)", 50, len(findings), elapsed, before, after)

	if len(findings) > 0 {
		t.Errorf("expected 0 findings in clean repo, got %d (false positives!)", len(findings))
		for _, f := range findings {
			rel, _ := filepath.Rel(dir, f.File)
			t.Logf("  FALSE POSITIVE: %s | Line %d | %s | %s", rel, f.Line, f.Type, f.Match)
		}
	}
}

func TestGoroutineLeakCheck(t *testing.T) {
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	goroutinesBefore := runtime.NumGoroutine()

	for i := 0; i < 5; i++ {
		dir := generateTestRepo(t, 1)
		runFullPipeline(dir)
		os.RemoveAll(dir)
	}

	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	goroutinesAfter := runtime.NumGoroutine()

	t.Logf("═══════════════════════════════════════════════════════════════")
	t.Logf("  GOROUTINE LEAK CHECK (5 consecutive scans)")
	t.Logf("═══════════════════════════════════════════════════════════════")
	t.Logf("  Goroutines before : %d", goroutinesBefore)
	t.Logf("  Goroutines after  : %d", goroutinesAfter)
	t.Logf("  Leaked            : %d", goroutinesAfter-goroutinesBefore)
	t.Logf("═══════════════════════════════════════════════════════════════")

	if leaked := goroutinesAfter - goroutinesBefore; leaked > 5 {
		t.Errorf("potential goroutine leak: %d goroutines added after 5 scans", leaked)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Real-repo tests
// ─────────────────────────────────────────────────────────────────────────────

// TestRealRepo_MediumOSS clones real OSS projects and measures false-positive
// rate and scanner throughput on production-quality code.
func TestRealRepo_MediumOSS(t *testing.T) {
	for _, spec := range mediumRepos {
		spec := spec
		t.Run(strings.ReplaceAll(spec.label, "/", "_"), func(t *testing.T) {
			t.Parallel()

			dir := cloneOrUseCache(t, spec)
			fileCount := countFiles(dir)

			findings, elapsed, before, after := runFullPipeline(dir)
			printStats(t, fmt.Sprintf("OSS REPO — %s", spec.label), fileCount, len(findings), elapsed, before, after)

			// Log every finding so false positives can be triaged.
			for i, f := range findings {
				rel, _ := filepath.Rel(dir, f.File)
				t.Logf("  [%2d] %-40s | Line %3d | %-20s | %s", i+1, rel, f.Line, f.Type, f.Match)
			}

			// Throughput guard: even large repos should complete in reasonable time.
			limit := time.Duration(fileCount)*5*time.Millisecond + 5*time.Second
			if elapsed > limit {
				t.Errorf("scan too slow: %v for %d files (limit %v)", elapsed, fileCount, limit)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Benchmarks
// ─────────────────────────────────────────────────────────────────────────────
//
// Run with: go test -bench=. -benchmem -benchtime=3x ./tests/
//
// The real-repo benchmarks use the cache, so the first run pays the clone cost
// but subsequent runs are instant.

func BenchmarkFullPipeline_10Files(b *testing.B) {
	dir := generateTestRepo(b, 1)
	defer os.RemoveAll(dir)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		jobs := make(chan worker.Job, 200)
		results := worker.StartWorkerPool(jobs)
		go func() { _ = walker.Walk(dir, jobs) }()
		_ = aggregator.Aggregate(results)
	}
}

func BenchmarkFullPipeline_100Files(b *testing.B) {
	dir := generateLargeRepo(b, 100, 0.05)
	defer os.RemoveAll(dir)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		jobs := make(chan worker.Job, 200)
		results := worker.StartWorkerPool(jobs)
		go func() { _ = walker.Walk(dir, jobs) }()
		_ = aggregator.Aggregate(results)
	}
}

func BenchmarkFullPipeline_500Files(b *testing.B) {
	dir := generateLargeRepo(b, 500, 0.02)
	defer os.RemoveAll(dir)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		jobs := make(chan worker.Job, 200)
		results := worker.StartWorkerPool(jobs)
		go func() { _ = walker.Walk(dir, jobs) }()
		_ = aggregator.Aggregate(results)
	}
}

func BenchmarkFullPipeline_1000Files(b *testing.B) {
	dir := generateLargeRepo(b, 1000, 0.02)
	defer os.RemoveAll(dir)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		jobs := make(chan worker.Job, 200)
		results := worker.StartWorkerPool(jobs)
		go func() { _ = walker.Walk(dir, jobs) }()
		_ = aggregator.Aggregate(results)
	}
}

func BenchmarkFullPipeline_5000Files(b *testing.B) {
	dir := generateLargeRepo(b, 5000, 0.01)
	defer os.RemoveAll(dir)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		jobs := make(chan worker.Job, 200)
		results := worker.StartWorkerPool(jobs)
		go func() { _ = walker.Walk(dir, jobs) }()
		_ = aggregator.Aggregate(results)
	}
}

// BenchmarkRealRepo_MediumOSS benchmarks the full pipeline against real OSS
// repos to measure throughput on production-quality code.
func BenchmarkRealRepo_MediumOSS(b *testing.B) {
	for _, spec := range mediumRepos {
		spec := spec
		b.Run(strings.ReplaceAll(spec.label, "/", "_"), func(b *testing.B) {
			dir := cloneOrUseCache(b, spec)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				jobs := make(chan worker.Job, 200)
				results := worker.StartWorkerPool(jobs)
				go func() { _ = walker.Walk(dir, jobs) }()
				_ = aggregator.Aggregate(results)
			}
		})
	}
}
