// 服务器端代码
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type Config struct {
	FeishuWebhookURL string  `json:"FeishuWebhookURL"`
	Mysql_Dsn        string  `json:"Mysql_Dsn"`
	Usage_Max        float64 `json:"Usage_Max"`
}

var config Config

type LogEntry struct {
	HostName   string `json:"主机名称"`
	HostInfo   string `json:"主机信息"`
	MemInfo    string `json:"内存信息"`
	CPUInfo    string `json:"CPU信息"`
	DiskInfo   string `json:"磁盘信息"`    // JSON string
	DiskIOInfo string `json:"磁盘I/O信息"` // JSON string
	ResultTime string `json:"当前时间"`
}

// 读取配置文件
func loadConfig(configFile string) (Config, error) {
	var config Config
	configData, err := ioutil.ReadFile(configFile)
	if err != nil {
		return config, err
	}
	err = json.Unmarshal(configData, &config)
	return config, err
}

// 发送告警到 Feishu
func sendAlertToFeishu(alertTitle, alertContent string) {
	alert := map[string]interface{}{
		"msg_type": "post",
		"content": map[string]interface{}{
			"post": map[string]interface{}{
				"zh_cn": map[string]interface{}{
					"title": alertTitle,
					"content": [][]map[string]interface{}{
						{
							{
								"tag":  "text",
								"text": "触发时间：",
							},
							{
								"tag":  "text",
								"text": time.Now().Format("2006-01-02 15:04:05") + "\n",
							},
							{
								"tag":  "text",
								"text": alertContent,
							},
						},
					},
				},
			},
		},
	}

	alertJSON, err := json.Marshal(alert)
	if err != nil {
		fmt.Printf("Failed to marshal alert JSON: %v\n", err)
		return
	}

	resp, err := http.Post(config.FeishuWebhookURL, "application/json", bytes.NewBuffer(alertJSON))
	if err != nil {
		fmt.Printf("Failed to send alert to Feishu: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("Alert sent to Feishu:", resp.Status)
}

// 检测内存使用率是否达到告警线
func checkMemUsage(hostname, memInfoJSON string, threshold float64) {
	var memInfo map[string]string
	err := json.Unmarshal([]byte(memInfoJSON), &memInfo)
	if err != nil {
		fmt.Printf("Failed to unmarshal mem info JSON: %v\n", err)
		return
	}
	usageStr := memInfo["使用率"]
	usageStr = strings.TrimSuffix(usageStr, "%")
	usage, err := strconv.ParseFloat(usageStr, 64)
	if err != nil {
		fmt.Printf("Failed to parse mem usage: %v\n", err)
		return
	}

	if usage > threshold {
		alertContent := fmt.Sprintf("告警信息: 主机 %s 的内存使用率超过了%.2f%%，当前值为: %.2f%%\n", hostname, threshold, usage)
		sendAlertToFeishu("内存使用超限", alertContent)
	}
}

func saveLogEntryToDB(db *sql.DB, entry LogEntry) error {
	query := `INSERT INTO alarm (
		host_name, host_info, mem_info, 
		cpu_info, disk_info, disk_io_info, result_time
	) VALUES (?, ?, ?, ?, ?, ?, ?)`

	// 打印执行的SQL语句
	fullQuery := fmt.Sprintf("INSERT INTO alarm (host_name, host_info, mem_info, cpu_info, disk_info, disk_io_info,result_time) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s')",
		entry.HostName,
		entry.HostInfo,
		entry.MemInfo,
		entry.CPUInfo,
		entry.DiskInfo,
		entry.DiskIOInfo,
		entry.ResultTime,
	)
	fmt.Println("Executing SQL: ", fullQuery)

	_, err := db.Exec(query,
		entry.HostName,
		entry.HostInfo,
		entry.MemInfo,
		entry.CPUInfo,
		entry.DiskInfo,
		entry.DiskIOInfo,
		entry.ResultTime,
	)
	return err
}

func reportHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var logEntry LogEntry
	err = json.Unmarshal(body, &logEntry)
	if err != nil {
		http.Error(w, "Failed to unmarshal JSON", http.StatusBadRequest)
		return
	}
	// 打印日志条目
	fmt.Printf("Received log entry: %+v\n", logEntry)

	db, err := sql.Open("mysql", config.Mysql_Dsn)
	if err != nil {
		http.Error(w, "Failed to connect to database", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// 触发内存使用率告警
	checkMemUsage(logEntry.HostName, logEntry.MemInfo, config.Usage_Max)

	// 保存日志条目到数据库
	err = saveLogEntryToDB(db, logEntry)
	if err != nil {
		http.Error(w, "Failed to save log entry to database", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func main() {

	// Read configuration file
	configFile := "server-config.json"
	file, err := os.Open(configFile)
	if err != nil {
		log.Fatalf("Failed to open config file: %v", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		log.Fatalf("Failed to decode config file: %v", err)
	}

	http.HandleFunc("/alarm", reportHandler)

	fmt.Println("Server listening on port 8080...")

	http_err := http.ListenAndServe(":8080", nil)
	if http_err != nil {
		fmt.Println("Error starting server:", err)
	}
}
