// //////////////////////////////
// 2022 SHLab all rights reserved
// //////////////////////////////

package db_ops

import (
	"container/list"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	. "github.com/common/zaplog"
	"go.uber.org/zap"

	definition "github.com/common/definition"
	range_code "github.com/common/range_code"
	"github.com/common/util"

	_ "github.com/go-sql-driver/mysql"
)

type DBOpsFile struct {
	mc       []*sql.DB
	RWLock   *sync.RWMutex
	ConnLeft int
}

func (opsFile *DBOpsFile) New() {
	ZapLogger.Debug("", zap.String("driverName", driverName),
		zap.String("dataSourceName", dataSourceName))

	for i := 0; i < definition.F_NUM_DB_CONN_OBJ; i++ {
		mc, dbErr := sql.Open(driverName, dataSourceName)
		if dbErr != nil {
			ZapLogger.Fatal(
				"DB connection",
				zap.Any("driverName", driverName),
				zap.Any("dataSourceName", dataSourceName),
				zap.Any("error", dbErr))
		}
		mc.SetMaxOpenConns(2000)
		mc.SetMaxIdleConns(1000)
		mc.SetConnMaxLifetime(time.Minute * 60)
		opsFile.mc = append(opsFile.mc, mc)
	}
	opsFile.RWLock = new(sync.RWMutex)
	opsFile.ConnLeft = definition.F_NUM_MAX_FILES_DB_CONN
	ZapLogger.Info("*DBOpsFile.Init() OK.")
}

// Transaction uses a special pool, or a special single connection.
// TODO: need to refactor, single connection for txn isn't enough
func (opsFile *DBOpsFile) GetConnForTxn() *sql.DB {
	return opsFile.mc[0]
}

func (opsFile *DBOpsFile) GetConn() (*sql.DB, error) {
	if opsFile.mc == nil {
		return nil, errors.New("initialization incomplete")
	}
	opsFile.RWLock.Lock()
	defer opsFile.RWLock.Unlock()
	if opsFile.ConnLeft > 0 {
		opsFile.ConnLeft--
		return opsFile.mc[rand.Intn(definition.F_NUM_DB_CONN_OBJ)], nil
	}
	return nil, errors.New("mysql connection exhausted")
}

func (opsFile *DBOpsFile) ReleaseConn() {
	opsFile.RWLock.Lock()
	opsFile.ConnLeft++
	defer opsFile.RWLock.Unlock()
}

func (opsFile *DBOpsFile) GetConnWithRetry() *sql.DB {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 1 * time.Minute

	var val *sql.DB
	var err error

	retryable := func() error {
		val, err = opsFile.GetConn()
		if err != nil {
			return err
		}
		return nil
	}

	notify := func(err error, t time.Duration) {
		ZapLogger.Error("", zap.Any("error happened", err), zap.Any("time", t))
	}

	err = backoff.RetryNotify(retryable, b, notify)
	if err != nil {
		ZapLogger.Fatal("", zap.Any("fatal after retry", err))
	}
	return val
}

type DBFileMeta struct {
	// eg: /f1
	Name string
	// a unique ID
	Id string
	// File's owner can be parent folder, or tags.
	// A file shall only have 1 parent folder,
	// but can have many tags pointing to it.
	OwnerList string
	// This is the reference to blobs meta. Blob is a heavy binary sitting
	// on distributed data storage.
	BlobId string

	RngList string

	Etag string
}

func DBFileMeta2FileMeta(dbfm *DBFileMeta) definition.FileMeta {
	ownerStr := dbfm.OwnerList
	ownerArr := strings.Split(ownerStr, ",")
	ownerll := list.New()
	for i := 0; i < len(ownerArr); i++ {
		ownerll.PushBack(ownerArr[i])
	}

	rngll := list.New()
	var fm definition.FileMeta = definition.FileMeta{
		Name:        dbfm.Name,
		Id:          dbfm.Id,
		OwnerList:   ownerll,
		BlobId:      dbfm.BlobId,
		RngCodeList: rngll,
		Etag:        dbfm.Etag,
	}

	if dbfm.RngList == "" {
		return fm
	}

	rngStr := dbfm.RngList
	rngArr := strings.Split(rngStr, definition.K_rng_code_dlmtr)
	for i := 0; i < len(rngArr); i++ {
		if len(rngArr[i]) == 0 {
			continue
		}
		rngll.PushBack(range_code.ToRangeCode(rngArr[i]))
	}

	fm.RngCodeList = rngll

	return fm
}

