package configx

import "github.com/dkoshenkov/packages-go/consterr"

const (
	errTargetMustBePointerToStruct consterr.Error = "target must be a non-nil pointer to struct"
	errTargetMustPointToStruct     consterr.Error = "target must point to a struct"
	errFlagSetIsNil                consterr.Error = "flag set must not be nil"
	errFlagSetAlreadyParsed        consterr.Error = "flag set is already parsed"
	errProfileIsNotSet             consterr.Error = "profile is not set; expected env=prod or env=dev in sources or WithProfile"
	errVaultCredentialsMissingPath consterr.Error = "vault credentials path must not be empty"
	errVaultCredentialsMissingAddr consterr.Error = "vault credentials address must not be empty"
	errSeedInvalidTarget           consterr.Error = "seed target must be one of: vault,yaml,env"
	errSeedVaultSourceMissing      consterr.Error = "vault source is not configured"
	errSeedVaultWriterMissing      consterr.Error = "vault source does not support seeding"
	errCfgxTagEmpty                consterr.Error = "cfgx tag must not be empty"
	errCfgxKeyEmpty                consterr.Error = "cfgx key must not be empty"
	errCfgxDuplicatedDefault       consterr.Error = "cfgx tag has duplicated default option"
)
