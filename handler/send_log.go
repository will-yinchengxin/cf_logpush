package handler

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func SendToTDAgent(logData string) error {
	for attempt := 1; attempt <= maxRetry; attempt++ {
		resp, err := http.Post(tdAgentURL, "application/json", bytes.NewBuffer([]byte(logData)))
		if err != nil {
			log.Printf("尝试 %d: 发送日志到 td-agent 时出错: %v\n", attempt, err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				err = fmt.Errorf("td-agent 返回状态码 %d: %s", resp.StatusCode, string(body))
				log.Printf("尝试 %d: %v\n", attempt, err)
			} else {
				return nil
			}
		}

		if attempt < maxRetry {
			time.Sleep(retryInterval)
			log.Printf("等待 %v 后重试...\n", retryInterval)
		}
	}

	return fmt.Errorf("所有 %d 次尝试发送日志到 td-agent 均失败", maxRetry)
}
