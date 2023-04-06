// //////////////////////////////
// 2022 SHLab all rights reserved
// //////////////////////////////

// TODO: It's is not eventual goal to use phy blb handler to directly handle
// blob writes. We need to create a logical_blob_handler file, wrap
// phy_blb_handler and provide cross region/platform availability
package blob_handler

import (
	"errors"
	"fmt"
	dbops "holder/src/db_ops"
	"log"
	"math/rand"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/common/definition"
	"github.com/common/util"
)

// A triplet is a combination of 3 harnessed blob operation headers.
const K_empty_idxmf_file_overhead = 8

type Triplet struct {
	Id        string
	IdxHeader *IndexHeader
	MFHeader  *MFHeader
	BinHeader *BinHeader
}

// Physical blob Handler holds blobs by multiple Triplets.
// A triplet combines 3 headers of writers in which they documents
// blob data in 3 different angle: content, idx of content, and the
// operation log againt the content. In this sense, blob is indexed
// by timeseries order.
// 1 shard of physical blob handler handles 1 portion of blob IOs.
// PhyBH is sharded by the deployment machine, since 1 machine can
// generate a series of triplets.
/*type PhyBH struct {
	ShardId int
	// 1 blob handler may contain multiple opened or closed headers.
	OpenTplt   map[string]*Triplet
	ClosedTplt map[string]*Triplet
}*/

type PhyBH struct {
	ShardId int
	// 1 blob handler may contain multiple opened or closed headers.
	OpenTplt     *LruCache
	ClosedTplt   *LruCache
	LargeObjTplt *LruCache
	// Protected by atomic operations.
	totalBytes int64
	// mtx is used by totalBytes
	mtx sync.Mutex
	FDb *dbops.DBOpsFile
}

func (tri *Triplet) New(shardId int, triId string, isLarge bool) int64 {
	var idx IndexHeader
	var mf MFHeader
	var bin BinHeader
	idxSize := idx.New(shardId, triId, isLarge)
	mfSize := mf.New(shardId, triId)
	binSize := bin.New(shardId, triId)

	tri.Id = triId
	tri.IdxHeader = &idx
	tri.MFHeader = &mf
	tri.BinHeader = &bin
	return idxSize + mfSize + binSize
}

// TODO: Add idx file and bin file cross check loading logic.
func (pbh *PhyBH) New(shardId int, fdb *dbops.DBOpsFile) {
	pbh.ShardId = shardId
	pbh.OpenTplt = new(LruCache)
	pbh.OpenTplt.New()
	pbh.ClosedTplt = new(LruCache)
	pbh.ClosedTplt.New()
	pbh.LargeObjTplt = new(LruCache)
	pbh.LargeObjTplt.New()
	pbh.totalBytes = 0
	// TODO: load from DB the triplet ids this shard holds, then
	// load from FS the triplets, check and hydrate the PhyBH.
	var triIds []string
	var totalSize int64
	log.Println("[PhyBH.NEW] ScanDB")
	var triIdsInDisk []string
	triIdsInDisk, totalSize = ScanLocalFS(shardId)

	pbh.FDb = fdb

	log.Printf("[PhyBH.NEW] PhyBH: DELETE PENDING FILES IN DB")
	pbh.FDb.DeleteAllPendingFileInDB()
	triIds, err := pbh.FDb.ListTripleIdOfAllFiles()
	if err != nil {
		log.Fatalln(err)
	}
	setDB := make(map[string]struct{})
	for _, v := range triIds {
		setDB[v] = struct{}{}
	}
	orphanSize := int64(0)
	for _, v := range triIdsInDisk {
		if _, ok := setDB[v]; !ok {
			log.Printf("[PhyBH.NEW] PhyBH: DELETE ORPHAN FILE ON DISK, tripleid: %s\n", v)
			orphanSize += DeleteTripletFilesOnDisk(v)
		}
	}
	log.Printf("[PhyBH.NEW] totalSize: %v,orphanSize: %v\n", totalSize, orphanSize)
	pbh.totalBytes = totalSize - orphanSize

	if pbh.totalBytes < 0 {
		log.Println("[PhyBH.NEW WARNING] PhyBH: totalBytes < 0 ", pbh.totalBytes)
		pbh.totalBytes = 0
	}
	log.Println("[PhyBH.NEW] PhyBH: Caculate totalBytes after deletion ", pbh.totalBytes)
	cnt := 0
	for _, triId := range triIds {
		var triplet Triplet
		// Although the third arg of LargeObjTplt should be true,but in this loop it is ok.
		// Because the file has already on disk. we only need to read triplet.IdxHeader.Info.State.
		triplet.New(pbh.ShardId, triId, false)
		switch triplet.IdxHeader.Info.State {
		case K_state_base_ascii + K_index_header_open:
			cnt++
			pbh.OpenTplt.Put(triId, &triplet)
		case K_state_base_ascii + K_index_header_closed:
			pbh.ClosedTplt.Put(triId, &triplet)
		case K_state_base_ascii + K_index_header_large:
			log.Printf("[PhyBH.NEW] PhyBH: RECREAT LARGE FILE ON DISK, tripleid: %s\n", triId)
			pbh.LargeObjTplt.Put(triId, &triplet)
		default:
			{
				err := errors.New("indexHeader state unrecognized")
				panic(err)
			}
		}
	}
	// Create a new triplet for taking write.
	if cnt == 0 {
		ptrTplt, tmpSize := pbh.openNewTplt(false)
		pbh.totalBytes += tmpSize
		pbh.OpenTplt.Put((*ptrTplt).Id, ptrTplt)
	}
	pbh.PrintTplts("Initialized")

	log.Println("[PhyBH.NEW] PhyBH: Caculate totalBytes after initialization ", pbh.totalBytes)
	// init goroutine for size checking and closing.
	go pbh.LoopHotSwap()
}

