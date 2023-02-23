// ///////////////////////////////////////
// 2022 SHAI Lab all rights reserved
// ///////////////////////////////////////
package definition

import (
	"container/list"

	range_code "github.com/common/range_code"
)

// A special segment that represents the file. You can imagine it as an
// inode in file system. It's indexed and sharded by file id.
type FileMeta struct {
	// eg: /f1
	Name string
	// a unique ID
	Id string
	// File's owner can be parent folder, or tags.
	// A file shall only have 1 parent folder,
	// but can have many tags pointing to it.
	OwnerList *list.List
	// This is the reference to blobs meta. Blob is a heavy binary sitting
	// on distributed data storage.
	BlobId string
	// A list of RangeCode
	RngCodeList *list.List
}

// Token can be used to access blob in triplet, or blob in cloud
// 1. For a blob in triplet, Token is the token returned by triplet
// 2. For a blob at cloud(OSS, COS), Token is the uri of the object
type BlobMeta struct {
	// The DB idx sorting order is in according with the materialized
	// range order.
	RngCode range_code.RangeCode
	// Owner can be a file, a folder, etc.
	OwnerId string
}
