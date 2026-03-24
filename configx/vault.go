package configx

import (
	"context"
	"fmt"
	"strings"
	"sync"

	vaultapi "github.com/hashicorp/vault/api"
)

type vaultReader struct {
	client *vaultapi.Client
	path   string

	mu     sync.Mutex
	loaded bool
	isKV2  bool
	data   map[string]any
	err    error
}

func newVaultReader(credentials VaultCredentials) (*vaultReader, error) {
	address := strings.TrimSpace(credentials.Address)
	path := strings.TrimSpace(credentials.Path)
	if address == "" {
		return nil, errVaultCredentialsMissingAddr
	}
	if path == "" {
		return nil, errVaultCredentialsMissingPath
	}

	cfg := vaultapi.DefaultConfig()
	cfg.Address = address

	client, err := vaultapi.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	if token := strings.TrimSpace(credentials.Token); token != "" {
		client.SetToken(token)
	}
	if namespace := strings.TrimSpace(credentials.Namespace); namespace != "" {
		client.SetNamespace(namespace)
	}

	return &vaultReader{
		client: client,
		path:   path,
		isKV2:  strings.Contains(path, "/data/"),
		data:   make(map[string]any),
	}, nil
}

func (v *vaultReader) Get(ctx context.Context, key string) (string, bool, error) {
	data, err := v.readData(ctx)
	if err != nil {
		return "", false, err
	}

	value, ok := data[key]
	if !ok || value == nil {
		return "", false, nil
	}

	return fmt.Sprint(value), true, nil
}

func (v *vaultReader) SeedDefaults(ctx context.Context, values map[string]any, force bool) error {
	current, err := v.readData(ctx)
	if err != nil {
		return err
	}

	next := cloneMap(current)
	changed := false

	for key, value := range values {
		existing, ok := next[key]
		if ok && existing != nil && !force {
			continue
		}

		if ok && existing == value {
			continue
		}

		next[key] = value
		changed = true
	}

	if !changed {
		return nil
	}

	payload := make(map[string]any)
	if v.isKV2 {
		payload["data"] = next
	} else {
		for key, value := range next {
			payload[key] = value
		}
	}

	if _, err := v.client.Logical().WriteWithContext(ctx, v.path, payload); err != nil {
		return err
	}

	v.mu.Lock()
	v.loaded = false
	v.data = make(map[string]any)
	v.err = nil
	v.mu.Unlock()

	return nil
}

func (v *vaultReader) readData(ctx context.Context) (map[string]any, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.loaded {
		return cloneMap(v.data), v.err
	}

	secret, err := v.client.Logical().ReadWithContext(ctx, v.path)
	if err != nil {
		v.loaded = true
		v.err = err
		return nil, err
	}

	data := make(map[string]any)
	if secret != nil && secret.Data != nil {
		if nested, ok := secret.Data["data"].(map[string]any); ok {
			data = cloneMap(nested)
			v.isKV2 = true
		} else {
			data = cloneMap(secret.Data)
		}
	}

	v.data = data
	v.err = nil
	v.loaded = true

	return cloneMap(v.data), nil
}

func cloneMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}

	return result
}
