// ///////////////////////////////////////
// 2022 SHAI Lab all rights reserved
// ///////////////////////////////////////
package config

import (
	"encoding/xml"
	"io/ioutil"
	"log"
	"os"
)

type DBConfig struct {
	XMLName xml.Name `xml:"db_config"`
	DbBases []DbBase `xml:"db_base"`
}

type DbBase struct {
	DbBaseIndex string    `xml:"db_base_index,attr"`
	DBType      string    `xml:"db_type"`
	Username    string    `xml:"username"`
	Password    string    `xml:"password"`
	IPProtocol  string    `xml:"ip_protocol"`
	DBName      string    `xml:"db_name"`
	IPAddress   string    `xml:"ip_address"`
	Port        string    `xml:"port"`
	Table_name  TableName `xml:"table_name"`
}

type TableName struct {
	SegmentTableName string `xml:"segments_table_name"`
	FileTableName    string `xml:"files_table_name"`
}

func ParseDBConfig(config_path string) *DBConfig {
	configFile := new(DBConfig)
	if config_path == "" {
		config_path = "db_config.xml"
	}
	xmlFile, err := os.Open(config_path)
	if err != nil {
		// this directory config is for unit test.
		log.Println("Error opening XML file! (", config_path, ")")
		dir, err := os.Getwd()
		if err != nil {
			log.Fatalf("Error: db_config.go [ParseDBConfig] os.Getwd() error! \n")
		}
		config_path = dir + "/../../../db_config.xml"
		xmlFile, err = os.Open(config_path)
		if err != nil {
			log.Fatalln("Error opening XML file!")
		}
		log.Println("Opening new XML file! (", config_path, ")")
	}
	defer xmlFile.Close()

	xmlData, err := ioutil.ReadAll(xmlFile)
	if err != nil {
		log.Fatalln("Error reading XML data:", err)
	}

	xml.Unmarshal(xmlData, configFile)
	log.Println("XMLName : ", configFile.XMLName)
	num_of_Dbs := len(configFile.DbBases)
	log.Println("num_of_Dbs : ", num_of_Dbs)
	log.Println()
	return configFile
}
