package main

import (
	"fmt"
	"log"
	"log_format/dto"
	"log_format/handler"
	"net/http"
	"os"
)

var (
	//defaultLogPath = "/Users/*****/Desktop/"
	defaultLogPath = "/var/log/hwcdn/cdnlogfiles/"
)

func init() {
	if os.Args != nil && len(os.Args) > 1 {
		dto.LogPath = os.Args[1]
	} else {
		dto.LogPath = defaultLogPath
	}
	fmt.Println("LogPath", dto.LogPath)
}

func main() {
	http.HandleFunc("/", handler.HandleLogs)
	port := "9880"
	log.Printf("启动日志接收服务器，监听端口 %s...\n", port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("无法启动服务器: %v\n", err)
	}
}
