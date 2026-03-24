package configx

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/pflag"
)

func bindSystemFlags(flagSet *pflag.FlagSet, cfg *config) error {
	if flagSet.Lookup(flagSeedDefaults) == nil {
		flagSet.Bool(flagSeedDefaults, cfg.seedDefaults, "")
	}
	if flagSet.Lookup(flagSeedTargets) == nil {
		flagSet.String(flagSeedTargets, joinSeedTargets(cfg.seedTargets), "")
	}
	if flagSet.Lookup(flagSeedForce) == nil {
		flagSet.Bool(flagSeedForce, cfg.seedForce, "")
	}
	if flagSet.Lookup(flagSeedYAMLFile) == nil {
		flagSet.String(flagSeedYAMLFile, cfg.seedYAMLFile, "")
	}
	if flagSet.Lookup(flagSeedENVFile) == nil {
		flagSet.String(flagSeedENVFile, cfg.seedENVFile, "")
	}
	if flagSet.Lookup(flagSeedOnly) == nil {
		flagSet.Bool(flagSeedOnly, cfg.seedOnly, "")
	}
	if flagSet.Lookup(flagVaultAddress) == nil {
		flagSet.String(flagVaultAddress, currentVaultAddress(cfg), "")
	}
	if flagSet.Lookup(flagVaultToken) == nil {
		flagSet.String(flagVaultToken, currentVaultToken(cfg), "")
	}
	if flagSet.Lookup(flagVaultNS) == nil {
		flagSet.String(flagVaultNS, currentVaultNamespace(cfg), "")
	}
	if flagSet.Lookup(flagVaultPath) == nil {
		flagSet.String(flagVaultPath, currentVaultPath(cfg), "")
	}

	return nil
}

func applySystemFlags(flagSet *pflag.FlagSet, cfg *config) error {
	if changed(flagSet, flagSeedDefaults) {
		enabled, err := flagSet.GetBool(flagSeedDefaults)
		if err != nil {
			return err
		}
		cfg.seedDefaults = enabled
	}

	if changed(flagSet, flagSeedTargets) {
		rawTargets, err := flagSet.GetString(flagSeedTargets)
		if err != nil {
			return err
		}
		targets, err := parseSeedTargets(rawTargets)
		if err != nil {
			return err
		}
		cfg.seedTargets = targets
		cfg.seedDefaults = true
	}

	if changed(flagSet, flagSeedForce) {
		force, err := flagSet.GetBool(flagSeedForce)
		if err != nil {
			return err
		}
		cfg.seedForce = force
	}

	if changed(flagSet, flagSeedYAMLFile) {
		path, err := flagSet.GetString(flagSeedYAMLFile)
		if err != nil {
			return err
		}
		cfg.seedYAMLFile = strings.TrimSpace(path)
	}

	if changed(flagSet, flagSeedENVFile) {
		path, err := flagSet.GetString(flagSeedENVFile)
		if err != nil {
			return err
		}
		cfg.seedENVFile = strings.TrimSpace(path)
	}

	if changed(flagSet, flagSeedOnly) {
		only, err := flagSet.GetBool(flagSeedOnly)
		if err != nil {
			return err
		}
		cfg.seedOnly = only
		if only {
			cfg.seedDefaults = true
		}
	}

	if changed(flagSet, flagVaultAddress) {
		address, err := flagSet.GetString(flagVaultAddress)
		if err != nil {
			return err
		}
		ensureVaultCredentials(cfg)
		cfg.vaultCredentials.Address = strings.TrimSpace(address)
	}

	if changed(flagSet, flagVaultToken) {
		token, err := flagSet.GetString(flagVaultToken)
		if err != nil {
			return err
		}
		ensureVaultCredentials(cfg)
		cfg.vaultCredentials.Token = strings.TrimSpace(token)
	}

	if changed(flagSet, flagVaultNS) {
		namespace, err := flagSet.GetString(flagVaultNS)
		if err != nil {
			return err
		}
		ensureVaultCredentials(cfg)
		cfg.vaultCredentials.Namespace = strings.TrimSpace(namespace)
	}

	if changed(flagSet, flagVaultPath) {
		path, err := flagSet.GetString(flagVaultPath)
		if err != nil {
			return err
		}
		ensureVaultCredentials(cfg)
		cfg.vaultCredentials.Path = strings.TrimSpace(path)
	}

	if cfg.seedDefaults && len(cfg.seedTargets) == 0 {
		cfg.seedTargets = allSeedTargets()
	}

	return nil
}

func changed(flagSet *pflag.FlagSet, name string) bool {
	flag := flagSet.Lookup(name)
	return flag != nil && flag.Changed
}

func joinSeedTargets(targets map[string]struct{}) string {
	if len(targets) == 0 {
		return ""
	}

	values := make([]string, 0, len(targets))
	for target := range targets {
		values = append(values, target)
	}
	sort.Strings(values)
	return strings.Join(values, ",")
}

func parseSeedTargets(raw string) (map[string]struct{}, error) {
	values := strings.Split(raw, ",")
	targets := make(map[string]struct{}, len(values))
	for _, value := range values {
		target := strings.ToLower(strings.TrimSpace(value))
		if target == "" {
			continue
		}
		switch target {
		case seedTargetVault, seedTargetYAML, seedTargetENV:
			targets[target] = struct{}{}
		default:
			return nil, fmt.Errorf("%w: %q", errSeedInvalidTarget, target)
		}
	}

	return targets, nil
}

func allSeedTargets() map[string]struct{} {
	return map[string]struct{}{
		seedTargetVault: {},
		seedTargetYAML:  {},
		seedTargetENV:   {},
	}
}

func ensureVaultCredentials(cfg *config) {
	if cfg.vaultCredentials != nil {
		return
	}
	cfg.vaultCredentials = &VaultCredentials{}
}

func currentVaultAddress(cfg *config) string {
	if cfg.vaultCredentials == nil {
		return ""
	}
	return cfg.vaultCredentials.Address
}

func currentVaultToken(cfg *config) string {
	if cfg.vaultCredentials == nil {
		return ""
	}
	return cfg.vaultCredentials.Token
}

func currentVaultNamespace(cfg *config) string {
	if cfg.vaultCredentials == nil {
		return ""
	}
	return cfg.vaultCredentials.Namespace
}

func currentVaultPath(cfg *config) string {
	if cfg.vaultCredentials == nil {
		return ""
	}
	return cfg.vaultCredentials.Path
}
