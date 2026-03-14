package flagx

import "github.com/dkoshenkov/packages-go/consterr"

const (
	errNilTarget = consterr.Error("flagx: nil target")
	errNilParser = consterr.Error("flagx: nil parser")
)
