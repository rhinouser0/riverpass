// //////////////////////////////
// 2022 SHLab all rights reserved
// //////////////////////////////
package definition

// variable definition

// //////////////////////////////////////
// holder
var DataPosition string
var BlobLocalPathPrefix string

// holder end
////////////////////////////////////////

// //////////////////////////////////////
// common
// consts from server_config.xml via config.go
var ROOT_ID string

// F_ prefixed consts are flags
var F_file_db_shard_num uint32
var F_file_cache_shard_num uint32

var F_batch_size uint32

// Used for setting up multi writers
var F_headers_per_index uint32
var F_4K_Align bool
var F_CACHE_MAX_SIZE int64
var Oss_dbNum uint32

// Closing the open triplet every 200MiB.
var K_triplet_closing_threshold int64

// Large object
var K_triplet_large_threshold int64

// common end
////////////////////////////////////////
