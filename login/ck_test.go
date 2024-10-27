package login

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGenerateCK1(t *testing.T) {
	ck1 := GenerateCK1("1", 3258, 1617815695, 805)

	expected := "+FO9W7DLYNuvLdwvnMaxtJrSD+/h7HHfpzSNKv6G4UomKKoy+uwknGbqrtz4KNHSIS6McowtSTXtQBwwq7bwSQ=="
	assert.Equal(t, expected, ck1)
}

func TestGenerateCK3(t *testing.T) {
	ck2 := "cZT3fu6MlQ7SBZWYYLvaq8ebpp51SwHuJWE+ubSn8+ddTIkb5Q6AEyZgfeWItMZLE68gF5CSkU3s+ayeDowj8w=="
	ck3 := GenerateCK3(ck2, 2996, 1620500010, 834)

	expected := "ntaVuE1BT+8UZlrRAEHwVsYE0LVSYnduw0DCplF4ra2PATs+p1Bta/33QpDjJ5w1L7ROANmgF0m7FMtQncdthg=="
	assert.Equal(t, expected, ck3)
}
