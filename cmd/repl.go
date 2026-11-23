package cmd

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chzyer/readline"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/ai"
	"gitlab-ai/pkg/audit"
	"gitlab-ai/pkg/config"
	"gitlab-ai/pkg/output"
	"gitlab-ai/pkg/platform"
	_ "gitlab-ai/pkg/platform/gitlab"
)

// replState holds the interactive session state.
type replState struct {
	cfg      *config.AppConfig
	provider platform.Provider
	aiClient ai.ChatClient
	rl       *readline.Instance
	username string

	startedAt     time.Time
	auditor       *audit.Recorder
	lastAIEventID int64
	reviews       map[string]*reviewEntry
	reviewOrder   []string

	// The command currently being dispatched, so handlers can attach AI
	// output and side effects to its audit event.
	curEventID int64
	curCommand string
	curProject string

	activeTeam      string
	projectCache    []models.ProjectInfo
	cacheGen        int // bumped on team/env switch; stale background loads are dropped
	cacheMu         sync.RWMutex
	cacheReady      chan struct{}
	fullCacheMu     sync.Mutex
	lastRefreshTime time.Time

	idleTimer *time.Timer
	timedOut  atomic.Bool
}

type reviewEntry struct {
	project    string
	mrNumber   int
	mrTitle    string
	filePath   string
	comment    string
	reviewedAt time.Time
}

// ─── Auto-complete ───────────────────────────────────────────────────────────

func (r *replState) projectSuggestions() []string {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	seen := make(map[string]bool, len(r.projectCache)*2)
	suggestions := make([]string, 0, len(r.projectCache)*2)

	for _, p := range r.projectCache {
		if !seen[p.Path] {
			suggestions = append(suggestions, p.Path)
			seen[p.Path] = true
		}
	}
	for _, p := range r.projectCache {
		if !seen[p.Name] {
			suggestions = append(suggestions, p.Name)
			seen[p.Name] = true
		}
	}
	return suggestions
}

// ─── Project Cache ───────────────────────────────────────────────────────────

// waitCacheReady blocks until the startup fetch finishes, showing a loader
// only if the cache isn't ready yet.
func (r *replState) waitCacheReady(timeout time.Duration) {
	ready := r.readyChan()
	if ready == nil {
		return
	}
	select {
	case <-ready:
		return
	default:
	}
	s := output.NewLoader(" Loading projects...")
	s.Start()
	defer s.Stop()
	select {
	case <-ready:
	case <-time.After(timeout):
	}
}

// readyChan returns the channel closed when the current startup fetch is done.
// startCacheLoad replaces it on team/env switch, so it is read under cacheMu.
func (r *replState) readyChan() chan struct{} {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()
	return r.cacheReady
}

func (r *replState) waitForCache() []models.ProjectInfo {
	r.waitCacheReady(30 * time.Second)
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()
	result := make([]models.ProjectInfo, len(r.projectCache))
	copy(result, r.projectCache)
	return result
}

const startupProjectLimit = 10

// fetchProjectCache loads the first projects and closes ready. The channel is
// passed in rather than read from r: after a switch, r.cacheReady is a new
// channel that belongs to the new load and must not be closed by this one.
func (r *replState) fetchProjectCache(ready chan struct{}) {
	defer close(ready)
	r.refreshCacheWithLimit(startupProjectLimit)
}

func (r *replState) refreshCache() {
	r.refreshCacheWithLimit(0)
}

func (r *replState) refreshCacheWithLimit(limit int) {
	r.cacheMu.RLock()
	prov, gen, since := r.provider, r.cacheGen, r.lastRefreshTime
	r.cacheMu.RUnlock()
	if prov == nil {
		return
	}

	var projects []models.ProjectInfo
	var err error

	if since.IsZero() {
		if limit > 0 {
			projects, err = prov.Repos().ListRecentProjects(limit)
		} else {
			projects, err = prov.Repos().ListProjects()
		}
	} else {
		projects, err = prov.Repos().ListProjectsSince(since, 1)
	}
	if err != nil {
		return
	}

	r.cacheMu.Lock()
	if gen != r.cacheGen {
		// Team or environment switched while we were fetching.
		r.cacheMu.Unlock()
		return
	}
	if r.lastRefreshTime.IsZero() {
		r.projectCache = r.filterByTeam(projects)
	} else {
		r.mergeIntoCache(r.filterByTeam(projects))
	}

	sort.Slice(r.projectCache, func(i, j int) bool {
		return r.projectCache[i].LastActivity.After(r.projectCache[j].LastActivity)
	})
	r.lastRefreshTime = time.Now()
	r.cacheMu.Unlock()
}

