package main

import (
	"bufio"
	"log"
	"os"
	"os/exec"
)

func go_test_exec(url string) {
	cmd := exec.Command("wget", "http://localhost:10008/getFile?url="+url)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = "/home/xxx/"
	err := cmd.Run()
	if err != nil {
		log.Printf("failed to call cmd.Run(): %v\n", err)
	}
}

func main() {
	var urlSet []string
	urlPath := "/home/xxx/urlPath.txt"
	file, err := os.Open(urlPath)
	if err != nil {
		log.Printf("Error when opening files!")
	}
	fileScanner := bufio.NewScanner(file)
	for fileScanner.Scan() {
		urlSet = append(urlSet, fileScanner.Text())
	}
	file.Close()
	for _, url := range urlSet {
		go_test_exec(url)
	}
}
