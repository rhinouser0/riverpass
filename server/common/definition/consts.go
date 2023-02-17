package definition

const K_KiB = 1024
const K_MiB = 1024 * K_KiB
const K_GiB = 1024 * K_MiB
const K_TiB = 1024 * K_GiB

// K_ prefixed consts are constants
const K_rng_code_dlmtr = "|"

// entries per index header
const F_num_entries_per_header = 10000
const F_size_blobs_per_header = 1 * K_GiB

const F_GRPC_CONN_TIMEOUT_SEC = 600

const F_DB_STATE_PENDING = 0
const F_DB_STATE_READY = 1
const F_DB_STATE_DELETED = 2

const F_BLOB_STATE_PENDING = 0
const F_BLOB_STATE_READY = 1

const F_DB_STATE_INT32_PENDING int32 = 0
const F_DB_STATE_INT32_READY int32 = 1
const F_DB_STATE_INT32_DELETED int32 = 2

const F_NUM_MAX_FILES_DB_CONN = 250
const F_NUM_DB_CONN_OBJ = 10

const MAX_UPLOADER_DATA_SIZE int32 = int32(0x7FFFFFFF)

const MAX_GOROUTINE_IN_FILE_UPLOADER = 100

const DIRECT_DELIVERY = "DirectDelivery"

// 4K Align
const F_BLOBID_SIZE = 128
const F_CHECKSUM_SIZE = 32
const F_CONTENT_SIZE = 4*K_KiB - F_BLOBID_SIZE - 8 - F_CHECKSUM_SIZE

const K_LARGE_OBJECT_PREFIX = "lobj"

// TODO(csun): For cache, uncategorized.
const K_PENDDING_FID_PREFIX = "PD_"
const F_num_chars_pending_file_id = 4
const F_num_batch_write = 5
