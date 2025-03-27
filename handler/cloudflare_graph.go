package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var (
	cfKey = "Bearer JXuzJ0Q65bE_boN_Y5296VliYzQGH04h50jwf-2K"
	cfUrl = "https://api.cloudflare.com/client/v4/graphql"
)

type CloudFlareResponse struct {
	Data struct {
		Viewer struct {
			Zones []struct {
				HttpRequestsAdaptiveGroups []struct {
					Count      int `json:"count"`
					Dimensions struct {
						CacheStatus          string `json:"cacheStatus"`
						ClientCountryName    string `json:"clientCountryName"`
						DatetimeMinute       string `json:"datetimeMinute"`
						EdgeResponseStatus   int    `json:"edgeResponseStatus"`
						OriginResponseStatus int    `json:"originResponseStatus"`
					} `json:"dimensions"`
					Sum struct {
						EdgeResponseBytes int `json:"edgeResponseBytes"`
					} `json:"sum"`
				} `json:"httpRequestsAdaptiveGroups"`
				ZoneTag string `json:"zoneTag"`
			} `json:"zones"`
		} `json:"viewer"`
	} `json:"data"`
	Errors interface{} `json:"errors"`
}

type OutputLog struct {
	StartTime     int64  `json:"start_time"`
	Country       string `json:"country"`
	Region        string `json:"region"`
	Domain        string `json:"domain"`
	BW            int    `json:"bw"`
	Flux          int    `json:"flux"`
	BSBW          int    `json:"bs_bw"`
	BSFlux        int    `json:"bs_flux"`
	ReqNum        int    `json:"req_num"`
	HitNum        int    `json:"hit_num"`
	BSNum         int    `json:"bs_num"`
	BSFailNum     int    `json:"bs_fail_num"`
	HitFlux       int    `json:"hit_flux"`
	HTTPCode2XX   int    `json:"http_code_2xx"`
	HTTPCode3XX   int    `json:"http_code_3xx"`
	HTTPCode4XX   int    `json:"http_code_4xx"`
	HTTPCode5XX   int    `json:"http_code_5xx"`
	BSHTTPCode2XX int    `json:"bs_http_code_2xx"`
	BSHTTPCode3XX int    `json:"bs_http_code_3xx"`
	BSHTTPCode4XX int    `json:"bs_http_code_4xx"`
	BSHTTPCode5XX int    `json:"bs_http_code_5xx"`
}

