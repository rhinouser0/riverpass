// //////////////////////////////
// 2022 SHLab all rights reserved
// //////////////////////////////

package main

import (
	"errors"
	"flag"
	"fmt"
	blobs "holder/src/blob_handler"
	cache "holder/src/cache_ops"
	_ "holder/src/db_ops"
	db_ops "holder/src/db_ops"
	files "holder/src/file_handler"
	"log"
	"net/http"
	"os"
	"strconv"
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
}

func argsfunc() {
	log.Println("main input args 3:")
	if len(os.Args) == 3 {
		maxSize, err := strconv.Atoi(os.Args[2])
		if err != nil {
			log.Fatalln(err)
		}
		ratio := 0.95
		definition.F_CACHE_MAX_SIZE = int64(definition.K_MiB) * int64(float64(maxSize)*ratio)
		log.Println("Args F_CACHE_MAX_SIZE : ", definition.F_CACHE_MAX_SIZE)
	}
}

func init() {
	// Config initialization...
	dir, err := os.Getwd()
	if err != nil {
		log.Fatalf("os.Getwd() error! \n")
	}
	dirConfig := dir + "/../oss_server_config.xml"
	log.Println("Directory of oss_server_config file:", dirConfig)

	var cfg config.OssConfig
	cfg.LoadXMLConfig(dirConfig)
	if err != nil {
		log.Fatalln(err)
	}
	if err != nil {
		log.Fatalln(err)
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
	fmt.Println(" ")

	log.Print("[init] End of main::init().\t Address:(", Address,
		")\t DataPosition:(", definition.DataPosition, ").\n ")
}

// register http path handlers
func RegisterHttpHandler() {
	for map_key, _ := range RequestHandlers {
		http.HandleFunc(map_key, RequestHandlers[map_key])
	}
}

func HttpRead(w http.ResponseWriter, r *http.Request) {
	var arg string
	values := r.URL.Query()
	arg = values.Get("url")
	log.Printf("[HttpRead] url=%v\n", arg)
	//offset := 0,size := 0 means read all data from 0 to len(data).
	data, err := OssServer.TryReadFromCache(arg, 0, 0)
	if err != nil {
		log.Fatalln(err)
	}
	if data == nil {
		log.Printf("[HttpRead] file not found on disk, get from oss\n")
		w.WriteHeader(404)
	} else {
		log.Println("[HttpRead] READ SUCCESSFULLY!")
		h := w.Header()
		h.Set("Content-type", "application/octet-stream")
		h.Set("Content-Disposition", "attachment;filename="+arg)
		w.WriteHeader(200)
		w.Write(data)
	}
}

func (oSvr *OssHolderServer) New(cm *cache.CacheManager, fdb *db_ops.DBOpsFile) {
	oSvr.mgr = cm
	oSvr.dbOpsFile = fdb
}

func (s *OssHolderServer) TryReadFromCache(
	fileName string, offset int32, size int32) ([]byte, error) {
	listTs := time.Now()
	var fm *definition.FileMeta
	fm, err := s.ListFile(fileName, int32(definition.F_DB_STATE_READY))
	if err != nil {
		log.Fatalln(err)
	}
	log.Println(fm)
	// Didn't find the file in cache.
	if fm == nil {
		pendingFid, err := s.CreateFileForCache(fileName)
		if err != nil {
			log.Fatalln(err)
		}
		s.mgr.EnqueueWriteReq(pendingFid, fileName)
		return nil, nil
	}
	// Read the file from cache.
	fid := fileName
	if fm.RngCodeList == nil {
		log.Println("[TryReadFromCache] fm.RngCodeList is nil")
		return nil, nil
	}

	fr := files.FileReader{
		Pbh:    PhyBH,
		FileDb: s.dbOpsFile,
	}
	var readBytes []byte
	offset = fm.RngCodeList.Front().Value.(range_code.RangeCode).Start
	size = fm.RngCodeList.Front().Value.(range_code.RangeCode).End
	log.Printf("[TryReadFromCache] read from start=%v  end=%v\n", offset, size)
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

func (s *OssHolderServer) ListFile(fileName string, state int32) (*definition.FileMeta, error) {
	var fm *definition.FileMeta
	var err error
	nameOrId := fileName
	if state == definition.F_DB_STATE_INT32_PENDING {
		nameOrId = cache.NormalFidToPending(fileName)
	}

	fm, err = s.dbOpsFile.ListFileFromDB(nameOrId, state)
	if err != nil {
		return nil, err
	}
	return fm, nil
}

func (s *OssHolderServer) CreateFileForCache(fileName string) (string, error) {
	fm := definition.FileMeta{
		Name:   fileName,
		Id:     "",
		BlobId: "",
	}
	pendingFid := cache.NormalFidToPending(fileName)
	err := s.dbOpsFile.CreateFileWithFidInDB(pendingFid, &fm)
	if err != nil {
		log.Printf(
			"[ERROR][CreateFileWithFid]: CreateFileWithFid to DB failed: %v",
			err)
		return "", err
	}
	return pendingFid, nil
}

// file_handler end
//////////////////////////////

func main() {
	flag.Parse()
	RegisterHttpHandler()
	err := http.ListenAndServe(Address, nil)
	if err != nil {
		fmt.Println("Listen to http requests failed", err)
	}
}
