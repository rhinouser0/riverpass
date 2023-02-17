// ///////////////////////////////////////
// 2022 PJLab Storage all rights reserved
// ///////////////////////////////////////
package util

import "regexp"

func GenerateTriId() string {
	return ShordGuidGenerator()
}

// Token started with triplet id. Returning them to client so user can use
// token to identify which triplet to read the blob from.
func GenerateBlobToken(tpltId string, blbId string) string {
	return "tr_" + tpltId + "_bb_" + blbId
}

func GetTripletIdFromToken(token string) string {
	reToken := regexp.MustCompile(`tr_(?P<id>.+)_bb_`)
	grab := reToken.FindStringSubmatch(token)
	return grab[1]
}

func GetBlobIdFromToken(token string) string {
	reToken := regexp.MustCompile(`_bb_(?P<id>.+)`)
	grab := reToken.FindStringSubmatch(token)
	return grab[1]
}

func Full2PartialToken(fullToken string) string {
	return "tr__bb_" + GetBlobIdFromToken(fullToken)
}
