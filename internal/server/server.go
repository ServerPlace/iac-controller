package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/ServerPlace/iac-controller/internal/async"
	"github.com/ServerPlace/iac-controller/internal/compliance"
	"github.com/ServerPlace/iac-controller/internal/config"
	"github.com/ServerPlace/iac-controller/internal/credentials"
	"github.com/ServerPlace/iac-controller/internal/iam"
	middleware2 "github.com/ServerPlace/iac-controller/internal/middleware"
	pipeline_azure "github.com/ServerPlace/iac-controller/internal/pipeline/azure"
	"github.com/ServerPlace/iac-controller/internal/scm"
	scm_azure "github.com/ServerPlace/iac-controller/internal/scm/azure"
	scm_github "github.com/ServerPlace/iac-controller/internal/scm/github"
	"github.com/ServerPlace/iac-controller/internal/server/controllers/admin"
	credctrl "github.com/ServerPlace/iac-controller/internal/server/controllers/credentials"
	"github.com/ServerPlace/iac-controller/internal/server/controllers/plans"
	tasks_ctrl "github.com/ServerPlace/iac-controller/internal/server/controllers/tasks"
	"github.com/ServerPlace/iac-controller/internal/server/controllers/webhook"
	"github.com/ServerPlace/iac-controller/internal/service"
	fsadapter "github.com/ServerPlace/iac-controller/internal/storage/firestore"
	"github.com/ServerPlace/iac-controller/internal/webhook/auth"
	"github.com/ServerPlace/iac-controller/internal/webhook/providers"
	"github.com/ServerPlace/iac-controller/pkg/api"
	"github.com/ServerPlace/iac-controller/pkg/log"
)

const (
	AsyncEndpoint = "/internal/async/run"
)

// Server runs the IaC Controller web server
// Following Atlantis pattern: Server struct holds all dependencies
type Server struct {
	// Configuration
	Config config.Config
	Port   string
	Logger zerolog.Logger

	// HTTP
	Router *chi.Mux

	// Controllers (handlers prontos)
	CredentialsController  *credctrl.CredentialsController
	AdminController        *admin.AdminController
	AzureWebhookController *webhook.WebhookController
	PlansController        *plans.PlansController
	// GitHubWebhookController *controllers.WebhookController // Future

	// Business Logic
	DeploymentService *service.DeploymentService
	ComplianceEngine  *compliance.Engine

	// Infrastructure
	IAM      *iam.Service
	SCM      scm.Client
	Storage  *fsadapter.Adapter
	Pipeline *pipeline_azure.Adapter

	// Async
	AsyncEngine   *async.Engine
	AsyncRegistry *async.Registry
}

// NewServer creates a new IaC Controller server following Atlantis pattern
// All dependencies are initialized here
func NewServer(cfg config.Config, logger zerolog.Logger) (*Server, error) {
	ctx := context.Background()

	srv := &Server{
		Config: cfg,
		Port:   cfg.Port,
		Logger: logger,
		Router: chi.NewRouter(),
	}

	// Initialize all dependencies
	if err := srv.initDependencies(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize dependencies: %w", err)
	}

	// Initialize controllers
	srv.initControllers()

	return srv, nil
}

