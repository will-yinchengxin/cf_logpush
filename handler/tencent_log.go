package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	cdn "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cdn/v20180606"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
)

var (
	metrics       = []string{"flux", "hitFlux", "request", "hitRequest", "bandwidth", "2xx", "3xx", "4xx", "5xx"}
	originMetrics = []string{"flux", "request", "bandwidth", "statusCode", "2xx", "3xx", "4xx", "5xx"}
	dimensions    = []string{"overseas", "mainland"}
	tRegion       = []int{2000000004, 2000000001, 2000000002, 2000000003, 2000000005, 2000000006, 2000000007, 2000000008, 1176, 1195, 73}
)

type reqForTencentLog struct {
	SecretID  string `json:"secretId"`
	SecretKey string `json:"secretKey"`
	Domains   string `json:"domains"`
	StartTime int64  `json:"startTime"`
	EndTime   int64  `json:"endTime"`
	EndPoint  string `json:"endPoint"`
}

type CDNDataResult struct {
	Domain     string
	DataType   string
	Location   int
	Metric     string
	Timestamps []string
	Values     []int64
}

func HandleTencentOnTimeLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST 方法", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	defer r.Body.Close()
	var reader io.Reader = r.Body
	body, err := io.ReadAll(reader)
	if err != nil {
		log.Printf("读取请求体时出错: %v\n", err)
		http.Error(w, "读取请求体时出错", http.StatusBadRequest)
		return
	}
	req := reqForTencentLog{}
	err = json.Unmarshal(body, &req)
	if err != nil {
		log.Printf("无效的 JSON 数据: %s\n", err.Error())
		http.Error(w, "无效的 JSON 数据", http.StatusBadRequest)
		return
	}

	m := GetMetric(req)
	marshal, _ := json.Marshal(m)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(marshal)
}

func HandleTencentZipLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST 方法", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	defer r.Body.Close()
	var reader io.Reader = r.Body
	body, err := io.ReadAll(reader)
	if err != nil {
		log.Printf("读取请求体时出错: %v\n", err)
		http.Error(w, "读取请求体时出错", http.StatusBadRequest)
		return
	}
	req := reqForTencentLog{}
	err = json.Unmarshal(body, &req)
	if err != nil {
		log.Printf("无效的 JSON 数据: %s\n", err.Error())
		http.Error(w, "无效的 JSON 数据", http.StatusBadRequest)
	}

	m := GetLog(req)
	marshal, _ := json.Marshal(m)
	w.Write(marshal)
	w.WriteHeader(http.StatusOK)
}

