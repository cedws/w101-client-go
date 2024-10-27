package login

import (
	"encoding/base64"
	"encoding/hex"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGenerateIV(t *testing.T) {
	iv := generateIV()[:16]
	enc := hex.EncodeToString(iv)

	expected := "b6b5b4b3b2b1b0afaeadacabaaa9a8a7"
	assert.Equal(t, expected, enc)
}

func TestGenerateKey(t *testing.T) {
	key := generateKey(3258, 1617815695, 805)
	enc := hex.EncodeToString(key)

	expected := "1718191aba000c1e8f6d2122e86025032728292a2b2c2d2e2f30313233343536"
	assert.Equal(t, expected, enc)
}

func TestEncryptRec1(t *testing.T) {
	ck1 := "+FO9W7DLYNuvLdwvnMaxtJrSD+/h7HHfpzSNKv6G4UomKKoy+uwknGbqrtz4KNHSIS6McowtSTXtQBwwq7bwSQ=="

	token := AuthenToken("1", ck1, 3258)
	rec1 := EncryptRec1(token, 3258, 1617815695, 805)

	enc := base64.StdEncoding.EncodeToString(rec1)
	expected := "VLZpUqHY04cULJ+dvYknBM2Y3xynINN3gB4svovYA0jzWUsVAXjdtz363K9pC049fhpK9zFjlGaC6awzXmUCeKMseu7+Bol3JiFmN46MAv6fOQ7pNvD6RFlpzzjZ8rQ="
	assert.Equal(t, expected, enc)
}

func TestDecryptRec1(t *testing.T) {
	rec1, _ := base64.StdEncoding.DecodeString("VLZpUqHY04cULJ+dvYknBM2Y3xynINN3gB4svovYA0jzWUsVAXjdtz363K9pC049fhpK9zFjlGaC6awzXmUCeKMseu7+Bol3JiFmN46MAv6fOQ7pNvD6RFlpzzjZ8rQ=")
	dec := string(DecryptRec1(rec1, 3258, 1617815695, 805))

	expected := "3258 1 +FO9W7DLYNuvLdwvnMaxtJrSD+/h7HHfpzSNKv6G4UomKKoy+uwknGbqrtz4KNHSIS6McowtSTXtQBwwq7bwSQ=="
	assert.Equal(t, expected, dec)
}
