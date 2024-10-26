package proto

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

const headerMagic uint16 = 0xF00D

type frameReader struct {
	reader io.Reader
}

type frameWriter struct {
	writer io.Writer
}

type controlReadWriter struct {
	frameReader
	frameWriter
}

type Frame struct {
	Control     bool
	Opcode      uint8
	MessageData []byte
}

func (f *Frame) UnmarshalBinary(data []byte) error {
	if len(data) < 5 {
		return fmt.Errorf("invalid frame, expected at least 5 bytes but got %v", len(data))
	}

	f.Control = data[0] == 0x1
	f.Opcode = data[1]
	f.MessageData = data[4:]

	return nil
}

func (f *Frame) MarshalBinary() ([]byte, error) {
	buf := make([]byte, 4+len(f.MessageData))
	if f.Control {
		buf[0] = 0x1
	}
	buf[1] = f.Opcode
	copy(buf[4:], f.MessageData)

	return buf, nil
}

func (r *frameReader) Read() (*Frame, error) {
	var (
		magic  uint16
		length uint16
	)

	if err := binary.Read(r.reader, binary.LittleEndian, &magic); err != nil {
		return nil, err
	}
	if magic != headerMagic {
		return nil, fmt.Errorf("invalid frame, expected %v in header but got %v", headerMagic, magic)
	}

	if err := binary.Read(r.reader, binary.LittleEndian, &length); err != nil {
		return nil, err
	}

	realLength := uint32(length)
	if length >= 0x8000 {
		if err := binary.Read(r.reader, binary.LittleEndian, &realLength); err != nil {
			return nil, err
		}
	}

	rawFrame := make([]byte, realLength)
	if _, err := io.ReadFull(r.reader, rawFrame); err != nil {
		return nil, err
	}

	frame := &Frame{}
	if err := frame.UnmarshalBinary(rawFrame); err != nil {
		return nil, err
	}

	return frame, nil
}

func (w *frameWriter) Write(frame *Frame) error {
	rawFrame, err := frame.MarshalBinary()
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer(make([]byte, 0, 5+len(rawFrame)))

	binary.Write(buf, binary.LittleEndian, headerMagic)

	if len(rawFrame) > 0x7FFF {
		if len(rawFrame) > math.MaxUint32 {
			return fmt.Errorf("frame too large, max size is %v but got %v", math.MaxUint32, len(rawFrame))
		}

		binary.Write(buf, binary.LittleEndian, uint16(0x8000))
		binary.Write(buf, binary.LittleEndian, uint32(len(rawFrame)+1))
	} else {
		binary.Write(buf, binary.LittleEndian, uint16(len(rawFrame)+1))
	}

	buf.Write(rawFrame)
	buf.WriteByte(0)

	if _, err := io.Copy(w.writer, buf); err != nil {
		return err
	}

	return nil
}
