package dml

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeTable1(t *testing.T) {
	file, err := os.Open("testdata/dml1.bin")
	require.NoError(t, err)

	tables, err := DecodeTable(file)
	require.NoError(t, err)

	first := (*tables)[0]

	assert.Equal(t, "_TableList", first.Name)
	assert.Equal(t, 1, len(first.Records))
	assert.Equal(t, "Test", first.Records[0]["Name"])
}

func TestDecodeTable2(t *testing.T) {
	file, err := os.Open("testdata/dml2.bin")
	require.NoError(t, err)

	tables, err := DecodeTable(file)
	require.NoError(t, err)

	first := (*tables)[0]

	assert.Equal(t, "_Shared-WorldData", first.Name)
	assert.Equal(t, 1, len(first.Records))
	assert.Equal(t, uint32(2647210788), first.Records[0]["HeaderCRC"])
	assert.Equal(t, "Data/GameData/_Shared-WorldData.wad", first.Records[0]["SrcFileName"])
}
