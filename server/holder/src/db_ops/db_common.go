// //////////////////////////////
// 2022 SHLab all rights reserved
// //////////////////////////////

package db_ops

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/common/config"
	"github.com/common/zaplog"
	"go.uber.org/zap"
)

type Encoded []byte

/*
	In order to be compatible with original code, my varible definition as follows:

	var alldbConfigInfo *config.DBConfig
	var dbConfigInfo0 *config.DbBase
	var dbConfigInfo *config.Tablename
*/

var alldbConfigInfo *config.DBConfig
var dbConfigInfo0 *config.DbBase
var dbConfigInfo *config.TableName

var ShardID int
var Address string

// Should be "mysql"
var driverName string

var dataSourceName string

func argsfunc() {
	log.Println("main input:")
	for idx, args := range os.Args {
		log.Println("    param", strconv.Itoa(idx), ":", args)
	}
	var err error
	ShardID, err = strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatalln(err)
	}
}

func init() {
	argsfunc()
	dir, err := os.Getwd()
	if err != nil {
		log.Fatalf("os.Getwd() error! \n")
	}
	var dirDBConfig string
	dirDBConfig = dir + "/../oss_db_config.xml"
	log.Println("Directory of oss_db_config file:", dirDBConfig)

	alldbConfigInfo = config.ParseDBConfig(dirDBConfig)
	dbConfigInfo0 = &alldbConfigInfo.DbBases[ShardID]
	dataSourceName = fmt.Sprintf("%s:%s@%s(%s:%s)/%s",
		dbConfigInfo0.Username, dbConfigInfo0.Password, dbConfigInfo0.IPProtocol,
		dbConfigInfo0.IPAddress, dbConfigInfo0.Port, dbConfigInfo0.DBName)
	driverName = dbConfigInfo0.DBType
	dbIndex := dbConfigInfo0.DbBaseIndex
	dbConfigInfo = &dbConfigInfo0.Table_name
	if alldbConfigInfo == nil || dbConfigInfo0 == nil || dbConfigInfo == nil {
		log.Fatalf("null pointer error! \n")
	}
	zaplog.ZapLogger.Debug("****************************")
	zaplog.ZapLogger.Debug("", zap.Any("driverName", driverName))
	zaplog.ZapLogger.Debug("", zap.Any("dataSourceName", dataSourceName))
	zaplog.ZapLogger.Debug("", zap.Any("dbIndex", dbIndex))
	zaplog.ZapLogger.Debug("", zap.Any("dbConfigInfo0.Table_name.FileTableName",
		dbConfigInfo0.Table_name.FileTableName))
}
