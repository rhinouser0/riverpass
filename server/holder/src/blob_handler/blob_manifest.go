// //////////////////////////////
// 2022 SHLab all rights reserved
// //////////////////////////////

package blob_handler

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/common/definition"
)

// Note that a manifest file never close, since it has deletion operation
const K_action_base_ascii = 65
const K_action_put = 15
const K_action_delete = 3
const K_mf_entry_len = 202

// Analogy: action.
type MFEntry struct {
	BlobId  string
	Action  uint8
	Padding string
}

// Analogy: log.
type MFHeader struct {
	// use `new(T)` to initial:
	RWLock *sync.RWMutex

	// The marking bit for flush formating
	Empty     bool
	ShardId   int
	TripletId string

	// Local fs name
	LocalName string
	// URL of position in object storage.
	RemoteName string

	// Currently only storing deletion log, for initialization.
	deletionLog map[string]uint8
}

// TODO: use index to wrap indexHeader, 1 index can contain
// multiple indexHeaders, use this way to improve the parallel
// write bandwidth.
// type Index struct {
// 	ShardId    int
// 	headers []IndexHeader
// }

// shardId is the holder instance id.
func (mfh *MFHeader) New(shardId int, triId string) int64 {
	mfh.RWLock = new(sync.RWMutex)

	mfh.Empty = true
	mfh.ShardId = shardId
	mfh.TripletId = triId
	// Maybe not need to be map. Leave for future purpose.
	mfh.deletionLog = make(map[string]uint8)
	fileName := fmt.Sprintf("mf_h_%d_%s.dat", shardId, triId)

	localfsPrefix := definition.BlobLocalPathPrefix
	if localfsPrefix == "" {
		localfsPrefix = "/var/lib/docker/.cache"
	}
	mfh.LocalName = fmt.Sprintf("%s/%s", localfsPrefix, fileName)

	info, err := os.Stat(mfh.LocalName)
	if os.IsNotExist(err) {
		return mfh.create()
	} else if err != nil {
		log.Fatalln("[MFHeader] NEW error ", err)
	}
	mfh.load(info.Size())
	return info.Size()
}

func (mfh *MFHeader) GetDeletionLog() map[string]uint8 {
	return mfh.deletionLog
}

func (mfh *MFHeader) ClearDeletionLog() {
	for k := range mfh.deletionLog {
		delete(mfh.deletionLog, k)
	}
}

func (ih *MFHeader) load(size int64) {
	fmt.Printf(
		"[INFO] File(%s) already exists, size(%d bytes), loading "+
			"blob actions from it.\n",
		ih.LocalName, size)
	ih.RWLock.Lock()
	defer ih.RWLock.Unlock()

	// File already checked before Load() call.
	f, _ := os.Open(ih.LocalName)

	logBytes := make([]byte, size)
	// TODO: consider doing for loop iterative read.
	_, err := f.Read(logBytes)
	Check(err)

	var ies []MFEntry
	err = json.Unmarshal([]byte(logBytes), &ies)
	Check(err)

	for i := 0; i < len(ies); i++ {
		blbId := ies[i].BlobId
		if ies[i].Action == K_action_delete+K_action_base_ascii {
			ih.deletionLog[blbId] = ies[i].Action
			log.Printf("[INFO] Emplaced %v deletion log in-memory.\n", blbId)
		}
	}
	log.Printf("[INFO] Loaded manifest file %s, parsed entry num(%d).\n",
		ih.LocalName, len(ies))
	if len(ies) > 0 {
		ih.Empty = false
	}
}

// created with open state
func (mfh *MFHeader) create() int64 {
	log.Printf("[INFO] Manifest file(%s) doesn't exist, creating a new one.\n",
		mfh.LocalName)
	f, err := os.Create(mfh.LocalName)
	Check(err)

	res, err := f.WriteAt([]byte("[\n]"), 0)
	Check(err)
	f.Close()
	return int64(res)
}

func (mfh *MFHeader) Put(blobId string) (int64, error) {
	mfh.RWLock.Lock()
	defer mfh.RWLock.Unlock()

	entry := MFEntry{
		BlobId: blobId,
		Action: K_action_put + K_action_base_ascii,
	}
	return mfh.flush(&entry)
}

func (mfh *MFHeader) Delete(blobId string) (int64, error) {
	mfh.RWLock.Lock()
	defer mfh.RWLock.Unlock()

	entry := MFEntry{
		BlobId: blobId,
		Action: K_action_delete + K_action_base_ascii,
	}
	return mfh.flush(&entry)
}

func (mfh *MFHeader) flush(entry *MFEntry) (int64, error) {
	f, err := os.OpenFile(mfh.LocalName, os.O_WRONLY, 0755)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	f.Seek(-1, 2)

	leading := make([]byte, 0)
	if !mfh.Empty {
		leading = []byte(",\n")
	}
	serializeBytes := entry.Serialize()
	paddingLen := K_mf_entry_len - len(serializeBytes) - 2
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
	if !mfh.Empty {
		if int64(res)-1 != K_mf_entry_len {
			log.Fatalf("[MF FLUSH ERROR] : datalen %v is not equal to size %v\n", int64(res)-1, K_mf_entry_len)
		}
	} else {
		if int64(res)-1 != K_mf_entry_len-2 {
			log.Fatalf("[MF FLUSH ERROR] : datalen %v is not equal to size %v\n", int64(res)-1, K_mf_entry_len-2)
		}
	}
	mfh.Empty = false
	return int64(res) - 1, nil
}

func (entry *MFEntry) Serialize() []byte {
	jsonbyte, err := json.Marshal(entry)
	if err != nil {
		panic(err)
	}
	return jsonbyte
}