func FileMeta2DBFileMeta(fm *definition.FileMeta) DBFileMeta {
	ownerStr, rngStr := "", ""
	if fm.OwnerList != nil {
		for e := fm.OwnerList.Front(); e != nil; e = e.Next() {
			ownerStr += fmt.Sprintf("%v", e.Value)
			if e.Next() != nil {
				ownerStr += ","
			}
		}
	}

	dbfm := DBFileMeta{
		Name:      fm.Name,
		Id:        fm.Id,
		OwnerList: ownerStr,
		// TODO: add the blob related code.
		BlobId:  fm.BlobId,
		RngList: "",
		Etag:    fm.Etag,
	}

	if fm.RngCodeList == nil {
		return dbfm
	}

	for e := fm.RngCodeList.Front(); e != nil; e = e.Next() {
		rngStr += fmt.Sprintf("%v", e.Value.(range_code.RangeCode).ToJson())
		if e.Next() != nil {
			rngStr += definition.K_rng_code_dlmtr
		}
	}

	dbfm.RngList = rngStr

	return dbfm
}

// ////////////////////////////
// DB(dir_service.files) Ops
// ////////////////////////////

func (opsFile *DBOpsFile) ListFileFromDB(fileId string, state int32) (*definition.FileMeta, error) {
	// Prepare ctx for executing query.
	var ctx context.Context
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	rows, qErr := opsFile.GetConnWithRetry().QueryContext(ctx,
		"SELECT file_meta FROM "+dbConfigInfo.FileTableName+" WHERE fid = ? AND state = ?;",
		fileId, state)
	opsFile.ReleaseConn()

	if qErr != nil {
		log.Printf("[ERROR][ListFileFromDB] Query file_meta from DB by fid(%s) + state(%d) failed: %v",
			fileId, state, qErr)
		return nil, qErr
	}
	defer rows.Close()

	var encoded []byte
	if rows.Next() {
		if err := rows.Scan(&encoded); err != nil {
			log.Fatal(err)
		}
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}

	if len(encoded) == 0 {
		return nil, nil
	}

	var fm definition.FileMeta
	var dbfm DBFileMeta
	jsErr := json.Unmarshal(encoded, &dbfm)
	if jsErr != nil {
		log.Printf(
			"ERROR:[ListFileFromDB] Convert db string(%s) to dbfm failed: %v",
			encoded, jsErr)
		return nil, jsErr
	}

	//DBres handle
	fm = DBFileMeta2FileMeta(&dbfm)
	return &fm, nil
}

func (opsFile *DBOpsFile) ListFileAndStateFromDB(fileId string) (*definition.FileMeta, int, error) {
	// Prepare ctx for executing query.
	var ctx context.Context
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	rows, qErr := opsFile.GetConnWithRetry().QueryContext(ctx,
		"SELECT file_meta, state FROM "+dbConfigInfo.FileTableName+" WHERE fid = ?;",
		fileId)
	opsFile.ReleaseConn()

	if qErr != nil {
		log.Printf("[ERROR][ListFileAndStateFromDB] Query file_meta from DB by fid(%s) + failed: %v",
			fileId, qErr)
		return nil, -1, qErr
	}
	defer rows.Close()

	var encoded []byte
	var state int
	if rows.Next() {
		if err := rows.Scan(&encoded, &state); err != nil {
			log.Fatal(err)
		}
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}

	if len(encoded) == 0 {
		return nil, -1, nil
	}

	var fm definition.FileMeta
	var dbfm DBFileMeta
	jsErr := json.Unmarshal(encoded, &dbfm)
	if jsErr != nil {
		log.Printf(
			"ERROR:[ListFileAndStateFromDB] Convert db string(%s) to dbfm failed: %v",
			encoded, jsErr)
		return nil, -1, jsErr
	}

	//DBres handle
	fm = DBFileMeta2FileMeta(&dbfm)
	return &fm, state, nil
}