func (pbh *PhyBH) PurgeTriplet(tpltId string) {
	pbh.ClosedTplt.DeleteFromCache(tpltId)
	pbh.LargeObjTplt.DeleteFromCache(tpltId)
	atomic.AddInt64(&pbh.totalBytes, ^int64(DeleteTripletFilesOnDisk(tpltId)-1))
}

// TODO: always purge small object tplt first. Need to change to more
// wise logic.
func (pbh *PhyBH) GetTailNameForEvict() (string, error) {
	if pbh.ClosedTplt.size > 0 {
		return pbh.ClosedTplt.GetCurTailNameForEvict(), nil
	} else if pbh.LargeObjTplt.size > 0 {
		return pbh.LargeObjTplt.GetCurTailNameForEvict(), nil
	}
	return "", errors.New("no tail to purge")
}

// Stateful:
// * when service starts, load bin and idx file, reconcile each other. Blob must appear at
// * both persistence to rebuild the in memory index and expose to user.
// * if service crashes between bin-flush and idx-flush, data not persisted.
// * if service crashes after bin-flush and idx-flush, data persisted.
// * it's ok mf file doesn't contain put record.
func (pbh *PhyBH) Put(blbId string, data []byte) (token string, err error) {
	payloadSize := util.GetPayloadSize(len(data))
	maxAllocSize := K_empty_idxmf_file_overhead + payloadSize +
		K_index_entry_len + K_mf_entry_len + 4

	pbh.mtx.Lock()
	if maxAllocSize > definition.F_CACHE_MAX_SIZE-atomic.LoadInt64(&pbh.totalBytes) {
		pbh.mtx.Unlock()
		return "", errors.New("cache full")
	}
	atomic.AddInt64(&pbh.totalBytes, maxAllocSize)
	pbh.mtx.Unlock()

	var triplet *Triplet

	var increaseBytes int64 = 0
	if payloadSize > definition.K_triplet_large_threshold {
		var size int64
		triplet, size = pbh.openNewTplt(true)
		increaseBytes += size
		pbh.LargeObjTplt.Put(triplet.Id, triplet)
		log.Printf("[INFO] PhyBH: Large triplet has created, id: %s", triplet.Id)
		token = definition.K_LARGE_OBJECT_PREFIX +
			util.GenerateBlobToken(triplet.Id, blbId)
		log.Printf("[INFO] PhyBH: Generated blob token: %s", token)
	} else {
		numOpenTplt := pbh.OpenTplt.size
		randNum := rand.Intn(numOpenTplt)
		var pick string
		i := 0
		dict := pbh.OpenTplt.dict
		dict.Range(func(k, value interface{}) bool {
			if randNum >= i {
				pick = k.(string)
				return false
			}
			i += 1
			return true
		})
		log.Printf("[INFO] PhyBH: Open triplet picked, id: %s", pick)
		token = util.GenerateBlobToken(pick, blbId)
		log.Printf("[INFO] PhyBH: Generated blob token: %s", token)
		triplet = pbh.OpenTplt.Get(pick)
	}
	// TODO: Error handling for each step.
	// step 1: Persist in binary. Flush must succeed.
	offset, size := triplet.BinHeader.Put(blbId, data)
	if size != payloadSize {
		atomic.AddInt64(&pbh.totalBytes, ^int64(maxAllocSize-1))
		log.Printf("[ERROR] PhyBH: datalen %v is not equal to size %v\n", payloadSize, size)
		return "", errors.New("BinHeader put error")
	}
	// step 2: Store the idx in memory; Flush must succeed
	idxBytes, idxErr := triplet.IdxHeader.Put(blbId, offset, size)
	if idxErr != nil {
		atomic.AddInt64(&pbh.totalBytes, ^int64(maxAllocSize-1))
		log.Println("[ERROR] PhyBH: idxErr: ", idxErr)
		return "", idxErr
	}
	// step 3: Persist action in MF. Flush may or may not succeed
	mfBytes, mfErr := triplet.MFHeader.Put(blbId)
	if mfErr != nil {
		atomic.AddInt64(&pbh.totalBytes, ^int64(maxAllocSize-1))
		log.Println("[ERROR] PhyBH: mfERROR", mfErr)
		return "", mfErr
	}
	increaseBytes += int64(payloadSize) + idxBytes + mfBytes
	atomic.AddInt64(&pbh.totalBytes, increaseBytes)
	atomic.AddInt64(&pbh.totalBytes, ^int64(maxAllocSize-1))
	return token, nil
}

