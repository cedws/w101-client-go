package codegen

import (
	"errors"
)

var (
	ErrorMissingProtocolInfo = errors.New("codegen: failed to locate protocol info")
	ErrorMissingRecords      = errors.New("codegen: failed to locate records")
	ErrorInvalidRecord       = errors.New("codegen: record is not valid")
	ErrorInvalidMessage      = errors.New("codegen: message is not valid")
	ErrorNoGoPackageEnv      = errors.New("codegen: $GOPACKAGE was not set in environment")
)
