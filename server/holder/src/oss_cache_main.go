/////////////////////////////////////////
// 2022 PJLab Storage all rights reserved
/////////////////////////////////////////

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	blobs "holder/src/blob_handler"
	_ "holder/src/db_ops"
	db_ops "holder/src/db_ops"
	files "holder/src/file_handler"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/cache_ops"
	config "github.com/common/config"
	definition "github.com/common/definition"
	"github.com/common/range_code"
	. "github.com/common/zaplog"
	_ "github.com/common/zaplog"
	"go.uber.org/zap"
)

var ShardID = db_ops.ShardID
var Address = db_ops.Address
var syncCh chan struct{}
var lock sync.Mutex
var cond *sync.Cond
var blockSlice []string
var threadLocks []sync.Mutex

const threadNums = 10

// Physical blob Handler:
var PhyBH *blobs.PhyBH

// HolderServer
var OssServer OssHolderServer

// All the handler func map for request from client
var RequestHandlers = map[string]func(http.ResponseWriter, *http.Request){
	"/getFile": HttpReadFromCache,
}

func hash(s string) int {
	h := fnv.New32a()
	h.Write([]byte(s))
	return int(h.Sum32())
}

type OssHolderServer struct {
	dbOpsFile db_ops.DBOpsFile
}

func (s *OssHolderServer) ListFile(fileName string, state int32) (*definition.FileMeta, time.Time, error) {
	var fm *definition.FileMeta
	var err error
	nameOrId := fileName
	if state == definition.F_DB_STATE_INT32_PENDING {
		nameOrId = cache_ops.NormalFidToPending(fileName)
	}

	fm, err = s.dbOpsFile.ListFileFromDB(nameOrId, state)
	if err != nil {
		return nil, err
	}
	return fm, time.Now(), nil
}

func (s *OssHolderServer) CreateFileForCache(fileName string) error {
	fm := definition.FileMeta{
		Name:   fileName,
		Id:     "",
		BlobId: "",
	}
	pendingFid := cache_ops.NormalFidToPending(fileName)
	err := s.dbOpsFile.CreateFileWithFidInDB(pendingFid, &fm)
	if err != nil {
		log.Printf("[ERROR][CreateFileWithFid]: CreateFileWithFid to DB failed: %v", err)
		return err
	}
	return nil
}

// DeleteFileWithFid
func (s *OssHolderServer) DeleteFileWithFid(fileId string) error {
	err := s.dbOpsFile.DeleteFileWithFidInDB(fileId)
	if err != nil {
		log.Fatalln(err)

	}
	return nil
}

func (s *OssHolderServer) WriteToCache(fid string, state int32, ossData []byte) (string, error) {
	fm, err := s.ListFile(fid, state)

	if fm == nil || err != nil || fm.Id == "" {
		ZapLogger.Error("[WriteToCache] ListFile faild:", zap.Any("err", err), zap.Any("fm", fm))
		return "", err
	}

	fw := files.FileWriter{
		Pbh:    PhyBH,
		FileDb: &s.dbOpsFile,
	}
	token := ""
	data := ossData
	size := int32(len(data))
	if token, err = fw.WriteFileToCache(fid, data); err != nil {
		ZapLogger.Error("[WriteToCache] faild:",
			zap.Any("err", err))
		return "", err
	}

	ZapLogger.Debug("[WriteToCache] OK:",
		zap.Any("fid", fid),
		zap.Any("size", size))
	return token, nil
}

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

func HttpReadFromCache(w http.ResponseWriter, r *http.Request) {
	var arg string
	values := r.URL.Query()
	arg = values.Get("url")
	log.Printf("[HttpReadFromCache] url=%v\n", arg)
	//offset := 0,size := 0 means read all data from 0 to len(data).
	data, err := OssServer.ReadFromCache(arg, 0, 0)
	if err != nil {
		log.Fatalln(err)
	}
	if data == nil {
		log.Printf("[HttpReadFromCache] file not found on disk, get from oss\n")
		w.WriteHeader(404)
	} else {
		log.Println("[HttpReadFromCache] READ SUCCESSFULLY!")
		h := w.Header()
		h.Set("Content-type", "application/octet-stream")
		h.Set("Content-Disposition", "attachment;filename="+arg)
		w.WriteHeader(200)
		w.Write(data)
	}
}

func writeChanWithTimeout(input struct{}, channel chan struct{}) error {
	timeout := time.NewTimer(time.Millisecond * 100)
	select {
	case channel <- input:
		return nil
	case <-timeout.C:
		return errors.New("write chan time out")
	}
}

func readFromChan(channel chan struct{}) {
	log.Println("[readFromChan] channel Size: ", len(channel))
	<-channel
}

var ossCacheRWLock sync.RWMutex