func GetMetric(req reqForTencentLog) interface{} {
	secretId := req.SecretID
	secretKey := req.SecretKey
	domains := SplitDomains(req.Domains)

	credential := common.NewCredential(secretId, secretKey)
	cpf := profile.NewClientProfile()
	//cpf.HttpProfile.Endpoint = "cdn.tencentcloudapi.com"
	cpf.HttpProfile.Endpoint = req.EndPoint
	client, _ := cdn.NewClient(credential, "ap-shanghai", cpf)

	totalQueryCount := len(domains) * len(metrics) * (1 + len(tRegion))
	results := make(chan CDNDataResult, totalQueryCount)
	originResults := make(chan CDNDataResult, len(domains)*len(dimensions)*len(originMetrics))

	var queriesStarted = 0
	for _, domain := range domains {
		// 先查询mainland数据
		for _, metric := range metrics {
			go queryDimensionData(req, client, metric, domain, "mainland", 0, results)
			queriesStarted++
			time.Sleep(60 * time.Millisecond)
		}

		// 再查询overseas数据，先不带district
		//for _, metric := range metrics {
		//	go queryDimensionData(req, client, metric, domain, "overseas", 0, results)
		//	queriesStarted++
		//	time.Sleep(50 * time.Millisecond)
		//}

		// 最后查询overseas带district的数据
		for _, metric := range metrics {
			for _, r := range tRegion {
				go queryDimensionData(req, client, metric, domain, "overseas", r, results)
				queriesStarted++
				time.Sleep(90 * time.Millisecond)
			}
		}

		// 回源数据查询
		for _, metric := range originMetrics {
			for _, dim := range dimensions {
				go queryDimensionOriginData(req, client, metric, domain, dim, originResults)
				time.Sleep(50 * time.Millisecond)
			}
		}
	}

	var dataResults, dataOriginResults []CDNDataResult
	for i := 0; i < queriesStarted; i++ {
		if res := <-results; res.Metric != "" {
			dataResults = append(dataResults, res)
		}
	}
	close(results)

	originQueryCount := len(domains) * len(dimensions) * len(originMetrics)
	for i := 0; i < originQueryCount; i++ {
		if res := <-originResults; res.Metric != "" {
			dataOriginResults = append(dataOriginResults, res)
		}
	}
	close(originResults)

	marshal, _ := json.Marshal(dataResults)
	os.WriteFile("./test/log/dataResults.json", marshal, 0664)
	marshal1, _ := json.Marshal(dataOriginResults)
	os.WriteFile("./test/log/dataOriginResults.json", marshal1, 0664)
	//printReport(dataResults)
	//printReport(dataOriginResults)
	formattedData := formatData(dataResults, dataOriginResults)

	var (
		res = make(map[string]interface{})
	)
	res["code"] = 200
	res["status"] = "success"
	res["data"] = formattedData
	return res
}

func queryDimensionOriginData(param reqForTencentLog, client *cdn.Client, metric, domain, dataType string, ch chan<- CDNDataResult) {
	req := cdn.NewDescribeOriginDataRequest()
	req.StartTime = common.StringPtr(GetTimeStr(param.StartTime))
	req.EndTime = common.StringPtr(GetTimeStr(param.EndTime))
	req.Metric = common.StringPtr(metric)
	req.Domains = []*string{&domain}
	req.Interval = common.StringPtr("min")

	if dataType == "overseas" {
		req.Area = common.StringPtr("overseas")
	} else {
		req.Area = common.StringPtr("mainland")
	}

	resp, err := client.DescribeOriginData(req)
	if err != nil {
		if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
			fmt.Printf("queryDimensionOriginData API Error[%s]Msg[%s]Id[%s]\n", sdkErr.GetCode(), sdkErr.GetMessage(), sdkErr.GetRequestId())
		}
		return
	}

	// 解析数据
	for _, item := range resp.Response.Data {
		result := CDNDataResult{
			Domain:   domain,
			DataType: dataType,
		}

		for _, metricData := range item.OriginData {
			result.Metric = *metricData.Metric
			for _, detail := range metricData.DetailData {
				result.Timestamps = append(result.Timestamps, *detail.Time)
				result.Values = append(result.Values, int64(*detail.Value))
			}
		}
		ch <- result
	}
}

