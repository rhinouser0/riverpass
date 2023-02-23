// ///////////////////////////////////////
// 2022 SHAI Lab all rights reserved
// ///////////////////////////////////////

package cache_ops

import (
	"log"

	"github.com/common/definition"
	"github.com/common/util"
)

func NormalFidToPending(fid string) string {
	randId := util.RandIdGenerator(definition.F_num_chars_pending_file_id)
	retId := definition.K_PENDDING_FID_PREFIX + randId + fid
	log.Println("[DEBUG] pendingFid ", retId)
	return retId
}

func PendingToNormalFid(pFid string) string {
	leadingChars := len(definition.K_PENDDING_FID_PREFIX) + definition.F_num_chars_pending_file_id
	log.Println("[DEBUG] normalFid ", pFid[leadingChars:])
	return pFid[leadingChars:]
}