func (r *replState) filterByTeam(projects []models.ProjectInfo) []models.ProjectInfo {
	if r.activeTeam == "" {
		return projects
	}
	team := strings.ToLower(strings.TrimSpace(r.activeTeam))
	filtered := make([]models.ProjectInfo, 0, len(projects))
	for _, p := range projects {
		path := strings.ToLower(p.Path)
		if strings.HasPrefix(path, team+"/") || strings.Contains(path, "/"+team+"/") {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// mergeIntoCache upserts updated projects into the existing cache.
// Must be called with cacheMu held.
func (r *replState) mergeIntoCache(updated []models.ProjectInfo) {
	index := make(map[int]int, len(r.projectCache))
	for i, p := range r.projectCache {
		index[p.ID] = i
	}
	for _, p := range updated {
		if idx, ok := index[p.ID]; ok {
			r.projectCache[idx] = p
		} else {
			r.projectCache = append(r.projectCache, p)
		}
	}
}

func (r *replState) refreshCacheAsync() {
	go r.refreshCache()
}

// loadFullCache upgrades the startup cache to the full project list. It prints
// nothing, so it is safe to run in the background; fullCacheMu makes a
// foreground caller wait for an in-flight background load instead of refetching.
func (r *replState) loadFullCache() {
	r.fullCacheMu.Lock()
	defer r.fullCacheMu.Unlock()

	if ready := r.readyChan(); ready != nil {
		select {
		case <-ready:
		case <-time.After(30 * time.Second):
		}
	}

	r.cacheMu.RLock()
	count := len(r.projectCache)
	r.cacheMu.RUnlock()

	if count <= startupProjectLimit {
		r.cacheMu.Lock()
		r.lastRefreshTime = time.Time{}
		r.cacheMu.Unlock()
		r.refreshCacheWithLimit(0)
	}
}

func (r *replState) ensureFullCache() []models.ProjectInfo {
	s := output.NewLoader(" Loading projects...")
	s.Start()
	r.loadFullCache()
	s.Stop()

	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()
	result := make([]models.ProjectInfo, len(r.projectCache))
	copy(result, r.projectCache)
	return result
}

func (r *replState) syncProjectCache() {
	s := output.NewLoader(" Syncing projects...")
	s.Start()
	r.refreshCache()
	s.Stop()

	r.cacheMu.RLock()
	count := len(r.projectCache)
	r.cacheMu.RUnlock()

	output.PrintSuccess(fmt.Sprintf("Synced %d projects", count))
}

func (r *replState) smartRefresh() {
	if r.lastRefreshTime.IsZero() {
		return
	}
	r.refreshCache()
}

func (r *replState) resolveProject(input string) string {
	cache := r.waitForCache()
	if len(cache) == 0 {
		return input
	}

	lower := strings.ToLower(input)

	for _, p := range cache {
		if p.Path == input {
			return p.Path
		}
	}
	for _, p := range cache {
		if strings.ToLower(p.Path) == lower {
			return p.Path
		}
	}
	for _, p := range cache {
		if p.Name == input {
			output.PrintSuccess(fmt.Sprintf("Resolved '%s' → %s", input, p.Path))
			return p.Path
		}
	}
	for _, p := range cache {
		if strings.ToLower(p.Name) == lower {
			output.PrintSuccess(fmt.Sprintf("Resolved '%s' → %s", input, p.Path))
			return p.Path
		}
	}
	for _, p := range cache {
		if strings.HasSuffix(strings.ToLower(p.Path), "/"+lower) {
			output.PrintSuccess(fmt.Sprintf("Resolved '%s' → %s", input, p.Path))
			return p.Path
		}
	}

	return input
}

func (r *replState) resetIdle() {
	if !r.timedOut.Load() && r.idleTimer != nil {
		idleTimeout := time.Duration(r.cfg.CLI.IdleTimeoutMinutes) * time.Minute
		if idleTimeout <= 0 {
			idleTimeout = 1 * time.Hour
		}
		r.idleTimer.Reset(idleTimeout)
	}
}

// ─── REPL Entry Point ───────────────────────────────────────────────────────
// REPL: Read-Eval-Print-Loop
// newSession opens the audit trail and the line editor shared by the
// interactive REPL and one-shot mode. close releases both.
func newSession(cfg *config.AppConfig) (r *replState, close func(), err error) {
	r = &replState{
		cfg:     cfg,
		reviews: make(map[string]*reviewEntry),
	}

	if err := os.MkdirAll(config.Dir(), 0o700); err != nil {
		output.PrintWarning(fmt.Sprintf("Cannot create %s: %v", config.Dir(), err))
	}
	auditDir, notice := config.AuditDir()
	if notice != "" {
		output.PrintSuccess(notice)
	}
	auditor, auditErr := audit.Open(auditDir)
	if auditErr != nil {
		fmt.Fprintf(os.Stderr, "⚠ audit trail disabled: %v\n", auditErr)
	} else {
		r.auditor = auditor
	}

	completer := r.buildCompleter()

	rl, err := readline.NewEx(&readline.Config{
		Prompt:            r.prompt(),
		AutoComplete:      completer,
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistoryFile:       config.HistoryFile(),
		HistoryLimit:      500,
		HistorySearchFold: true,
	})
	if err != nil {
		if r.auditor != nil {
			r.auditor.Close()
		}
		return nil, nil, err
	}
	r.rl = rl
	output.SetBrowserOpen(cfg.CLI.OpenInBrowser)

	return r, func() {
		rl.Close()
		if r.auditor != nil {
			r.auditor.Close()
		}
	}, nil
}

// RunOnce runs a single command non-interactively (git-agent mr-status x)
// and returns the process exit code: 1 if the command reported an error.
func RunOnce(cfg *config.AppConfig, args []string) int {
	r, _, err := newSession(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize: %v\n", err)
		return 1
	}
	// ponytail: readline.Close races with readline's own terminal goroutine
	// when no line was ever read (chzyer/readline v1.5.1), and the process
	// exits right after, so only the audit DB is closed.
	defer func() {
		if r.auditor != nil {
			r.auditor.Close()
		}
	}()

	before := output.ErrorCount()
	r.dispatch(strings.Join(args, " "))
	if output.ErrorCount() > before {
		return 1
	}
	return 0
}

func RunREPL(cfg *config.AppConfig) {
	r, closeSession, err := newSession(cfg)
	if err != nil {
		fmt.Printf("Failed to initialize: %v\n", err)
		return
	}
	defer closeSession()
	rl := r.rl

	idleTimeout := time.Duration(cfg.CLI.IdleTimeoutMinutes) * time.Minute
	if idleTimeout <= 0 {
		idleTimeout = 1 * time.Hour
	}
	printBanner(idleTimeout)
	if cfg.CLI.AutoConfirm {
		output.PrintWarning("cli.auto_confirm is deprecated and ignored; confirmations always prompt. Remove it from config.yaml.")
	}

	r.idleTimer = time.AfterFunc(idleTimeout, func() {
		r.timedOut.Store(true)
		rl.Close()
	})
	defer r.idleTimer.Stop()

	// run start command automatically
	r.handleStart()

	for {
		line, err := rl.Readline()

		if r.timedOut.Load() {
			fmt.Printf("\n⏰ Session timed out (%s of inactivity)\n", idleTimeout)
			r.handleExit()
			return
		}

		if err != nil {
			if err == readline.ErrInterrupt {
				r.resetIdle()
				continue
			}
			r.handleExit()
			return
		}

		r.resetIdle()

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if r.dispatch(line) {
			r.idleTimer.Stop()
			return
		}
	}
}

// ─── Command Dispatch ────────────────────────────────────────────────────────

func printBanner(idleTimeout time.Duration) {
	theme := output.GetTheme()
	fmt.Println()
	theme.Header.Println("  git-agent — Interactive Mode")
	theme.Muted.Println("  Type 'help' for commands, or ask anything in plain English.")
	theme.Muted.Printf("  Tab completes · ↑/↓ history · Ctrl+R search · auto-exit after %s idle\n", idleTimeout)
	fmt.Println()
}

func (r *replState) dispatch(line string) bool {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return false
	}

	name := strings.ToLower(parts[0])
	if name == "exit" {
		r.handleExit()
		return true
	}

	start := time.Now()
	output.TakeWorkTime()

	c := lookupCommand(name)
	var eventID int64
	if c != nil {
		project := auditProject(parts[1:])
		if r.auditor != nil && c.category != "" {
			eventID, _ = r.auditor.Record(audit.Event{
				Command: c.name, Category: c.category, Project: project, Success: true,
			})
		}
		// Nested dispatch (intent routing) must not clobber the outer command.
		prevID, prevCmd, prevProject := r.curEventID, r.curCommand, r.curProject
		r.curEventID, r.curCommand, r.curProject = eventID, c.name, project
		defer func() { r.curEventID, r.curCommand, r.curProject = prevID, prevCmd, prevProject }()
	}
	errorsBefore := output.ErrorCount()

	switch {
	case c != nil:
		c.run(r, parts[1:])
	case renamedCommands[name] != "":
		output.PrintWarning(renamedCommands[name])
	default:
		if s := suggestCommand(name); s != "" {
			fixed := strings.Join(append([]string{s}, parts[1:]...), " ")
			switch r.promptForChoice(fmt.Sprintf("Unknown command '%s'. Did you mean '%s'?", name, s),
				[]string{"Run: " + fixed, "Ask AI instead"}) {
			case 0:
				return r.dispatch(fixed)
			case 1:
				r.handleIntentOrChat(line)
			}
			break
		}
		r.handleIntentOrChat(line)
	}

	// ponytail: counts loader time only, so prompts the user answers don't inflate it.
	if d := output.TakeWorkTime(); d >= time.Second {
		output.PrintWorked(d)
	}

	if eventID > 0 {
		r.auditor.Finish(eventID, time.Since(start).Milliseconds(), output.ErrorCount() == errorsBefore)
	}

	return false
}

// auditProject picks the project a command acted on from its raw arguments:
// the -p flag, else the first non-numeric argument.
func auditProject(args []string) string {
	if p, rest := parseProjectFlag(args); p != "" {
		return p
	} else if len(rest) > 0 {
		if _, err := strconv.Atoi(rest[0]); err != nil {
			return rest[0]
		}
	}
	return ""
}

// recordEffect logs a side effect (file written, MR or ticket created) under
// the command that is currently running.
func (r *replState) recordEffect(category string) {
	if r.auditor == nil {
		return
	}
	r.auditor.Record(audit.Event{
		Command: r.curCommand, Category: category, Project: r.curProject, Success: true,
	})
}

// prompt shows the active environment and team: "git-agent[work/cnips]> ".
func (r *replState) prompt() string {
	var ctx []string
	for _, s := range []string{r.cfg.Environment, r.activeTeam} {
		if s != "" {
			ctx = append(ctx, s)
		}
	}
	if len(ctx) == 0 {
		return "git-agent> "
	}
	return "git-agent[" + strings.Join(ctx, "/") + "]> "
}

// ─── Session ─────────────────────────────────────────────────────────────────

func (r *replState) ensureSession() bool {
	if r.provider != nil {
		return true
	}

	output.PrintWarning("No active session — auto-starting...")
	return r.connect()
}

// connect authenticates against the configured platform and starts a session.
func (r *replState) connect() bool {
	platformName := r.cfg.Platform
	if platformName == "" {
		platformName = "gitlab"
	}

	s := output.NewLoader(fmt.Sprintf(" Authenticating with %s...", platformName))
	s.Start()

	prov, err := platform.NewProvider(r.cfg)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Authentication failed: %v", err))
		return false
	}

	r.cacheMu.Lock()
	r.provider = prov
	r.cacheMu.Unlock()
	r.username = prov.Username()
	r.startedAt = time.Now()

	fmt.Println()
	output.PrintSuccess(fmt.Sprintf("Authenticated as %s (@%s)", prov.UserDisplayName(), prov.Username()))
	output.PrintSuccess(fmt.Sprintf("Platform: %s", prov.Name()))
	if r.cfg.Environment != "" {
		output.PrintSuccess(fmt.Sprintf("Environment: %s", r.cfg.Environment))
	}
	output.PrintURL(r.cfg.GitLab.BaseURL)

	if len(r.cfg.Teams) == 1 {
		r.activeTeam = r.cfg.Teams[0].Name
		output.PrintSuccess(fmt.Sprintf("Team: %s", r.activeTeam))
	} else if len(r.cfg.Teams) > 1 {
		choice := r.interactiveSelect("Select team", r.cfg.TeamNames())
		if choice >= 0 {
			r.activeTeam = r.cfg.Teams[choice].Name
			output.PrintSuccess(fmt.Sprintf("Team: %s", r.activeTeam))
		}
	}

	r.startCacheLoad()
	if r.rl != nil {
		r.rl.SetPrompt(r.prompt())
	}

	output.PrintSuccess("Session started — ready for commands")
	fmt.Println()

	return true
}

func (r *replState) handleStart() {
	if r.provider != nil {
		output.PrintWarning("Session already active. Use 'exit' to end current session.")
		return
	}
	r.ensureSession()
}

// startCacheLoad fetches projects in the background; commands that need them
// wait with a loader (waitForCache / ensureFullCache).
func (r *replState) startCacheLoad() {
	ready := make(chan struct{})
	r.cacheMu.Lock()
	r.cacheReady = ready
	r.cacheMu.Unlock()
	go func() {
		r.fetchProjectCache(ready)
		r.loadFullCache()
	}()
}

func (r *replState) handleExit() {
	fmt.Println()
	if r.provider != nil && !r.startedAt.IsZero() {
		lines := []string{
			fmt.Sprintf("Duration:        %s", time.Since(r.startedAt).Round(time.Second)),
			fmt.Sprintf("Team:            %s", r.activeTeam),
		}
		// Same source as `stats`, so the two can never disagree.
		if r.auditor != nil {
			if s, err := r.auditor.SummarySince(r.startedAt); err == nil {
				lines = append(lines,
					fmt.Sprintf("Commands run:    %d (%d failed)", s.TotalEvents, s.Failed),
					fmt.Sprintf("MRs reviewed:    %d", s.MRsReviewed),
					fmt.Sprintf("MRs created:     %d", s.MRsCreated),
					fmt.Sprintf("Tickets created: %d", s.TicketsCreated),
					fmt.Sprintf("Files written:   %d", s.FilesWritten),
				)
			}
		}
		output.ThemeHeader("Session Summary")
		output.ThemeBox(lines)
	}
	fmt.Println()
	output.GetTheme().Muted.Println("  Goodbye.")
	fmt.Println()
}

// ─── Shared Utilities ────────────────────────────────────────────────────────

func now() time.Time {
	return time.Now()
}

func (r *replState) auditAI(eventID int64, result ai.ChatResult) {
	if r.auditor == nil || eventID <= 0 {
		return
	}
	r.auditor.RecordAIOutput(audit.AIOutput{
		EventID:      eventID,
		Provider:     r.aiClient.ProviderName(),
		Model:        result.Model,
		CharCount:    len(result.Text),
		DurationMs:   result.DurationMs,
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
	})
}

func (r *replState) auditOutcome(eventID int64, outcome string, editRatio *float64) {
	if r.auditor == nil || eventID <= 0 {
		return
	}
	r.auditor.RecordOutcome(eventID, outcome, editRatio)
}