func queryDimensionData(param reqForTencentLog, client *cdn.Client, metric, domain, dataType string, r int, ch chan<- CDNDataResult) {
	// 对于mainland数据类型或r为0，不设置District参数
	if dataType == "mainland" {
		req := cdn.NewDescribeCdnDataRequest()
		req.StartTime = common.StringPtr(GetTimeStr(param.StartTime))
		req.EndTime = common.StringPtr(GetTimeStr(param.EndTime))
		req.Metric = common.StringPtr(metric)
		req.Domains = []*string{&domain}
		req.Interval = common.StringPtr("min")

		// 设置区域参数
		//if dataType == "overseas" {
		//	req.Area = common.StringPtr("overseas")
		//} else {
		//	req.Area = common.StringPtr("mainland")
		//}
		req.Area = common.StringPtr("mainland")

		resp, err := client.DescribeCdnData(req)
		if err != nil {
			if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
				fmt.Printf("queryDimensionData(mainland) API Error[%s]Msg[%s]Id[%s]\n", sdkErr.GetCode(), sdkErr.GetMessage(), sdkErr.GetRequestId())
			}
			ch <- CDNDataResult{}
			return
		}

		for _, item := range resp.Response.Data {
			result := CDNDataResult{
				Domain:   domain,
				DataType: dataType,
				Location: 0,
			}

			for _, metricData := range item.CdnData {
				result.Metric = *metricData.Metric
				for _, detail := range metricData.DetailData {
					result.Timestamps = append(result.Timestamps, *detail.Time)
					result.Values = append(result.Values, int64(*detail.Value))
				}
			}
			ch <- result
		}
		return
	}

	// 只有overseas数据类型且r不为0时才使用District参数
	if dataType == "overseas" && r != 0 {
		req := cdn.NewDescribeCdnDataRequest()
		req.StartTime = common.StringPtr(GetTimeStr(param.StartTime))
		req.EndTime = common.StringPtr(GetTimeStr(param.EndTime))
		req.Metric = common.StringPtr(metric)
		req.Domains = []*string{&domain}
		req.Interval = common.StringPtr("min")
		// 查询中国境外CDN数据时，可指定地区类型查询
		//req.Area = common.StringPtr("overseas")
		//req.AreaType = common.StringPtr("server")
		req.District = common.Int64Ptr(int64(r))

		resp, err := client.DescribeCdnData(req)
		if err != nil {
			if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
				fmt.Printf("queryDimensionData(overseas) API Error[%s]Msg[%s]Id[%s]\n", sdkErr.GetCode(), sdkErr.GetMessage(), sdkErr.GetRequestId())
			}
			ch <- CDNDataResult{}
			return
		}

		for _, item := range resp.Response.Data {
			result := CDNDataResult{
				Domain:   domain,
				DataType: dataType,
				Location: r,
			}

			for _, metricData := range item.CdnData {
				result.Metric = *metricData.Metric
				for _, detail := range metricData.DetailData {
					result.Timestamps = append(result.Timestamps, *detail.Time)
					result.Values = append(result.Values, int64(*detail.Value))
				}
			}
			ch <- result
		}
	}
}

func GetLog(req reqForTencentLog) interface{} {
	secretId := req.SecretID
	secretKey := req.SecretKey
	domains := SplitDomains(req.Domains)
	startTime := GetTimeStr(req.StartTime)
	endTime := GetTimeStr(req.EndTime)

	credential := common.NewCredential(secretId, secretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = req.EndPoint
	client, _ := cdn.NewClient(credential, "", cpf)

	var (
		res = make(map[string]interface{})
		tmp = make(map[string][]interface{})
	)
	failRet := func() interface{} {
		res["code"] = 500
		res["status"] = "fail"
		res["data"] = "查询日志失败"
		return res
	}

	for _, domain := range domains {
		tmpMainLand := make(map[string]interface{})
		tmpOverSea := make(map[string]interface{})
		for _, dim := range dimensions {
			logs, err := queryDomainLogs(client, domain, startTime, endTime, dim)
			if err != nil {
				fmt.Printf("查询日志失败: %v\n", err)
				return failRet()
			}
			fmt.Printf("\n=== 日志下载链接 ===\n")
			for _, log := range logs {
				fmt.Printf("日志路径: %s\n区域: %s\n下载地址: %s\n\n",
					*log.LogPath, *log.Area, *log.LogPath)
			}
			if dim == "mainland" {
				tmpMainLand["mainland"] = logs
				continue
			}
			tmpOverSea["oversea"] = logs
		}
		tmp[domain] = append(tmp[domain], tmpMainLand, tmpOverSea)
	}

	res["code"] = 200
	res["status"] = "success"
	res["data"] = tmp
	return res
}

func queryDomainLogs(client *cdn.Client, domain, startTime, endTime, dim string) ([]*cdn.DomainLog, error) {
	req := cdn.NewDescribeCdnDomainLogsRequest()
	req.Domain = &domain
	req.StartTime = &startTime
	req.EndTime = &endTime
	req.Limit = common.Int64Ptr(1000)
	req.Area = &dim // mainland-境内 overseas-境外

	resp, err := client.DescribeCdnDomainLogs(req)
	//fmt.Println("resp", *resp.Response.RequestId)
	if err != nil {
		if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
			return nil, fmt.Errorf("API Error[%s] %s", sdkErr.GetCode(), sdkErr.GetMessage())
		}
		return nil, err
	}

	return resp.Response.DomainLogs, nil
}

