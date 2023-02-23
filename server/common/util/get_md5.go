// ///////////////////////////////////////
// 2022 SHAI Lab all rights reserved
// ///////////////////////////////////////
package util

import (
	"crypto/md5"
	"encoding/hex"
)

func GetStrMd5(str string) string {
	tmp := []byte(str)
	md5h := md5.New()
	md5h.Write(tmp)
	return hex.EncodeToString(md5h.Sum(nil))
}