// First check in index if blb exist. If exist obtain blob content from binary
// file and return.
func (pbh *PhyBH) Get(token string) (data []byte, err error) {
	prefix := token[:len(definition.K_LARGE_OBJECT_PREFIX)]
	if prefix == definition.K_LARGE_OBJECT_PREFIX {
		log.Println("[INFO] PhyBH: Get Large triplet")
		tpltId := util.GetTripletIdFromToken(token[len(definition.K_LARGE_OBJECT_PREFIX):])
		blbId := util.GetBlobIdFromToken(token[len(definition.K_LARGE_OBJECT_PREFIX):])
		var hostTplt *Triplet
		if triplet := pbh.LargeObjTplt.Get(tpltId); triplet != nil {
			hostTplt = triplet
		} else {
			err := errors.New("blob not exist in this blob handler shard")
			return nil, err
		}

		if ptrIdx := hostTplt.IdxHeader.Get(blbId); ptrIdx != nil {
			return hostTplt.BinHeader.Get(blbId, ptrIdx.Offset), nil
		}
		log.Printf("[INFO] PhyBH: Get failed, blob[%s] already deleted in tplt[%s]\n",
			blbId, tpltId)

	} else {
		tpltId := util.GetTripletIdFromToken(token)
		blbId := util.GetBlobIdFromToken(token)

		var hostTplt *Triplet
		if triplet := pbh.OpenTplt.Get(tpltId); triplet != nil {
			hostTplt = triplet
		} else if triplet := pbh.ClosedTplt.Get(tpltId); triplet != nil {
			hostTplt = triplet
		} else {
			err := errors.New("blob not exist in this blob handler shard")
			return nil, err
		}

		if ptrIdx := hostTplt.IdxHeader.Get(blbId); ptrIdx != nil {
			return hostTplt.BinHeader.Get(blbId, ptrIdx.Offset), nil
		}
		log.Printf("[INFO] PhyBH: Get failed, blob[%s] already deleted in tplt[%s]\n",
			blbId, tpltId)
	}
	return data, nil
}

func (pbh *PhyBH) openNewTplt(isLarge bool) (*Triplet, int64) {
	uuid := util.GenerateTriId()
	var newTplt Triplet
	size := newTplt.New(pbh.ShardId, uuid, isLarge)

	log.Printf(
		"[INFO] PhyBH: Shard(%d)-Openning new triplet for taking writes:"+
			" id(%s), idx file(%s), mf file(%s), bin file(%s)\n",
		pbh.ShardId, newTplt.Id, newTplt.IdxHeader.LocalName,
		newTplt.MFHeader.LocalName, newTplt.BinHeader.LocalName)
	return &newTplt, size
}

// For debug
func (pbh *PhyBH) PrintTplts(ctxStr string) {
	dict := pbh.OpenTplt.dict
	dict.Range(func(k, v interface{}) bool {
		log.Printf(
			"[INFO] PhyBH: %s Triplet[%s], state open, value: %v\n",
			ctxStr, k.(string), v.(*Node).value)
		return true
	})
	dict = pbh.ClosedTplt.dict
	dict.Range(func(k, v interface{}) bool {
		log.Printf(
			"[INFO] PhyBH: %s Triplet[%s], state closed, value: %v\n",
			ctxStr, k.(string), v.(*Node).value)
		return true
	})
}

