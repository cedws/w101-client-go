package codegen

import (
	"errors"
)

var (
	ErrorMissingProtocolInfo = errors.New("failed to locate protocol info")
	ErrorMissingRecords      = errors.New("failed to locate records")
	ErrorInvalidRecord       = errors.New("record is not valid")
	ErrorInvalidMessage      = errors.New("message is not valid")
	ErrorInvalidSyntax       = errors.New("generated code had invalid syntax")
	ErrorNoGoPackageEnv      = errors.New("$GOPACKAGE was not set in environment")
)