func formatData(reqData []CDNDataResult, originData []CDNDataResult) map[string][]map[string]interface{} {
	result := make(map[string][]map[string]interface{})
	domainDataMap := make(map[string]map[string]map[string]map[string]int64)

	// 初始化数据结构
	// 第一级 domain，第二级时间戳，第三级 DataType (mainland/overseas)，第四级指标名称
	for _, data := range reqData {
		if _, exists := domainDataMap[data.Domain]; !exists {
			domainDataMap[data.Domain] = make(map[string]map[string]map[string]int64)
		}

		for i, timestamp := range data.Timestamps {
			if _, exists := domainDataMap[data.Domain][timestamp]; !exists {
				domainDataMap[data.Domain][timestamp] = make(map[string]map[string]int64)
			}
			dataTypeKey := data.DataType
			if data.DataType == "overseas" && data.Location > 0 {
				dataTypeKey = fmt.Sprintf("overseas_%d", data.Location)
			}

			if _, exists := domainDataMap[data.Domain][timestamp][dataTypeKey]; !exists {
				domainDataMap[data.Domain][timestamp][dataTypeKey] = make(map[string]int64)
			}
			domainDataMap[data.Domain][timestamp][dataTypeKey][data.Metric] = data.Values[i]
		}
	}

	for _, data := range originData {
		if _, exists := domainDataMap[data.Domain]; !exists {
			continue
		}

		for i, timestamp := range data.Timestamps {
			if _, exists := domainDataMap[data.Domain][timestamp]; !exists {
				continue
			}

			dataTypeKey := data.DataType
			if data.DataType == "overseas" && data.Location > 0 {
				dataTypeKey = fmt.Sprintf("overseas_%d", data.Location)
			}

			if _, exists := domainDataMap[data.Domain][timestamp][dataTypeKey]; !exists {
				continue
			}

			metricName := "bs_" + data.Metric
			if data.Metric == "2xx" || data.Metric == "3xx" || data.Metric == "4xx" || data.Metric == "5xx" {
				metricName = "bs_http_code_" + data.Metric
			}

			domainDataMap[data.Domain][timestamp][dataTypeKey][metricName] = data.Values[i]
		}
	}

	for domain, timestampData := range domainDataMap {
		for timestamp, typeData := range timestampData {
			for dataTypeKey, metricsData := range typeData {
				if len(metricsData) == 0 {
					continue
				}
				t, err := time.Parse("2006-01-02 15:04:05", timestamp)
				if err != nil {
					fmt.Printf("解析时间失败: %v\n", err)
					continue
				}
				startTimeMs := t.UnixNano() / 1e6

				var locationCode = 0
				var dataType = dataTypeKey

				if strings.HasPrefix(dataTypeKey, "overseas_") {
					parts := strings.Split(dataTypeKey, "_")
					if len(parts) > 1 {
						code, err := strconv.Atoi(parts[1])
						if err == nil {
							locationCode = code
						}
					}
					dataType = "overseas"
				}

				country := getCountry(dataType, locationCode)
				region := getRegionByDataType(dataType)
				if dataType == "overseas" && locationCode > 0 {
					region = getTencentRegion(locationCode)
				}

				record := map[string]interface{}{
					"start_time":       startTimeMs,
					"country":          country,
					"region":           region,
					"domain":           domain,
					"bw":               getValueOrDefault(metricsData, "bandwidth", 0),
					"flux":             getValueOrDefault(metricsData, "flux", 0),
					"bs_bw":            getValueOrDefault(metricsData, "bs_bandwidth", 0),
					"bs_flux":          getValueOrDefault(metricsData, "bs_flux", 0),
					"req_num":          getValueOrDefault(metricsData, "request", 0),
					"hit_num":          getValueOrDefault(metricsData, "hitRequest", 0),
					"bs_num":           getValueOrDefault(metricsData, "bs_request", 0),
					"bs_fail_num":      0,
					"hit_flux":         getValueOrDefault(metricsData, "hitFlux", 0),
					"http_code_2xx":    getValueOrDefault(metricsData, "2xx", 0),
					"http_code_3xx":    getValueOrDefault(metricsData, "3xx", 0),
					"http_code_4xx":    getValueOrDefault(metricsData, "4xx", 0),
					"http_code_5xx":    getValueOrDefault(metricsData, "5xx", 0),
					"bs_http_code_2xx": getValueOrDefault(metricsData, "bs_http_code_2xx", 0),
					"bs_http_code_3xx": getValueOrDefault(metricsData, "bs_http_code_3xx", 0),
					"bs_http_code_4xx": getValueOrDefault(metricsData, "bs_http_code_4xx", 0),
					"bs_http_code_5xx": getValueOrDefault(metricsData, "bs_http_code_5xx", 0),
				}
				if _, exists := result[domain]; !exists {
					result[domain] = []map[string]interface{}{}
				}
				result[domain] = append(result[domain], record)
			}
		}
	}

	return result
}

