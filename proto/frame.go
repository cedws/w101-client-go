package proto

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

const headerMagic uint16 = 0xF00D

type FrameReader struct {
	Reader io.Reader
}

type FrameWriter struct {
	writer io.Writer
}

type frameReadWriter struct {
	FrameReader
	FrameWriter
}

type Frame struct {
	Control     bool
	Opcode      uint8
	MessageData []byte
}

func (f *Frame) Unmarshal(data []byte) error {
	if len(data) < 5 {
		return fmt.Errorf("invalid frame, expected at least 5 bytes but got %v", len(data))
	}

	f.Control = data[0] == 0x1
	f.Opcode = data[1]
	f.MessageData = data[4:]

	return nil
}

func (f *Frame) Marshal() []byte {
	buf := make([]byte, 4+len(f.MessageData))
	if f.Control {
		buf[0] = 0x1
	}
	buf[1] = f.Opcode
	copy(buf[4:], f.MessageData)

	return buf
}

func (r *FrameReader) Read() (*Frame, error) {
	var (
		magic  uint16
		length uint16
	)

	if err := binary.Read(r.Reader, binary.LittleEndian, &magic); err != nil {
		return nil, err
	}
	if magic != headerMagic {
		return nil, fmt.Errorf("invalid frame, expected %v in header but got %v", headerMagic, magic)
	}

	if err := binary.Read(r.Reader, binary.LittleEndian, &length); err != nil {
		return nil, err
	}

	realLength := uint32(length)
	if length >= 0x8000 {
		if err := binary.Read(r.Reader, binary.LittleEndian, &realLength); err != nil {
			return nil, err
		}
	}

	rawFrame := make([]byte, realLength)
	if _, err := io.ReadFull(r.Reader, rawFrame); err != nil {
		return nil, err
	}

	frame := &Frame{}
	if err := frame.Unmarshal(rawFrame); err != nil {
		return nil, err
	}

	return frame, nil
}

func (w *FrameWriter) Write(frame *Frame) error {
	rawFrame := frame.Marshal()

	if err := binary.Write(w.writer, binary.LittleEndian, headerMagic); err != nil {
		return err
	}

	if len(rawFrame) > 0x7FFF {
		if len(rawFrame) > math.MaxUint32 {
			return fmt.Errorf("frame too large, max size is %v but got %v", math.MaxUint32, len(rawFrame))
		}

		if err := binary.Write(w.writer, binary.LittleEndian, uint16(0x8000)); err != nil {
			return err
		}
		if err := binary.Write(w.writer, binary.LittleEndian, uint32(len(rawFrame)+1)); err != nil {
			return err
		}
	} else {
		if err := binary.Write(w.writer, binary.LittleEndian, uint16(len(rawFrame)+1)); err != nil {
			return err
		}
	}

	if _, err := w.writer.Write(rawFrame); err != nil {
		return err
	}
	if _, err := w.writer.Write([]byte{0}); err != nil {
		return err
	}

	return nil
}
