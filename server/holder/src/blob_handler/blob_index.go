/////////////////////////////////////////
// 2022 SHAI Lab all rights reserved
/////////////////////////////////////////

package blob_handler

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"unsafe"

	"github.com/common/definition"
)

// All state must add their value upon base to get actual state value.
// The value prints ASCII code in file, and so as how it used in memory
// (we don't translate to char value to use in memory)
const K_state_base_ascii = 48
const K_index_header_open = 2
const K_index_header_closed = 3
const K_index_header_large = 4
const K_index_entry_len = 252

// Analogy: row in a list.
type IndexEntry struct {
	// maxlen = 128
	BlobId string
	//maxlen = 22
	Offset int64
	//maxlen = 22
	Size     int64
	Checksum string
	Fid      string
	Padding  string
	// TODO: use this ptr to obtain the chain of blobs that's correlated.
	// In this sense, IndexHeader composites a keylist data structure that
	// holds reference to blob positions on disk.
	// keyPrev  *IndexEntry
}

type IndexBaseInfo struct {
	State uint8
}

// Analogy: inventory list.
type IndexHeader struct {
	// use `new(T)` to initial:
	RWLock *sync.RWMutex

	Info      IndexBaseInfo
	ShardId   int
	TripletId string

	// Local fs name
	LocalName string
	// URL of position in object storage.
	RemoteName string

	// List of index entries
	Entries *list.List
	// A map holds reference to the certain blob.
	RefMap map[string]*IndexEntry

	Empty bool
}

func (ie *IndexEntry) Serialize() []byte {
	jsonbyte, err := json.Marshal(ie)
	if err != nil {
		panic(err)
	}
	return jsonbyte
}

// TODO: use index to wrap indexHeader, 1 index can contain
// multiple indexHeaders, use this way to improve the parallel
// write bandwidth.
// type Index struct {
// 	ShardId    int
// 	headers []IndexHeader
// }

// shardId is the holder instance id.
func (ih *IndexHeader) New(shardId int, triId string, isLarge bool) int64 {
	ih.RWLock = new(sync.RWMutex)

	ih.ShardId = shardId
	ih.TripletId = triId
	ih.Entries = list.New()
	ih.RefMap = make(map[string]*IndexEntry)

	ih.Empty = true
	localfsPrefix := definition.BlobLocalPathPrefix
	if localfsPrefix == "" {
		localfsPrefix = "/tmp/localfs/"
	}
	ih.LocalName = fmt.Sprintf("%s/idx_h_%d_%s.dat", localfsPrefix, shardId, triId)
	info, err := os.Stat(ih.LocalName)
	// TODO: stat error not necessary means file doesn't exist
	if err != nil {
		if isLarge {
			state := K_index_header_large + K_state_base_ascii
			return ih.create(uint8(state))
		} else {
			state := K_index_header_open + K_state_base_ascii
			return ih.create(uint8(state))
		}
	}
	ih.load(info.Size())
	if len(ih.RefMap) > 0 {
		ih.Empty = false
	}
	return info.Size()
}

// created with open state
func (ih *IndexHeader) create(state uint8) int64 {
	log.Printf("[INFO] Index file(%s) doesn't exist, creating a new one.\n",
		ih.LocalName)
	f, err := os.Create(ih.LocalName)
	Check(err)
	// Prepare encoded state bytes.
	ih.Info.State = state
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, &ih.Info)

	// Seek to offset 0 and flush state struct.
	f.Seek(0, 0)
	byteSize, err := f.Write(buf.Bytes())
	Check(err)

	otherSize, err := f.WriteAt([]byte("\n[\n]"), int64(len(buf.Bytes())))
	Check(err)
	f.Close()

	// Better offload state to file before set in memory.
	ih.Info = IndexBaseInfo{
		State: K_index_header_open + K_state_base_ascii,
	}
	return int64(byteSize) + int64(otherSize)
}

// Hydrate IndexHeader by loading from local file
func (ih *IndexHeader) load(size int64) {
	log.Printf(
		"[INFO] >>>> Index file(%s) already exists, size(%d bytes), loading state"+
			" and blob indices from it.\n",
		ih.LocalName, size)
	ih.RWLock.Lock()
	defer ih.RWLock.Unlock()

	log.Printf("size of ih.info: %d", unsafe.Sizeof(ih.Info))
	idxBaseInfoLen := int64(unsafe.Sizeof(ih.Info))
	stateBytes := make([]byte, idxBaseInfoLen)

	// File already checked before Load() call.
	f, _ := os.Open(ih.LocalName)
	f.Read(stateBytes)

	buf := bytes.NewReader(stateBytes)
	err := binary.Read(buf, binary.LittleEndian, &ih.Info)
	Check(err)

	// 1st byte state byte, 2nd byte '\n', starting from '['
	// Second parameter is used for setting path mode
	f.Seek(int64(idxBaseInfoLen)+1, 0)
	// Exclude the leading 2 bytes.
	logBytes := make([]byte, size-idxBaseInfoLen-1)
	// TODO: consider doing for loop iterative read.
	_, err = f.Read(logBytes)
	Check(err)

	var ies []IndexEntry
	err = json.Unmarshal([]byte(logBytes), &ies)
	Check(err)

	if ih.Entries.Len() != 0 || len(ih.RefMap) != 0 {
		err = errors.New("loading loaded IndexHeader")
		panic(err)
	}
	for i := 0; i < len(ies); i++ {
		// TODO: Use priority list sorted by offset instead.
		// Append to list
		ih.Entries.PushBack(ies[i])
		// Store in map for lookup
		ih.RefMap[ies[i].BlobId] = &ies[i]
	}

	log.Printf(
		"[INFO] <<<< Loaded index file %s, state(%d), entry num(%d).",
		ih.LocalName, ih.Info.State, len(ih.RefMap))
}

