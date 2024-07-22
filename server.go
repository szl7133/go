package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	FeishuWebhookURL string  `json:"FeishuWebhookURL"`
	Mysql_Dsn        string  `json:"Mysql_Dsn"`
	Mem_Usage_Max    float64 `json:"Mem_Usage_Max"`
	Cpu_Usage_Max    float64 `json:"Cpu_Usage_Max"`
	Disk_Usage_Max   float64 `json:"Disk_Usage_Max"`
}

var config Config

type DataEntry struct {
	ID         string `json:"id"`
	HostName   string `json:"主机名称"`
	HostInfo   string `json:"主机信息"`
	MemInfo    string `json:"内存信息"`
	CPUInfo    string `json:"CPU信息"`
	DiskInfo   string `json:"磁盘信息"`    // JSON string
	DiskIOInfo string `json:"磁盘I/O信息"` // JSON string
	ResultTime string `json:"当前时间"`
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
func checkMemUsage(hostname, memInfoJSON string, threshold float64) bool {
	var memInfo map[string]string
	err := json.Unmarshal([]byte(memInfoJSON), &memInfo)
	if err != nil {
		fmt.Printf("Failed to unmarshal mem info JSON: %v\n", err)
		return false
	}
	usageStr := memInfo["使用率"]
	usageStr = strings.TrimSuffix(usageStr, "%")
	usage, err := strconv.ParseFloat(usageStr, 64)
	if err != nil {
		fmt.Printf("Failed to parse mem usage: %v\n", err)
		return false
	}

	if usage > threshold {
		alertContent := fmt.Sprintf("告警信息: 主机 %s 的内存使用率超过了%.2f%%，当前值为: %.2f%%\n", hostname, threshold, usage)
		sendAlertToFeishu("内存使用超限", alertContent)
		return true
	}
	return false
}

// 检测CPU使用率是否达到告警线
func checkCpuUsage(hostname, cpuInfoJSON string, threshold float64) bool {
	// 反序列化 CPU 信息数组
	var cpuInfos []map[string]string
	err := json.Unmarshal([]byte(cpuInfoJSON), &cpuInfos)
	if err != nil {
		fmt.Printf("Failed to unmarshal CPU info JSON: %v\n", err)
		return false
	}

	for _, cpuInfo := range cpuInfos {
		usageStr := cpuInfo["使用率"]
		usageStr = strings.TrimSuffix(usageStr, "%")
		usage, err := strconv.ParseFloat(usageStr, 64)
		if err != nil {
			fmt.Printf("Failed to parse CPU usage: %v\n", err)
			continue
		}

		if usage > threshold {
			alertContent := fmt.Sprintf("告警信息: 主机 %s 的 CPU 使用率超过了 %.2f%%，当前值为: %.2f%%\n", hostname, threshold, usage)
			sendAlertToFeishu("CPU使用超限", alertContent)
			return true
		}
	}

	return false
}

// 检测磁盘空间使用率是否达到告警线
func checkDiskUsage(hostname, diskInfoJSON string, threshold float64, db *sql.DB, dataEntryID string, resultTime string) bool {
	var diskInfos []map[string]string
	err := json.Unmarshal([]byte(diskInfoJSON), &diskInfos)
	if err != nil {
		fmt.Printf("Failed to unmarshal disk info JSON: %v\n", err)
		return false
	}

	alerts := []string{}
	for _, diskInfo := range diskInfos {
		usageStr := diskInfo["使用率"]
		usageStr = strings.TrimSuffix(usageStr, "%")
		usage, err := strconv.ParseFloat(usageStr, 64)
		if err != nil {
			fmt.Printf("Failed to parse disk usage: %v\n", err)
			continue
		}

		if usage > threshold {
			alertContent := fmt.Sprintf("告警信息: 主机 %s 的挂载盘 %s 磁盘空间使用率超过了 %.2f%%，当前值为: %.2f%%", hostname, diskInfo["挂载点"], threshold, usage)
			alerts = append(alerts, alertContent)
		}
	}

	if len(alerts) > 0 {
		for _, alert := range alerts {
			sendAlertToFeishu("磁盘空间使用超限", alert)
			ID := uuid.New().String()
			saveLogEntryToDB(db, ID, dataEntryID, hostname, "磁盘空间使用超限告警", alert, resultTime)
		}
		return true
	}

	return false
}

// 存储监控数据到数据库
func saveDataEntryToDB(db *sql.DB, entry DataEntry) error {
	query := `INSERT INTO alarm_data (
		id, host_name, host_info, mem_info, 
		cpu_info, disk_info, disk_io_info, result_time
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	// 打印执行的SQL语句
	fullQuery := fmt.Sprintf("INSERT INTO alarm (id, host_name, host_info, mem_info, cpu_info, disk_info, disk_io_info,result_time) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s')",
		entry.ID,
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
		entry.ID,
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

// 存储告警记录到数据库
func saveLogEntryToDB(db *sql.DB, ID string, Data_ID string, HostName string, Alarm_Type string, Alarm_Data string, ResultTime string) error {
	query := `INSERT INTO alarm_log (
		id, data_id, host_name, alarm_type, alarm_data, result_time
	) VALUES (?, ?, ?, ?, ?, ?)`

	_, err := db.Exec(query,
		ID,
		Data_ID,
		HostName,
		Alarm_Type,
		Alarm_Data,
		ResultTime,
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

	var DataEntry DataEntry
	err = json.Unmarshal(body, &DataEntry)
	if err != nil {
		http.Error(w, "Failed to unmarshal JSON", http.StatusBadRequest)
		return
	}

	// 生成 UUID
	DataEntry.ID = uuid.New().String()

	// 打印日志条目
	fmt.Printf("Received log entry: %+v\n", DataEntry)

	db, err := sql.Open("mysql", config.Mysql_Dsn)
	if err != nil {
		http.Error(w, "Failed to connect to database", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// 保存日志条目到数据库
	err = saveDataEntryToDB(db, DataEntry)
	if err != nil {
		http.Error(w, "Failed to save log entry to database", http.StatusInternalServerError)
		return
	}

	// 检测内存使用率
	if checkMemUsage(DataEntry.HostName, DataEntry.MemInfo, config.Mem_Usage_Max) {
		Alarm_Type := "内存使用超限告警"
		ID := uuid.New().String()
		// 保存告警记录
		saveLogEntryToDB(db, ID, DataEntry.ID, DataEntry.HostName, Alarm_Type, DataEntry.MemInfo, DataEntry.ResultTime)
	}

	// 检测CPU使用率
	if checkCpuUsage(DataEntry.HostName, DataEntry.CPUInfo, config.Cpu_Usage_Max) {
		Alarm_Type := "CPU使用超限告警"
		ID := uuid.New().String()
		// 保存告警记录
		saveLogEntryToDB(db, ID, DataEntry.ID, DataEntry.HostName, Alarm_Type, DataEntry.CPUInfo, DataEntry.ResultTime)
	}

	// 检测磁盘空间使用率
	checkDiskUsage(DataEntry.HostName, DataEntry.DiskInfo, config.Disk_Usage_Max, db, DataEntry.ID, DataEntry.ResultTime)

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
