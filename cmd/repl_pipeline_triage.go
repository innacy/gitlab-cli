package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"

	"gitlab-ai/pkg/ai"
	"gitlab-ai/pkg/output"
)

func (r *replState) handlePipelineTriage(args []string) {
	if !r.ensureSession() {
		return
	}

	project, remaining := parseProjectFlag(args)

	var pipelineIDStr string
	if project == "" {
		for _, arg := range remaining {
			if _, err := strconv.Atoi(arg); err == nil && pipelineIDStr == "" {
				pipelineIDStr = arg
			} else if project == "" {
				project = arg
			}
		}
	} else if len(remaining) > 0 {
		pipelineIDStr = remaining[0]
	}

	if project == "" {
		project = r.promptForProject("Select project for pipeline triage")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
	}
	project = r.resolveProject(project)

	var pipelineID int
	if pipelineIDStr != "" {
		n, err := strconv.Atoi(pipelineIDStr)
		if err != nil {
			output.PrintError(fmt.Sprintf("Invalid pipeline ID: %s", pipelineIDStr))
			return
		}
		pipelineID = n
	} else {
		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " Finding latest failed pipeline..."
		s.Start()

		pipelines, err := r.provider.CI().ListPipelines(project, 10)
		s.Stop()

		if err != nil {
			output.PrintError(fmt.Sprintf("Failed to fetch pipelines: %v", err))
			return
		}

		var failedPipelines []int
		var failedOptions []string
		for _, pl := range pipelines {
			if pl.Status == "failed" {
				failedPipelines = append(failedPipelines, pl.ID)
				failedOptions = append(failedOptions, fmt.Sprintf("#%d — %s (%s)", pl.ID, pl.Ref, pl.Status))
			}
		}

		if len(failedPipelines) == 0 {
			output.PrintSuccess("No failed pipelines found — all clear!")
			return
		}

		if len(failedPipelines) == 1 {
			pipelineID = failedPipelines[0]
		} else {
			choice := r.promptForChoice("Select failed pipeline to triage", failedOptions)
			if choice < 0 {
				return
			}
			pipelineID = failedPipelines[choice]
		}
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching pipeline #%d jobs...", pipelineID)
	s.Start()

	pipeline, err := r.provider.CI().GetPipeline(project, pipelineID)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to fetch pipeline: %v", err))
		return
	}

	output.PrintPipelineStatus(pipeline)

	var failedJobs []struct {
		id   int
		name string
	}
	for _, job := range pipeline.Jobs {
		if job.Status == "failed" {
			failedJobs = append(failedJobs, struct {
				id   int
				name string
			}{job.ID, job.Name})
		}
	}

	if len(failedJobs) == 0 {
		output.PrintWarning("No failed jobs in this pipeline.")
		return
	}

	if err := r.ensureAI(); err != nil {
		output.PrintError(fmt.Sprintf("AI required for triage: %v", err))
		return
	}

	var triageResults []string
	for _, job := range failedJobs {
		s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = fmt.Sprintf(" Analyzing failed job '%s' (#%d)...", job.name, job.id)
		s.Start()

		log, logErr := r.provider.CI().GetJobLog(project, job.id)
		if logErr != nil {
			s.Stop()
			output.PrintWarning(fmt.Sprintf("Could not fetch log for job '%s': %v", job.name, logErr))
			continue
		}

		prompt := ai.BuildPipelineFailurePrompt(job.name, log)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	chatResult, aiErr := r.aiClient.Chat(ctx, ai.BuildSystemPrompt(), prompt)
		cancel()
		s.Stop()

		if aiErr != nil {
			output.PrintWarning(fmt.Sprintf("AI analysis failed for '%s': %v", job.name, aiErr))
			continue
		}

		analysis := chatResult.Text

		triageResults = append(triageResults, fmt.Sprintf("### Job: %s (ID: %d)\n\n%s", job.name, job.id, analysis))
	}

	if len(triageResults) == 0 {
		output.PrintWarning("Could not analyze any failed jobs.")
		return
	}

	fmt.Println()
	output.PrintSuccess(fmt.Sprintf("Pipeline Triage Report — #%d (%d failed jobs analyzed)", pipelineID, len(triageResults)))
	fmt.Println()
	fmt.Println(strings.Join(triageResults, "\n\n---\n\n"))
	fmt.Println()

	if r.promptForYesNo("Retry the failed pipeline?") {
		rs := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		rs.Suffix = fmt.Sprintf(" Retrying pipeline #%d...", pipelineID)
		rs.Start()
		pl, retryErr := r.provider.CI().RetryPipeline(project, pipelineID)
		rs.Stop()
		if retryErr != nil {
			output.PrintError(fmt.Sprintf("Retry failed: %v", retryErr))
			return
		}
		output.PrintSuccess(fmt.Sprintf("Pipeline retried — new status: %s", pl.Status))
		if pl.WebURL != "" {
			output.PrintURLOpen(pl.WebURL)
		}
	}
}