func getRegionByDataType(dataType string) string {
	if dataType == "mainland" {
		return "asia"
	}
	return "asia"
}

func getValueOrDefault(metrics map[string]int64, key string, defaultValue int64) int64 {
	if value, exists := metrics[key]; exists {
		return value
	}
	return defaultValue
}

func getTencentRegion(c int) string {
	/*
		// https://cloud.tencent.com/document/product/228/6316#.E5.8C.BA.E5.9F.9F-.2F-.E8.BF.90.E8.90.A5.E5.95.86.E6.98.A0.E5.B0.84.E8.A1.A8
			中东 (2000000004) → asia
			亚太一区 (2000000001) → asia
			亚太二区 (2000000002) → asia
			亚太三区 (2000000003) → asia
			北美 (2000000005) → north_america
			欧洲 (2000000006) → europe
			南美 (2000000007) → south_america
			非洲 (2000000008) → africa
	*/
	region := map[int]string{
		2000000004: "asia",
		2000000001: "asia",
		2000000002: "asia",
		2000000003: "asia",
		2000000005: "north_america",
		2000000006: "europe",
		2000000007: "south_america",
		2000000008: "africa",
		//  新加坡 (1176)
		1176: "asia",
		// 印度尼西亚 (1195)
		1195: "asia",
		// "IN": "asia", // 印度 (73)
		73: "asia",
	}
	if c == 0 {
		return "asia"
	}
	return region[c]
}

func getTencentCountry(c int) string {
	cmap := map[int]string{
		1176: "sg", // 新加坡 (1176)
		1195: "id", // 印度尼西亚 (1195)
		73:   "in", // 印度 (73)
	}
	if c == 0 {
		return "unkonw"
	}
	return cmap[c]
}

func getCountry(dataType string, location int) string {
	if dataType == "mainland" {
		return "cn"
	}
	return getTencentCountry(location)
}

func SplitDomains(input string) []string {
	if input == "" {
		return []string{}
	}
	return strings.Split(input, ",")
}

func GetTimeStr(t int64) string {
	return time.Unix(t, 0).Format("2006-01-02 15:04:05")
}
