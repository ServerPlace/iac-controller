package async

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"github.com/ServerPlace/iac-controller/pkg/log"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type CloudTasksEnqueuer struct {
	client *cloudtasks.Client

	projectID string
	location  string
	queue     string

	// URL completa do Cloud Run endpoint, ex:
	// https://<service>-<hash>-<region>.a.run.app/internal/async/run
	runURL string

	// Service Account que terá permissão invoker no Cloud Run.
	invokerServiceAccountEmail string

	// Audience para OIDC (normalmente o próprio runURL ou a base do serviço Cloud Run).
	oidcAudience string
}

type CloudTasksEnqueuerConfig struct {
	ProjectID string
	Location  string
	Queue     string

	RunURL string

	InvokerServiceAccountEmail string
	OIDCAudience               string
}

func NewCloudTasksEnqueuer(client *cloudtasks.Client, cfg CloudTasksEnqueuerConfig) *CloudTasksEnqueuer {
	return &CloudTasksEnqueuer{
		client:                     client,
		projectID:                  cfg.ProjectID,
		location:                   cfg.Location,
		queue:                      cfg.Queue,
		runURL:                     cfg.RunURL,
		invokerServiceAccountEmail: cfg.InvokerServiceAccountEmail,
		oidcAudience:               cfg.OIDCAudience,
	}
}

func (e *CloudTasksEnqueuer) queuePath() string {
	return fmt.Sprintf("projects/%s/locations/%s/queues/%s", e.projectID, e.location, e.queue)
}

func (e *CloudTasksEnqueuer) EnqueueRun(ctx context.Context, ref ExecutionRef, delay time.Duration) error {
	body, err := json.Marshal(map[string]string{
		"kind": ref.Kind,
		"key":  ref.Key,
	})
	if err != nil {
		return err
	}

	req := &taskspb.CreateTaskRequest{
		Parent: e.queuePath(),
		Task: &taskspb.Task{
			MessageType: &taskspb.Task_HttpRequest{
				HttpRequest: &taskspb.HttpRequest{
					HttpMethod: taskspb.HttpMethod_POST,
					Url:        e.runURL,
					Body:       body,
					Headers: map[string]string{
						"Content-Type": "application/json",
					},
					AuthorizationHeader: &taskspb.HttpRequest_OidcToken{
						OidcToken: &taskspb.OidcToken{
							ServiceAccountEmail: e.invokerServiceAccountEmail,
							Audience:            e.oidcAudience,
						},
					},
				},
			},
		},
	}

	if delay > 0 {
		req.Task.ScheduleTime = timestamppb.New(time.Now().Add(delay))
	}

	logger := log.FromContext(ctx)
	logger.Debug().
		Str("kind", ref.Kind).
		Str("key", ref.Key).
		Str("run_url", e.runURL).
		Str("oidc_sa", e.invokerServiceAccountEmail).
		Str("oidc_audience", e.oidcAudience).
		Str("queue", e.queuePath()).
		Msg("async: enqueuing Cloud Tasks task")

	task, err := e.client.CreateTask(ctx, req)
	if err != nil {
		logger.Error().Err(err).
			Str("run_url", e.runURL).
			Str("oidc_sa", e.invokerServiceAccountEmail).
			Msg("async: failed to create Cloud Tasks task")
		return err
	}
	logger.Info().
		Str("task_name", task.GetName()).
		Str("run_url", e.runURL).
		Msg("async: Cloud Tasks task created successfully")
	return nil
}
