package azure

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ServerPlace/iac-controller/internal/core/model"
)

type Adapter struct {
	OrgURL     string
	Project    string
	PAT        string
	PipelineID string
	Client     *http.Client
}

func New(org, proj, pat, pipeID string) *Adapter {
	return &Adapter{
		OrgURL:     org,
		Project:    proj,
		PAT:        pat,
		PipelineID: pipeID,
		Client:     &http.Client{},
	}
}

func (a *Adapter) TriggerApply(ctx context.Context, req model.PipelineTriggerRequest) (string, error) {
	// Mapeia as variáveis de segurança (Job ID, API Key/SignatureRequest Key)
	// O formato da API do Azure exige: "NomeDaVar": { "value": "ValorDaVar" }
	adoVars := make(map[string]interface{})
	for k, v := range req.Variables {
		adoVars[k] = map[string]string{"value": v}
	}

	// Payload Limpo (Sem templateParameters de stack)
	// O CLI descobrirá as stacks afetadas via 'git diff' dentro do runner.
	payload := map[string]interface{}{
		"resources": map[string]interface{}{
			"repositories": map[string]interface{}{
				"self": map[string]string{
					"refName": req.Branch,
					"version": req.CommitSHA,
				},
			},
		},
		"variables": adoVars,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/%s/_apis/pipelines/%s/runs?api-version=7.0", a.OrgURL, a.Project, a.PipelineID)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}

	auth := base64.StdEncoding.EncodeToString([]byte(":" + a.PAT))
	httpReq.Header.Add("Authorization", "Basic "+auth)
	httpReq.Header.Add("Content-Type", "application/json")

	resp, err := a.Client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("azure api error (%d): %s", resp.StatusCode, string(b))
	}

	// Tenta extrair a URL de acompanhamento da execução
	var result struct {
		Links struct {
			Web struct {
				Href string `json:"href"`
			} `json:"web"`
		} `json:"_links"`
	}

	// Se o decode falhar, não é crítico, retornamos sucesso sem URL
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "pipeline-started (url unavailable)", nil
	}

	return result.Links.Web.Href, nil
}
