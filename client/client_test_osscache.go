// ///////////////////////////////////////////////
// 2023 Shanghai AI Laboratory all rights reserved
// ///////////////////////////////////////////////

package main

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

func goHttpGet(wg *sync.WaitGroup, url string) {
	urlArr := strings.Split(url, "/")
	fileName := urlArr[len(urlArr)-1]
	cmd := exec.Command("wget", "-O", fileName, "http://localhost:xxxxx/getFile?url="+url)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = "/home/!/oss_test/"
	err := cmd.Run()
	if err != nil {
		log.Println("failed to call cmd.Run()", err)
	}
	if wg != nil {
		wg.Done()
	}
}

func createTestFile(urlPath string) {
	urlFile, _ := os.OpenFile(urlPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
	writer := bufio.NewWriter(urlFile)
	createFileNum := 5
	for idx := 0; idx < createFileNum; idx++ {
		filePath := fmt.Sprintf("/home/!/oss_test/oss_test_%d", idx)
		writer.WriteString(filePath + "\n")
		f1, _ := os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
		w1 := bufio.NewWriter(f1)
		num := rand.Intn(2000000)
		for i := 0; i <= num; i++ {
			data := fmt.Sprintf("io_%d_%d_abcdefghijklmnoqwe123pqrstuvwxyz", idx, i)
			str := data[0:32]
			w1.WriteString(str)
		}
		w1.Flush()
		f1.Close()
	}
	writer.Flush()
	urlFile.Close()
}

func scanFile(urlPath string) []string {
	var urlSet []string
	file, err := os.Open(urlPath)
	if err != nil {
		log.Println("Error when opening files!")
	}
	fileScanner := bufio.NewScanner(file)
	for fileScanner.Scan() {
		urlSet = append(urlSet, fileScanner.Text())
	}
	file.Close()
	return urlSet
}

func main() {
	rand.Seed(time.Now().UnixNano())
	// createTestFile("/home/!/localUrlPath")
	urlSet := scanFile("/home/!/ossUrlPath")
	urlSetLen := len(urlSet)
	queryNum := 30
	isLoop := false
	if isLoop == true {
		for idx := 0; idx < queryNum; idx++ {
			i := idx % urlSetLen
			goHttpGet(nil, urlSet[i])
		}
	} else {
		var wg sync.WaitGroup
		for idx := 0; idx < queryNum; idx++ {
			i := idx % urlSetLen
			wg.Add(1)
			go goHttpGet(&wg, urlSet[i])
		}
		wg.Wait()
	}
}
