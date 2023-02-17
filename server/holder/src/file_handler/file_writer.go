/////////////////////////////////////////
// 2022 PJLab Storage all rights reserved
/////////////////////////////////////////

package file_handler

import (
	blobs "holder/src/blob_handler"
	dbops "holder/src/db_ops"
	"log"

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
		log.Printf(
			"[ERROR] writer: Create blob entry(%s) in DB failed for fid(%s)",
			partialToken, fid)
		return err
	}

	// TODO: Implement blacklist gc.
	fullToken, err := fu.Pbh.Put(blobId, data)
	if err != nil {
		log.Printf("[ERROR] writer: Put data(offset: %d) failed for fid(%s).", offset, fid)
		return err
	} else {
		log.Printf(
			"[INFO] writer: Put data(offset: %d) succeeded for fid(%s), token: %s",
			offset, fid, fullToken)
	}

	err = fu.BlobSegDb.CommitBlobInDB(
		[]int32{offset, offset + size}, fid, fullToken)
	if err != nil {
		log.Printf(
			"[ERROR] writer: Commit blob(token: %s) failed for file(%s).",
			fullToken, fid)
		return err
	}

	log.Printf(
		"[INFO] writer: Successfully put blob(token: %s) to file(%s)",
		fullToken, fid)
	return nil
}

func (fu *FileWriter) WriteFileToCache(fid string, data []byte) (string, error) {
	fu.checkUploader()

	blobId := util.ShordGuidGenerator()
	// TODO: Implement blacklist gc.
	fullToken, err := fu.Pbh.Put(blobId, data)
	if err != nil {
		log.Printf("[ERROR] writer: Put data failed for fid(%s).", fid)
		return "", err
	}
	log.Printf("[INFO] writer: Put data succeeded for fid(%s), token: %s", fid, fullToken)
	log.Printf("[INFO] writer: Successfully put blob(token: %s) to file(%s)", fullToken, fid)
	return fullToken, nil
}

func (fu *FileWriter) Close(fid string) error {
	err := fu.FileDb.CommitFileInDB(fid)
	if err != nil {
		log.Printf("[ERROR] writer: Seal file(%s) failed.", fid)
		return err
	}
	return nil
}

func (fu *FileWriter) checkUploader() {
	if fu.Pbh == nil {
		log.Fatal("writer: FileWriter init not finished: Pbh")
	}
	if fu.BlobSegDb == nil {
		if !dbops.IsOss {
			log.Fatal("writer: FileWriter init not finished: BlobSegDb")
		}
	}
	if fu.FileDb == nil {
		log.Fatal("writer: FileWriter init not finished: FileDb")
	}
}
