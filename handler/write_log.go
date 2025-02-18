package handler

import (
	"fmt"
	"log_format/dto"
	"os"
	"sync"
	"time"
)

const (
	channelSize = 20000
	fileMode    = 0755
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
	line := fmt.Sprintf("LT %s %s %d %s %s %s %s %d %d %d %d %d %d \"%s\" \"%s\" \"%s\" \"%s\" %s %s %s",
		getStr(l.ClientIP),               // 客户端 IP
		getStr(l.OriginIP),               // 源 IP
		l.ClientSrcPort,                  // 端口
		getStr(l.ClientRequestHost),      // 主机名
		getStr(timeStr),                  // 格式化的时间
		fmt.Sprintf("%d", t.UnixMilli()), // 时间戳（毫秒）
		requestLine,                      // 请求行
		l.EdgeResponseStatus,             // 状态码
		l.EdgeResponseBodyBytes,          // 响应体大小
		l.EdgeResponseBytes,              // 总响应大小
		l.EdgeResponseBodyBytes,          // 响应体大小
		l.OriginResponseDurationMs,       // 源站响应时间
		l.OriginResponseDurationMs,       // 源站响应时间（重复）
		getStr(l.ClientRequestReferer),   // Referer
		getStr(l.ClientRequestUserAgent), // User-Agent
		getStr(l.XForwardedFor),          // X-Forwarded-For
		"-",                              // 占位符
		getStr(l.CacheCacheStatus),       // 缓存状态
		getStr(l.RayID),                  // RayID
		"-",                              // 占位符
	)

	fiveMinTime := t.Truncate(5 * time.Minute)
	filename := fmt.Sprintf("%s-%s",
		fiveMinTime.Format("200601021504"),
		l.ClientRequestHost,
		//region,
		//region,
	)

	select {
	case logChan <- &LogEntry{
		content:  line + "\n",
		filename: dto.LogPath + filename,
		logTime:  t,
	}:
	default:
		fmt.Printf("Warning: Log channel is full, dropping log entry for %s\n", filename)
	}
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
	fmt.Println("\n")
	fmt.Println("write success", entry.content)
	fmt.Println("\n")
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
		fileTime, err := time.Parse("200601021504", filename[:12])
		if err != nil {
			return true
		}
		if now.Sub(fileTime) > 10*time.Minute {
			file.Close()
			fileHandles.Delete(key)
			fmt.Println("\n")
			fmt.Println("close file cause now.Sub(fileTime) > 10*time.Minute", filename)
			fmt.Println("\n")
		}
		return true
	})
}
