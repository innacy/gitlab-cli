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
	"unicode/utf8"
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

	code := m.Run()
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
		case path == proj+"/merge_requests/7" && r.Method == http.MethodGet:
			fmt.Fprintf(w, `{"id":70,"iid":7,"title":"Add login","state":"opened","author":{"username":"alice","name":"Alice"},"source_branch":"feat/login","target_branch":"main","web_url":"http://x/mr/7","updated_at":%q}`, now)
		case path == proj+"/merge_requests/7/changes":
			io.WriteString(w, `{"id":70,"iid":7,"title":"Add login","changes":[{"old_path":"login.go","new_path":"login.go","diff":"@@ -1 +1,2 @@\n+func Login() {}\n"}]}`)
		case path == proj+"/merge_requests/7/approve" && r.Method == http.MethodPost:
			io.WriteString(w, `{"id":70,"iid":7}`)
		case path == proj+"/pipelines/99/cancel" && r.Method == http.MethodPost:
			io.WriteString(w, `{"id":99,"status":"canceled","ref":"main"}`)
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

// ─── Fake Anthropic ──────────────────────────────────────────────────────────

// newFakeAnthropic answers the Messages API. reply maps the request (system
// prompt, user message) to the response text.
func newFakeAnthropic(t *testing.T, reply func(system, user string) string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "sk-test" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"bad key"}}`)
			return
		}
		var req struct {
			System   string `json:"system"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		text, _ := json.Marshal(reply(req.System, req.Messages[0].Content))
		fmt.Fprintf(w, `{"content":[{"type":"text","text":%s}],"usage":{"input_tokens":100,"output_tokens":20}}`, text)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// aiWorkdir is workdir plus an Anthropic provider pointed at the fake.
func aiWorkdir(t *testing.T, gitlabURL, aiURL string) string {
	t.Helper()
	dir := workdir(t, gitlabURL)
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg, _ := os.ReadFile(cfgPath)
	cfg = fmt.Appendf(cfg, "ai:\n  provider: anthropic\n  anthropic:\n    api_key: sk-test\n    model: test-model\n    base_url: %q\n", aiURL)
	cfg = append(cfg, `review:
  template:
    sections:
      - name: Summary
        prompt: Summarise
        required: true
`...)
	os.WriteFile(cfgPath, cfg, 0o600)
	return dir
}

func isIntentCall(system string) bool { return strings.Contains(system, "command classifier") }

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
	// HOME is redirected so a developer's real ~/.netrc, config and history
	// are never read or written.
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
	failOnRace(t, out.String())
	return out.String(), cmd.ProcessState.ExitCode()
}

// failOnRace catches data races in the binary when the suite runs with
// GOFLAGS=-race (TestMain's go build inherits it).
func failOnRace(t *testing.T, out string) {
	t.Helper()
	if strings.Contains(out, "WARNING: DATA RACE") {
		t.Errorf("data race in git-agent:\n%s", out)
	}
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

func TestConfigInUserDirWorksFromAnyCwd(t *testing.T) {
	gl := newFakeGitLab(t)
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "git-agent")
	os.MkdirAll(cfgDir, 0o700)
	os.WriteFile(filepath.Join(cfgDir, "config.yaml"),
		fmt.Appendf(nil, "gitlab:\n  base_url: %q\nteams:\n  - team\n", gl.URL), 0o600)

	runIn := func(cwd string, commands ...string) string {
		cmd := agentCmd(cwd, goodToken)
		cmd.Env = append(cmd.Env, "HOME="+home) // later HOME wins
		cmd.Stdin = strings.NewReader(strings.Join(append(commands, "exit"), "\n") + "\n")
		out, _ := cmd.CombinedOutput()
		return string(out)
	}

	out := runIn(t.TempDir(), "mr-approve team/backend 7")
	mustContain(t, out, "Authenticated as Test User", "MR #7 approved")

	// A different cwd sees the same audit trail.
	out = runIn(t.TempDir(), "stats week")
	mustContain(t, out, "Commands run:     1")

	hist, err := os.ReadFile(filepath.Join(cfgDir, "history"))
	if err != nil || !strings.Contains(string(hist), "mr-approve team/backend 7") {
		t.Errorf("history not written to %s: %v %q", cfgDir, err, hist)
	}
	if _, err := os.Stat(filepath.Join(cfgDir, "audit", "audit.db")); err != nil {
		t.Errorf("audit DB not in config dir: %v", err)
	}
}

