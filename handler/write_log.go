package handler

import (
	"cf_logpush/dto"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	channelSize = 20000
	fileMode    = 0644
)

var (
	logChan     chan *LogEntry
	fileHandles = sync.Map{}
	once        sync.Once
	bufferPool  = sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 1024)
		},
	}
)

type LogEntry struct {
	content  string
	filename string
	logTime  time.Time
}

func init() {
	initLogWriter()
}

func initLogWriter() {
	once.Do(func() {
		logChan = make(chan *LogEntry, channelSize)
		go processLogs()
		fmt.Println("LogWriter initialized")
	})
}

func WriteToFile(l dto.InputLogForDownLoad) {
	t, err := time.Parse(time.RFC3339, l.EdgeStartTimestamp)
	if err != nil {
		fmt.Println("Error parsing time")
		return
	}
	timeLoc, _ := time.LoadLocation("Asia/Shanghai")
	t = t.In(timeLoc)
	timeStr := ""
	if err == nil {

		timeStr = t.Format("[02/Jan/2006:15:04:05 +0000]")
	}
	requestLine := fmt.Sprintf("\"%s %s %s\"",
		l.ClientRequestMethod,
		l.ClientRequestURI,
		l.ClientRequestProtocol,
	)

	srvPort := 0
	if l.ClientRequestScheme == "http" {
		srvPort = 80
	} else if l.ClientRequestScheme == "https" {
		srvPort = 443
	} else {
		srvPort = 0
	}
	reqTD := getSubTime(l.EdgeStartTimestamp, l.EdgeEndTimestamp)
	line := fmt.Sprintf("LT %s %s %d %s %s %s %s %d %d %d %d %d %d \"%s\" \"%s\" \"%s\" \"%s\" %s %s %s",
		getStr(l.ClientIP),               // 客户端 IP
		getStr(l.EdgeServerIP),           // 源 IP
		srvPort,                          // 源端口
		getStr(l.ClientRequestHost),      // 主机名
		getStr(timeStr),                  // 格式化的时间
		fmt.Sprintf("%d", t.UnixMilli()), // 时间戳（毫秒）
		requestLine,                      // 请求行
		l.EdgeResponseStatus,             // 状态码
		l.EdgeResponseBodyBytes,          // 响应体大小
		l.EdgeResponseBytes,              // 总响应大小
		l.EdgeResponseBodyBytes,          // 响应体大小

		l.EdgeTimeToFirstByteMs, // 本机收到请求后，到首包响应时间，单位毫秒
		reqTD,                   // 请求响应时间，单位毫秒

		getStr(l.ClientRequestReferer),   // Referer
		getStr(l.ClientRequestUserAgent), // User-Agent
		getStr(l.XForwardedFor),          // X-Forwarded-For
		"-",                              // Http Range
		getStr(l.CacheCacheStatus),       // 缓存状态
		getStr(l.RayID),                  // RayID 请求
		"1",                              // 距用户最近的边缘用1标示：同客户源日志用0标示（如果由存储回客户源，记为0)
	)

	fiveMinTime := t.Truncate(5 * time.Minute)
	filename := fmt.Sprintf("%s-%s",
		fiveMinTime.Format("200601021504"),
		l.ClientRequestHost,
		//region,
		//region,
	)
	filePath := fiveMinTime.Format("200601021504")[:8]
	err = createDirIfNotExist(filePath)
	if err != nil {
		fmt.Println("Error creating directory: " + err.Error())
		return
	}
	select {
	case logChan <- &LogEntry{
		content:  line + "\n",
		filename: dto.LogPath + filePath + "/" + filename,
		logTime:  t,
	}:
	default:
		fmt.Printf("Warning: Log channel is full, dropping log entry for %s\n", filename)
	}
}

func getSubTime(s, e string) int64 {
	layout := "2006-01-02T15:04:05Z"
	t1, err := time.Parse(layout, s)
	if err != nil {
		fmt.Println("解析时间错误:", err)
		return 0
	}

	t2, err := time.Parse(layout, e)
	if err != nil {
		fmt.Println("解析时间错误:", err)
		return 0
	}

	diff := t2.Sub(t1)
	return diff.Milliseconds()
}

func getStr(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func processLogs() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case entry := <-logChan:
			writeLogToFile(entry)
		case <-ticker.C:
			cleanupOldFiles()
		}
	}
}

func createDirIfNotExist(filePath string) error {
	if _, err := os.Stat(dto.LogPath + filePath); os.IsNotExist(err) {
		if err := os.MkdirAll(dto.LogPath+filePath, os.ModePerm); err != nil {
			return err
		}
	}
	return nil
}

func writeLogToFile(entry *LogEntry) {
	fh, ok := fileHandles.Load(entry.filename)
	if !ok {
		file, err := createOrOpenFile(entry.filename)
		if err != nil {
			fmt.Printf("Error creating file %s: %v\n", entry.filename, err)
			return
		}
		fileHandles.Store(entry.filename, file)
		fh = file
	}

	file := fh.(*os.File)
	if _, err := file.WriteString(entry.content); err != nil {
		fmt.Printf("Error writing to file %s: %v\n", entry.filename, err)
		file.Close()
		fileHandles.Delete(entry.filename)
	}
	fmt.Println("write success", entry.content)
	fmt.Println("write success time: ", time.Now().Format(time.DateTime))
}

func createOrOpenFile(filename string) (*os.File, error) {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, fileMode)
	if err != nil {
		return nil, err
	}
	return file, nil
}

func cleanupOldFiles() {
	now := time.Now()
	fileHandles.Range(func(key, value interface{}) bool {
		filename := key.(string)
		file := value.(*os.File)

		split := strings.Split(filename, "/")
		timeStr := split[len(split)-1][:12]

		loc, _ := time.LoadLocation("Asia/Shanghai")
		fileTime, err := time.ParseInLocation("200601021504", timeStr, loc)
		if err != nil {
			fmt.Printf("cleanupOldFiles Err: Error parsing time for file %s: %v\n", filename, err)
			return true
		}

		nowInLoc := now.In(loc)
		diff := nowInLoc.Sub(fileTime)
		fmt.Printf("File: %s, FileTime: %s, Now: %s, Diff: %v\n",
			split[len(split)-1],
			fileTime.Format("2006-01-02 15:04:05"),
			nowInLoc.Format("2006-01-02 15:04:05"),
			diff,
		)

		if diff > 10*time.Minute {
			file.Close()
			fileHandles.Delete(key)
			fmt.Printf("Closed file %s (age: %v)\n", split[len(split)-1], diff)
		} else {
			fmt.Printf("File %s does not need cleaning (age: %v)\n", split[len(split)-1], diff)
		}

		return true
	})
}
