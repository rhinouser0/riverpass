// ///////////////////////////////////////
// 2022 SHAI Lab all rights reserved
// ///////////////////////////////////////
package main

import (
	"container/list"
	"context"
	"database/sql"
	"fmt"
	"holder/src/dbops"
	"log"

	meta "github.com/common/definition"
)

var DBOpsFile *dbops.DBOpsFile

func TEST_db_conn() {
	db, err := sql.Open("mysql", "dirservice:dirservice123@tcp(172.17.0.11:3306)/dir_service")
	if err != nil {
		panic(err)
	}

	var ctx context.Context
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	rows, err := db.QueryContext(ctx, "SELECT child_name FROM segments2")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var child_name string
		if err := rows.Scan(&child_name); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("name is %s\n", child_name)
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}
}

func TEST_create_and_list_file() {
	var fms []*meta.FileMeta
	var parentId, fileId string
	parentId = "#D1r3ct0ry_0"
	for i := 0; i < 10; i++ {
		lt := list.New()
		fm := meta.FileMeta{
			Name:      fmt.Sprintf("dir_name_%d", i),
			Id:        fmt.Sprintf("#f1l3_%c", 97+i),
			OwnerList: lt,
			// TODO: add the blob related code.
			BlobId: "",
		}
		fm.OwnerList.PushBack(parentId)
		fm.OwnerList.PushBack("#D1r3ct0ry_1")
		fms = append(fms, &fm)
	}

	fmt.Println("CreateSeg:")

	for i := 0; i < 10; i++ {
		fileId = fmt.Sprintf("dir_name_%d", i)
		DBOpsFile.CreateFileWithFidInDB(fileId, fms[i])
	}
	fmt.Println("List:")
	for i := 0; i < 10; i++ {
		fileId = fmt.Sprintf("dir_name_%d", i)
		fmRes, _ := DBOpsFile.ListFileFromDB(fileId)
		fmt.Println(fmRes)
	}

}

func TEST_list_file_and_owner() {
	var fileId string
	fmt.Println("List:")
	for i := 0; i < 10; i++ {
		fileId = fmt.Sprintf("dir_name_%d", i)
		fmRes, owners, _ := DBOpsFile.ListFileAndOwnersFromDB(fileId)
		fmt.Println(fileId, fmRes, owners)
		fmt.Println()
	}
}

