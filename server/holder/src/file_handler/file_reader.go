// //////////////////////////////
// 2022 SHLab all rights reserved
// //////////////////////////////

package file_handler

import (
	"container/list"
	"errors"
	blobs "holder/src/blob_handler"
	dbops "holder/src/db_ops"
	"math"
	"sync"

	range_code "github.com/common/range_code"
	. "github.com/common/zaplog"
	"go.uber.org/zap"
)

type FileReader struct {
	// Reference to a initialized physical blob holder
	Pbh       *blobs.PhyBH
	BlobSegDb *dbops.DBOpsBlobSeg
	FileDb    *dbops.DBOpsFile
}

func (fr *FileReader) ReadAt(
	fid string, offset int32, size int32) (data []byte, err error) {
	// Shall be already ordered.
	bms, err := fr.BlobSegDb.ListBlobSegsByFidFromDB(fid)
	if err != nil {
		return []byte{}, err
	}
	allBytes := make([]byte, size)
	var tmpErr error
	wg := &sync.WaitGroup{}
	for i := 0; i < len(*bms); i++ {
		if (*bms)[i].RngCode.End < offset {
			continue
		}
		bm := (*bms)[i]
		// Ending criteria
		if bm.RngCode.Start >= offset+size {
			break
		}
		var curBlobData []byte
		curStart := int32(math.Max(float64(bm.RngCode.Start), float64(offset)))
		curEnd := int32(math.Min(float64(bm.RngCode.End), float64(offset+size)))
		wg.Add(1)
		go func(token string, start int32, end int32,
			curStart int32, curEnd int32, offset int32) {
			defer wg.Done()
			curBlobData, err = fr.readPiece(
				token,
				start, end)
			if err != nil {
				tmpErr = err
				return
			} else {
				copy(allBytes[(curStart-offset):(curEnd-offset)], curBlobData)
			}
		}(bm.RngCode.Token, curStart-bm.RngCode.Start, curEnd-bm.RngCode.Start, curStart, curEnd, offset)
	}
	wg.Wait()
	if tmpErr != nil {
		ZapLogger.Error("", zap.Error(tmpErr))
		return nil, tmpErr
	}
	return allBytes, nil
}

func (fr *FileReader) ReadFromCache(
	fid string, offset int32, size int32, rngCodeList *list.List) (data []byte, err error) {
	// Shall be already ordered.
	start := rngCodeList.Front().Value.(range_code.RangeCode).Start
	end := rngCodeList.Front().Value.(range_code.RangeCode).End
	token := rngCodeList.Front().Value.(range_code.RangeCode).Token
	allBytes := make([]byte, size)
	if end < offset || start > offset || offset+size > end {
		ZapLogger.Warn("index out of range", zap.Any("fid", fid),
			zap.Any("start", start), zap.Any("end", end),
			zap.Any("offset", offset), zap.Any("size", size))
	}
	var curBlobData []byte
	curStart := int32(math.Max(float64(start), float64(offset)))
	curEnd := int32(math.Min(float64(end), float64(offset+size)))
	curBlobData, err = fr.readPiece(token, curStart-start, curEnd-start)
	if err != nil {
		ZapLogger.Error("readPiece", zap.Any("token", token), zap.Any("err", err))
		return nil, err
	} else {
		copy(allBytes[(curStart-offset):(curEnd-offset)], curBlobData)
	}
	return allBytes, nil
}

func (fr *FileReader) readPiece(
	token string, start int32, end int32) (piece []byte, err error) {
	data, err := fr.Pbh.Get(token)
	if err != nil {
		return nil, err
	}
	dataLen := len(data)
	if start >= int32(dataLen) || end >= int32(dataLen) {
		ZapLogger.Error("index out of range", zap.Any("token", token),
			zap.Any("start", start), zap.Any("end", end),
			zap.Any("dataLen", dataLen))
		return nil, errors.New("index out of range")
	}
	return data[start:end], nil
}
