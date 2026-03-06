package secrets

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/mitchellh/mapstructure"
)

const secretPrefix = "_secret://"

type Resolver struct {
	client *secretmanager.Client
}

func NewResolver(ctx context.Context) (*Resolver, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret manager client: %w", err)
	}
	return &Resolver{client: client}, nil
}

func (r *Resolver) Close() error {
	return r.client.Close()
}

// Fetch vai no GCP buscar o valor
func (r *Resolver) Fetch(ctx context.Context, name string) (string, error) {
	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	}
	result, err := r.client.AccessSecretVersion(ctx, req)
	if err != nil {
		return "", err
	}
	return string(result.Payload.Data), nil
}

// MapStructureHook é a mágica que conecta com o Viper/Mapstructure
func (r *Resolver) MapStructureHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String || t.Kind() != reflect.String {
			return data, nil
		}

		val := data.(string)

		// Se a string começar com _secret://, resolvemos o valor
		if strings.HasPrefix(val, secretPrefix) {
			secretVersionName := strings.TrimPrefix(val, secretPrefix)

			// Usamos context.Background pois o hook é síncrono
			payload, err := r.Fetch(context.Background(), secretVersionName)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve secret '%s': %w", secretVersionName, err)
			}
			return payload, nil
		}

		return data, nil
	}
}
