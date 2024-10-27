package login

import (
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"golang.org/x/crypto/twofish"
)

func generateIV() []byte {
	const ivConstant = 0xB6

	// NOTE: Only 16 bytes of the IV are used
	iv := make([]byte, 32)
	for i := 0; i < len(iv); i++ {
		iv[i] = ivConstant - byte(i)
	}

	return iv
}

func generateKey(sid uint16, timeSecs uint32, timeMillis uint32) []byte {
	const keyConstant = 0x17

	key := make([]byte, 32)
	for i := 0; i < len(key); i++ {
		key[i] = keyConstant + byte(i)
	}

	le := make([]byte, 4)

	binary.LittleEndian.PutUint16(le, sid)
	key[4] = le[0]
	key[5] = le[2] // This is always zero
	key[6] = le[1]

	binary.LittleEndian.PutUint32(le, timeSecs)
	key[8] = le[0]
	key[9] = le[2]
	key[12] = le[1]
	key[13] = le[3]

	binary.LittleEndian.PutUint32(le, timeMillis)
	key[14] = le[0]
	key[15] = le[1]

	return key
}

// AuthenToken returns a token to be encrypted by the client in the authentication stage
func AuthenToken(username string, ck1 string, sid uint16) []byte {
	return []byte(fmt.Sprintf("%v %v %v", sid, username, ck1))
}

func xorRec1(buf []byte, sid uint16, timeSecs uint32, timeMillis uint32) []byte {
	key := generateKey(sid, timeSecs, timeMillis)
	iv := generateIV()[:16]

	block, err := twofish.NewCipher(key)
	if err != nil {
		panic(err)
	}

	xor := make([]byte, len(buf))

	stream := cipher.NewOFB(block, iv)
	stream.XORKeyStream(xor, buf)

	return xor
}

// EncryptRec1 encrypts a given plaintext with a key derived from the client/server mutally agreed parameters
func EncryptRec1(plaintext []byte, sid uint16, timeSecs uint32, timeMillis uint32) []byte {
	return xorRec1(plaintext, sid, timeSecs, timeMillis)
}

// DecryptRec1 decrypts a given ciphertext with a key derived from the client/server mutally agreed parameters
func DecryptRec1(rec1 []byte, sid uint16, timeSecs uint32, timeMillis uint32) []byte {
	return xorRec1(rec1, sid, timeSecs, timeMillis)
}