// TODO: Add fid as backward reference to the file it belongs.
func (ih *IndexHeader) Put(blobId string, offset int64, size int64) (int64, error) {
	ih.RWLock.Lock()
	defer ih.RWLock.Unlock()

	if ih.Info.State == K_index_header_closed+K_state_base_ascii {
		return 0, errors.New("index header already closed")
	}

	ie := IndexEntry{
		BlobId:   blobId,
		Offset:   offset,
		Size:     size,
		Checksum: "",
		Fid:      "",
	}

	// If we store in memory first, reader could try to read binary with RLock
	// and fail with no blob.
	idxBytes, err := ih.flush(ie)
	if err != nil {
		return 0, err
	}
	// Append to list
	ih.Entries.PushBack(ie)
	// Store in map for lookup
	ih.RefMap[blobId] = &ie

	log.Printf("[INFO] Index File: Put blob entry(%s) succeeded, offset(%d), "+
		"sizeWritten(%d)", blobId, offset, idxBytes)
	return idxBytes, nil
}

// We return the blob slices entries referenced by blobId
func (ih *IndexHeader) Get(blobId string) *IndexEntry {
	ih.RWLock.RLock()
	defer ih.RWLock.RUnlock()

	pEntry, exist := ih.RefMap[blobId]

	if exist {
		log.Printf("[INFO] Index File: Get blob entry(%s) succeeded,"+
			" entry(%v)", blobId, *pEntry)
		return pEntry
	}
	return nil
}

func (ih *IndexHeader) Delete(blobId string) (err error) {
	ih.RWLock.Lock()
	defer ih.RWLock.Unlock()

	_, exist := ih.RefMap[blobId]

	if exist {
		delete(ih.RefMap, blobId)
		return nil
	}

	return errors.New("index entry already deleted")
}

func (ih *IndexHeader) flush(entry IndexEntry) (int64, error) {
	f, err := os.OpenFile(ih.LocalName, os.O_WRONLY, 0755)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	f.Seek(-1, 2)

	leading := make([]byte, 0)
	if !ih.Empty {
		leading = []byte(",\n")
	}
	serializeBytes := entry.Serialize()
	paddingLen := K_index_entry_len - len(serializeBytes) - 2
	var builder strings.Builder
	for i := 0; i < paddingLen; i++ {
		builder.WriteString("#")
	}
	entry.Padding = builder.String()
	bytes := append(append(leading, entry.Serialize()...), ']')
	// Persist
	res, err := f.Write(bytes)
	if err != nil {
		return 0, err
	}
	if !ih.Empty {
		if int64(res)-1 != K_index_entry_len {
			log.Fatalf("[INDEX FLUSH ERROR] : datalen %v is not equal to size %v\n", int64(res)-1, K_index_entry_len)
		}
	} else {
		if int64(res)-1 != K_index_entry_len-2 {
			log.Fatalf("[INDEX FLUSH ERROR] : datalen %v is not equal to size %v\n", int64(res)-1, K_index_entry_len-2)
		}
	}
	ih.Empty = false
	return int64(res) - 1, nil
}

// Idempotent. Close the index list and the local file, manager will open
// new headers for writes.
func (ih IndexHeader) Close() {
	ih.RWLock.Lock()
	defer ih.RWLock.Unlock()
	if ih.Info.State == K_index_header_closed+K_state_base_ascii {
		log.Printf("File(%s) already closed.", ih.LocalName)
		return
	}

	// File already checked before Load() call.
	f, err := os.OpenFile(ih.LocalName, os.O_WRONLY, 0755)
	Check(err)
	ih.Info.State = K_index_header_closed + K_state_base_ascii
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, &ih.Info)

	// Go to beginning and write state
	_, err = f.WriteAt(buf.Bytes(), 0)
	Check(err)

	log.Printf("[INFO] Closing the %s", ih.LocalName)
	f.Close()
}

func Check(err error) {
	if err != nil {
		panic(err)
	}
}
