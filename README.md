# go-dml-codegen
Generates Go code from a DML service definition.

## TODO:
* Tests/cleanup
* Generate numeric constant for each message type

## Usage:
```
$ ./go-dml-codegen LoginMessages.xml
// ServiceID:           7
// ProtocolType:        LOGIN
// ProtocolVersion:     1
// ProtocolDescription: Login Server Messages

...
```