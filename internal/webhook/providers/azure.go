// FILE: internal/webhook/providers/azure.go
package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ServerPlace/iac-controller/internal/webhook"
)

type AzureProcessor struct{}

func NewAzureProcessor() *AzureProcessor {
	return &AzureProcessor{}
}

func (p *AzureProcessor) Parse(r *http.Request) (*webhook.ProcessorResult, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	var payload struct {
		EventType string `json:"eventType"`
		Resource  struct {
			Comment struct {
				Content string `json:"content"`
			} `json:"comment"`
			PullRequest struct {
				ID         int `json:"pullRequestId"`
				Repository struct {
					Name string `json:"name"`
				} `json:"repository"`
				CreatedBy struct {
					UniqueName string `json:"uniqueName"`
				} `json:"createdBy"`
				LastMergeSourceCommit struct {
					CommitID string `json:"commitId"`
				} `json:"lastMergeSourceCommit"`
			} `json:"pullRequest"`
		} `json:"resource"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("json parse error: %w", err)
	}

	result := &webhook.ProcessorResult{
		Event: &webhook.NormalizedEvent{
			Provider:  "azure",
			Repo:      payload.Resource.PullRequest.Repository.Name,
			PRNumber:  payload.Resource.PullRequest.ID,
			Sender:    payload.Resource.PullRequest.CreatedBy.UniqueName,
			CommitSHA: payload.Resource.PullRequest.LastMergeSourceCommit.CommitID,
			Action:    payload.EventType,
		},
	}

	switch payload.EventType {
	case "ms.vss-code.git-pullrequest-comment-event":
		result.Event.Type = webhook.EventTypeComment
		result.Event.Body = payload.Resource.Comment.Content
		result.ShouldQueue = strings.HasPrefix(strings.TrimSpace(result.Event.Body), "/apply")
		result.Message = "comment processed"

	case "git.pullrequest.updated":
		result.Event.Type = webhook.EventTypePRUpdate
		result.ShouldQueue = true
		result.Message = "pr update queued"

	default:
		result.Event.Type = webhook.EventTypeUnknown
		result.ShouldQueue = false
		result.Message = "ignored"
	}

	return result, nil
}
