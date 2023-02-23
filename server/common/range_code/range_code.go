// //////////////////////////////
// 2022 SHLab all rights reserved
// //////////////////////////////
package range_code

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
)

// Range Code is for checking if there is overlap between 2 ranges. It is
// not well implemented yet. The impl here is temporary solution. One
// naive approach is to devide fullSize by 4k and do bitwise comparison.
// Note that currently all blob ranges of a same file are exclusive.
// Maybe we can use bloom filter.
// TODO: implement a Range hash instead of simple range struct..
type RangeCode struct {
	Start int32
	End   int32
	Token string
}

func (rc RangeCode) ToJson() string {
	var hashStr []byte
	hashStr, jsErr := json.Marshal(&rc)
	if jsErr != nil {
		log.Fatal(jsErr)
	}
	// log.Printf("[DEBUG] RangeCode to json str: %s", hashStr)
	return string(hashStr)
}

func ToRangeCode(jsStr string) RangeCode {
	// log.Printf("[DEBUG] json str %s to RangeCode", jsStr)
	var rc RangeCode
	jsErr := json.Unmarshal([]byte(jsStr), &rc)
	if jsErr != nil {
		log.Fatal(jsErr)
	}
	return rc
}

// TODO: Currently using offset of ranger. Need to verify the idx order in DB
func (rc RangeCode) ToDbEntry() string {
	numOfZeros := 9 - countDigits(int(rc.Start))
	leadingZeros := ""
	for i := 0; i < numOfZeros; i++ {
		leadingZeros += "0"
	}
	rg := leadingZeros + strconv.Itoa(int(rc.Start))
	hash := fmt.Sprintf("rg(%s)_tk(%s)", rg, rc.Token)
	return hash
}

func countDigits(num int) int {
	if num == 0 {
		return 1
	}
	ret := 0
	for num > 0 {
		num /= 10
		ret += 1
	}
	return ret
}
