package azure

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ServerPlace/iac-controller/internal/scm"
)

func (c *AzureClient) ParseWebhook(r *http.Request) (*scm.WebhookEvent, error) {
	// 1. Azure Security: BASIC AUTH
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, fmt.Errorf("missing auth")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Basic" {
		return nil, fmt.Errorf("invalid header")
	}

	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("base64 error")
	}

	creds := strings.SplitN(string(decoded), ":", 2)
	// Compara a senha do payload com o webhook secret configurado
	if len(creds) != 2 || creds[1] != c.WebhookPassword {
		return nil, fmt.Errorf("invalid credentials")
	}

	// 2. Parse Payload
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	var ev struct {
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
			} `json:"pullRequest"`
		} `json:"resource"`
	}

	if err := json.Unmarshal(body, &ev); err != nil {
		return nil, fmt.Errorf("json error: %w", err)
	}

	if ev.EventType == "ms.vss-code.git-pull-request-comment-event" {
		return &scm.WebhookEvent{
			Type:     scm.EventComment,
			Repo:     ev.Resource.PullRequest.Repository.Name,
			PRNumber: ev.Resource.PullRequest.ID,
			Sender:   ev.Resource.PullRequest.CreatedBy.UniqueName,
			Body:     ev.Resource.Comment.Content,
		}, nil
	}
	return nil, nil
}
