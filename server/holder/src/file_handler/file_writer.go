// //////////////////////////////
// 2022 SHLab all rights reserved
// //////////////////////////////

package file_handler

import (
	blobs "holder/src/blob_handler"
	dbops "holder/src/db_ops"

	. "github.com/common/zaplog"
	"go.uber.org/zap"

	"github.com/common/util"
)

type FileWriter struct {
	// Reference to a initialized physical blob holder
	Pbh       *blobs.PhyBH
	BlobSegDb *dbops.DBOpsBlobSeg
	FileDb    *dbops.DBOpsFile
}

// Positional Write. Temporarily deprecated in this code base.
func (fu *FileWriter) WriteAt(fid string, offset int32, size int32, data []byte) error {
	fu.checkUploader()

	blobId := util.ShordGuidGenerator()
	// Partial Token is the full token without triplet id.
	partialToken := util.GenerateBlobToken("", blobId)

	err := fu.BlobSegDb.CreateBlobSegInDB(
		[]int32{offset, offset + size}, fid, partialToken)
	if err != nil {
		ZapLogger.Error("Create blob entry in DB failed",
			zap.Any("blob entry", partialToken),
			zap.Any("fid", fid))
		return err
	}

	// TODO: Implement blacklist gc.
	fullToken, err := fu.Pbh.Put(blobId, data)
	if err != nil {
		ZapLogger.Error("Put data failed", zap.Any("offset", offset), zap.Any("fid", fid))
		return err
	}

	err = fu.BlobSegDb.CommitBlobInDB(
		[]int32{offset, offset + size}, fid, fullToken)
	if err != nil {
		ZapLogger.Error("Commit blob failed", zap.Any("token", fullToken), zap.Any("fid", fid))
		return err
	}

	ZapLogger.Info("Put data succeeded", zap.Any("offset", offset),
		zap.Any("fid", fid),
		zap.Any("token", fullToken))
	return nil
}

func (fu *FileWriter) WriteFileToCache(fid string, data []byte) (string, error) {
	fu.checkUploader()

	blobId := util.ShordGuidGenerator()
	// TODO: Implement blacklist gc.
	fullToken, err := fu.Pbh.Put(blobId, data)
	if err != nil {
		ZapLogger.Error("Put data failed", zap.Any("fid", fid), zap.Any("err", err))
		return "", err
	}
	ZapLogger.Info("Put data succeeded", zap.Any("fid", fid), zap.Any("token", fullToken))
	return fullToken, nil
}

func (fu *FileWriter) Close(fid string) error {
	err := fu.FileDb.CommitFileInDB(fid)
	if err != nil {
		ZapLogger.Error("writer: Seal file failed", zap.Any("file", fid))
		return err
	}
	return nil
}

func (fu *FileWriter) checkUploader() {
	if fu.Pbh == nil {
		ZapLogger.Fatal("FileWriter init not finished: Pbh")
	}
	if fu.FileDb == nil {
		ZapLogger.Fatal("FileWriter init not finished: FileDb")
	}
}
