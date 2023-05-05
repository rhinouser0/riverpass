// //////////////////////////////
// 2022 SHLab all rights reserved
// //////////////////////////////

package cache_ops

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	blob "holder/src/blob_handler"
	db_ops "holder/src/db_ops"
	"holder/src/file_handler"

	"github.com/common/definition"
	. "github.com/common/zaplog"
	"go.uber.org/zap"
)

// TODO:
// 1. Need GC db metadata loop
// 2. Need optimize the OSS download
// 3. Need improving the cache eviction algorithm
type CacheManager struct {
	wMtx sync.Mutex
	// fileName->fid
	// TODO: currently fid is fileName, so we can refactor writeItemMap.
	writeItemMap map[string]string
	wQueue       []string

	pMtx         sync.Mutex
	purgeItemMap map[string]time.Time
	pQueue       []string

	dbOpsFile *db_ops.DBOpsFile
	pbh       *blob.PhyBH
}

func (mgr *CacheManager) New(fdb *db_ops.DBOpsFile, bh *blob.PhyBH) {
	mgr.writeItemMap = make(map[string]string)
	mgr.purgeItemMap = make(map[string]time.Time)
	mgr.wQueue = make([]string, 0)
	mgr.pQueue = make([]string, 0)
	mgr.dbOpsFile = fdb
	mgr.pbh = bh

	// Dispatch background thread.
	go mgr.loopBatchWrite()
	go mgr.loopGarbageCollection()
}

func (mgr *CacheManager) EnqueueWriteReq(
	fid string, fileName string) {
	mgr.wMtx.Lock()
	defer mgr.wMtx.Unlock()
	_, exist := mgr.writeItemMap[fileName]
	if exist {
		return
	}
	mgr.writeItemMap[fileName] = fid
	mgr.wQueue = append(mgr.wQueue, fileName)
}

// Assuming with lock.
func (mgr *CacheManager) EnqueueDeletionReq() {
	mgr.pMtx.Lock()
	defer mgr.pMtx.Unlock()
	tpltId, err := mgr.pbh.GetTailNameForEvict()
	if err != nil {
		return
	}
	err = mgr.dbOpsFile.DeleteFileWithTripleIdInDB(tpltId)
	if err != nil {
		ZapLogger.Error("DELETE FILE IN DB ERROR", zap.Any("error", err))
		return
	}
	mgr.purgeItemMap[tpltId] = time.Now()
	mgr.pQueue = append(mgr.pQueue, tpltId)
}

func (mgr *CacheManager) loopBatchWrite() {
	for {
		time.Sleep(200 * time.Millisecond)

		mgr.wMtx.Lock()
		if len(mgr.wQueue) == 0 {
			mgr.wMtx.Unlock()
			continue
		}
		numToFetch := min(definition.F_num_batch_write, len(mgr.wQueue))
		var namesAtHand = mgr.wQueue[:numToFetch]
		mgr.wQueue = mgr.wQueue[numToFetch:]

		var fids []string
		for _, name := range namesAtHand {
			fids = append(fids, mgr.writeItemMap[name])
			delete(mgr.writeItemMap, name)
		}
		mgr.wMtx.Unlock()

		// Start handling jobs. This will block the main job pulling thread.
		wg := &sync.WaitGroup{}
		for i, fileName := range namesAtHand {
			wg.Add(1)
			go func(filename string, fid string) {
				mgr.dowloadAndWriteCache(filename, fid)
				wg.Done()
			}(fileName, fids[i])
		}
		wg.Wait()
	}
}

func (mgr *CacheManager) loopGarbageCollection() {
	for {
		time.Sleep(200 * time.Millisecond)
		mgr.pMtx.Lock()
		if len(mgr.purgeItemMap) == 0 {
			mgr.pMtx.Unlock()
			continue
		}
		wg := &sync.WaitGroup{}
		for _, tpltId := range mgr.pQueue {
			if time.Now().Sub(mgr.purgeItemMap[tpltId]).Milliseconds() <
				definition.F_cache_purge_waiting_ms {
				continue
			}
			wg.Add(1)
			go func(id string) {
				mgr.pbh.PurgeTriplet(id)
				wg.Done()
			}(tpltId)
			delete(mgr.purgeItemMap, tpltId)
		}
		wg.Wait()
		mgr.pMtx.Unlock()
	}
}

