package configx

import "github.com/dkoshenkov/packages-go/consterr"

const (
	errTargetMustBePointerToStruct consterr.Error = "target must be a non-nil pointer to struct"
	errTargetMustPointToStruct     consterr.Error = "target must point to a struct"
	errFlagSetIsNil                consterr.Error = "flag set must not be nil"
	errFlagSetAlreadyParsed        consterr.Error = "flag set is already parsed"
	errProfileIsNotSet             consterr.Error = "profile is not set; expected env=prod or env=dev in sources or WithProfile"
	errCfgxTagEmpty                consterr.Error = "cfgx tag must not be empty"
	errCfgxKeyEmpty                consterr.Error = "cfgx key must not be empty"
	errCfgxDuplicatedDefault       consterr.Error = "cfgx tag has duplicated default option"
)
