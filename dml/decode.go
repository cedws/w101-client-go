package dml

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	TypeRecordTemplate = 1
	TypeRecord         = 2
)

const (
	GID = iota
	INT
	UINT
	FLT
	BYT
	UBYT
	USHRT
	DBL
	STR
	WSTR
)

type Record map[string]any

type Table struct {
	Name    string
	Records []Record
}

type RecordTemplate struct {
	Size   uint16
	Fields []RecordField
	Table  string
}

type RecordField struct {
	Name string
	Type uint8
}

func (r *RecordField) decode(reader *bufio.Reader) error {
	var nameLen uint16
	if err := binary.Read(reader, binary.LittleEndian, &nameLen); err != nil {
		return err
	}

	name := make([]byte, nameLen)
	if err := binary.Read(reader, binary.LittleEndian, &name); err != nil {
		return err
	}
	r.Name = string(name)

	if err := binary.Read(reader, binary.LittleEndian, &r.Type); err != nil {
		return err
	}
	if _, err := reader.Discard(1); err != nil {
		return err
	}

	return nil
}

type TargetTable struct {
	Name string
}

func (t *TargetTable) decode(reader *bufio.Reader) error {
	var nameLen uint16
	if err := binary.Read(reader, binary.LittleEndian, &nameLen); err != nil {
		return err
	}

	name := make([]byte, nameLen)
	if err := binary.Read(reader, binary.LittleEndian, &name); err != nil {
		return err
	}
	t.Name = string(name)

	return nil
}

func DecodeTable(r io.Reader) (*[]Table, error) {
	bufReader := bufio.NewReader(r)

	var tables []Table

	for {
		var length uint32
		if err := binary.Read(bufReader, binary.LittleEndian, &length); err == io.EOF {
			break
		}

		table, err := readTable(bufReader, length)
		if err == io.EOF {
			return nil, fmt.Errorf("expected table with length %v", length)
		}
		if err != nil {
			return nil, err
		}

		tables = append(tables, *table)
	}

	return &tables, nil
}

func readTableHeader(r *bufio.Reader) (uint8, error) {
	_, err := r.Discard(1)
	if err != nil {
		return 0, err
	}
	srv, err := r.ReadByte()
	if err != nil {
		return 0, err
	}

	return srv, nil
}

func readTable(r *bufio.Reader, length uint32) (*Table, error) {
	srv, err := readTableHeader(r)
	if err != nil {
		return nil, err
	}
	if srv != TypeRecordTemplate {
		return nil, fmt.Errorf("failed to read record template")
	}

	// RecordTemplate always precedes the Records
	rc, err := readRecordTemplate(r)
	if err != nil {
		return nil, err
	}

	var records []Record

	for i := uint32(0); i < length; i++ {
		srv, err := readTableHeader(r)
		if err != nil {
			return nil, err
		}
		if srv != TypeRecord {
			return nil, fmt.Errorf("unknown value type %v", srv)
		}

		record, err := readRecord(r, rc)
		if err != nil {
			return nil, err
		}

		records = append(records, record)
	}

	return &Table{
		Name:    rc.Table,
		Records: records,
	}, nil
}

func readRecordTemplate(r *bufio.Reader) (*RecordTemplate, error) {
	var size uint16
	if err := binary.Read(r, binary.LittleEndian, &size); err != nil {
		return nil, err
	}

	var (
		target TargetTable
		fields []RecordField
	)

	for {
		var field RecordField
		if err := field.decode(r); err != nil {
			return nil, err
		}

		// This field is assumed to always be present
		if field.Name == "_TargetTable" {
			if err := target.decode(r); err != nil {
				return nil, err
			}

			break
		}

		fields = append(fields, field)
	}

	return &RecordTemplate{
		Size:   size,
		Fields: fields,
		Table:  target.Name,
	}, nil
}

func readRecord(r *bufio.Reader, rc *RecordTemplate) (Record, error) {
	var size uint16
	if err := binary.Read(r, binary.LittleEndian, &size); err != nil {
		return nil, err
	}

	record := make(Record)

	for _, field := range rc.Fields {
		var err error

		switch field.Type {
		case GID:
			var v uint64
			err = binary.Read(r, binary.LittleEndian, &v)
			record[field.Name] = v
		case INT:
			var v uint32
			err = binary.Read(r, binary.LittleEndian, &v)
			record[field.Name] = v
		case UINT:
			var v uint32
			err = binary.Read(r, binary.LittleEndian, &v)
			record[field.Name] = v
		case FLT:
			var v uint32
			err = binary.Read(r, binary.LittleEndian, &v)
			record[field.Name] = v
		case BYT:
			var v uint8
			err = binary.Read(r, binary.LittleEndian, &v)
			record[field.Name] = v
		case UBYT:
			var v uint8
			err = binary.Read(r, binary.LittleEndian, &v)
			record[field.Name] = v
		case USHRT:
			var v uint16
			err = binary.Read(r, binary.LittleEndian, &v)
			record[field.Name] = v
		case DBL:
			var v uint64
			err = binary.Read(r, binary.LittleEndian, &v)
			record[field.Name] = v
		case STR:
			fallthrough
		case WSTR:
			var len uint16
			if err := binary.Read(r, binary.LittleEndian, &len); err != nil {
				return nil, err
			}

			v := make([]byte, len)
			_, err = io.ReadFull(r, v)

			record[field.Name] = string(v)
		default:
			panic("unknown field type")
		}

		if err != nil {
			return nil, err
		}
	}

	return record, nil
}
