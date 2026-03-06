package config

import (
	"context"
	"fmt"
	"github.com/ServerPlace/iac-controller/internal/compliance"

	"github.com/ServerPlace/iac-controller/internal/secrets"
	"github.com/go-playground/validator/v10"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

type Config struct {
	Port string `mapstructure:"PORT"`

	// --- GCP Core ---
	GCPProject          string `mapstructure:"GCP_PROJECT" validate:"required"`
	PlanServiceAccount  string `mapstructure:"PLAN_SERVICE_ACCOUNT" validate:"required"`
	ApplyServiceAccount string `mapstructure:"APPLY_SERVICE_ACCOUNT" validate:"required"`

	// --- SCM Configuration ---
	SCMProvider string `mapstructure:"SCM_PROVIDER" validate:"required,oneof=github azure"`

	// --- GITHUB ---
	GithubAppID         string `mapstructure:"GITHUB_APP_ID"`
	GithubInstallID     string `mapstructure:"GITHUB_INSTALL_ID"`
	GithubPrivateKey    string `mapstructure:"GITHUB_PRIVATE_KEY"`
	GithubWebhookSecret string `mapstructure:"GITHUB_WEBHOOK_SECRET"`

	// Sign
	JITSecretKey string `mapstructure:"JIT_SECRET_KEY" validate:"required"`

	// --- Azure DevOps ---
	ADOOrgURL          string `mapstructure:"ADO_ORG_URL" validate:"required,url"`
	ADOProject         string `mapstructure:"ADO_PROJECT" validate:"required"`
	ADOPAT             string `mapstructure:"ADO_PAT" validate:"required"`
	ADOPipelineID      string `mapstructure:"ADO_PIPELINE_ID" validate:"required"`
	ADOWebhookPassword string `mapstructure:"ADO_WEBHOOK_PASSWORD"`
	ADOWebhookUsername string `mapstructure:"ADO_WEBHOOK_USERNAME"`

	// --- Security ---
	Security struct {
		AllowedInvokers   []string `mapstructure:"allowed_invokers"`
		AllowedAdmins     []string `mapstructure:"allowed_admins"`
		ExpectedAudiences []string `mapstructure:"expected_audiences" validate:"required"`
		AllowedAzps       []string `mapstructure:"allowed_azps" validate:"required"`
	} `mapstructure:"security"`
	// --- Compliance (NOVO) ---
	Compliance struct {
		Rules []compliance.RuleConfig `mapstructure:"rules"`
	} `mapstructure:"compliance"`

	// --- Cloud Tasks ---
	CloudTasks struct {
		QueuePath         string `mapstructure:"queue_path"`
		ServiceURL        string `mapstructure:"service_url"`
		ServiceAccount    string `mapstructure:"service_account"`
		MergeDelaySeconds int    `mapstructure:"merge_delay_seconds"`
	} `mapstructure:"cloud_tasks"`

	// --- Async Security (callback /internal/async/run) ---
	// source: "metadata" → deriva email+sub do GCP metadata server (Cloud Run)
	// source: "config"   → usa allowed_invokers e allowed_azps explícitos
	// ausente            → endpoint /internal/async/run desabilitado
	AsyncSecurity struct {
		Source          string   `mapstructure:"source"` // "metadata" | "config"
		AllowedInvokers []string `mapstructure:"allowed_invokers"`
		AllowedAzps     []string `mapstructure:"allowed_azps"`
	} `mapstructure:"async_security"`
}

func Load(ctx context.Context, dir string) (Config, error) {
	v := viper.New()
	v.AutomaticEnv()
	v.SetDefault("PORT", "8080")
	v.SetDefault("cloud_tasks.merge_delay_seconds", 120)

	// O Terraform monta o arquivo em /app/config/config.yaml
	v.AddConfigPath(dir)
	v.SetConfigName("config")
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("failed to read config file: %w", err)
	}

	// Inicializa o Resolver de Secrets
	resolver, err := secrets.NewResolver(ctx)
	if err != nil {
		return Config{}, err
	}
	defer resolver.Close()

	// Hooks:
	// 1. StringToSlice: permite ler listas do YAML
	// 2. Resolver: Intercepta strings com "_secret://"
	decodeHook := mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
		resolver.MapStructureHook(),
	)

	var cfg Config
	if err := v.Unmarshal(&cfg, viper.DecodeHook(decodeHook)); err != nil {
		return Config{}, fmt.Errorf("failed to load config via viper: %w", err)
	}

	validate := validator.New()
	if err := validate.Struct(cfg); err != nil {
		return Config{}, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}
