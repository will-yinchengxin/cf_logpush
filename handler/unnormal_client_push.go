package handler

import (
	"cf_logpush/dto"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"github.com/olivere/elastic/v7"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	StatisticalData = iota + 1
	BillingData
)

var (
	esClient *elastic.Client
	esOnce   sync.Once
)

func init() {
	esOnce.Do(func() {
		esURL := os.Getenv("ES_URL")
		if esURL == "" {
			//esURL = "http://172.16.27.45:9200"
			esURL = "http://192.168.1.131:9200"
		}

		username := os.Getenv("ES_USERNAME")
		password := os.Getenv("ES_PASSWORD")
		if username == "" {
			username = "elastic"
		}
		if password == "" {
			password = "2wsxzaq1~"
		}

		var client *elastic.Client
		var err error

		if username != "" && password != "" {
			client, err = elastic.NewClient(
				elastic.SetURL(esURL),
				elastic.SetBasicAuth(username, password),
				elastic.SetSniff(false),
				elastic.SetHealthcheck(false),
			)
		} else {
			client, err = elastic.NewClient(
				elastic.SetURL(esURL),
				elastic.SetSniff(false),
				elastic.SetHealthcheck(false),
			)
		}
		if err != nil {
			fmt.Printf("创建 ES 客户端失败: %w", err)
		}
		esClient = client
	})
}

func HandleStatisticalData(w http.ResponseWriter, r *http.Request) {
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
	outputLog := dto.OutputLog{}
	err = json.Unmarshal(body, &outputLog)
	if err != nil {
		log.Printf("无效的 JSON 数据: %s\n", err.Error())
		http.Error(w, "无效的 JSON 数据, 请注意检察参数类型", http.StatusBadRequest)
		return
	}
	if outputLog.TenantId == "" {
		http.Error(w, "tenantId 不能为空", http.StatusBadRequest)
		return
	}
	if outputLog.StartTime == 0 {
		http.Error(w, "start_time 不能为 0", http.StatusBadRequest)
		return
	}
	err = sendToES(outputLog, StatisticalData)
	if err != nil {
		log.Printf("### client_push_statistical_data err ### [time: " + time.Now().Format(time.DateTime) + "] [Err: " + err.Error() + "]")
		http.Error(w, "服务出错", http.StatusInternalServerError)
		return
	}
	marshal, _ := json.Marshal(outputLog)
	fmt.Println("### client_push_statistical_data ### [time: " + time.Now().Format(time.DateTime) + "] data:" + string(marshal))
	w.Write([]byte("success"))
	w.WriteHeader(http.StatusOK)
}

func HandleBillingData(w http.ResponseWriter, r *http.Request) {
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
	outputLog := dto.OutputLog{}
	err = json.Unmarshal(body, &outputLog)
	if err != nil {
		log.Printf("无效的 JSON 数据: %s\n", err.Error())
		http.Error(w, "无效的 JSON 数据, 请注意检察参数类型", http.StatusBadRequest)
		return
	}
	if outputLog.TenantId == "" {
		http.Error(w, "tenantId 不能为空", http.StatusBadRequest)
		return
	}
	if outputLog.Flux == 0 && outputLog.BW == 0 {
		http.Error(w, "flux 和 bw 不能同时为 0", http.StatusBadRequest)
		return
	}
	if outputLog.StartTime == 0 {
		http.Error(w, "start_time 不能为 0", http.StatusBadRequest)
		return
	}
	err = sendToES(outputLog, BillingData)
	if err != nil {
		log.Printf("### client_push_billingData err ### [time: " + time.Now().Format(time.DateTime) + "] [Err: " + err.Error() + "]")
		http.Error(w, "服务出错", http.StatusInternalServerError)
		return
	}
	marshal, _ := json.Marshal(outputLog)
	fmt.Println("### client_push_billing_data ### [time: " + time.Now().Format(time.DateTime) + "] data:" + string(marshal))
	w.Write([]byte("success"))
	w.WriteHeader(http.StatusOK)
}

func sendToES(d dto.OutputLog, t uint8) error {
	if esClient == nil {
		return fmt.Errorf("ES 客户端未初始化")
	}
	timestamp := time.Unix(0, d.StartTime*int64(time.Millisecond))
	month := timestamp.Format("01")
	year := timestamp.Format("2006")

	var indexName string
	var dataToIndex interface{}
	if t == StatisticalData {
		indexName = fmt.Sprintf("log_push_statistical_data-%s.%s", year, month)
		dataToIndex = dto.OutputStatisticalData{
			StartTime:     d.StartTime / 1000,
			Country:       d.Country,
			Region:        d.Region,
			Domain:        d.Domain,
			BW:            d.BW,
			Flux:          d.Flux,
			BSBW:          d.BSBW,
			BSFlux:        d.BSFlux,
			ReqNum:        d.ReqNum,
			HitNum:        d.HitNum,
			BSNum:         d.BSNum,
			BSFailNum:     d.BSFailNum,
			HitFlux:       d.HitFlux,
			HTTPCode2XX:   d.HTTPCode2XX,
			HTTPCode3XX:   d.HTTPCode3XX,
			HTTPCode4XX:   d.HTTPCode4XX,
			HTTPCode5XX:   d.HTTPCode5XX,
			BSHTTPCode2XX: d.BSHTTPCode2XX,
			BSHTTPCode3XX: d.BSHTTPCode3XX,
			BSHTTPCode4XX: d.BSHTTPCode4XX,
			BSHTTPCode5XX: d.BSHTTPCode5XX,
			TenantId:      d.TenantId,
			TimeLocal:     d.StartTime / 1000,
		}
	} else if t == BillingData {
		indexName = fmt.Sprintf("log_push_billing_data-%s.%s", year, month)
		dataToIndex = dto.OutputLogBillDate{
			TenantId:  d.TenantId,
			StartTime: d.StartTime / 1000,
			TimeLocal: d.StartTime / 1000,
			Domain:    d.Domain,
			BW:        d.BW,
			Flux:      d.Flux,
		}

	} else {
		return fmt.Errorf("未知的数据类型: %d", t)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := esClient.Index().
		Index(indexName).
		BodyJson(dataToIndex).
		Do(ctx)

	if err != nil {
		return fmt.Errorf("推送数据到 ES 失败: %w", err)
	}

	return nil
}
