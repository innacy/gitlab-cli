// Package e2e drives the real git-agent binary against a fake GitLab API.
//
// Each test runs the binary in its own temp dir (config.yaml, audit DB and
// output dirs are all cwd-relative), feeds REPL commands on stdin and asserts
// on stdout. No network, no real token, no AI provider.
package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const goodToken = "e2e-token"

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "git-agent-e2e")
	if err != nil {
		panic(err)
	}
	binPath = filepath.Join(dir, "git-agent")
	if out, err := exec.Command("go", "build", "-o", binPath, "gitlab-ai").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s", err, out)
		os.Exit(1)
	}

	// ponytail: the REPL hardcodes its history file in /tmp, so every run would
	// append test commands to the developer's real history. Snapshot and restore
	// it; drop this once the history path is configurable.
	const history = "/tmp/git-agent-history.tmp"
	saved, histErr := os.ReadFile(history)

	code := m.Run()

	if histErr == nil {
		os.WriteFile(history, saved, 0o644)
	} else {
		os.Remove(history)
	}
	os.RemoveAll(dir)
	os.Exit(code)
}

// ─── Fake GitLab ─────────────────────────────────────────────────────────────

type fakeGitLab struct {
	*httptest.Server
	mu    sync.Mutex
	calls []string // "METHOD path"
}

func (f *fakeGitLab) called(call string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.calls, call)
}

func newFakeGitLab(t *testing.T) *fakeGitLab {
	t.Helper()
	f := &fakeGitLab{}
	now := time.Now().UTC().Format(time.RFC3339)
	backend := fmt.Sprintf(`{"id":10,"name":"backend","path_with_namespace":"team/backend","default_branch":"main","last_activity_at":%q}`, now)

	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.EscapedPath(), "/api/v4")
		f.mu.Lock()
		f.calls = append(f.calls, r.Method+" "+path)
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("PRIVATE-TOKEN") != goodToken {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"message":"401 Unauthorized"}`)
			return
		}

		// The CLI addresses a project by path for reads and by numeric ID
		// (after resolving it) for some mutations; accept both.
		path = strings.Replace(path, "/projects/10/", "/projects/team%2Fbackend/", 1)
		const proj = "/projects/team%2Fbackend"
		switch {
		case path == "/user":
			io.WriteString(w, `{"id":1,"username":"tester","name":"Test User"}`)
		case path == "/projects":
			// "elsewhere/other" must be hidden by the team filter.
			fmt.Fprintf(w, `[%s,{"id":11,"name":"other","path_with_namespace":"elsewhere/other","default_branch":"main","last_activity_at":%q}]`, backend, now)
		case path == proj:
			io.WriteString(w, backend)
		case path == proj+"/merge_requests" && r.Method == http.MethodGet:
			fmt.Fprintf(w, `[{"id":70,"iid":7,"title":"Add login","state":"opened","author":{"username":"alice","name":"Alice"},"source_branch":"feat/login","target_branch":"main","updated_at":%q,"web_url":"http://x/mr/7"}]`, now)
		case path == proj+"/merge_requests/7/approve" && r.Method == http.MethodPost:
			io.WriteString(w, `{"id":70,"iid":7}`)
		case path == proj+"/pipelines":
			io.WriteString(w, `[{"id":99,"status":"failed","ref":"main","web_url":"http://x/p/99"}]`)
		default:
			t.Logf("fake gitlab: unhandled %s %s", r.Method, path)
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"message":"404 Not Found"}`)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// ─── Harness ─────────────────────────────────────────────────────────────────