// initDependencies initializes all infrastructure dependencies
func (s *Server) initDependencies(ctx context.Context) error {
	var err error

	// 1. IAM Service
	s.IAM, err = iam.New(ctx, s.Config.PlanServiceAccount, s.Config.ApplyServiceAccount)
	if err != nil {
		return fmt.Errorf("failed to initialize IAM service: %w", err)
	}

	// 2. SCM Client Factory
	switch s.Config.SCMProvider {
	case "azure":
		s.SCM, err = scm_azure.NewClient(ctx, s.Config)
	case "github":
		s.SCM, err = scm_github.NewClient(ctx, s.Config)
	default:
		return fmt.Errorf("unsupported SCM provider: %s", s.Config.SCMProvider)
	}
	if err != nil {
		return fmt.Errorf("failed to initialize SCM client: %w", err)
	}

	// 3. Firestore Client
	fsClient, err := firestore.NewClient(ctx, s.Config.GCPProject)
	if err != nil {
		return fmt.Errorf("failed to connect to Firestore: %w", err)
	}
	s.Storage = fsadapter.New(fsClient)

	// 4. Async Security — resolve origem conforme async_security.source
	switch s.Config.AsyncSecurity.Source {
	case "metadata":
		email, uid, merr := deriveIdentityFromMetadata(ctx, s.Config.CloudTasks.ServiceURL)
		if merr != nil {
			return fmt.Errorf("async_security source=metadata: failed to derive identity: %w", merr)
		}
		s.Config.AsyncSecurity.AllowedInvokers = []string{email}
		s.Config.AsyncSecurity.AllowedAzps = []string{uid}
		s.Logger.Info().Str("email", email).Msg("async: identity derived from GCP metadata")
	case "config":
		s.Logger.Info().Strs("invokers", s.Config.AsyncSecurity.AllowedInvokers).Msg("async: using explicit security config")
	default:
		s.Logger.Warn().Msgf("async: async_security.source not set; %s will be disabled", AsyncEndpoint)
	}

	// 5. Async Engine (Cloud Tasks) — opcional: só inicializa se QueuePath configurado
	if ct := s.Config.CloudTasks; ct.QueuePath != "" {
		ctClient, err := cloudtasks.NewClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create cloud tasks client: %w", err)
		}
		parts := strings.Split(ct.QueuePath, "/")
		// formato esperado: projects/{project}/locations/{location}/queues/{queue}
		if len(parts) != 6 {
			return fmt.Errorf("invalid cloud_tasks.queue_path: %q", ct.QueuePath)
		}
		enqueuer := async.NewCloudTasksEnqueuer(ctClient, async.CloudTasksEnqueuerConfig{
			ProjectID:                  parts[1],
			Location:                   parts[3],
			Queue:                      parts[5],
			RunURL:                     strings.TrimRight(ct.ServiceURL, "/") + "/internal/async/run",
			InvokerServiceAccountEmail: ct.ServiceAccount,
			OIDCAudience:               ct.ServiceURL,
		})
		store := fsadapter.NewFirestoreStore(fsClient, "")
		s.AsyncRegistry = async.NewRegistry()
		s.AsyncEngine = async.NewEngine(store, enqueuer, s.AsyncRegistry, 0)
	} else {
		s.Logger.Warn().Msg("async: Engine not configured")
	}

	// 5. Pipeline Adapter
	s.Pipeline = pipeline_azure.New(
		s.Config.ADOOrgURL,
		s.Config.ADOProject,
		s.Config.ADOPAT,
		s.Config.ADOPipelineID,
	)

	// 5. Compliance Engine
	s.ComplianceEngine, err = compliance.BuildEngine(s.Config.Compliance.Rules)
	if err != nil {
		return fmt.Errorf("failed to initialize compliance engine: %w", err)
	}

	// 6. Business Services
	s.DeploymentService = service.NewDeploymentService(
		s.Config,
		s.SCM,
		s.Pipeline,
		s.Storage,
		s.ComplianceEngine,
	)

	return nil
}

// initControllers initializes all HTTP controllers
func (s *Server) initControllers() {
	// Credentials Controller
	s.CredentialsController = credctrl.NewCredentialsController(
		s.Config,
		s.Storage,
		s.SCM,
		s.IAM,
	)

	// Admin Controller - ATUALIZADO: agora recebe SCM
	s.AdminController = admin.NewAdminController(
		s.Config,
		s.Storage,
		s.SCM, // SCM injetado para FetchRepositoryMetadata
	)

	// Azure Webhook Controller
	azureProcessor := providers.NewAzureProcessor()
	s.AzureWebhookController = webhook.NewWebhookController(
		azureProcessor,
		s.DeploymentService,
		"azure",
	)

	// Plans Controller
	if s.AsyncRegistry != nil {
		s.AsyncRegistry.Register(tasks_ctrl.KindMergePR, tasks_ctrl.NewMergePRHandler(s.Storage, s.SCM))
	}
	s.PlansController = plans.NewPlansController(s.Storage, s.Config, s.SCM, s.AsyncEngine)

	// Future: GitHub Webhook Controller
	// githubProcessor := providers.NewGitHubProcessor()
	// s.GitHubWebhookController = controllers.NewWebhookController(
	// 	githubProcessor,
	// 	s.DeploymentService,
	// 	"github",
	// )
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	r := s.Router

	// Global middleware(s)
	r.Use(middleware2.WithLogger(s.Logger))

	// Health check (no auth)
	r.Get("/healthz", s.handleHealth)

	// Reusable OIDC middlewares
	oidcInvokers := middleware2.OIDCAuth(
		s.Config.Security.AllowedInvokers,
		s.Config.Security.ExpectedAudiences,
		s.Config.Security.AllowedAzps,
	)
	oidcAdmins := middleware2.OIDCAuth(
		s.Config.Security.AllowedAdmins,
		s.Config.Security.ExpectedAudiences,
		s.Config.Security.AllowedAzps,
	)

	// Credentials API (with OIDC auth)
	r.Route("/v1", func(r chi.Router) {
		r.Use(oidcInvokers)
		r.Post("/credentials", s.CredentialsController.Handle)
	})

	// ==========================================
	// ADMIN API - RESTful Repository Management
	// ==========================================
	r.Route("/admin", func(r chi.Router) {
		r.Use(oidcAdmins)
		// POST /admin/repositories - Register new repository
		r.Post("/repositories", s.AdminController.CreateRepository)

		// GET /admin/repositories - Future: List repositories
		// GET /admin/repositories/{id} - Future: Get repository details
		// PUT /admin/repositories/{id} - Future: Update repository metadata
	})

	// ==========================================
	// WEBHOOKS
	// ==========================================
	azureAuthenticator := auth.NewBasicAuthenticator(
		s.Config.ADOWebhookUsername,
		s.Config.ADOWebhookPassword,
	)

	// Azure webhook (with basic auth)
	r.With(middleware2.WebhookAuth(azureAuthenticator)).
		Method(http.MethodPost, "/webhook/azure", s.AzureWebhookController)

	// Future: GitHub webhook
	// githubAuthenticator := auth.NewHMACAuthenticator(
	// 	s.Config.GithubWebhookSecret,
	// 	"X-Hub-Signature-256",
	// 	"sha256",
	// )
	// r.With(middleware2.WebhookAuth(githubAuthenticator)).
	// 	Method(http.MethodPost, "/webhook/github", s.GitHubWebhookController)

	// ==========================================
	// PLANS API
	// ==========================================

	// HMAC middleware per endpoint (validates signature + resolves repo into ctx).
	hmacPlan := middleware2.HMACAuth(
		func(r api.RegisterPlanRequest) string { return r.Repo },
		credentials.NSPlan,
		s.Storage,
		s.Config.JITSecretKey,
	)
	hmacApprove := middleware2.HMACAuth(
		func(r api.ApproveRequest) string { return r.Repo },
		credentials.NSApply,
		s.Storage,
		s.Config.JITSecretKey,
	)
	hmacClose := middleware2.HMACAuth(
		func(r api.ClosePlanRequest) string { return r.Repo },
		credentials.NSApply,
		s.Storage,
		s.Config.JITSecretKey,
	)

	// ==========================================
	// INTERNAL ASYNC (Cloud Tasks callback)
	// Usa OIDC dedicado com o controller SA — separado dos invokers externos
	// ==========================================
	if s.AsyncEngine != nil && len(s.Config.AsyncSecurity.AllowedInvokers) > 0 {
		oidcAsync := middleware2.OIDCAuth(
			s.Config.AsyncSecurity.AllowedInvokers,
			s.Config.Security.ExpectedAudiences,
			s.Config.AsyncSecurity.AllowedAzps,
		)
		s.Logger.Info().Msgf("async: Endpoint configured in %s", AsyncEndpoint)
		r.With(oidcAsync).Post("/internal/async/run", s.handleAsyncRun)
	} else {
		s.Logger.Warn().Msgf("async: Endpoint not configured")
	}

	r.Route("/api/v1", func(r chi.Router) {
		// LOCAL_DEV: skip OIDC, keep HMAC so request signing is still exercised.
		if os.Getenv("LOCAL_DEV") != "true" {
			r.Use(oidcInvokers)
		}

		// HMAC differs per route
		r.With(hmacPlan).
			Post("/plans", s.PlansController.RegisterPlan)

		r.With(hmacApprove).
			Post("/approve", s.PlansController.Approve)

		r.With(hmacClose).
			Post("/plans/close", s.PlansController.ClosePlan)
	})
}

