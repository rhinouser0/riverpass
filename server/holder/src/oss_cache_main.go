// //////////////////////////////
// 2022 SHLab all rights reserved
// //////////////////////////////

package main

import (
	"errors"
	"flag"
	blobs "holder/src/blob_handler"
	cache "holder/src/cache_ops"
	_ "holder/src/db_ops"
	db_ops "holder/src/db_ops"
	files "holder/src/file_handler"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	config "github.com/common/config"
	definition "github.com/common/definition"
	"github.com/common/range_code"
	. "github.com/common/zaplog"
	_ "github.com/common/zaplog"
	"go.uber.org/zap"
)

var ShardID = db_ops.ShardID
var Address = db_ops.Address

// Physical blob Handler:
var PhyBH *blobs.PhyBH
var FDb *db_ops.DBOpsFile
var CMgr *cache.CacheManager
var OssServer *OssHolderServer

// All the handler func map for request from client
var RequestHandlers = map[string]func(http.ResponseWriter, *http.Request){
	"/getFile": HttpRead,
}

type OssHolderServer struct {
	mgr       *cache.CacheManager
	dbOpsFile *db_ops.DBOpsFile
	mtx       sync.Mutex
}

func argsfunc() {
	ZapLogger.Info("main input args 3:")
	if len(os.Args) == 3 {
		maxSize, err := strconv.Atoi(os.Args[2])
		if err != nil {
			ZapLogger.Fatal("strconv.Atoi", zap.Any("err", err))
		}
		ratio := 0.95
		definition.F_CACHE_MAX_SIZE = int64(definition.K_MiB) * int64(float64(maxSize)*ratio)
		ZapLogger.Info("Args", zap.Any("F_CACHE_MAX_SIZE", definition.F_CACHE_MAX_SIZE))
	}
}

func init() {
	// Config initialization...
	dir, err := os.Getwd()
	if err != nil {
		ZapLogger.Fatal("os.Getwd()", zap.Any("err", err))
	}
	dirConfig := dir + "/../oss_server_config.xml"
	ZapLogger.Info("", zap.Any("Directory of oss_server_config", dirConfig))

	var cfg config.OssConfig
	cfg.LoadXMLConfig(dirConfig)
	if err != nil {
		ZapLogger.Fatal("LoadXMLConfig", zap.Any("err", err))
	}
	Address = cfg.ParseOssHolderConfigAddress(ShardID)

	// Object initalizations...
	FDb = new(db_ops.DBOpsFile)
	FDb.New()
	ZapLogger.Debug("[init] DBOpsFile initialization finished:",
		zap.Any("PhyBH", *FDb))

	// Physical blob Handler:
	PhyBH = new(blobs.PhyBH)
	PhyBH.New(ShardID, FDb)
	ZapLogger.Debug("[init] PhyBH initialization finished:",
		zap.Any("PhyBH", *PhyBH))

	CMgr = new(cache.CacheManager)
	CMgr.New(FDb, PhyBH)
	ZapLogger.Debug("[init] CacheManager initialization finished:",
		zap.Any("PhyBH", *CMgr))

	OssServer = new(OssHolderServer)
	OssServer.New(CMgr, FDb)

	// Cache size initialization from cmd inputs...
	argsfunc()
	ZapLogger.Info("End of main::init().",
		zap.Any("Address", Address),
		zap.Any("DataPosition", definition.DataPosition))
}

// register http path handlers
func RegisterHttpHandler() {
	for map_key, _ := range RequestHandlers {
		http.HandleFunc(map_key, RequestHandlers[map_key])
	}
}

func HttpRead(w http.ResponseWriter, r *http.Request) {
	var url string
	values := r.URL.Query()
	url = values.Get("url")
	ZapLogger.Info("HttpRead", zap.Any("url", url))
	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		ZapLogger.Error("url is not available", zap.Any("url", url))
		w.WriteHeader(404)
		return
	}
	// only support get Etag from oss object response's header
	etag := resp.Header.Get("Etag")
	//offset := 0,size := 0 means read all data from 0 to len(data).
	data, err := OssServer.TryReadFromCache(url, 0, 0, etag)
	if err != nil {
		ZapLogger.Error("TryReadFromCache", zap.Any("err", err))
		w.WriteHeader(404)
		return
	}
	if data == nil {
		ZapLogger.Info("file not found on disk, get from oss")
		w.WriteHeader(404)
	} else {
		ZapLogger.Info("READ SUCCESSFULLY", zap.Any("url", url))
		h := w.Header()
		h.Set("Content-type", "application/octet-stream")
		h.Set("Content-Disposition", "attachment;filename="+url)
		w.WriteHeader(200)
		w.Write(data)
	}
}