func TestConfigFlag(t *testing.T) {
	gl := newFakeGitLab(t)
	cfgPath := filepath.Join(t.TempDir(), "custom.yaml")
	os.WriteFile(cfgPath, fmt.Appendf(nil, "gitlab:\n  base_url: %q\n", gl.URL), 0o600)

	cmd := agentCmd(t.TempDir(), goodToken)
	cmd.Args = append(cmd.Args, "--config", cfgPath)
	cmd.Stdin = strings.NewReader("exit\n")
	out, _ := cmd.CombinedOutput()
	mustContain(t, string(out), "Authenticated as Test User")
}

func TestEnvironmentProfiles(t *testing.T) {
	glA, glB := newFakeGitLab(t), newFakeGitLab(t)
	dir := workdir(t, "")
	cfg := fmt.Sprintf(`gitlab:
  base_url: %q
teams:
  - team
environments:
  a:
    gitlab:
      base_url: %q
  b:
    gitlab:
      base_url: %q
`, glA.URL, glA.URL, glB.URL)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600)

	cmd := agentCmd(dir, goodToken)
	cmd.Args = append(cmd.Args, "--env", "b")
	cmd.Stdin = strings.NewReader("switch-env nope\nswitch-env a\nconfig\nexit\n")
	raw, _ := cmd.CombinedOutput()
	out := string(raw)
	failOnRace(t, out)

	mustContain(t, out,
		"Environment: b",
		`unknown environment "nope" (configured: a, b)`,
		"Switched environment: b → a",
		"Environment:    a (available: a, b)",
	)
	if !glB.called("GET /user") || !glA.called("GET /user") {
		t.Errorf("expected auth against both servers; A=%v B=%v", glA.calls, glB.calls)
	}
	// The session after the switch must point at A.
	if i := strings.Index(out, "Switched environment"); !strings.Contains(out[i:], glA.URL) {
		t.Errorf("after switch, output does not show %s", glA.URL)
	}
}

func TestUnknownEnvFlagFailsFast(t *testing.T) {
	gl := newFakeGitLab(t)
	cmd := agentCmd(workdir(t, gl.URL), goodToken)
	cmd.Args = append(cmd.Args, "--env", "prod")
	cmd.Stdin = strings.NewReader("exit\n")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected non-zero exit for unknown --env")
	}
	mustContain(t, string(out), `unknown environment "prod"`)
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
	mustContain(t, out, "Pipelines — team/backend", "99", "✗ failed", "Total: 1 pipelines")
	mustNotContain(t, out, "❌")
}

func TestConfigShowsAIKeyFromEnv(t *testing.T) {
	gl := newFakeGitLab(t)
	dir := workdir(t, gl.URL)
	cfg, _ := os.ReadFile(filepath.Join(dir, "config.yaml"))
	os.WriteFile(filepath.Join(dir, "config.yaml"), append(cfg, "ai:\n  provider: nvidia\n"...), 0o600)

	cmd := agentCmd(dir, goodToken)
	cmd.Env = append(cmd.Env, "NVIDIA_API_KEY=nv-secret")
	cmd.Stdin = strings.NewReader("config\nexit\n")
	out, _ := cmd.CombinedOutput()
	mustContain(t, string(out), "NVIDIA NIM", "key from env NVIDIA_API_KEY")
	mustNotContain(t, string(out), "unknown provider", "nv-secret")
}

func TestDoctor(t *testing.T) {
	gl := newFakeGitLab(t)
	dir := workdir(t, gl.URL)
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg, _ := os.ReadFile(cfgPath)
	os.WriteFile(cfgPath, append(cfg, "ai:\n  anthropic:\n    api_key: sk-ant-supersecret-9876\n"...), 0o644)
	os.Chmod(cfgPath, 0o644) // WriteFile keeps the existing 0600 mode

	out, _ := run(t, dir, goodToken, "doctor")
	mustContain(t, out,
		"contains secrets but has permissions 644",
		"Config file: "+cfgPath,
		"GitLab token: ****",
		"(from env GITLAB_TOKEN)",
		"GitLab: reachable, authenticated as @tester",
		"key ****9876 (from config)",
	)
	mustNotContain(t, out, "supersecret", goodToken)
}

