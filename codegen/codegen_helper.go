package codegen

import (
	"bytes"
	"encoding/binary"
	"unsafe"
)

func WriteString(b *bytes.Buffer, v string) {
	binary.Write(b, binary.LittleEndian, uint16(len(v)))
	b.WriteString(v)
}

func ReadString(buf *bytes.Reader) (string, error) {
	var length uint16
	if err := binary.Read(buf, binary.LittleEndian, &length); err != nil {
		return "", err
	}
	data := make([]byte, length)
	if _, err := buf.Read(data); err != nil {
		return "", err
	}
	return *(*string)(unsafe.Pointer(&data)), nil
}