func (opsFile *DBOpsFile) CreateFileWithFidInDB(fileId string, fileMeta *definition.FileMeta) error {
	// Prepare ctx for executing query.
	var ctx context.Context
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	log.Println("[DEBUG] fileId:{", fileId, "} fileMeta:{", fileMeta, "} owners:{", fileMeta.OwnerList, "}")
	dbfm := FileMeta2DBFileMeta(fileMeta)
	log.Println("[DEBUG] fileId:{", fileId, "} fileMeta:{", dbfm, "} owners:", dbfm.OwnerList, "}")

	var encoded []byte
	encoded, jsErr := json.Marshal(&dbfm)
	if jsErr != nil {
		log.Printf("[ERROR][CreateFileWithFidInDB] Convert file_meta(%v) to json string failed: %v", dbfm, jsErr)
		return jsErr
	}
	rows, qErr := opsFile.GetConnWithRetry().QueryContext(ctx,
		"INSERT INTO "+dbConfigInfo.FileTableName+" (fid, file_meta, owners, state) VALUES (?, ?, ?, ?);",
		fileId, encoded, dbfm.OwnerList, definition.F_DB_STATE_PENDING)
	opsFile.ReleaseConn()

	if qErr != nil {
		log.Printf("[ERROR][CreateFileWithFidInDB] Insert file_meta(%v) to DB failed: %v", dbfm, qErr)
		return qErr
	}
	defer rows.Close()

	return nil
}

func (opsFile *DBOpsFile) ListFileAndOwnersFromDB(fileId string) (*definition.FileMeta, string, error) {
	// Prepare ctx for executing query.
	var ctx context.Context
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	rows, qErr := opsFile.GetConnWithRetry().QueryContext(ctx,
		"SELECT file_meta, owners FROM "+dbConfigInfo.FileTableName+" WHERE fid = ?",
		fileId)
	opsFile.ReleaseConn()

	if qErr != nil {
		log.Printf("[ERROR][ListFileAndOwnersFromDB] Query file_meta, owners from DB by fide(%s) failed: %v",
			fileId, qErr)
		return nil, "", qErr
	}
	defer rows.Close()

	var encoded []byte
	var owners string
	if rows.Next() {
		if err := rows.Scan(&encoded, &owners); err != nil {
			log.Fatal(err)
		}
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}

	var fm definition.FileMeta
	var dbfm DBFileMeta
	jsErr := json.Unmarshal(encoded, &dbfm)
	if jsErr != nil {
		log.Printf("[ERROR][ListFileAndOwnersFromDB] Convert file_meta(%v) to json string failed: %v", dbfm, jsErr)
		return nil, "", jsErr
	}

	//DBres handle
	fm = DBFileMeta2FileMeta(&dbfm)

	return &fm, dbfm.OwnerList, nil
}

func (opsFile *DBOpsFile) UpdateFilemetaAndOwnerInDB(fileId string, dbfm *DBFileMeta) error {
	// Prepare ctx for executing query.
	var ctx context.Context
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	var encoded []byte
	encoded, jsErr := json.Marshal(&dbfm)
	if jsErr != nil {
		log.Printf("[ERROR][UpdateFilemetaAndOwnerInDB] Convert file_meta(%v) to json string failed: %v", dbfm, jsErr)
		return jsErr
	}

	rows, qErr := opsFile.GetConnWithRetry().QueryContext(ctx,
		"UPDATE "+dbConfigInfo.FileTableName+" SET file_meta = ?, owners = ? WHERE fid = ?;",
		encoded, dbfm.OwnerList, fileId)
	opsFile.ReleaseConn()

	if qErr != nil {
		log.Printf("[ERROR][UpdateFilemetaAndOwnerInDB] UPDATE file_meta(%v), owners(%s) to DB failed: %v", dbfm, dbfm.OwnerList, qErr)
		return qErr
	}
	defer rows.Close()

	return nil
}

