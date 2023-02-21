package main

import (
        "bufio"
        "log"
        "os"
        "os/exec"

        "go.uber.org/zap"
)

func go_test_exec(url string){
        cmd := exec.Command("wget", "http://localhost:10009/getFile?url="+url)
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr
        cmd.Dir = "/home/sqyan/"
        err := cmd.Run()
        if err != nil {
                log.Printf("failed to call cmd.Run(): %v\n", err)
        }
}

func main() {
        var zapLogger *zap.Logger
        var urlSet []string
        urlPath := "/home/sqyan/urlPath.txt"
        file, err := os.Open(urlPath)
        if err != nil {
                zapLogger.Debug("Error when opening files!")
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