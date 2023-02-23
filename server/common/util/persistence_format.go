// ///////////////////////////////////////
// 2022 SHAI Lab all rights reserved
// ///////////////////////////////////////
package util

import "github.com/common/definition"

func GetPayloadSize(dataLen int) int64 {
	var payloadSize int64
	if definition.F_4K_Align {
		nums := (dataLen + definition.F_CONTENT_SIZE - 1) / definition.F_CONTENT_SIZE
		payloadSize = 4 * definition.K_KiB * int64(nums)
	} else {
		payloadSize = int64(definition.F_BLOBID_SIZE+8) + int64(dataLen)
	}
	return payloadSize
}
