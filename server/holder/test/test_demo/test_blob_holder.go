/////////////////////////////////////////
// 2022 PJLab Storage all rights reserved
// Author: Chen Sun
/////////////////////////////////////////

package main

import (
	"bytes"
	"container/list"
	"errors"
	blobs "holder/src/blob_handler"
	dbops "holder/src/db_ops"
	"holder/src/file_handler"
	"log"

	"github.com/common/config"
	definition "github.com/common/definition"
	util "github.com/common/util"
)

func TEST_blob_index_ops() {
	ih := new(blobs.IndexHeader)
	ih.New(0, "xx433", false)
	ih.Put("0x884", 0, 1024)
	ih.Put("0x882", 1160, 1024)
	ih.Put("0x881", 2320, 1024)
	ih.Put("0x888", 3480, 1024)
	log.Printf("[TEST] Test func Get: %v\n", *ih.Get("0x884"))
	log.Printf("[TEST] Test func Get: %v\n", *ih.Get("0x882"))
	log.Printf("[TEST] Test func Get: %v\n", *ih.Get("0x881"))
	log.Printf("[TEST] Test func Get: %v\n", *ih.Get("0x888"))
	ih.Close()

	ih = new(blobs.IndexHeader)
	ih.New(0, "xx433", false)
	log.Printf("[TEST] Test func Get: %v\n", *ih.Get("0x884"))
	log.Printf("[TEST] Test func Get: %v\n", *ih.Get("0x882"))
	log.Printf("[TEST] Test func Get: %v\n", *ih.Get("0x881"))
	log.Printf("[TEST] Test func Get: %v\n", *ih.Get("0x888"))

	log.Println("[INFO] Test pass")
}

func TEST_physical_blob_holder_ops() {
	pbh := new(blobs.PhyBH)
	pbh.New(0)

	var err error
	var token1 string
	var token2 string
	var token3 string
	var token4 string
	var read []byte

	data1 := []byte("Baby like to eat,")
	data2 := []byte("The bite is so ease,")
	data3 := []byte("They need no chopsticks,")
	data4 := []byte("Baby enjoy the feast.")

	log.Printf("[TEST] Start Put operation")
	if token1, err = pbh.Put(util.ShordGuidGenerator(), data1); err != nil {
		panic(err)
	}
	if token2, err = pbh.Put(util.ShordGuidGenerator(), data2); err != nil {
		panic(err)
	}
	if token3, err = pbh.Put(util.ShordGuidGenerator(), data3); err != nil {
		panic(err)
	}
	if token4, err = pbh.Put(util.ShordGuidGenerator(), data4); err != nil {
		panic(err)
	}

	log.Printf("[TEST] Start Get operation")
	if read, err = pbh.Get(token1); err != nil {
		panic(err)
	}
	log.Printf("[TEST] read data: %v", string(read))

	if read, err = pbh.Get(token2); err != nil {
		panic(err)
	}
	log.Printf("[TEST] read data: %v", string(read))

	if read, err = pbh.Get(token3); err != nil {
		panic(err)
	}
	log.Printf("[TEST] read data: %v", string(read))

	if read, err = pbh.Get(token4); err != nil {
		panic(err)
	}
	log.Printf("[TEST] read data: %v", string(read))

	log.Printf("[TEST] Start Delete operation")
	if err = pbh.Delete(token1); err != nil {
		panic(err)
	}

	log.Printf("[TEST] Start Get operation")
	if read, err = pbh.Get(token1); len(read) > 0 {
		panic(errors.New("Deleted blob shall not be readable"))
	}

	// Tore down everything and reinitialize
	pbh = new(blobs.PhyBH)
	pbh.New(0)

	log.Printf("[TEST] Start Get operation")
	if read, err = pbh.Get(token1); len(read) > 0 {
		panic(errors.New("Deleted blob shall not be readable"))
	}
	if read, err = pbh.Get(token2); err != nil {
		panic(err)
	}

	log.Printf("[TEST] Test finished with all case succeeded close lru size %v\n", pbh.ClosedTplt.GetSize())

}

func TEST_file_io_ops() {
	fid := "mock_fid_" + util.ShordGuidGenerator()
	fm := definition.FileMeta{
		Name:      "mock_no_seg_fname.txt",
		Id:        fid,
		OwnerList: list.New(),
		// TODO: add the blob related code.
		BlobId:      "",
		RngCodeList: list.New(),
	}
	fm.OwnerList.PushBack("mock_pid")

	var dbOpsFile *dbops.DBOpsFile
	dbOpsFile = new(dbops.DBOpsFile)
	dbOpsFile.Init()
	err := dbOpsFile.CreateFileWithFidInDB(fm.Id, &fm)
	if err != nil {
		log.Fatal("[ERROR] file creation failed.")
	}

	pbh := new(blobs.PhyBH)
	pbh.New(0)
	var blbDb dbops.DBOpsBlobSeg
	blbDb.Init()
	fw := file_handler.FileWriter{
		Pbh:       pbh,
		BlobSegDb: &blbDb,
		FileDb:    dbOpsFile,
	}
	lvs := []byte("ILoveSushi")
	toi := []byte("TasteOfIndia")
	svs := []byte("SevenSpice")
	var offset int32 = 0
	if err = fw.WriteAt(fm.Id, offset, int32(len(lvs)), lvs); err != nil {
		log.Fatal(err)
	}
	offset += int32(len(lvs))
	if err = fw.WriteAt(fm.Id, offset, int32(len(toi)), toi); err != nil {
		log.Fatal(err)
	}
	offset += int32(len(toi))
	if err = fw.WriteAt(fm.Id, offset, int32(len(svs)), svs); err != nil {
		log.Fatal(err)
	}

	fw.Close(fm.Id)

	fr := file_handler.FileReader{
		Pbh:       pbh,
		BlobSegDb: &blbDb,
		FileDb:    dbOpsFile,
	}
	offset = 0
	var readBytes []byte
	if readBytes, err = fr.ReadAt(fm.Id, offset, int32(len(lvs))); err != nil {
		log.Fatal(err)
	}
	if bytes.Compare(readBytes, lvs) != 0 {
		log.Printf("[ERROR] inconsistent read: %v", readBytes)
	}

	offset += int32(len(lvs))
	if readBytes, err = fr.ReadAt(fm.Id, offset, int32(len(toi))); err != nil {
		log.Fatal(err)
	}
	if bytes.Compare(readBytes, toi) != 0 {
		log.Printf("[ERROR] inconsistent read: %v", readBytes)
	}

	offset += int32(len(toi))
	if readBytes, err = fr.ReadAt(fm.Id, offset, int32(len(svs))); err != nil {
		log.Fatal(err)
	}
	if bytes.Compare(readBytes, svs) != 0 {
		log.Printf("[ERROR] inconsistent read: %v", readBytes)
	}

	log.Println("[INFO] Test pass")
}

func main() {
	var cfg config.Config
	cfg.LoadXMLConfig("/home/!/rhinofs/server/server_config.xml")

	// TEST_blob_index_ops()
	TEST_physical_blob_holder_ops()
	//TEST_file_io_ops()
}