func TestHelpConfigAndRenamedCommands(t *testing.T) {
	gl := newFakeGitLab(t)
	out, _ := run(t, workdir(t, gl.URL), goodToken, "help", "config", "raise-mr")
	mustContain(t, out,
		"git-agent — Command Reference",
		"pipeline-triage <project> [id]",
		"stats [today|week|month [YYYY-MM]|year [YYYY]|dashboard|export csv|json]",
		"switch-team",
		"Current Configuration",
		gl.URL,
		"Active Team:    team",
		"use 'mr-open' instead",
	)
}

// ─── One-shot mode ───────────────────────────────────────────────────────────

func oneShot(t *testing.T, dir string, args ...string) (string, int) {
	t.Helper()
	cmd := agentCmd(dir, goodToken)
	cmd.Args = append(cmd.Args, args...)
	cmd.Stdin = strings.NewReader("")
	out, _ := cmd.CombinedOutput()
	failOnRace(t, string(out))
	return string(out), cmd.ProcessState.ExitCode()
}

func TestOneShotCommand(t *testing.T) {
	gl := newFakeGitLab(t)
	dir := workdir(t, gl.URL)

	out, code := oneShot(t, dir, "mr-status", "team/backend")
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	mustContain(t, out, "Open MRs — team/backend", "Add login")
	mustNotContain(t, out, "Interactive Mode", "Session Summary")

	out, code = oneShot(t, dir, "mr-approve", "team/backend", "8") // 404
	if code != 1 {
		t.Errorf("failed command exit code = %d, want 1", code)
	}
	mustContain(t, out, "Failed to approve MR")

	// One-shot runs land in the same audit trail.
	out, _ = oneShot(t, dir, "stats", "today")
	mustContain(t, out, "Commands run:     2  (1 failed, 50% success)")
}

func TestOneShotFlagsBeforeCommand(t *testing.T) {
	gl := newFakeGitLab(t)
	cfgPath := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(cfgPath, fmt.Appendf(nil, "gitlab:\n  base_url: %q\n", gl.URL), 0o600)
	out, code := oneShot(t, t.TempDir(), "--config", cfgPath, "pipeline", "team/backend")
	if code != 0 {
		t.Errorf("exit code = %d\n%s", code, out)
	}
	mustContain(t, out, "Pipelines — team/backend")
}

// ─── Typos and confirmations ─────────────────────────────────────────────────

func TestTypoSuggestsCommand(t *testing.T) {
	gl := newFakeGitLab(t)
	// "1" picks "Run: mr-status team/backend" from the did-you-mean menu.
	out, _ := run(t, workdir(t, gl.URL), goodToken, "mr-staus team/backend", "1")
	mustContain(t, out,
		"Unknown command 'mr-staus'. Did you mean 'mr-status'?",
		"Open MRs — team/backend",
	)
}

func TestDestructiveCommandsAskFirst(t *testing.T) {
	gl := newFakeGitLab(t)
	out, _ := run(t, workdir(t, gl.URL), goodToken,
		"mr-merge team/backend 7", "2", // decline merge
		"pipeline-cancel team/backend 99", "1", // confirm cancel
	)
	mustContain(t, out, "Merge MR #7 in team/backend?", "Cancelled.", "Cancel pipeline #99 in team/backend?")
	if gl.called("PUT /projects/10/merge_requests/7/merge") || gl.called("PUT /projects/team%2Fbackend/merge_requests/7/merge") {
		t.Error("merge was sent although the user declined")
	}
	if !gl.called("POST /projects/team%2Fbackend/pipelines/99/cancel") && !gl.called("POST /projects/10/pipelines/99/cancel") {
		t.Errorf("cancel not sent after confirming; calls: %v", gl.calls)
	}
	mustContain(t, out, "Commands run:    2 (0 failed)") // declining is not a failure
}

// ─── AI commands ─────────────────────────────────────────────────────────────

func TestFreeFormQuestionGoesToChat(t *testing.T) {
	gl := newFakeGitLab(t)
	ai := newFakeAnthropic(t, func(system, user string) string {
		if isIntentCall(system) {
			return `{"command":"chat","args":{},"confidence":1.0}`
		}
		return "A rebase replays your commits on top of another branch."
	})
	out, _ := run(t, aiWorkdir(t, gl.URL, ai.URL), goodToken, "what is a rebase")
	mustContain(t, out, "A rebase replays your commits",
		"ai.anthropic.base_url in ", "sends your anthropic API key to 127.0.0.1")
}