// deriveIdentityFromMetadata obtém email e unique_id (sub) da SA atual via
// GCP metadata server. Funciona dentro do Cloud Run/GCE; falha em local dev.
func deriveIdentityFromMetadata(ctx context.Context, audience string) (email, uniqueID string, err error) {
	if audience == "" {
		audience = "https://placeholder.internal"
	}
	url := "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity?format=full&audience=" + audience
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	// Decode JWT payload (second segment, base64url, no signature verification needed)
	parts := strings.Split(strings.TrimSpace(string(raw)), ".")
	if len(parts) != 3 {
		return "", "", fmt.Errorf("unexpected identity token format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("failed to decode identity token payload: %w", err)
	}

	var claims struct {
		Email string `json:"email"`
		Sub   string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", "", fmt.Errorf("failed to parse identity token claims: %w", err)
	}
	if claims.Email == "" || claims.Sub == "" {
		return "", "", fmt.Errorf("identity token missing email or sub claim")
	}
	return claims.Email, claims.Sub, nil
}

// handleAsyncRun é o callback HTTP chamado pelo Cloud Tasks.
// Recebe {kind, key}, adquire lease via Engine e executa o handler registrado.
func (s *Server) handleAsyncRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)

	var body struct {
		Kind string `json:"kind"`
		Key  string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	ref := async.ExecutionRef{Kind: body.Kind, Key: body.Key}
	owner := uuid.New().String()

	status, err := s.AsyncEngine.RunOnce(ctx, ref, owner)
	if err != nil {
		logger.Error().Err(err).Str("kind", body.Kind).Str("key", body.Key).Msg("async run failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(status)
}

// handleHealth is a simple health check endpoint
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// ListenAndServe starts the HTTP server
func (s *Server) ListenAndServe() error {
	// Setup routes
	s.setupRoutes()

	// Create HTTP server
	httpServer := &http.Server{
		Addr:              ":" + s.Port,
		Handler:           s.Router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		s.Logger.Info().
			Str("port", s.Port).
			Msg("IaC Controller starting")

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for shutdown signal or error
	select {
	case <-stop:
		s.Logger.Info().Msg("Shutdown signal received, gracefully stopping...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("server shutdown failed: %w", err)
		}

		s.Logger.Info().Msg("Server stopped gracefully")
		return nil

	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	}
}
