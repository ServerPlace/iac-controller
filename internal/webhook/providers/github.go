// FILE: internal/webhook/providers/github.go
package providers

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ServerPlace/iac-controller/internal/webhook"
	"github.com/google/go-github/v57/github"
)

type GitHubProcessor struct{}

func NewGitHubProcessor() *GitHubProcessor {
	return &GitHubProcessor{}
}

func (p *GitHubProcessor) Parse(r *http.Request) (*webhook.ProcessorResult, error) {
	eventType := github.WebHookType(r)

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	event, err := github.ParseWebHook(eventType, payload)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	result := &webhook.ProcessorResult{
		Event: &webhook.NormalizedEvent{
			Provider: "github",
		},
	}

	switch e := event.(type) {
	case *github.IssueCommentEvent:
		if !e.GetIssue().IsPullRequest() {
			result.Event.Type = webhook.EventTypeUnknown
			result.Message = "not a PR comment"
			return result, nil
		}

		result.Event.Type = webhook.EventTypeComment
		result.Event.Repo = e.GetRepo().GetFullName()
		result.Event.PRNumber = e.GetIssue().GetNumber()
		result.Event.Sender = e.GetSender().GetLogin()
		result.Event.Body = e.GetComment().GetBody()
		result.ShouldQueue = strings.HasPrefix(strings.TrimSpace(result.Event.Body), "/apply")
		result.Message = "comment processed"

	case *github.PullRequestEvent:
		result.Event.Type = webhook.EventTypePRUpdate
		result.Event.Repo = e.GetRepo().GetFullName()
		result.Event.PRNumber = e.GetNumber()
		result.Event.Sender = e.GetSender().GetLogin()
		result.Event.CommitSHA = e.GetPullRequest().GetHead().GetSHA()
		result.ShouldQueue = true
		result.Message = "pr event queued"

	case *github.PingEvent:
		result.Event.Type = webhook.EventTypePing
		result.Event.Repo = e.GetRepo().GetFullName()
		result.Message = "pong"

	default:
		result.Event.Type = webhook.EventTypeUnknown
		result.Message = "ignored"
	}

	return result, nil
}