// workdir creates an isolated cwd with a config.yaml pointing at baseURL.
// Pass baseURL "" to skip writing the config.
func workdir(t *testing.T, baseURL string) string {
	t.Helper()
	dir := t.TempDir()
	if baseURL != "" {
		cfg := fmt.Sprintf("gitlab:\n  base_url: %q\nteams:\n  - team\ncli:\n  open_in_browser: false\n", baseURL)
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func agentCmd(dir, token string) *exec.Cmd {
	cmd := exec.Command(binPath)
	cmd.Dir = dir
	// HOME is redirected so a developer's real ~/.netrc can never authenticate.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "NO_COLOR=1", "GITLAB_TOKEN=" + token}
	return cmd
}

// run feeds the commands (plus "exit") to the REPL and returns combined output
// and the exit code.
func run(t *testing.T, dir, token string, commands ...string) (string, int) {
	t.Helper()
	cmd := agentCmd(dir, token)
	cmd.Stdin = strings.NewReader(strings.Join(append(commands, "exit"), "\n") + "\n")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		cmd.Process.Kill()
		t.Fatalf("git-agent hung; output so far:\n%s", out.String())
	}
	return out.String(), cmd.ProcessState.ExitCode()
}

func mustContain(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q\n--- output ---\n%s", w, out)
		}
	}
}

func mustNotContain(t *testing.T, out string, bads ...string) {
	t.Helper()
	for _, b := range bads {
		if strings.Contains(out, b) {
			t.Errorf("output unexpectedly contains %q\n--- output ---\n%s", b, out)
		}
	}
}

// ─── Startup / config ────────────────────────────────────────────────────────

func TestMissingConfigFailsFast(t *testing.T) {
	out, code := run(t, workdir(t, ""), goodToken)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	mustContain(t, out, "Configuration errors", "gitlab.base_url is required")
}

func TestInvalidProviderFailsFast(t *testing.T) {
	dir := workdir(t, "")
	cfg := "gitlab:\n  base_url: \"http://127.0.0.1:1/\"\nai:\n  provider: bogus\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	out, code := run(t, dir, goodToken)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	mustContain(t, out, "ai.provider 'bogus' is not supported")
}

func TestEnvVarSuppliesBaseURL(t *testing.T) {
	gl := newFakeGitLab(t)
	cmd := agentCmd(workdir(t, ""), goodToken)
	cmd.Env = append(cmd.Env, "GITLAB_AI_GITLAB_BASE_URL="+gl.URL)
	cmd.Stdin = strings.NewReader("exit\n")
	out, _ := cmd.CombinedOutput()
	mustContain(t, string(out), "Authenticated as Test User (@tester)")
	mustNotContain(t, string(out), "Configuration errors")
}

func TestStartupAuthenticates(t *testing.T) {
	gl := newFakeGitLab(t)
	out, code := run(t, workdir(t, gl.URL), goodToken)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	mustContain(t, out,
		"Authenticated as Test User (@tester)",
		"Team: team",
		"Session started",
		"Session Summary",
		"Goodbye.",
		"Type 'help' for commands",
	)
	// The full command reference belongs to `help`, not the banner.
	mustNotContain(t, out, "Command Reference", "mr-status")
}

func TestBadTokenReportsAuthFailure(t *testing.T) {
	gl := newFakeGitLab(t)
	out, code := run(t, workdir(t, gl.URL), "wrong-token", "list")
	if code != 0 {
		t.Errorf("exit code = %d, want 0 (REPL stays usable after auth failure)", code)
	}
	mustContain(t, out, "Authentication failed")
	mustNotContain(t, out, "Authenticated as", "team/backend")
}

// ─── Read commands ───────────────────────────────────────────────────────────

func TestListAppliesTeamFilter(t *testing.T) {
	gl := newFakeGitLab(t)
	out, _ := run(t, workdir(t, gl.URL), goodToken, "list")
	mustContain(t, out, "Accessible Projects", "team/backend", "Total: 1 projects")
	mustNotContain(t, out, "elsewhere/other")
}

func TestMRStatusResolvesShortProjectName(t *testing.T) {
	gl := newFakeGitLab(t)
	out, _ := run(t, workdir(t, gl.URL), goodToken, "mr-status backend")
	mustContain(t, out,
		"Resolved 'backend' → team/backend",
		"Open MRs — team/backend",
		"Add login",
		"alice",
		"Total: 1 merge requests",
	)
}

func TestPipelineList(t *testing.T) {
	gl := newFakeGitLab(t)
	out, _ := run(t, workdir(t, gl.URL), goodToken, "pipeline team/backend")
	mustContain(t, out, "Pipelines — team/backend", "99", "failed", "Total: 1 pipelines")
}

