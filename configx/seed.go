package configx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type seedEntry struct {
	fieldPath    string
	envKey       string
	yamlKey      string
	stringValue  string
	yamlValue    any
	vaultValue   any
	hasDefault   bool
	defaultParse error
}

func maybeSeedDefaults(ctx context.Context, rt *runtime, fields []fieldSpec) error {
	if !rt.cfg.seedDefaults {
		return nil
	}

	targets := rt.cfg.seedTargets
	if len(targets) == 0 {
		targets = allSeedTargets()
	}

	entries, buildErr := buildSeedEntries(rt, fields)
	if buildErr != nil {
		return buildErr
	}

	var errs []error

	if _, ok := targets[seedTargetYAML]; ok {
		yamlPath := chooseSeedYAMLPath(rt.cfg)
		if err := seedYAMLFile(yamlPath, entries, rt.cfg.seedForce); err != nil {
			errs = append(errs, fmt.Errorf("seed yaml: %w", err))
		} else if rt.cfg.yamlFile == yamlPath {
			reloaded, err := loadYAMLFile(yamlPath)
			if err != nil {
				errs = append(errs, fmt.Errorf("reload yaml: %w", err))
			} else {
				rt.yaml = reloaded
			}
		}
	}

	if _, ok := targets[seedTargetENV]; ok {
		envPath := chooseSeedENVPath(rt.cfg)
		if err := seedENVFile(envPath, entries, rt.cfg.seedForce); err != nil {
			errs = append(errs, fmt.Errorf("seed env: %w", err))
		}
	}

	if _, ok := targets[seedTargetVault]; ok {
		if rt.cfg.vault == nil {
			errs = append(errs, errSeedVaultSourceMissing)
		} else {
			seeder, ok := rt.cfg.vault.(VaultSeeder)
			if !ok {
				errs = append(errs, errSeedVaultWriterMissing)
			} else {
				values := make(map[string]any, len(entries))
				for _, entry := range entries {
					values[entry.envKey] = entry.vaultValue
				}
				if err := seeder.SeedDefaults(ctx, values, rt.cfg.seedForce); err != nil {
					errs = append(errs, fmt.Errorf("seed vault: %w", err))
				}
			}
		}
	}

	return errors.Join(errs...)
}

func buildSeedEntries(rt *runtime, fields []fieldSpec) ([]seedEntry, error) {
	entries := make([]seedEntry, 0, len(fields))
	var errs []error

	for _, field := range fields {
		entry := seedEntry{
			fieldPath:   field.path,
			envKey:      rt.profileEnvPrefix + field.envKey,
			yamlKey:     rt.profileYAMLPrefix + field.yamlKey,
			stringValue: "",
			yamlValue:   nil,
			vaultValue:  nil,
			hasDefault:  field.hasDefault,
		}

		if field.hasDefault {
			decoded, err := decodeYAMLBytes([]byte(field.defaultRaw), field.typ)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: decode default: %w", field.path, err))
			} else {
				entry.stringValue = field.defaultRaw
				entry.yamlValue = decoded.Interface()
				entry.vaultValue = field.defaultRaw
			}
		}

		entries = append(entries, entry)
	}

	return entries, errors.Join(errs...)
}

func chooseSeedYAMLPath(cfg config) string {
	if strings.TrimSpace(cfg.seedYAMLFile) != "" {
		return cfg.seedYAMLFile
	}
	if strings.TrimSpace(cfg.yamlFile) != "" {
		return cfg.yamlFile
	}
	return defaultSeedYAMLFile
}

func chooseSeedENVPath(cfg config) string {
	if strings.TrimSpace(cfg.seedENVFile) != "" {
		return cfg.seedENVFile
	}
	return defaultSeedENVFile
}

func seedYAMLFile(path string, entries []seedEntry, force bool) error {
	doc, err := readYAMLMap(path)
	if err != nil {
		return err
	}

	changed := false
	for _, entry := range entries {
		if upsertYAMLPath(doc, entry.yamlKey, entry.yamlValue, force) {
			changed = true
		}
	}
	if !changed {
		return nil
	}

	content, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, content, 0o600)
}

func readYAMLMap(path string) (map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]any), nil
		}
		return nil, err
	}

	if len(content) == 0 {
		return make(map[string]any), nil
	}

	var doc map[string]any
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		doc = make(map[string]any)
	}

	return doc, nil
}

func upsertYAMLPath(doc map[string]any, path string, value any, force bool) bool {
	parts := strings.Split(path, yamlPartsSeparator)
	current := doc

	for _, part := range parts[:len(parts)-1] {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		next, ok := current[part]
		if !ok || next == nil {
			created := make(map[string]any)
			current[part] = created
			current = created
			continue
		}

		asMap, ok := next.(map[string]any)
		if !ok {
			if !force {
				return false
			}
			created := make(map[string]any)
			current[part] = created
			current = created
			continue
		}

		current = asMap
	}

	last := strings.TrimSpace(parts[len(parts)-1])
	if last == "" {
		return false
	}

	existing, exists := current[last]
	if exists && existing != nil && !force {
		return false
	}

	current[last] = value
	return true
}

func seedENVFile(path string, entries []seedEntry, force bool) error {
	values, err := readENVMap(path)
	if err != nil {
		return err
	}

	changed := false
	for _, entry := range entries {
		current, exists := values[entry.envKey]
		if exists && current != "" && !force {
			continue
		}
		if exists && current == entry.stringValue {
			continue
		}

		values[entry.envKey] = entry.stringValue
		changed = true
	}

	if !changed {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(values[key])
		b.WriteByte('\n')
	}

	return os.WriteFile(path, []byte(b.String()), 0o600)
}

func readENVMap(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, err
	}

	values := make(map[string]string)
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		name, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(name)] = value
	}

	return values, nil
}
