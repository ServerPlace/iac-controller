package plans

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ServerPlace/iac-controller/internal/core/model"
)

// extractPlanSummary extrai o summary do terraform plan output
// Procura por padrões como "Plan: X to add, Y to change, Z to destroy"
func extractPlanSummary(output string) string {
	// Regex para encontrar linha de summary do Terraform
	// Exemplos:
	// "Plan: 2 to add, 1 to change, 0 to destroy."
	// "Plan: 5 to add, 0 to change, 3 to destroy"
	// "No changes. Your infrastructure matches the configuration."
	summaryRegex := regexp.MustCompile(`(?m)^Plan: \d+ to add, \d+ to change, \d+ to destroy\.?$`)
	noChangesRegex := regexp.MustCompile(`(?m)^No changes\. .*$`)

	// Tenta encontrar linha de summary
	if match := summaryRegex.FindString(output); match != "" {
		return match
	}

	// Tenta encontrar "No changes"
	if match := noChangesRegex.FindString(output); match != "" {
		return match
	}

	// Fallback: procura manualmente linha por linha
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Plan:") {
			return trimmed
		}
		if strings.HasPrefix(trimmed, "No changes") {
			return trimmed
		}
	}

	return "Plan summary not available"
}

// parsePlanCounts extrai os números do plan summary
// Retorna: add, change, destroy
func parsePlanCounts(summary string) (int, int, int) {
	// Parse "Plan: 2 to add, 1 to change, 0 to destroy"
	var add, change, destroy int

	fmt.Sscanf(summary, "Plan: %d to add, %d to change, %d to destroy", &add, &change, &destroy)

	return add, change, destroy
}

// FormatPlanComment formata o output do terraform plan em markdown para comentário no PR
// Mostra apenas summary para evitar problemas com caracteres especiais
func FormatPlanComment(deployment model.Deployment) string {
	var sb strings.Builder

	// Header com badge de status
	badge := "✅ **Plan Succeeded**"
	if !deployment.PlanSucceeded {
		badge = "❌ **Plan Failed**"
	}

	sb.WriteString(fmt.Sprintf("## %s\n\n", badge))
	sb.WriteString(fmt.Sprintf("**Deployment ID:** `%s`\n", deployment.ID))
	sb.WriteString(fmt.Sprintf("**Plan Version:** `#%d`\n", deployment.PlanVersion))
	sha := deployment.HeadSHA
	if len(sha) > 7 {
		sha = sha[:7]
	}
	sb.WriteString(fmt.Sprintf("**Commit SHA:** `%s`\n", sha))

	if deployment.User != "" {
		sb.WriteString(fmt.Sprintf("**Triggered by:** @%s\n", deployment.User))
	}

	sb.WriteString(fmt.Sprintf("**Planned at:** %s\n\n", deployment.PlanAt.Format("2006-01-02 15:04:05 UTC")))

	// Stacks afetados
	if len(deployment.Stacks) > 0 {
		sb.WriteString("### 📦 Affected Stacks\n\n")
		for _, stack := range deployment.Stacks {
			sb.WriteString(fmt.Sprintf("- `%s`\n", stack))
		}
		sb.WriteString("\n")
	}

	// Plan Summary com visual badges
	summary := extractPlanSummary(deployment.PlanOutput)
	add, change, destroy := parsePlanCounts(summary)

	sb.WriteString("### 📊 Plan Summary\n\n")

	if summary == "Plan summary not available" {
		sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", deployment.PlanOutput))
	} else if strings.HasPrefix(summary, "No changes") {
		sb.WriteString("✨ **No changes.** Your infrastructure matches the configuration.\n\n")
	} else {
		// Mostrar com badges visuais
		sb.WriteString("| Action | Count |\n")
		sb.WriteString("|--------|-------|\n")
		sb.WriteString(fmt.Sprintf("| ➕ **Add** | %d |\n", add))
		sb.WriteString(fmt.Sprintf("| 🔄 **Change** | %d |\n", change))
		sb.WriteString(fmt.Sprintf("| ❌ **Destroy** | %d |\n", destroy))
		sb.WriteString("\n")
	}

	// Footer
	sb.WriteString("---\n")
	sb.WriteString("*Powered by iac-controller* 🚀\n")

	return sb.String()
}
