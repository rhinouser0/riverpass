// ///////////////////////////////////////////////
// 2023 Shanghai AI Laboratory all rights reserved
// ///////////////////////////////////////////////
package main

import (
	meta "demo_r/src/meta"
	"fmt"
)

func TEST_list_files() {
	fh := new(meta.FileHolder)
	// var RWLock sync.RWMutex
	// fh.New(0, &RWLock)
	fh.New(0)
	fid1 := fh.CreateFile("cat.txt", "pid_0_1")
	fid2 := fh.CreateFile("heo.txt", "pid_0_1")
	fid3 := fh.CreateFile("dog.txt", "pid_0_3")

	f1, _ := fh.ListFile(fid1)
	fmt.Println(f1.Name)
	f2, _ := fh.ListFile(fid2)
	fmt.Println(f2.Name)
	f3, _ := fh.ListFile(fid3)
	fmt.Println(f3.Name)

	fh.PrintFiles()

	fh.TagFile(fid1, "tag_id_1")
	fh.TagFile(fid2, "tag_id_1")
	fh.TagFile(fid3, "tag_id_1")
}
func main() {
	TEST_list_files()
}
