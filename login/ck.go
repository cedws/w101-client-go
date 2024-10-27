package login

import (
	"crypto/sha512"
	"encoding/base64"
	"fmt"
)

// GenerateCK1 returns a ClientKey1 string derived from the inputs
func GenerateCK1(password string, sid uint16, timeSecs uint32, timeMillis uint32) string {
	s := salt(sid, timeSecs, timeMillis)

	pHash := hashPassword(password)
	sHash := secondaryEncrypt(pHash, s)

	return sHash
}

// GenerateCK3 returns a ClientKey3 string derived from the inputs
func GenerateCK3(password string, sid uint16, timeSecs uint32, timeMillis uint32) string {
	s := salt(sid, timeSecs, timeMillis)

	return secondaryEncrypt(password, s)
}

func salt(sid uint16, timeSecs uint32, timeMillis uint32) string {
	return fmt.Sprintf("%v%v%v", sid, timeSecs, timeMillis)
}

// secondaryEncrypt is obviously not encryption, this is just what Kingsisle call it
func secondaryEncrypt(password string, salt string) string {
	h := sha512.New()
	h.Write([]byte(password))
	h.Write([]byte(salt))
	sum := h.Sum(nil)

	return base64.StdEncoding.EncodeToString(sum)
}

func hashPassword(password string) string {
	h := sha512.New()
	h.Write([]byte(password))
	sum := h.Sum(nil)

	return base64.StdEncoding.EncodeToString(sum)
}
