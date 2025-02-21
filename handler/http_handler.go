package handler

import (
	"cf_logpush/dto"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func HandleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST 方法", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	defer r.Body.Close()

	var reader io.Reader = r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		gzipReader, err := gzip.NewReader(r.Body)
		if err != nil {
			log.Printf("创建 gzip 解压缩器时出错: %v\n", err)
			http.Error(w, "无法解压缩请求体", http.StatusBadRequest)
			return
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		log.Printf("读取请求体时出错: %v\n", err)
		http.Error(w, "读取请求体时出错", http.StatusBadRequest)
		return
	}

	bodyStr := string(body)
	lines := strings.Split(bodyStr, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var inputLog dto.InputLog
		if err := json.Unmarshal([]byte(line), &inputLog); err != nil {
			log.Printf("无效的 JSON 数据: %s\n", line)
			continue
		}

		outputLog := TransformLog(inputLog)

		outputJSON, err := json.Marshal(outputLog)
		if err != nil {
			log.Printf("序列化输出日志时出错: %v\n", err)
			continue
		}
		if err := SendToTDAgent(string(outputJSON)); err != nil {
			log.Printf("发送到 td-agent 失败: %v\n", err)
		}
		fmt.Println("send success", string(outputJSON))
		fmt.Println("send success time: ", time.Now().Format(time.DateTime))
		var inputDownLoadLog dto.InputLogForDownLoad
		if err := json.Unmarshal([]byte(line), &inputDownLoadLog); err != nil {
			fmt.Println("HandleLogs json.Unmarshal Err", err.Error())
			log.Printf("无效的 JSON 数据(离线日志): %s\n", line)
			continue
		}
		WriteToFile(inputDownLoadLog)
	}

	w.WriteHeader(http.StatusOK)
}