func (opsFile *DBOpsFile) UpdateFilemetaAndStateInDB(fileName string,
	fileMeta *definition.FileMeta, state int) error {
	// Prepare ctx for executing query.
	var ctx context.Context
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	dbfm := FileMeta2DBFileMeta(fileMeta)

	var encoded []byte
	encoded, jsErr := json.Marshal(&dbfm)
	if jsErr != nil {
		log.Printf("[ERROR][UpdateFilemetaAndStateInDB]"+
			"Convert file_meta(%v) to json string failed: %v", dbfm, jsErr)
		return jsErr
	}

	rows, qErr := opsFile.GetConnWithRetry().QueryContext(ctx,
		"UPDATE "+dbConfigInfo.FileTableName+" SET file_meta = ?, state = ? WHERE fid = ?;",
		encoded, state, fileName)
	opsFile.ReleaseConn()

	if qErr != nil {
		log.Printf("[ERROR][UpdateFilemetaAndStateInDB]"+
			"UPDATE file_meta(%v), state(%d) to DB failed: %v", dbfm, state, qErr)
		return qErr
	}
	defer rows.Close()

	return nil
}

// Check file if it's full moon (all ranges are filled). If yes, update file state.
// TODO: If too many blobs, easily this query slow & timeout.
func (opsFile *DBOpsFile) CommitFileInDB(fid string) error {
	// Prepare ctx for executing query.
	var ctx context.Context
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	// Transaction locks: file entry
	// Transaction updates: file entry
	tx, err := opsFile.GetConnForTxn().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	// Defer a rollback in case anything fails.
	defer tx.Rollback()

	// Query file entry to fetch lunar hash list.
	row, qErr := tx.QueryContext(ctx,
		"SELECT file_meta FROM "+dbConfigInfo.FileTableName+" WHERE fid = ? FOR UPDATE",
		fid)
	if qErr != nil {
		log.Printf(
			"[ERROR] CommitFileInDB Lock file(%s) in DB failed: %v",
			fid, qErr)
		return qErr
	}
	// Extract file meta.
	var encoded []byte
	if row.Next() {
		if err := row.Scan(&encoded); err != nil {
			log.Fatal(err)
		}
	}
	if err := row.Err(); err != nil {
		log.Fatal(err)
	}
	row.Close()
	var fm definition.FileMeta
	var dbfm DBFileMeta
	jsErr := json.Unmarshal(encoded, &dbfm)
	if jsErr != nil {
		log.Printf(
			"ERROR:[ListFileFromDB] Convert db string(%s) to dbfm failed: %v",
			encoded, jsErr)
		return jsErr
	}

	//DBres handle
	fm = DBFileMeta2FileMeta(&dbfm)

	if dbfm.RngList != "" && !IsRangeFullCoverage(fm.RngCodeList) {
		log.Printf("[ERROR] CommitFileInDB file(%s) not ready to commit", fid)
		return errors.New("file not ready to commit")
	}

	_, qErr = tx.ExecContext(
		ctx, "UPDATE "+dbConfigInfo.FileTableName+" SET state = ? WHERE fid = ?", definition.F_DB_STATE_READY, fid)
	if qErr != nil {
		log.Printf(
			"[ERROR] CommitFileInDB failed on fid(%s): %v", fid, qErr)
		return qErr
	}

	// Commit the transaction.
	if err = tx.Commit(); err != nil {
		return err
	}
	log.Printf("[INFO] Successfully committed file in DB: %s", fid)
	return nil
}