func (oSvr *OssHolderServer) New(cm *cache.CacheManager, fdb *db_ops.DBOpsFile) {
	oSvr.mgr = cm
	oSvr.dbOpsFile = fdb
}

func (s *OssHolderServer) TryReadFromCache(
	fileName string, offset int32, size int32, etag string) ([]byte, error) {
	listTs := time.Now()
	var fm *definition.FileMeta
	// TODO: optimize this db lock
	s.mtx.Lock()
	fm, state, err := s.ListFileAndState(fileName)
	if err != nil {
		ZapLogger.Error("ListFileAndState", zap.Any("err", err))
		return nil, err
	}
	if state == -1 {
		// Didn't find the file in cache.
		fid, err := s.CreateFileForCache(fileName, etag)
		if err != nil {
			ZapLogger.Error("CreateFileForCache", zap.Any("err", err))
			return nil, err
		}
		s.mgr.EnqueueWriteReq(fid, fileName)
		s.mtx.Unlock()
		return nil, nil
	}
	s.mtx.Unlock()
	if state == definition.F_BLOB_STATE_PENDING {
		// cache is downloading
		ZapLogger.Info("Didn't find the file in cache(cache is downloading)",
			zap.Any("file", fileName))
		return nil, nil
	} else if state == definition.F_BLOB_STATE_READY {
		if fm == nil {
			ZapLogger.Error("file meta is nil in db", zap.Any("file", fileName))
			return nil, errors.New("file meta is nil in db")
		}
		if etag != fm.Etag {
			fm.Etag = etag
			s.dbOpsFile.UpdateFilemetaAndStateInDB(fileName,
				fm, definition.F_BLOB_STATE_PENDING)
			ZapLogger.Info("Cache is outdate, redownload", zap.Any("file", fileName))
			s.mgr.EnqueueWriteReq(fileName, fileName)
			return nil, nil
		}
		// Read the file from cache.
		fid := fileName
		if fm.RngCodeList == nil {
			ZapLogger.Info("fm.RngCodeList is nil")
			return nil, nil
		}
		fr := files.FileReader{
			Pbh:    PhyBH,
			FileDb: s.dbOpsFile,
		}
		var readBytes []byte
		offset = fm.RngCodeList.Front().Value.(range_code.RangeCode).Start
		size = fm.RngCodeList.Front().Value.(range_code.RangeCode).End
		ZapLogger.Info("read from", zap.Any("start", offset), zap.Any("size", size))
		if time.Now().Sub(listTs).Milliseconds() > definition.F_cache_purge_waiting_ms {
			ZapLogger.Error("[TryReadFromCache] faild:",
				zap.Any("fail to avoid stale cache data: ", fileName))
			return nil, errors.New("data not in cache")
		}
		if readBytes, err = fr.ReadFromCache(fid, offset, size, fm.RngCodeList); err != nil {
			ZapLogger.Error("[TryReadFromCache] faild:",
				zap.Any("err", err))
			return nil, err
		}
		return readBytes, nil
	}
	ZapLogger.Error("logical error, state is invalid",
		zap.Any("file", fileName),
		zap.Any("state", state))
	return nil, errors.New("logical error, state is invalid.")
}

func (s *OssHolderServer) ListFile(fileName string, state int32) (*definition.FileMeta, error) {
	var fm *definition.FileMeta
	var err error
	fm, err = s.dbOpsFile.ListFileFromDB(fileName, state)
	if err != nil {
		return nil, err
	}
	return fm, nil
}

func (s *OssHolderServer) ListFileAndState(fileName string) (*definition.FileMeta, int, error) {
	fm, state, err := s.dbOpsFile.ListFileAndStateFromDB(fileName)
	if err != nil {
		return nil, -1, err
	}
	return fm, state, nil
}

func (s *OssHolderServer) CreateFileForCache(fileName string, etag string) (string, error) {
	fm := definition.FileMeta{
		Name:   fileName,
		Id:     "",
		BlobId: "",
		Etag:   etag,
	}
	err := s.dbOpsFile.CreateFileWithFidInDB(fileName, &fm)
	if err != nil {
		ZapLogger.Error("CreateFileWithFid to DB failed", zap.Any("err", err))
		return "", err
	}
	return fileName, nil
}

// file_handler end
//////////////////////////////

func main() {
	flag.Parse()
	RegisterHttpHandler()
	err := http.ListenAndServe(Address, nil)
	if err != nil {
		ZapLogger.Error("Listen to http requests failed", zap.Any("err", err))
	}
}