func (mgr *CacheManager) dowloadAndWriteCache(
	fileName string, fid string) {
	exist, ossDataLen := CheckUrl(fileName)
	if !exist {
		return
	}

	// 1.Get from OSS
	ossData := mgr.DownLoad(fileName, ossDataLen)
	if ossData == nil {
		return
	}
	// 2. Write To Cache
	token, err := mgr.WriteToCache(fileName, ossData)

	if err != nil {
		mgr.RollbackFileInDB(fid)
		if strings.Contains(err.Error(), "cache full") {
			mgr.EnqueueDeletionReq()
			return
		} else {
			ZapLogger.Error("WriteToCache failed", zap.Any("err", err))
			return
		}
	}
	err = mgr.SealFileAtCache(fid, token, int32(len(ossData)))
	// TODO: if the error is conflict, return
	if err != nil {
		// TODO: handle error
		ZapLogger.Error("SealFileAtCache failed", zap.Any("err", err))
	}
}

func (mgr *CacheManager) WriteToCache(
	fid string, ossData []byte) (string, error) {
	fw := file_handler.FileWriter{
		Pbh:    mgr.pbh,
		FileDb: mgr.dbOpsFile,
	}
	token := ""
	var err error
	if token, err = fw.WriteFileToCache(fid, ossData); err != nil {
		return "", err
	}
	return token, nil
}

func (mgr *CacheManager) SealFileAtCache(fid string, token string, size int32) error {
	err := mgr.dbOpsFile.CommitCacheFileInDB(
		fid, token, size)
	if err != nil {
		ZapLogger.Error("Seal file failed", zap.Any("fid", fid))
		return err
	}
	return nil
}

// rollback file meta in db, if write cache failed
func (mgr *CacheManager) RollbackFileInDB(fid string) error {
	err := mgr.dbOpsFile.DeletePendingFileWithFIdInDB(fid)
	if err != nil {
		ZapLogger.Error("rollback file failed", zap.Any("fid", fid))
		return err
	}
	ZapLogger.Info("sucessfully rollback", zap.Any("fid", fid))
	return nil
}

// Utility function
func CheckUrl(url string) (bool, int64) {
	//check url
	if definition.F_local_mode {
		stat, err := os.Stat(url)
		if err == nil {
			return true, stat.Size()
		}
		if os.IsNotExist(err) {
			ZapLogger.Info("local file not found", zap.Any("file", url))
			return false, 0
		}
		ZapLogger.Error("CheckUrl failed", zap.Any("err", err))
		return false, 0

	} else {
		// TODO: http.Head with presign url maybe failed
		resp, err := http.Head(url)
		if err != nil {
			// maybe timeout , cannot crash the server.
			ZapLogger.Error("http.Head", zap.Any("err", err))
			return false, 0
		}
		if resp.StatusCode == 404 {
			ZapLogger.Error("url is not exist", zap.Any("url", url))
			resp.Body.Close()
			return false, 0
		}
		contentlength := resp.ContentLength
		ZapLogger.Info("CheckUrl", zap.Any("url", url), zap.Any("size", contentlength))
		if contentlength >= definition.F_CACHE_MAX_SIZE {
			ZapLogger.Warn("url is too large", zap.Any("url", url),
				zap.Any("cache size MB", definition.F_CACHE_MAX_SIZE/1024/1024))
			resp.Body.Close()
			return false, 0
		}
		resp.Body.Close()
		return true, contentlength
	}
}

// Utility function
func (mgr *CacheManager) DownLoad(url string, ossDataLen int64) []byte {
	// Get the data
	if definition.F_local_mode { // only for test
		data, err := ioutil.ReadFile(url)
		if err != nil {
			ZapLogger.Error("read local file failed", zap.Any("err", err))
			return nil
		}
		if len(data) != int(ossDataLen) {
			ZapLogger.Error("datalen is not equal to size",
				zap.Any("dataLen", len(data)), zap.Any("size", ossDataLen))
			return nil
		}
		return data

	} else {
		start := time.Now()
		resp, err := http.Get(url)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == 404 {
			ZapLogger.Error("ossData not found", zap.Any("url", url))
			return nil
		}
		var buf bytes.Buffer
		_, err = io.Copy(&buf, resp.Body)
		ossData := buf.Bytes()
		if err != nil {
			ZapLogger.Error("DownLoad failed", zap.Any("err", err))
			if len(ossData) != int(ossDataLen) {
				ZapLogger.Error("Download dataSize is not equal to inputdataLen",
					zap.Any("download dataSize", len(ossData)), zap.Any("inputdataLen", ossDataLen))
				return nil
			}
		}
		duration := time.Now().Sub(start)
		ZapLogger.Info("Download finish",
			zap.Any("download dataSize", len(ossData)),
			zap.Any("duration seconds", duration.Seconds()))
		return ossData
	}
}

// Utility function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