func TEST_tag_file() {
	var fileId string
	// fmt.Println("List:")
	// for i := 0; i < 10; i++ {
	// 	fileId = fmt.Sprintf("dir_name_%d", i)
	// 	fmRes, owners, _ := DBOpsFile.ListFileAndOwnersFromDB(fileId)
	// 	fmt.Println(fileId, fmRes, owners)
	// 	fmt.Println()
	// }
	/*
		dir_name_0
		dbfm: {dir_name_0 #f1l3_a #D1r3ct0ry_0,#D1r3ct0ry_1 } OwnerList: #D1r3ct0ry_0,#D1r3ct0ry_1
		fm: {dir_name_0 #f1l3_a 0xc00007eb10 } OwnerList: &{{0xc00007eb40 0xc00007eb70 <nil> <nil>} 2}
		dir_name_0 &{dir_name_0 #f1l3_a 0xc00007eb10 }
	*/

	// //[1] no deduplication
	// dbfm := dbops.DBFileMeta{}
	// for i := 0; i < 10; i++ {
	// 	fileId = fmt.Sprintf("dir_name_%d", i)
	// 	fmt.Println("Before:")
	// 	fmRes, owners, _ := DBOpsFile.ListFileAndOwnersFromDB(fileId)

	// 	owners += ",#D1r3ct0ry_2"
	// 	dbfm = dbops.DBFileMeta{
	// 		Name:      fmRes.Name,
	// 		Id:        fileId,
	// 		OwnerList: owners,
	// 		BlobId:    fmRes.BlobId,
	// 	}
	// 	DBOpsFile.UpdateFilemetaAndOwnerInDB(fileId, &dbfm)

	// 	fmt.Println("Then:")
	// 	fmt.Println(DBOpsFile.ListFileAndOwnersFromDB(fileId))
	// 	fmt.Println()
	// }
	/*
		Before:
		dir_name_0
		dbfm: {dir_name_0 #f1l3_a #D1r3ct0ry_0,#D1r3ct0ry_1 } OwnerList: #D1r3ct0ry_0,#D1r3ct0ry_1
		fm: {dir_name_0 #f1l3_a 0xc0001841e0 } OwnerList: &{{0xc000184210 0xc000184240 <nil> <nil>} 2}

		Then:
		dir_name_0
		dbfm: {dir_name_0 dir_name_0 #D1r3ct0ry_0,#D1r3ct0ry_1,#D1r3ct0ry_2 } OwnerList: #D1r3ct0ry_0,#D1r3ct0ry_1,#D1r3ct0ry_2
		fm: {dir_name_0 dir_name_0 0xc00009c1e0 } OwnerList: &{{0xc00009c210 0xc00009c270 <nil> <nil>} 3}
		&{dir_name_0 dir_name_0 0xc00009c1e0 } #D1r3ct0ry_0,#D1r3ct0ry_1,#D1r3ct0ry_2 <nil>
	*/
	//[2] TagFileInDB() - deduplication
	for i := 0; i < 10; i++ {
		fileId = fmt.Sprintf("dir_name_%d", i)
		fmt.Println("Before:")
		fmRes, owners, _ := DBOpsFile.ListFileAndOwnersFromDB(fileId)
		fmt.Println("fm:", fmRes, "  owners:", owners)

		new_tagId := "#D1r3ct0ry_2"
		DBOpsFile.TagFileInDB(fileId, new_tagId)

		fmt.Println("Then:")
		fmRes, owners, _ = DBOpsFile.ListFileAndOwnersFromDB(fileId)
		fmt.Println("fm:", fmRes, "  owners:", owners)
		fmt.Println()
	}

}

func main() {

	// TEST_create_and_list_file()
	/*
		List:
		{dir_name_0 #f1l3_a 0xc00007f8c0 }
		{dir_name_1 #f1l3_b 0xc00009c7e0 }
		{dir_name_2 #f1l3_c 0xc00009ca20 }
		{dir_name_3 #f1l3_d 0xc00007fbf0 }
		{dir_name_4 #f1l3_e 0xc00007fe00 }
		{dir_name_5 #f1l3_f 0xc000164060 }
		{dir_name_6 #f1l3_g 0xc0001642a0 }
		{dir_name_7 #f1l3_h 0xc000164450 }
		{dir_name_8 #f1l3_i 0xc000164690 }
		{dir_name_9 #f1l3_j 0xc0001847e0 }
	*/
	// TEST_list_file_and_owner()
	/*
		dir_name_0
		dbfm: {dir_name_0 #f1l3_a #D1r3ct0ry_0,#D1r3ct0ry_1 } OwnerList: #D1r3ct0ry_0,#D1r3ct0ry_1
		fm: {dir_name_0 #f1l3_a 0xc00007eb10 } OwnerList: &{{0xc00007eb40 0xc00007eb70 <nil> <nil>} 2}
		dir_name_0 &{dir_name_0 #f1l3_a 0xc00007eb10 }
	*/

	TEST_tag_file()
	/*
		Before:
		&{dir_name_0 #f1l3_a 0xc00009e210 } #D1r3ct0ry_0,#D1r3ct0ry_1 <nil>
		Then:
		&{dir_name_0 #f1l3_a 0xc00009e870 } #D1r3ct0ry_0,#D1r3ct0ry_1,#D1r3ct0ry_2 <nil>

		Before:
		fm: &{dir_name_0 #f1l3_a 0xc00009e240 }   owners: #D1r3ct0ry_0,#D1r3ct0ry_1,#D1r3ct0ry_2
		Then:
		fm: &{dir_name_0 #f1l3_a 0xc00009e7b0 }   owners: #D1r3ct0ry_0,#D1r3ct0ry_1,#D1r3ct0ry_2
	*/

}