// check if open triplets are ready for close and open new triplets
// for taking writes
func (pbh *PhyBH) LoopHotSwap() {
	for {
		time.Sleep(200 * time.Millisecond)
		var idToClose []string
		var newOpens []*Triplet
		// Scan open triplets, find those can be closed
		dict := pbh.OpenTplt.dict
		dict.Range(func(k, v interface{}) bool {
			if v.(*Node).value.BinHeader.CurOff > definition.K_triplet_closing_threshold {
				idToClose = append(idToClose, k.(string))
				tplt, size := pbh.openNewTplt(false)
				atomic.AddInt64(&pbh.totalBytes, size)
				newOpens = append(newOpens, tplt)
			}
			return true
		})
		// Open equivalent amount of new triplets for taking write.
		for _, tplt := range newOpens {
			pbh.OpenTplt.Put(tplt.Id, tplt)
		}
		// Close those triplets meets closing bar.
		for _, id := range idToClose {
			// IdxHeader is the only one need to close, manifest may grow,
			// binary follows IdxHeader's state.
			log.Println("[LoopHotSwap] close Id", id)
			pbh.ClosedTplt.Put(id, pbh.OpenTplt.Get(id))
			pbh.OpenTplt.Get(id).IdxHeader.Close()
			pbh.OpenTplt.DeleteFromCache(id)
		}
	}
}

// TODO: finish the implementation of migration:
// 1. copy the idx file to OSS/COS, override the origin
// 2. copy the mf file to OSS/COS, override the origin
// 3. move binary file to OSS/COS, remove local.
// to OSS or COS, but leaving the info in memory.
// func (pbh *PhyBH) LoopMigration() {}

func ScanLocalFS(shardId int) ([]string, int64) {
	localfsPrefix := definition.BlobLocalPathPrefix
	if localfsPrefix == "" {
		localfsPrefix = "/var/lib/docker/.cache"
	}
	localFSDir := localfsPrefix
	totalSize := int64(0)
	files, err := os.ReadDir(localFSDir)
	Check(err)
	regStr := fmt.Sprintf("idx_h_%d+_(.+).dat", shardId)
	reIdxFile := regexp.MustCompile(regStr)
	var triIds []string
	for _, file := range files {
		triId := reIdxFile.FindStringSubmatch(file.Name())
		if triId != nil {
			log.Printf(
				"Found triplet id(%s) by scanning idx file name in localFS.",
				triId[1])
			triIds = append(triIds, triId[1])
		}
	}
	for i := 0; i < len(triIds); i++ {
		binaryFilePath := fmt.Sprintf("%s/binary_%d_%s.dat", localfsPrefix, shardId, triIds[i])
		idxFilePath := fmt.Sprintf("%s/idx_h_%d_%s.dat", localfsPrefix, shardId, triIds[i])
		mfFilePath := fmt.Sprintf("%s/mf_h_%d_%s.dat", localfsPrefix, shardId, triIds[i])
		totalSize += GetFileSize(binaryFilePath) + GetFileSize(idxFilePath) + GetFileSize(mfFilePath)
	}
	return triIds, totalSize
}

func GetFileSize(path string) int64 {
	file, err := os.Stat(path)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		log.Fatalln("[ScanLocalFS] error ", err)
	}
	return file.Size()
}

func RemoveFile(path string) int64 {
	res := int64(0)
	if ok, deleteSize, pathErr := PathExists(path); ok {
		err := os.Remove(path)
		if err != nil {
			log.Fatalln("REMOVE ERROR", err)
		} else {
			res += deleteSize
		}
	} else if pathErr != nil {
		log.Fatalln("PATH ERROR", pathErr)
	}
	return res
}

func DeleteTripletFilesOnDisk(tripleId string) int64 {
	localfsPrefix := definition.BlobLocalPathPrefix
	if localfsPrefix == "" {
		localfsPrefix = "/var/lib/docker/.cache"
	}
	res := int64(0)
	shardId := dbops.ShardID
	binName := fmt.Sprintf("%s/binary_%d_%s.dat", localfsPrefix, shardId, tripleId)
	idxName := fmt.Sprintf("%s/idx_h_%d_%s.dat", localfsPrefix, shardId, tripleId)
	mfName := fmt.Sprintf("%s/mf_h_%d_%s.dat", localfsPrefix, shardId, tripleId)
	log.Println("remove files of tripId", tripleId)
	res += RemoveFile(binName) + RemoveFile(idxName) + RemoveFile(mfName)
	return res
}

func PathExists(path string) (bool, int64, error) {
	file, err := os.Stat(path)
	if err == nil {
		return true, file.Size(), nil
	}
	if os.IsNotExist(err) {
		return false, 0, nil
	}
	return false, 0, err
}
