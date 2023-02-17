/////////////////////////////////////////
// 2022 PJLab Storage all rights reserved
/////////////////////////////////////////

package cache_ops

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	blob "holder/src/blob_handler"
	db_ops "holder/src/db_ops"
	"holder/src/file_handler"

	"github.com/common/definition"
)

type CacheManager struct {
	wMtx sync.Mutex
	pMtx sync.Mutex

	writeItemMap map[string]int
	queue        []string

	purgeEnqueued bool

	dbOpsFile db_ops.DBOpsFile
	pbh       blob.PhyBH
}

func (mgr *CacheManager) EnqueueWriteReq(fileName string) {
	mgr.wMtx.Lock()
	_, exist := mgr.writeItemMap[fileName]
	if exist {
		return
	}
	mgr.writeItemMap[fileName] = 1
	mgr.queue = append(mgr.queue, fileName)
	mgr.wMtx.Unlock()
}

func (mgr *CacheManager) EnqueuePurgeReq() {

}

func (mgr *CacheManager) loopBatchWrite() {
	for {
		time.Sleep(200 * time.Millisecond)
		mgr.wMtx.Lock()
		if len(mgr.queue) == 0 {
			mgr.wMtx.Unlock()
			continue
		}
		numToFetch := min(definition.F_num_batch_write, len(mgr.queue))
		var itemsAtHand = mgr.queue[:numToFetch]
		mgr.queue = mgr.queue[numToFetch:]
		mgr.wMtx.Unlock()
		// Start handling jobs. This will block the main job pulling thread.
		wg := &sync.WaitGroup{}
		for _, fileName := range itemsAtHand {
			wg.Add(1)
			go func(filename string) {
				mgr.dowloadAndWriteCache(filename)
				wg.Done()
			}(fileName)
		}
		wg.Wait()
	}
}

func (mgr *CacheManager) dowloadAndWriteCache(fileName string) {
	exist, ossDataLen := CheckUrl(fileName)
	if !exist {
		return
	}

	mgr.pMtx.Lock()
	if mgr.purgeEnqueued || int64(ossDataLen) > definition.F_CACHE_MAX_SIZE-mgr.pbh.totalBytes {
		mgr.purgeEnqueued = true
		mgr.pMtx.Unlock()
		return
	}

	// 1.Get from OSS
	ossData := mgr.DownLoad(fileName, ossDataLen)
	if ossData == nil {
		return
	}
	// 2. Write To Cache
	token, err := mgr.WriteToCache(fileName, definition.F_DB_STATE_READY, ossData)
	// TODO(csun): if the error is cache full, return
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("[GetFromOssAndWriteToCache] token:", token)
	err = mgr.SealFileAtCache(fileName, token, int32(len(ossData)))
	// TODO(csun): if the error is conflict, return
	if err != nil {
		log.Fatalln(err)
	}
}

func (mgr *CacheManager) WriteToCache(fid string, state int32, ossData []byte) (string, error) {
	fw := file_handler.FileWriter{
		Pbh:    mgr.pbh,
		FileDb: &mgr.dbOpsFile,
	}
	token := ""
	var err error
	if token, err = fw.WriteFileToCache(fid, ossData); err != nil {
		return "", err
	}
	return token, nil
}

func (mgr *CacheManager) SealFileAtCache(fid string, token string, size int32) error {
	err := mgr.dbOpsFile.CommitCacheFileInDB(fid, token, size)
	if err != nil {
		log.Printf("[ERROR] SealFileAtCache: Seal file(%s) failed.", fid)
		return err
	}
	return nil
}

// Utility function
func CheckUrl(arg string) (bool, int64) {
	//check url
	resp, err := http.Head(arg)
	if err != nil {
		// maybe timeout , cannot crash the server.
		log.Println("[CheckUrl] error: ", err)
		return false, 0
	}
	log.Printf("[HttpReadFromCache] check url =%v finish \n", arg)
	if resp.StatusCode == 404 {
		log.Printf("[HttpReadFromCache] url: %s is not exist\n", arg)
		resp.Body.Close()
		return false, 0
	}
	contentlength := resp.ContentLength
	log.Printf("[HttpReadFromCache] url: %s size: %v\n", arg, contentlength)
	if contentlength >= definition.F_CACHE_MAX_SIZE {
		log.Printf("[HttpReadFromCache] url: %s is larger than 100G\n", arg)
		resp.Body.Close()
		return false, 0
	}
	resp.Body.Close()
	return true, contentlength
}

// Utility function
func (mgr *CacheManager) DownLoad(url string, ossDataLen int64) []byte {
	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		log.Println("[DownLoad] ossData not found")
		return nil
	}
	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	ossData := buf.Bytes()
	if err != nil {
		log.Println("[DownLoad] error: ", err)
		if len(ossData) != int(ossDataLen) {
			log.Println("[DownLoad] error: datalen %v is not equal to size %v\n", len(ossData), int(ossDataLen))
			return nil
		}
	}
	log.Printf("[DownLoad] ossData len %v\n", len(ossData))
	return ossData
}

// Utility function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