func (s *OssHolderServer) GetFromOssAndWriteToCache(fileName string, needChan bool) {
	index := hash(fileName) % threadNums
	threadLocks[index].Lock()
	if needChan {
		defer readFromChan(syncCh)
	}
	state := int32(definition.F_DB_STATE_READY)
	fm, err := s.ListFile(fileName, state)
	if err != nil {
		log.Fatalln(err)
	}
	if fm != nil && fm.Id != "" {
		log.Println("[GetFromOssAndWriteToCache] data has been downloaded :", fileName)
		threadLocks[index].Unlock()
		return
	}
	state = int32(definition.F_DB_STATE_PENDING)
	fm, err = s.ListFile(fileName, state)
	if err != nil {
		log.Fatalln(err)
	}
	if fm != nil && fm.Id != "" {
		log.Println("[GetFromOssAndWriteToCache] data is downloading :", fileName)
		threadLocks[index].Unlock()
		return
	}
	//1.Get from OSS
	res, ossDataLen := CheckUrl(fileName)
	if !res {
		threadLocks[index].Unlock()
		return
	}
	ossData := s.DownLoad(fileName, ossDataLen)
	if ossData == nil {
		threadLocks[index].Unlock()
		return
	}
	//2. Write To Cache
	err = s.CreateFileWithFid(fileName, "", "")
	if err != nil {
		log.Fatalln(err)
	}
	threadLocks[index].Unlock()
	ossCacheRWLock.Lock()
	defer ossCacheRWLock.Unlock()
	token, err := s.WriteToCache(fileName, state, ossData)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("[GetFromOssAndWriteToCache] token:", token)
	err = s.SealFileAtCache(fileName, token, int32(len(ossData)))
	if err != nil {
		log.Fatalln(err)
	}
}

func (s *OssHolderServer) DownLoad(url string, ossDataLen int64) []byte {
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

func (s *OssHolderServer) SealFileAtCache(fid string, token string, size int32) error {
	err := s.dbOpsFile.CommitCacheFileInDB(fid, token, size)
	if err != nil {
		log.Printf("[ERROR] SealFileAtCache: Seal file(%s) failed.", fid)
		return err
	}
	return nil
}

func (s *OssHolderServer) EnqueueDownloadReq(fileName string) {

}

func (s *OssHolderServer) EnqueueEvictionReq() {

}

func (s *OssHolderServer) ReadFromCache(fileName string, offset int32, size int32) ([]byte, error) {
	ossCacheRWLock.RLock()
	var fm *definition.FileMeta
	var t time.Time
	fm, t, err := s.ListFile(fileName, int32(definition.F_DB_STATE_READY))
	if err != nil {
		log.Fatalln(err)
	}
	if fm == nil || fm.Id == "" {
		fm, err = s.createFileForCache(fileName)
		if err != nil {
			log.Fatalln(err)
		}
		if fm == nil || fm.Id == "" {
			//TODO, merge the async queries within and exceeding "threadNums" number of goroutines
			err := writeChanWithTimeout(struct{}{}, syncCh)
			if err == nil {
				ossCacheRWLock.RUnlock()
				go OssServer.GetFromOssAndWriteToCache(fileName, true)
				return nil, nil
			} else {
				// timeout
				ossCacheRWLock.RUnlock()
				lock.Lock()
				blockSlice = append(blockSlice, fileName)
				cond.Broadcast()
				lock.Unlock()
			}
		} else {
			ossCacheRWLock.RUnlock()
			log.Printf("[ReadFromCache] File: %s is downloading\n", fileName)
		}
		return nil, nil
	}
	fid := fileName
	if fm.RngCodeList == nil {
		log.Println("[ReadFromCache] fm.RngCodeList is nil")
		ossCacheRWLock.RUnlock()
		return nil, nil
	}

	fr := files.FileReader{
		Pbh:    PhyBH,
		FileDb: &s.dbOpsFile,
	}
	var readBytes []byte
	offset = fm.RngCodeList.Front().Value.(range_code.RangeCode).Start
	size = fm.RngCodeList.Front().Value.(range_code.RangeCode).End
	log.Printf("[ReadFromCache] read from start=%v  end=%v\n", offset, size)
	if readBytes, err = fr.ReadFromCache(fid, offset, size, fm.RngCodeList); err != nil {
		ZapLogger.Error("[ReadFromCache] faild:",
			zap.Any("err", err))
		ossCacheRWLock.RUnlock()
		return nil, err
	}
	ossCacheRWLock.RUnlock()
	return readBytes, nil
}

// file_handler end
//////////////////////////////

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
	// Physical blob Handler:
	PhyBH = new(blobs.PhyBH)
	PhyBH.New(ShardID)
	syncCh = make(chan struct{}, threadNums)
	threadLocks = make([]sync.Mutex, threadNums)
	blockSlice = make([]string, 0)
	cond = sync.NewCond(&lock)
	fmt.Println(" ")
	ZapLogger.Debug("[init] End of PhyBH init:",
		zap.Any("PhyBH", PhyBH))
	fmt.Println(" ")

	// HolderServer
	OssServer.dbOpsFile.Init()
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

func BackgroundThread() {
	for {
		lock.Lock()
		for len(blockSlice) == 0 {
			cond.Wait()
		}
		fileName := blockSlice[0]
		blockSlice = blockSlice[1:]
		log.Println("[BackgroundThread] blockSlice Size: ", len(blockSlice))
		lock.Unlock()
		OssServer.GetFromOssAndWriteToCache(fileName, false)
	}
}

func main() {
	flag.Parse()
	RegisterHttpHandler()
	cond = sync.NewCond(&lock)
	go BackgroundThread()
	err := http.ListenAndServe(Address, nil)
	if err != nil {
		fmt.Println("Listen to http requests failed", err)
	}
}
