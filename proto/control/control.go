package control

import (
	"bytes"
	"encoding/binary"
	"io"
)

const (
	PktSessionOffer  byte = 0x0
	PktSessionAccept byte = 0x5
)

type SessionOffer struct {
	SessionID  uint16
	TimeSecs   uint32
	TimeMillis uint32
	RawMessage []byte
	Signature  []byte
}

func (s *SessionOffer) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer

	write := func(v any) {
		binary.Write(&buf, binary.LittleEndian, v)
	}

	writeMessage := func() {
		length := uint32(len(s.RawMessage) + len(s.Signature))
		write(length)
		buf.Write(s.RawMessage)
		buf.Write(s.Signature)
	}

	write(s.SessionID)
	buf.Write(make([]byte, 4))
	write(s.TimeSecs)
	write(s.TimeMillis)
	writeMessage()
	buf.WriteByte(0)

	return buf.Bytes(), nil
}

func (s *SessionOffer) UnmarshalBinary(data []byte) error {
	buf := bytes.NewReader(data)

	read := func(v any) error {
		return binary.Read(buf, binary.LittleEndian, v)
	}

	readMessage := func() ([]byte, error) {
		var length uint32
		if err := read(&length); err != nil {
			return nil, err
		}
		msg := make([]byte, length)
		if _, err := buf.Read(msg); err != nil {
			return nil, err
		}
		return msg, nil
	}

	if err := read(&s.SessionID); err != nil {
		return err
	}
	if _, err := buf.Seek(4, io.SeekCurrent); err != nil {
		return err
	}
	if err := read(&s.TimeSecs); err != nil {
		return err
	}
	if err := read(&s.TimeMillis); err != nil {
		return err
	}
	msg, err := readMessage()
	if err != nil {
		return err
	}

	if len(msg) > 256 {
		s.RawMessage = msg[:len(msg)-256]
		s.Signature = msg[len(msg)-256:]
	}

	return nil
}

type SessionAccept struct {
	TimeSecs         uint32
	TimeMillis       uint32
	SessionID        uint16
	EncryptedMessage []byte
}

func (s *SessionAccept) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer

	write := func(v any) {
		binary.Write(&buf, binary.LittleEndian, v)
	}

	if len(s.EncryptedMessage) == 0 {
		s.EncryptedMessage = make([]byte, 1)
	}

	buf.Write(make([]byte, 6))
	write(s.TimeSecs)
	write(s.TimeMillis)
	write(s.SessionID)
	write(uint32(len(s.EncryptedMessage)))
	buf.Write(s.EncryptedMessage)
	buf.WriteByte(0)

	return buf.Bytes(), nil
}

func (s *SessionAccept) UnmarshalBinary(data []byte) error {
	buf := bytes.NewReader(data)

	read := func(v any) error {
		return binary.Read(buf, binary.LittleEndian, v)
	}

	readMessage := func() ([]byte, error) {
		var length uint32
		if err := read(&length); err != nil {
			return nil, err
		}
		msg := make([]byte, length)
		if _, err := buf.Read(msg); err != nil {
			return nil, err
		}
		return msg, nil
	}

	if _, err := buf.Seek(6, io.SeekCurrent); err != nil {
		return err
	}
	if err := read(&s.TimeSecs); err != nil {
		return err
	}
	if err := read(&s.TimeMillis); err != nil {
		return err
	}
	if err := read(&s.SessionID); err != nil {
		return err
	}
	msg, err := readMessage()
	if err != nil {
		return err
	}
	s.EncryptedMessage = msg

	return nil
}
