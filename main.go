package main

import (
	"cf_logpush/dto"
	"cf_logpush/handler"
	"fmt"
	"log"
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
	http.HandleFunc("/tencent/onTimeLog", handler.HandleTencentOnTimeLog)
	http.HandleFunc("/tencent/zipLog", handler.HandleTencentZipLog)
	// http.HandleFunc("/client/log_push", handler.HandleClientLogPush)
	port := "9880"
	log.Printf("启动日志接收服务器，监听端口 %s...\n", port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("无法启动服务器: %v\n", err)
	}
}