func HandleCloudFlareOnTimeLog(w http.ResponseWriter, r *http.Request) {
	zoneId := r.URL.Query().Get("zoneId")
	startTimeStr := r.URL.Query().Get("startTime")
	endTimeStr := r.URL.Query().Get("endTime")

	if zoneId == "" || startTimeStr == "" || endTimeStr == "" {
		http.Error(w, "缺少必要参数: zoneId, startTime, endTime", http.StatusBadRequest)
		return
	}

	startTimeUnix, err := strconv.ParseInt(startTimeStr, 10, 64)
	if err != nil {
		http.Error(w, "无效的开始时间格式", http.StatusBadRequest)
		return
	}
	endTimeUnix, err := strconv.ParseInt(endTimeStr, 10, 64)
	if err != nil {
		http.Error(w, "无效的结束时间格式", http.StatusBadRequest)
		return
	}

	startTime := time.Unix(startTimeUnix, 0).UTC().Format(time.RFC3339)
	endTime := time.Unix(endTimeUnix, 0).UTC().Format(time.RFC3339)
	fmt.Println("startTime", startTime, "endTime", endTime)

	var (
		res = make(map[string]interface{})
	)
	zoneIds := strings.Split(zoneId, ",")
	for _, zone := range zoneIds {
		cloudflareData, err := queryCloudFlare(zone, startTime, endTime)
		if err != nil {
			http.Error(w, "查询CloudFlare数据失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		outputLogs := processCloudFlareData(cloudflareData, startTimeUnix, zone, endTime)
		res[zone] = outputLogs
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func queryCloudFlare(zoneId, startTime, endTime string) (CloudFlareResponse, error) {
	queryJSON := fmt.Sprintf(`{
  "query": "{ viewer { zones(filter: {zoneTag_in: [\"%s\"]}) { zoneTag httpRequestsAdaptiveGroups(filter: {datetime_geq: \"%s\", datetime_leq: \"%s\"}, orderBy: [datetimeMinute_ASC], limit: 9999) { dimensions { datetimeMinute originResponseStatus cacheStatus clientCountryName edgeResponseStatus } sum { edgeResponseBytes } count } } }}"
}`, zoneId, startTime, endTime)
	//fmt.Println("queryJSON", queryJSON)
	var (
		err error
	)
	defer func() {
		if err != nil {
			fmt.Println("### cloudflare-err ###", "[zoneId: "+zoneId+"] [startTime: "+startTime+"] [endTime: "+endTime+"] [resDataErr: "+err.Error()+"]")
		}
	}()
	req, err := http.NewRequest("POST", cfUrl, strings.NewReader(queryJSON))
	if err != nil {
		return CloudFlareResponse{}, fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Authorization", cfKey)
	req.Header.Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return CloudFlareResponse{}, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return CloudFlareResponse{}, fmt.Errorf("请求返回非200状态码: %d, 响应体: %s", resp.StatusCode, string(bodyBytes))
	}

	var respData CloudFlareResponse
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return CloudFlareResponse{}, fmt.Errorf("读取响应体失败: %v", err)
	}
	err = json.Unmarshal(bodyBytes, &respData)
	if err != nil {
		return CloudFlareResponse{}, fmt.Errorf("解析响应失败: %v", err)
	}
	fmt.Println("### cloudflare ###", "[zoneId: "+zoneId+"] [startTime: "+startTime+"] [endTime: "+endTime+"]"+" [status: success]")
	return respData, nil
}

func processCloudFlareData(cloudflareData CloudFlareResponse, startTimeUnix int64, zoneId, endTime string) []OutputLog {
	countryMap := make(map[string]*OutputLog)
	timeMap := make(map[string]time.Time)

	if len(cloudflareData.Data.Viewer.Zones) == 0 {
		return []OutputLog{}
	}
	zone := cloudflareData.Data.Viewer.Zones[0]

	for _, group := range zone.HttpRequestsAdaptiveGroups {
		country := group.Dimensions.ClientCountryName
		if country == "" {
			country = "Unknown"
		}
		if group.Dimensions.DatetimeMinute == endTime {
			continue
		}
		dateTime, err := time.Parse("2006-01-02T15:04:05Z", group.Dimensions.DatetimeMinute)
		if err != nil {
			dateTime = time.Unix(startTimeUnix/1000, 0)
		}

		timestamp := dateTime.UnixNano() / int64(time.Millisecond)

		timeKey := fmt.Sprintf("%s_%s", country, dateTime.Format("2006-01-02T15:04:05Z"))
		log, exists := countryMap[timeKey]
		if !exists {
			log = &OutputLog{
				StartTime: timestamp,
				Country:   stringToLowCase(country),
				Domain:    zone.ZoneTag,
			}
			timeMap[timeKey] = dateTime
			countryMap[timeKey] = log
		}

		log.ReqNum += group.Count

		responseBytes := group.Sum.EdgeResponseBytes
		log.Flux += responseBytes
		log.BW += responseBytes * 8

		isHit := group.Dimensions.OriginResponseStatus == 0 || group.Dimensions.CacheStatus == "hit"

		if isHit {
			log.HitNum += group.Count
			log.HitFlux += responseBytes
		} else {
			log.BSNum += group.Count
			log.BSFlux += responseBytes
			log.BSBW += responseBytes * 8

			if group.Dimensions.OriginResponseStatus >= 400 {
				log.BSFailNum += group.Count
			}

			statusCode := group.Dimensions.OriginResponseStatus
			switch statusCode / 100 {
			case 2:
				log.BSHTTPCode2XX += group.Count
			case 3:
				log.BSHTTPCode3XX += group.Count
			case 4:
				log.BSHTTPCode4XX += group.Count
			case 5:
				log.BSHTTPCode5XX += group.Count
			}
		}

		edgeStatusCode := group.Dimensions.EdgeResponseStatus
		switch edgeStatusCode / 100 {
		case 2:
			log.HTTPCode2XX += group.Count
		case 3:
			log.HTTPCode3XX += group.Count
		case 4:
			log.HTTPCode4XX += group.Count
		case 5:
			log.HTTPCode5XX += group.Count
		}
		log.Region = getRegion(country)
	}

	var result []OutputLog
	for _, log := range countryMap {
		result = append(result, *log)
	}

	return result
}

func stringToLowCase(s string) string {
	return strings.ToLower(s)
}

func getRegion(s string) string {
	region := map[string]string{
		// Chinese Mainland
		"CN": "mainland_china",

		// Asia
		"AF": "asia", "AM": "asia", "AZ": "asia", "BH": "asia", "BD": "asia",
		"BT": "asia", "BN": "asia", "KH": "asia", "GE": "asia", "IN": "asia",
		"ID": "asia", "IO": "asia", "IR": "asia", "IQ": "asia", "IL": "asia",
		"JP": "asia", "JO": "asia", "KZ": "asia", "KP": "asia", "KR": "asia",
		"KW": "asia", "KG": "asia", "LA": "asia", "LB": "asia", "MY": "asia",
		"MV": "asia", "MN": "asia", "MM": "asia", "NP": "asia", "OM": "asia",
		"PK": "asia", "PH": "asia", "QA": "asia", "SA": "asia", "SG": "asia",
		"LK": "asia", "SY": "asia", "TW": "asia", "TJ": "asia", "TH": "asia",
		"TL": "asia", "TM": "asia", "TR": "asia", "AE": "asia", "UZ": "asia",
		"VN": "asia", "YE": "asia", "PS": "asia", "HK": "asia", "MO": "asia",

		// Europe
		"AX": "europe", "AL": "europe", "AD": "europe", "AT": "europe", "BY": "europe",
		"BE": "europe", "BA": "europe", "BG": "europe", "HR": "europe", "CY": "europe",
		"CZ": "europe", "DK": "europe", "EE": "europe", "FO": "europe", "FI": "europe",
		"FR": "europe", "DE": "europe", "GI": "europe", "GR": "europe", "GG": "europe",
		"HU": "europe", "IS": "europe", "IE": "europe", "IM": "europe", "IT": "europe",
		"JE": "europe", "XK": "europe", "LV": "europe", "LI": "europe", "LT": "europe",
		"LU": "europe", "MK": "europe", "MT": "europe", "MD": "europe", "MC": "europe",
		"ME": "europe", "NL": "europe", "NO": "europe", "PL": "europe", "PT": "europe",
		"RO": "europe", "RU": "europe", "SM": "europe", "RS": "europe", "SK": "europe",
		"SI": "europe", "ES": "europe", "SE": "europe", "CH": "europe", "UA": "europe",
		"GB": "europe", "VA": "europe", "SJ": "europe",

		// Africa
		"DZ": "africa", "AO": "africa", "BJ": "africa", "BW": "africa", "BF": "africa",
		"BI": "africa", "CM": "africa", "CV": "africa", "CF": "africa", "TD": "africa",
		"KM": "africa", "CG": "africa", "CD": "africa", "CI": "africa", "DJ": "africa",
		"EG": "africa", "GQ": "africa", "ER": "africa", "ET": "africa", "GA": "africa",
		"GM": "africa", "GH": "africa", "GN": "africa", "GW": "africa", "KE": "africa",
		"LS": "africa", "LR": "africa", "LY": "africa", "MG": "africa", "MW": "africa",
		"ML": "africa", "MR": "africa", "MU": "africa", "MA": "africa", "MZ": "africa",
		"NA": "africa", "NE": "africa", "NG": "africa", "RW": "africa", "ST": "africa",
		"SN": "africa", "SC": "africa", "SL": "africa", "SO": "africa", "ZA": "africa",
		"SS": "africa", "SD": "africa", "SZ": "africa", "TZ": "africa", "TG": "africa",
		"TN": "africa", "UG": "africa", "ZM": "africa", "ZW": "africa", "EH": "africa",
		"SH": "africa", "RE": "africa", "YT": "africa",

		// North America
		"AS": "north_america", "AG": "north_america", "AI": "north_america",
		"AN": "north_america", "AW": "north_america", "BS": "north_america",
		"BB": "north_america", "BZ": "north_america", "BM": "north_america",
		"CA": "north_america", "BQ": "north_america", "CW": "north_america",
		"KY": "north_america", "CR": "north_america", "CU": "north_america",
		"DM": "north_america", "DO": "north_america", "SV": "north_america",
		"GL": "north_america", "GD": "north_america", "GP": "north_america",
		"GT": "north_america", "HT": "north_america", "HN": "north_america",
		"JM": "north_america", "MQ": "north_america", "MX": "north_america",
		"MS": "north_america", "NI": "north_america", "PA": "north_america",
		"PR": "north_america", "KN": "north_america", "LC": "north_america",
		"PM": "north_america", "VC": "north_america", "TT": "north_america",
		"TC": "north_america", "US": "north_america", "VG": "north_america",
		"VI": "north_america", "BL": "north_america", "MF": "north_america",
		"SX": "north_america", "UM": "north_america",

		// South America
		"AR": "south_america", "BO": "south_america", "BR": "south_america",
		"CL": "south_america", "CO": "south_america", "EC": "south_america",
		"FK": "south_america", "GF": "south_america", "GY": "south_america",
		"PY": "south_america", "PE": "south_america", "SR": "south_america",
		"UY": "south_america", "VE": "south_america",

		// Oceania
		"AU": "oceania", "CX": "oceania", "CC": "oceania", "CK": "oceania",
		"FJ": "oceania", "PF": "oceania", "GU": "oceania", "KI": "oceania",
		"MH": "oceania", "FM": "oceania", "NR": "oceania", "NC": "oceania",
		"NZ": "oceania", "NU": "oceania", "NF": "oceania", "MP": "oceania",
		"PW": "oceania", "PG": "oceania", "PN": "oceania", "WS": "oceania",
		"SB": "oceania", "TK": "oceania", "TO": "oceania", "TV": "oceania",
		"VU": "oceania", "WF": "oceania",

		// Antarctica
		"AQ": "antarctica", "BV": "antarctica", "TF": "antarctica",
		"HM": "antarctica", "GS": "antarctica",

		// Other
		"T1": "other", "XX": "other",
	}
	if region[s] == "" {
		return "other"
	}
	return region[s]
}