func TestNaturalLanguageIsRoutedToCommand(t *testing.T) {
	gl := newFakeGitLab(t)
	ai := newFakeAnthropic(t, func(system, user string) string {
		return `{"command":"mr-status","args":{"project":"team/backend"},"confidence":0.95}`
	})
	out, _ := run(t, aiWorkdir(t, gl.URL, ai.URL), goodToken, "show me open merge requests in backend")
	mustContain(t, out, "→ mr-status team/backend", "Open MRs — team/backend", "Add login")
}

func TestMRReviewSavesFileAndRecordsTokens(t *testing.T) {
	gl := newFakeGitLab(t)
	ai := newFakeAnthropic(t, func(system, user string) string {
		if !strings.Contains(user, "func Login()") {
			return "diff missing from prompt"
		}
		return "## Summary\n\nAdds a Login stub."
	})
	dir := aiWorkdir(t, gl.URL, ai.URL)
	out, _ := run(t, dir, goodToken, "mr-review team/backend 7", "2", "stats today") // 2 = don't post
	mustContain(t, out,
		"Review saved to:",
		"Commands run:     1  (0 failed, 100% success)",
		"MRs reviewed:     1",
		"Tokens in",
	)
	if !regexp.MustCompile(`test-model\s+1\s+.*\s100\s+20\b`).MatchString(out) {
		t.Errorf("model performance row with 100/20 tokens not found:\n%s", out)
	}
	mustNotContain(t, out, "diff missing from prompt")
	reviews, _ := filepath.Glob(filepath.Join(dir, ".data", "*7.md"))
	if len(reviews) != 1 {
		t.Fatalf("review files: %v", reviews)
	}
	if b, _ := os.ReadFile(reviews[0]); !strings.Contains(string(b), "Adds a Login stub.") {
		t.Errorf("review file content:\n%s", b)
	}
}

func TestAIErrorIsReported(t *testing.T) {
	gl := newFakeGitLab(t)
	ai := newFakeAnthropic(t, func(string, string) string { return "" })
	dir := aiWorkdir(t, gl.URL, ai.URL)
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg, _ := os.ReadFile(cfgPath)
	os.WriteFile(cfgPath, []byte(strings.Replace(string(cfg), "sk-test", "sk-wrong", 1)), 0o600)

	out, _ := run(t, dir, goodToken, "mr-review team/backend 7", "stats today")
	mustContain(t, out, "bad key", "(1 failed")
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

func TestStatsBreakdownAndExport(t *testing.T) {
	gl := newFakeGitLab(t)
	dir := workdir(t, gl.URL)
	out, _ := run(t, dir, goodToken,
		"mr-approve team/backend 7", "mr-approve team/backend 8", "mr-status team/backend",
		"stats today", "stats export csv today", "stats export json", "stats export xml")
	mustContain(t, out,
		"Today",
		"Commands run:     3  (1 failed, 66% success)",
		"Top Commands:", "mr-approve                2  (1 failed)",
		"Top Projects:", "team/backend",
		"Exported 3 events",
		"Usage: stats export csv|json",
	)
	// Every line of the stats box ("Activity — Today" has a multi-byte dash)
	// must be the same width, or the right border is ragged.
	box := out[strings.LastIndex(out[:strings.Index(out, "Activity — Today")], "\n"):strings.Index(out, "Session Summary")]
	var widths []int
	for _, l := range strings.Split(box, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "│") {
			widths = append(widths, utf8.RuneCountInString(l))
		}
	}
	for _, w := range widths {
		if w != widths[0] {
			t.Errorf("stats box lines have different widths: %v", widths)
			break
		}
	}
	today := time.Now().Format("2006-01-02")
	csvData, err := os.ReadFile(filepath.Join(dir, ".data", "stats_"+today+"_"+today+".csv"))
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(string(csvData), "\n"); lines != 4 || !strings.Contains(string(csvData), "mr-approve,gitlab_mutation,team/backend") {
		t.Errorf("csv export (%d lines):\n%s", lines, csvData)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".data", "stats_*.json"))
	if len(matches) != 1 {
		t.Fatalf("json export files: %v", matches)
	}
	var events []map[string]any
	data, _ := os.ReadFile(matches[0])
	if err := json.Unmarshal(data, &events); err != nil || len(events) != 3 {
		t.Errorf("json export: %v, %d events", err, len(events))
	}
}

func TestStatsRejectsBadMonth(t *testing.T) {
	gl := newFakeGitLab(t)
	out, _ := run(t, workdir(t, gl.URL), goodToken, "stats month 2026-13", "stats fortnight")
	mustContain(t, out, "invalid date format: 2026-13", `unknown range "fortnight"`)
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
