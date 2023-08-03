// ///////////////////////////////////////////////
// 2023 Shanghai AI Laboratory all rights reserved
// ///////////////////////////////////////////////

package db_ops

import (
	"container/list"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"time"

	. "github.com/common/zaplog"
	_ "github.com/common/zaplog"

	definition "github.com/common/definition"
	range_code "github.com/common/range_code"
	"github.com/common/util"
	"go.uber.org/zap"
)

type DBOpsBlobSeg struct {
	mc []*sql.DB
}

func (opsBlb *DBOpsBlobSeg) Init() {
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
		opsBlb.mc = append(opsBlb.mc, mc)
	}
}

func (opsBlb *DBOpsBlobSeg) GetConnForTxn() *sql.DB {
	return opsBlb.mc[0]
}

func (opsBlb *DBOpsBlobSeg) GetConn() *sql.DB {
	return opsBlb.mc[rand.Intn(definition.F_NUM_DB_CONN_OBJ)]
}

// TODO: We currently don't support mutable blob. So there is no inclusive
// rangers.
// For newly created, we always create as pending. Pending blob records is for
// easier GC.
// Pending blob uses a token that purely contains blob id. After blob commits,
// we turn token into the final token which contains both triplet id and blob
// id and also modify its state to ready.
func (opsBlb *DBOpsBlobSeg) CreateBlobSegInDB(
	rng []int32,
	fileId string,
	// User shall create blob id before passing in as token.
	partialToken string) error {
	hashObj := range_code.RangeCode{
		Start: rng[0],
		End:   rng[1],
		Token: partialToken,
	}

	bMeta := definition.BlobMeta{
		RngCode: hashObj,
		OwnerId: fileId,
	}
	// Prepare ctx for executing query.
	var ctx context.Context
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	var encoded []byte
	encoded, jsErr := json.Marshal(&bMeta)
	if jsErr != nil {
		log.Printf(
			"ERROR:[CreateBlobSgmntInDB] Convert blob_meta(%v) to json string failed: %v",
			bMeta, jsErr)
		return jsErr
	}

	rows, qErr := opsBlb.GetConn().QueryContext(
		ctx,
		"INSERT INTO "+dbConfigInfo.SegmentTableName+" (parent_id, child_name, seg_meta, state) VALUES (?, ?, ?, ?);",
		fileId, hashObj.ToDbEntry(), encoded, definition.F_BLOB_STATE_PENDING)
	if qErr != nil {
		log.Printf(
			"ERROR:[CreateBlobSgmntInDB] Insert blob_meta(%v) to DB failed: %v",
			bMeta, qErr)
		return qErr
	}
	defer rows.Close()

	return nil
}

// TODO: currently we assume no versioning, and ranges are non-overlapped.
// Returned BlobMeta are sorted by data range.
func (opsBlb *DBOpsBlobSeg) ListBlobSegsByFidFromDB(fid string) (*[]definition.BlobMeta, error) {
	// Prepare ctx for executing query.
	var ctx context.Context
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	// Ordering by child_name will order by the offset.
	rows, qErr := opsBlb.GetConn().QueryContext(ctx,
		"SELECT seg_meta FROM "+dbConfigInfo.SegmentTableName+" WHERE parent_id = ? ORDER BY child_name",
		fid)
	if qErr != nil {
		log.Printf(
			"[ERROR] ListBlobSegsByFidFromDB Query seg_meta from DB by "+
				"fid(%s) failed: %v",
			fid, qErr)
		return nil, qErr
	}
	defer rows.Close()
	log.Println("Query OK:", rows)

	bms := make([]definition.BlobMeta, 0)
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			log.Fatal(err)
		}
		if err := rows.Err(); err != nil {
			log.Fatal(err)
		}
		var bMeta definition.BlobMeta
		jsErr := json.Unmarshal(encoded, &bMeta)
		if jsErr != nil {
			log.Printf(
				"[ERROR] ListBlobSegsByFidFromDB Convert seg_meta(%v) to json string failed: %v",
				bMeta, jsErr)
			return nil, jsErr
		}
		bms = append(bms, bMeta)
	}

	return &bms, nil
}