func TestHelpConfigAndRenamedCommands(t *testing.T) {
	gl := newFakeGitLab(t)
	out, _ := run(t, workdir(t, gl.URL), goodToken, "help", "config", "raise-mr")
	mustContain(t, out,
		"git-agent — Command Reference",
		"pipeline-triage <project> [id]",
		"stats [week|month [YYYY-MM]|dashboard]",
		"switch-team",
		"Current Configuration",
		gl.URL,
		"Active Team:    team",
		"use 'mr-open' instead",
	)
}

// ─── Mutations + audit trail / stats ─────────────────────────────────────────

func TestApproveIsRecordedAndStatsPersistAcrossRuns(t *testing.T) {
	gl := newFakeGitLab(t)
	dir := workdir(t, gl.URL)

	out, _ := run(t, dir, goodToken, "mr-approve team/backend 7", "stats")
	mustContain(t, out, "MR #7 approved", "Commands run:     1")
	if !gl.called("POST /projects/10/merge_requests/7/approve") {
		t.Errorf("approve endpoint was never called; calls: %v", gl.calls)
	}

	// A second process in the same dir must read the same audit DB.
	out, _ = run(t, dir, goodToken, "stats week")
	mustContain(t, out, "Last 7 Days", "Commands run:     1")
}

func TestFailedCommandIsRecordedAsFailure(t *testing.T) {
	gl := newFakeGitLab(t)
	// MR 8 does not exist on the fake server, so its approve returns 404.
	out, _ := run(t, workdir(t, gl.URL), goodToken,
		"mr-approve team/backend 7", "mr-approve team/backend 8", "list")
	mustContain(t, out,
		"Failed to approve MR",
		"Commands run:    3 (1 failed)", // session summary, read from the audit DB
	)
}

func TestStatsRejectsBadMonth(t *testing.T) {
	gl := newFakeGitLab(t)
	out, _ := run(t, workdir(t, gl.URL), goodToken, "stats month 2026-13")
	mustContain(t, out, "Invalid date format")
}

func TestStatsDashboardServesAPI(t *testing.T) {
	gl := newFakeGitLab(t)
	dir := workdir(t, gl.URL)
	run(t, dir, goodToken, "mr-approve team/backend 7") // seed one event

	cmd := agentCmd(dir, goodToken)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()

	// Collect stdout; signal the dashboard address as soon as it is printed.
	addrCh := make(chan string, 1)
	rest := make(chan string, 1)
	go func() {
		re := regexp.MustCompile(`Dashboard running at (http://\S+)`)
		var all strings.Builder
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			all.WriteString(sc.Text())
			all.WriteString("\n")
			if m := re.FindStringSubmatch(sc.Text()); m != nil {
				addrCh <- m[1]
			}
		}
		rest <- all.String()
	}()

	io.WriteString(stdin, "stats dashboard\n")
	var addr string
	select {
	case addr = <-addrCh:
	case <-time.After(30 * time.Second):
		t.Fatal("dashboard address never printed")
	}

	resp, err := http.Get(addr + "/api/summary?range=week")
	if err != nil {
		t.Fatal(err)
	}
	var summary struct {
		TotalEvents int `json:"total_events"`
	}
	err = json.NewDecoder(resp.Body).Decode(&summary)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalEvents != 1 {
		t.Errorf("total_events = %d, want 1", summary.TotalEvents)
	}

	resp, err = http.Get(addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Contains(page, []byte("Activity Dashboard")) {
		t.Error("dashboard HTML not served at /")
	}

	io.WriteString(stdin, "\nexit\n") // Enter stops the dashboard, then exit
	stdin.Close()
	select {
	case out := <-rest:
		mustContain(t, out, "Dashboard stopped.", "Goodbye.")
	case <-time.After(30 * time.Second):
		t.Fatal("git-agent did not exit after dashboard stop")
	}
	if _, err := http.Get(addr + "/api/summary"); err == nil {
		t.Error("dashboard still reachable after stop")
	}
}
