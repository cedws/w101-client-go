package wad

import (
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"strings"
)

var ErrMissingMagic = errors.New("missing WAD magic bytes")

const magic = "KIWAD"

type Archive struct {
	file    *os.File
	header  header
	entries []Entry
}

type header struct {
	Version uint32
	Count   uint32
}

func (h *header) UnmarshalBinary(data []byte) error {
	h.Version = binary.LittleEndian.Uint32(data[0:4])
	h.Count = binary.LittleEndian.Uint32(data[4:8])
	return nil
}

type Entry struct {
	Offset     uint32
	Size       uint32
	CompSize   uint32
	Compressed bool
	Checksum   uint32
	Path       string
}

func readEntry(r io.Reader) (Entry, error) {
	entry := Entry{}

	if err := binary.Read(r, binary.LittleEndian, &entry.Offset); err != nil {
		return entry, err
	}
	if err := binary.Read(r, binary.LittleEndian, &entry.Size); err != nil {
		return entry, err
	}
	if err := binary.Read(r, binary.LittleEndian, &entry.CompSize); err != nil {
		return entry, err
	}
	if err := binary.Read(r, binary.LittleEndian, &entry.Compressed); err != nil {
		return entry, err
	}
	if err := binary.Read(r, binary.LittleEndian, &entry.Checksum); err != nil {
		return entry, err
	}

	var pathLen uint32
	if err := binary.Read(r, binary.LittleEndian, &pathLen); err != nil {
		return entry, err
	}

	pathBuf := make([]byte, pathLen)
	if _, err := io.ReadFull(r, pathBuf); err != nil {
		return entry, err
	}

	entry.Path = strings.TrimRight(string(pathBuf), "\x00")
	return entry, nil
}

func readHeader(r io.Reader) (*header, error) {
	magicBuf := make([]byte, len(magic))
	if _, err := io.ReadFull(r, magicBuf); err != nil {
		return nil, err
	}
	if string(magicBuf) != magic {
		return nil, ErrMissingMagic
	}

	headerBuf := make([]byte, 8)
	if _, err := io.ReadFull(r, headerBuf); err != nil {
		return nil, err
	}

	var h header
	if err := h.UnmarshalBinary(headerBuf); err != nil {
		return nil, err
	}

	if h.Version >= 2 {
		// discard byte
		if _, err := r.Read(make([]byte, 1)); err != nil {
			return nil, err
		}
	}

	return &h, nil
}

func Open(path string) (*Archive, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	return OpenFile(file)
}

func OpenFile(file *os.File) (*Archive, error) {
	header, err := readHeader(file)
	if err != nil {
		return nil, fmt.Errorf("wad: error reading header: %w", err)
	}

	var entries []Entry

	for i := uint32(0); i < header.Count; i++ {
		entry, err := readEntry(file)
		if err != nil {
			return nil, fmt.Errorf("wad: error reading entry: %w", err)
		}
		entries = append(entries, entry)
	}

	archive := &Archive{
		file:    file,
		header:  *header,
		entries: entries,
	}

	return archive, nil
}

func (a *Archive) Close() error {
	return a.file.Close()
}

func (a *Archive) Entries() iter.Seq[Entry] {
	return func(yield func(Entry) bool) {
		for _, entry := range a.entries {
			if !yield(entry) {
				break
			}
		}
	}
}

// Entry returns a reader for the given entry. The caller may only read one entry at a time.
func (a *Archive) Entry(entry Entry) (io.Reader, error) {
	var (
		offset   = int64(entry.Offset)
		compSize = int64(entry.CompSize)
		size     = int64(entry.Size)
	)

	if entry.Compressed {
		section := io.NewSectionReader(a.file, offset, compSize)
		return zlib.NewReader(section)
	}

	return io.NewSectionReader(a.file, offset, size), nil
}
