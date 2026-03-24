package logx

import "github.com/dkoshenkov/packages-go/consterr"

const (
	errWriterIsNil                    = consterr.Error("logx: writer must not be nil")
	errCallerSkipFrameCountNegative   = consterr.Error("logx: caller skip frame count must be zero or greater")
	errServiceFieldNameMustNotBeEmpty = consterr.Error("logx: service field name must not be empty")
)
