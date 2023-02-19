// ///////////////////////////////////////
// 2022 PJLab Storage all rights reserved
// ///////////////////////////////////////
package config

import (
	"encoding/xml"
	"io/ioutil"
	"log"
	"os"

	"github.com/common/definition"
)

type OssConfig struct {
	XMLName          xml.Name         `xml:"oss_server_config"`
	OssHolderConfigs OssHolderConfigs `xml:"oss_holder_config"`
	OssCommonConfigs OssCommonConfigs `xml:"oss_common_config"`
}

type OssCommonConfigs struct {
	Is4kAlign               bool   `xml:"oss_4k_align"`
	CacheMaxSizeMB          int64  `xml:"oss_max_cache_size_mb"`
	TripletClosingThreshold int    `xml:"oss_triplet_closing_threshold_mb"`
	TripletLargeThreshold   int    `xml:"oss_triplet_large_threshold_mb"`
	DbNum                   uint32 `xml:"oss_db_num"`
}

type OssHolderConfigs struct {
	ConfigFlag             string      `xml:"oss_sub_sys_name,attr"`
	OssHolders             []OssHolder `xml:"oss_holder"`
	OssBlobLocalPathPrefix string      `xml:"oss_blob_local_path_prefix"`
}

type OssHolder struct {
	OssHolderIndex string `xml:"oss_holder_index,attr"`
	OssHolderIp    string `xml:"oss_holder_ip"`
	OssHolderPort  string `xml:"oss_holder_port"`
}

// The 'loadXMLConfig()' is used to load xml file which include the configuration.
func (cfg *OssConfig) LoadXMLConfig(config_path string) {
	if config_path == "" {
		config_path = "oss_server_config.xml"
	}
	xmlFile, err := os.Open(config_path)
	if err != nil {
		log.Fatalf("Error opening Oss XML file! path: %v\n", config_path)
	}
	defer xmlFile.Close()

	xmlData, err := ioutil.ReadAll(xmlFile)
	if err != nil {
		log.Fatalln("Error reading XML data:", err)
	}

	xml.Unmarshal(xmlData, &cfg)
	log.Println("XMLName : ", cfg.XMLName)

	num_of_Holders := len(cfg.OssHolderConfigs.OssHolders)
	log.Println("num_of_Oss_Holders : ", num_of_Holders)
	log.Println("OssHolderConfigs : ", cfg.OssHolderConfigs)

	cfg.ParseXMLConfig2Definition()
	log.Println()

}

func (cfg *OssConfig) ParseXMLConfig2Definition() {

	// holder

	definition.BlobLocalPathPrefix = cfg.OssHolderConfigs.OssBlobLocalPathPrefix
	log.Println("BlobLocalPathPrefix : ", definition.BlobLocalPathPrefix)
	// holder end

	definition.Oss_dbNum = cfg.OssCommonConfigs.DbNum
	definition.F_4K_Align = cfg.OssCommonConfigs.Is4kAlign
	definition.F_CACHE_MAX_SIZE = int64(definition.K_MiB) * cfg.OssCommonConfigs.CacheMaxSizeMB
	definition.K_triplet_closing_threshold = int64(cfg.OssCommonConfigs.TripletClosingThreshold) * int64(definition.K_MiB)
	definition.K_triplet_large_threshold = int64(cfg.OssCommonConfigs.TripletLargeThreshold) * int64(definition.K_MiB)
	log.Println("Oss_dbNum : ", definition.Oss_dbNum)
	log.Println("F_4K_Align : ", definition.F_4K_Align)
	log.Println("F_CACHE_MAX_SIZE : ", definition.F_CACHE_MAX_SIZE)
	log.Println("K_triplet_closing_threshold : ", definition.K_triplet_closing_threshold)
	log.Println("K_triplet_large_threshold : ", definition.K_triplet_large_threshold)
}

func (cfg *OssConfig) ParseOssHolderConfigAddress(_shardID int) string {
	holders := cfg.OssHolderConfigs.OssHolders
	// fmt.Println("Holders : ", holders)
	holder := holders[_shardID]
	log.Println("OssHolders[", _shardID, "] : ", holder)

	_ip := holder.OssHolderIp
	_port := holder.OssHolderPort
	_address := _ip + ":" + _port

	return _address
}
