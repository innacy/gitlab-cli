package cmd

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/config"
	"gitlab-ai/pkg/platform"
)

type mockRepoService struct {
	listProjectsCalls       int
	listRecentProjectsCalls int
	lastRecentLimit         int
	projects                []models.ProjectInfo
}

func (m *mockRepoService) ListProjects() ([]models.ProjectInfo, error) {
	m.listProjectsCalls++
	return m.projects, nil
}

func (m *mockRepoService) ListRecentProjects(limit int) ([]models.ProjectInfo, error) {
	m.listRecentProjectsCalls++
	m.lastRecentLimit = limit
	if limit > 0 && limit < len(m.projects) {
		return m.projects[:limit], nil
	}
	return m.projects, nil
}

func (m *mockRepoService) ListProjectsSince(_ time.Time, _ int) ([]models.ProjectInfo, error) {
	return nil, nil
}

func (m *mockRepoService) ListBranches(_ string, _ int) ([]models.BranchInfo, error) {
	return nil, nil
}
func (m *mockRepoService) ListActiveBranches(_ string, _ int) ([]models.BranchInfo, error) {
	return nil, nil
}
func (m *mockRepoService) ListMergedBranches(_ string) ([]models.BranchInfo, error) {
	return nil, nil
}
func (m *mockRepoService) DeleteBranch(_, _ string) error { return nil }
func (m *mockRepoService) GetBranch(_, _ string) (*models.BranchInfo, error) {
	return nil, nil
}
func (m *mockRepoService) ListTags(_ string, _ int) ([]models.TagInfo, error) {
	return nil, nil
}
func (m *mockRepoService) TagExists(_, _ string) (bool, error)    { return false, nil }
func (m *mockRepoService) BranchExists(_, _ string) (bool, error) { return false, nil }
func (m *mockRepoService) GetRefDiff(_, _, _ string) (*models.DiffResult, error) {
	return nil, nil
}
func (m *mockRepoService) GetMRDiff(_ string, _ int) (*models.DiffResult, error) {
	return nil, nil
}
func (m *mockRepoService) GetMRDiffByProjectID(_, _ int) (*models.DiffResult, error) {
	return nil, nil
}
func (m *mockRepoService) GetFileContent(_, _, _ string) (string, error) { return "", nil }
func (m *mockRepoService) CheckProjectRelease(_ string) models.ProjectReleaseInfo {
	return models.ProjectReleaseInfo{}
}

type mockProvider struct {
	repos *mockRepoService
}

func (m *mockProvider) Name() string               { return "mock" }
func (m *mockProvider) Username() string            { return "test" }
func (m *mockProvider) UserDisplayName() string     { return "Test User" }
func (m *mockProvider) UserID() int                 { return 1 }
func (m *mockProvider) Repos() platform.RepoService   { return m.repos }
func (m *mockProvider) MRs() platform.MRService       { return nil }
func (m *mockProvider) Issues() platform.IssueService  { return nil }
func (m *mockProvider) CI() platform.CIService         { return nil }
func (m *mockProvider) Epics() platform.EpicService    { return nil }

func makeTestProjects(n int) []models.ProjectInfo {
	projects := make([]models.ProjectInfo, n)
	for i := range projects {
		projects[i] = models.ProjectInfo{
			ID:           i + 1,
			Name:         fmt.Sprintf("project-%d", i+1),
			Path:         fmt.Sprintf("team/project-%d", i+1),
			LastActivity: time.Now().Add(-time.Duration(i) * time.Hour),
		}
	}
	return projects
}

func newTestReplState(repos *mockRepoService) *replState {
	return &replState{
		cfg:      &config.AppConfig{},
		provider: &mockProvider{repos: repos},
	}
}

func TestFetchProjectCache_UsesLimitedFetch(t *testing.T) {
	repos := &mockRepoService{projects: makeTestProjects(50)}
	r := newTestReplState(repos)
	r.cacheReady = make(chan struct{})

	r.fetchProjectCache()

	if repos.listRecentProjectsCalls != 1 {
		t.Errorf("expected ListRecentProjects to be called once, got %d", repos.listRecentProjectsCalls)
	}
	if repos.lastRecentLimit != startupProjectLimit {
		t.Errorf("expected limit=%d, got %d", startupProjectLimit, repos.lastRecentLimit)
	}
	if repos.listProjectsCalls != 0 {
		t.Errorf("expected ListProjects NOT to be called on startup, got %d calls", repos.listProjectsCalls)
	}

	r.cacheMu.RLock()
	count := len(r.projectCache)
	r.cacheMu.RUnlock()

	if count != startupProjectLimit {
		t.Errorf("expected cache to contain %d projects, got %d", startupProjectLimit, count)
	}
}

func TestEnsureFullCache_FetchesAllAfterLimitedStartup(t *testing.T) {
	repos := &mockRepoService{projects: makeTestProjects(50)}
	r := newTestReplState(repos)
	r.cacheReady = make(chan struct{})

	r.fetchProjectCache()

	r.cacheMu.RLock()
	countBefore := len(r.projectCache)
	r.cacheMu.RUnlock()
	if countBefore != startupProjectLimit {
		t.Fatalf("precondition failed: cache should have %d, got %d", startupProjectLimit, countBefore)
	}

	r.lastRefreshTime = time.Time{}
	projects := r.ensureFullCache()

	if repos.listProjectsCalls != 1 {
		t.Errorf("expected ListProjects to be called once for full cache, got %d", repos.listProjectsCalls)
	}
	if len(projects) != 50 {
		t.Errorf("expected 50 projects from ensureFullCache, got %d", len(projects))
	}
}

func TestRefreshCacheWithLimit_ZeroFetchesAll(t *testing.T) {
	repos := &mockRepoService{projects: makeTestProjects(50)}
	r := newTestReplState(repos)

	r.refreshCacheWithLimit(0)

	if repos.listProjectsCalls != 1 {
		t.Errorf("expected ListProjects called once with limit=0, got %d", repos.listProjectsCalls)
	}
	if repos.listRecentProjectsCalls != 0 {
		t.Errorf("expected ListRecentProjects NOT called with limit=0, got %d", repos.listRecentProjectsCalls)
	}

	r.cacheMu.RLock()
	count := len(r.projectCache)
	r.cacheMu.RUnlock()
	if count != 50 {
		t.Errorf("expected 50 projects in cache, got %d", count)
	}
}

func TestWaitForCache_ReturnsLimitedProjectsQuickly(t *testing.T) {
	repos := &mockRepoService{projects: makeTestProjects(50)}
	r := newTestReplState(repos)
	r.cacheReady = make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.fetchProjectCache()
	}()

	projects := r.waitForCache()
	wg.Wait()

	if len(projects) != startupProjectLimit {
		t.Errorf("expected %d projects from waitForCache after startup, got %d", startupProjectLimit, len(projects))
	}
}