// Check file if it's full moon (all ranges are filled). If yes, update file state.
// TODO: If too many blobs, easily this query slow & timeout.
func (opsFile *DBOpsFile) CommitCacheFileInDB(
	fid, token string, size int32) error {
	// Prepare ctx for executing query.
	var ctx context.Context
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	// Transaction locks: file entry
	// Transaction updates: file entry
	tx, err := opsFile.GetConnForTxn().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	// Defer a rollback in case anything fails.
	defer tx.Rollback()

	// Query file entry to fetch lunar hash list.
	row, qErr := tx.QueryContext(ctx,
		"SELECT file_meta FROM "+dbConfigInfo.FileTableName+" WHERE fid = ? FOR UPDATE",
		fid)
	if qErr != nil {
		log.Printf(
			"[ERROR] CommitCacheFileInDB Lock file(%s) in DB failed: %v",
			fid, qErr)
		return qErr
	}
	// Extract file meta.
	var encoded []byte
	if row.Next() {
		if err := row.Scan(&encoded); err != nil {
			log.Fatal(err)
		}
	}
	if err := row.Err(); err != nil {
		log.Fatal(err)
	}
	row.Close()
	var fm definition.FileMeta
	var dbfm DBFileMeta
	jsErr := json.Unmarshal(encoded, &dbfm)
	if jsErr != nil {
		log.Printf(
			"ERROR:[CommitCacheFileInDB] Convert db string(%s) to dbfm failed: %v",
			encoded, jsErr)
		return jsErr
	}
	fm = DBFileMeta2FileMeta(&dbfm)
	rngCode := range_code.RangeCode{
		Start: 0,
		End:   size,
		Token: token,
	}
	tid := util.GetTripletIdFromToken(token)
	fm.RngCodeList = list.New()
	fm.RngCodeList.PushBack(rngCode)
	dbfm = FileMeta2DBFileMeta(&fm)
	encoded, jsErr = json.Marshal(&dbfm)
	if jsErr != nil {
		log.Printf(
			"ERROR:[CommitCacheFileInDB] Convert file_meta(%v) to json string failed: %v",
			fm, jsErr)
		return jsErr
	}
	_, qErr = tx.ExecContext(
		ctx,
		"UPDATE "+dbConfigInfo.FileTableName+" SET state = ?, owners = ?,file_meta = ? WHERE fid = ?",
		definition.F_DB_STATE_READY, tid, encoded, fid)
	if qErr != nil {
		log.Printf(
			"[ERROR] CommitCacheFileInDB failed on fid(%s): %v", fid, qErr)
		return qErr
	}

	// Commit the transaction.
	if err = tx.Commit(); err != nil {
		return err
	}
	log.Printf("[INFO] Successfully committed cache file in DB: %s", fid)
	return nil
}

func (opsFile *DBOpsFile) TagFileInDB(fileId string, tagId string) error {
	fm, owners, errorListFileAndOwnersFromDB := opsFile.ListFileAndOwnersFromDB(fileId)
	owner_slices := strings.Split(owners, ",")
	if errorListFileAndOwnersFromDB != nil {
		log.Printf("[ERROR][TagFileInDB] Query file_meta, owners by fid(%v) failed: %v", fileId, errorListFileAndOwnersFromDB)
		return errorListFileAndOwnersFromDB
	}

	i := 0
	list_exit := false
	for e := fm.OwnerList.Front(); e != nil; e = e.Next() {
		if e.Value != owner_slices[i] {
			errMsg := fmt.Sprintf(
				"[ERROR][TagFileInDB] fm.OwnerList[%s] != owner_slices[%s].",
				e.Value, owner_slices[i])
			dbDateErr := errors.New(errMsg)
			return dbDateErr
		}
		i++
		if e.Value == tagId {
			list_exit = true
			break
		}
	}
	var dbfm DBFileMeta
	if !list_exit {
		fm.OwnerList.PushBack(tagId)
		dbfm = FileMeta2DBFileMeta(fm)
		errorUpdateFilemetaAndOwnerInDB := opsFile.UpdateFilemetaAndOwnerInDB(fileId, &dbfm)
		return errorUpdateFilemetaAndOwnerInDB
	}
	return nil
}

// Function: Delete a file entry with fileId in files2 table.
//
// Real ops: Update files.state by fileId
//
// Input: fileId
//
// Output: error
func (opsFile *DBOpsFile) DeleteFileWithFidInDB(fileId string) error {
	// Prepare ctx for executing query.
	var ctx context.Context
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	//update filed
	time := strconv.FormatInt(time.Now().UnixMilli(), 10)
	trashFileId := util.GetHashedIdFromStr("trash") + "_" + fileId + "_" + time

	// Op DB.table
	rows, qErr := opsFile.GetConnWithRetry().QueryContext(ctx,
		"UPDATE "+dbConfigInfo.FileTableName+" SET fid = ?, state = ? WHERE fid = ? AND state = ?;",
		trashFileId, definition.F_DB_STATE_DELETED,
		fileId, definition.F_DB_STATE_READY)
	opsFile.ReleaseConn()

	if qErr != nil {
		log.Printf("[ERROR][DeleteFileWithFidInDB] UPDATE files.state by fid(%s), state(%d) to DB failed: %v", fileId, definition.F_DB_STATE_READY, qErr)
		return qErr
	}
	defer rows.Close()

	return nil

}

