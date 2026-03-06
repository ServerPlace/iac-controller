package iam

import (
	"context"
	"fmt"
	"time"

	iamcredentials "cloud.google.com/go/iam/credentials/apiv1"
	"cloud.google.com/go/iam/credentials/apiv1/credentialspb"
	"google.golang.org/protobuf/types/known/durationpb"
)

// Service encapsula a lógica de Identity e as configs das Service Accounts
type Service struct {
	planSA  string
	applySA string
	client  *iamcredentials.IamCredentialsClient
}

// New cria o serviço de IAM já com as configurações carregadas.
func New(ctx context.Context, planSA, applySA string) (*Service, error) {
	client, err := iamcredentials.NewIamCredentialsClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create iam client: %w", err)
	}
	return &Service{
		planSA:  planSA,
		applySA: applySA,
		client:  client,
	}, nil
}

// GenerateAccessToken gera um token OAuth2 para a SA de destino configurada.
func (s *Service) GenerateAccessToken(ctx context.Context, mode string) (string, time.Time, error) {
	target := s.planSA
	if mode == "apply" {
		target = s.applySA
	}

	req := &credentialspb.GenerateAccessTokenRequest{
		Name: "projects/-/serviceAccounts/" + target,
		Scope: []string{
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/userinfo.email",
			"openid",
		},
		Lifetime: durationpb.New(1 * time.Hour),
	}

	resp, err := s.client.GenerateAccessToken(ctx, req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to generate token for %s: %w", target, err)
	}

	return resp.AccessToken, resp.ExpireTime.AsTime(), nil
}