// Update check if file already contains committed blob for this range. If clear
// change blob state, update blob child_name field from range+blb_id to
// range+triplet+blb_id.
func (opsBlb *DBOpsBlobSeg) CommitBlobInDB(
	rng []int32, fid string, fullToken string) error {
	// Prepare ctx for executing query.
	var ctx context.Context
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	// Transaction locks: file entry
	// Transaction updates: block entry.
	tx, err := opsBlb.GetConnForTxn().BeginTx(ctx, nil)
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
			"[ERROR] CommitBlobInDB Lock file(%s) in DB failed: %v",
			fid, qErr)
		return qErr
	}
	defer row.Close()
	// Extract file meta.
	var dbfm DBFileMeta
	var encoded []byte
	row.Next()
	if err := row.Scan(&encoded); err != nil {
		log.Fatal(err)
	}
	row.Close()
	jsErr := json.Unmarshal(encoded, &dbfm)
	if jsErr != nil {
		log.Printf(
			"[ERROR] CommitBlobInDB Convert json string(%s) to file_meta failed: %v",
			encoded, jsErr)
		return jsErr
	}
	fm := DBFileMeta2FileMeta(&dbfm)

	partialToken := util.Full2PartialToken(fullToken)
	rngCode := range_code.RangeCode{
		Start: rng[0],
		End:   rng[1],
		Token: partialToken,
	}
	oldCode := rngCode.ToDbEntry()
	row, qErr = tx.QueryContext(
		ctx, "SELECT seg_meta FROM "+dbConfigInfo.SegmentTableName+" WHERE parent_id = ? AND child_name = ?;", fid, oldCode)
	if qErr != nil {
		log.Printf(
			"[ERROR] CommitBlobInDB Query seg_meta from DB by "+
				"fid(%s) and child_name(%s) failed: %v",
			fid, oldCode, qErr)
		return qErr
	}
	// Extract blob meta.
	var bm definition.BlobMeta
	row.Next()
	if err := row.Scan(&encoded); err != nil {
		log.Fatal(err)
	}
	row.Close()
	jsErr = json.Unmarshal(encoded, &bm)
	if jsErr != nil {
		log.Printf(
			"[ERROR] CommitBlobInDB: Convert json string(%v) to blob_meta failed: %v",
			encoded, jsErr)
		return jsErr
	}
	if IsRangeCollision(fm.RngCodeList, bm.RngCode) {
		log.Printf(
			"[ERROR] CommitBlobInDB: Collision for blob(%s) ranger hash: %v",
			fullToken, bm.RngCode)
		return errors.New("range code collision")
	}

	rngCode.Token = fullToken
	newCode := rngCode.ToDbEntry()

	bm.RngCode.Token = fullToken
	var segMetaJson []byte
	segMetaJson, jsErr = json.Marshal(&bm)
	if jsErr != nil {
		log.Printf(
			"ERROR:[CreateBlobSgmntInDB] Convert blob_meta(%v) to json string failed: %v",
			bm, jsErr)
		return jsErr
	}

	_, qErr = tx.ExecContext(ctx,
		"UPDATE "+dbConfigInfo.SegmentTableName+" SET state = ?, child_name = ?, seg_meta = ?"+
			" WHERE parent_id = ? AND child_name = ?",
		definition.F_BLOB_STATE_READY, newCode, segMetaJson, fid, oldCode)
	if qErr != nil {
		log.Printf(
			"ERROR:[CommitBlobInDB] failed on child_name(%s): %v",
			partialToken, qErr)
		return qErr
	}
	InsertRangeCodeList(fm.RngCodeList, rngCode)
	dbfm = FileMeta2DBFileMeta(&fm)
	encoded, jsErr = json.Marshal(&dbfm)
	if jsErr != nil {
		log.Printf(
			"ERROR:[CreateBlobSgmntInDB] Convert file_meta(%v) to json string failed: %v",
			fm, jsErr)
		return jsErr
	}

	_, qErr = tx.ExecContext(
		ctx, "UPDATE "+dbConfigInfo.FileTableName+" SET file_meta = ? WHERE fid = ?", encoded, fid)
	if qErr != nil {
		log.Printf(
			"ERROR:[CommitBlobInDB] failed on child_name(%s): %v",
			partialToken, qErr)
		return qErr
	}

	// Commit the transaction.
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

// True means there is collision. False means no collision.
func IsRangeCollision(hList *list.List, rngCode range_code.RangeCode) bool {
	for h := hList.Front(); h != nil; h = h.Next() {
		if (h.Value.(range_code.RangeCode).Start >= rngCode.End) ||
			(h.Value.(range_code.RangeCode).End <= rngCode.Start) {
			continue
		} else {
			return true
		}
	}
	return false
}

func IsRangeFullCoverage(hList *list.List) bool {
	for h := hList.Front(); h.Next() != nil; h = h.Next() {
		if h.Value.(range_code.RangeCode).End !=
			h.Next().Value.(range_code.RangeCode).Start {
			return false
		}
	}
	return true
}

func InsertRangeCodeList(hList *list.List, rngCode range_code.RangeCode) {
	dummy := range_code.RangeCode{
		Start: -1,
		End:   -1,
		Token: "",
	}
	hList.PushFront(dummy)
	var h *list.Element
	for h = hList.Back(); h != nil; h = h.Prev() {
		var rc = h.Value.(range_code.RangeCode)
		if rc.End <= rngCode.Start {
			hList.InsertAfter(rngCode, h)
			hList.Remove(hList.Front())
			return
		}
	}
}