func (opsFile *DBOpsFile) DeleteFileWithTripleIdInDB(tripleId string) error {
	// Prepare ctx for executing query.
	var ctx context.Context
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	_, err := opsFile.GetConnWithRetry().ExecContext(ctx,
		"DELETE FROM "+dbConfigInfo.FileTableName+" WHERE owners = ?;",
		tripleId)
	opsFile.ReleaseConn()
	if err != nil {
		log.Printf("[ERROR][DeleteFileWithTripleIdInDB] DELETE files.state by tripleId(%s) to DB failed: %v", tripleId, err)
		return err
	}
	return nil
}

func (opsFile *DBOpsFile) DeletePendingFileWithFIdInDB(fileId string) error {
	// Prepare ctx for executing query.
	var ctx context.Context
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	_, err := opsFile.GetConnWithRetry().ExecContext(ctx,
		"DELETE FROM "+dbConfigInfo.FileTableName+" WHERE fid = ? AND state = ?;",
		fileId, definition.F_BLOB_STATE_PENDING)
	opsFile.ReleaseConn()
	if err != nil {
		log.Printf("[ERROR][DeletePendingFileWithFIdInDB] DELETE pending file by fileId(%s) to DB failed: %v", fileId, err)
		return err
	}
	return nil
}

func (opsFile *DBOpsFile) DeleteAllPendingFileInDB() error {
	// Prepare ctx for executing query.
	var ctx context.Context
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	_, err := opsFile.GetConnWithRetry().ExecContext(ctx,
		"DELETE FROM "+dbConfigInfo.FileTableName+" WHERE state = ?;",
		definition.F_BLOB_STATE_PENDING)
	opsFile.ReleaseConn()
	if err != nil {
		log.Printf("[ERROR][DeleteAllPendingFileInDB] DELETE files.state F_BLOB_STATE_PENDING to DB failed: %v", err)
		return err
	}
	return nil
}

func (opsFile *DBOpsFile) ListTripleIdOfAllFiles() ([]string, error) {
	// Prepare ctx for executing query.
	var ctx context.Context
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	rows, err := opsFile.GetConnWithRetry().QueryContext(ctx,
		"SELECT DISTINCT owners FROM "+dbConfigInfo.FileTableName+";")
	opsFile.ReleaseConn()
	if err != nil {
		log.Printf("[ERROR][ListTripleIdOfAllFiles] DB failed: %v", err)
		return nil, err
	}
	defer rows.Close()
	res := make([]string, 0)
	for rows.Next() {
		var tripleId string
		rows.Scan(&tripleId)
		res = append(res, tripleId)
	}

	return res, nil
}

// ////////////////////////////

// /**************************************************
// # Below are db schema creation.
// CREATE DATABASE dir_service;
// use dir_service;

// create table oss_files (
// 	fid varchar(255) NOT NULL DEFAULT "",
// 	file_meta json DEFAULT NULL,
//     owners varchar(64) DEFAULT "",
//     state tinyint(1) NOT NULL DEFAULT 0,
//     created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
//     updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
//     PRIMARY KEY (fid),
//     INDEX created_at_fid (created_at, fid), owners (owners)
// );
// CREATE TABLE `ss_files` (
// 	`fid` varchar(32) NOT NULL DEFAULT '',
// 	`file_meta` json DEFAULT NULL,
// 	`owners` varchar(255) DEFAULT '',
// 	`state` tinyint(1) NOT NULL DEFAULT '0',
// 	`created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
// 	`updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
// 	PRIMARY KEY (`fid`),
// 	KEY `created_at_fid` (`created_at`,`fid`),
// 	KEY `tplt_id` (`owners`)
// );

//
