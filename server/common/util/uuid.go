// ///////////////////////////////////////////////
// 2023 Shanghai AI Laboratory all rights reserved
// ///////////////////////////////////////////////
package util

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

const (
	Size = 16
)

// Mode 1:
// If this is true, means we allow duplicate file or directory names. This
// implies the mapped path destination ID shall be universally unique. We
// use UUID generator to generate unique IDs
// Mode 2:
// If this is false, means we won't allow duplicate file or directory names.
// In this case we can use a name generated ID in internal storage purpose.
// In real world, we use mode 1. Mode 2 is to bind name to a certain id or
// shard to facilitate debugging (internal path placement in metadata system
// remains the same)
const Allow_duplicate_names = false

//Use mode 2 (false) to creat seg and file in production environment, after adding the operation of list and judge.

var (
	ErrUUIDSize = errors.New("uuid size error")
)

// New generate a uuid.
func New() (str string, err error) {
	var (
		n    int
		uuid = make([]byte, Size)
	)
	if n, err = io.ReadFull(rand.Reader, uuid); err != nil {
		return
	}
	if n != Size {
		return "", ErrUUIDSize
	}
	uuid[8] = uuid[8]&^0xc0 | 0x80
	uuid[6] = uuid[6]&^0xf0 | 0x40
	str = fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])
	return
}

// Generate a unique randomized ID for use.
func ShordGuidGenerator() string {
	uuid, _ := New()
	// TODO: Risky simplification.
	// Only return 8 bytes for debugging readability.
	retStr := uuid[0:4] + uuid[27:31]
	return retStr
}

func GetHashedIdFromStr(str string) string {
	md5 := GetStrMd5(str)
	retStr := md5[0:8]
	return retStr
}

func GetInternalId(name string) string {
	var hashedId string
	if Allow_duplicate_names {
		hashedId = GetHashedIdFromStr(name)
	} else {
		// Universally unique.
		hashedId = ShordGuidGenerator()
	}
	return hashedId
}

// Generate a unique randomized ID with given size of characters.
func RandIdGenerator(numChars int) string {
	uuid, _ := New()
	leadingLen := numChars / 2
	endingLen := numChars - numChars/2
	retStr := uuid[0:leadingLen] + uuid[(31-endingLen):31]
	return retStr
}
