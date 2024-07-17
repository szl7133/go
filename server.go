// 服务器端代码
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	_ "github.com/go-sql-driver/mysql"
)

type LogEntry struct {
	CurrentTime string              `json:"当前时间"`
	MemInfo     map[string]string   `json:"内存信息"`
	HostInfo    map[string]string   `json:"主机信息"`
	CPUInfo     []map[string]string `json:"CPU信息"`
	DiskInfo    string              `json:"磁盘信息"`    // JSON string
	DiskIOInfo  string              `json:"磁盘I/O信息"` // JSON string
}

func saveLogEntryToDB(db *sql.DB, entry LogEntry) error {
	query := `INSERT INTO alarm (
		result_time, mem_total, mem_available, mem_used, mem_free, mem_used_percent, 
		host_name, host_os, host_platform, host_kernel, cpu_model, cpu_cores, cpu_usage, 
		disk_info, disk_io_info
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// 打印执行的SQL语句
	fullQuery := fmt.Sprintf("INSERT INTO alarm (result_time, mem_total, mem_available, mem_used, mem_free, mem_used_percent, host_name, host_os, host_platform, host_kernel, cpu_model, cpu_cores, cpu_usage, disk_info, disk_io_info) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s')",
		entry.CurrentTime,
		entry.MemInfo["总量"],
		entry.MemInfo["可用"],
		entry.MemInfo["已使用"],
		entry.MemInfo["空闲"],
		entry.MemInfo["使用率"],
		entry.HostInfo["主机名称"],
		entry.HostInfo["系统"],
		entry.HostInfo["平台"],
		entry.HostInfo["内核"],
		entry.CPUInfo[0]["型号"],
		entry.CPUInfo[0]["数量"],
		entry.CPUInfo[0]["使用率"],
		entry.DiskInfo,
		entry.DiskIOInfo,
	)
	fmt.Println("Executing SQL: ", fullQuery)

	_, err := db.Exec(query,
		entry.CurrentTime,
		entry.MemInfo["总量"],
		entry.MemInfo["可用"],
		entry.MemInfo["已使用"],
		entry.MemInfo["空闲"],
		entry.MemInfo["使用率"],
		entry.HostInfo["主机名称"],
		entry.HostInfo["系统"],
		entry.HostInfo["平台"],
		entry.HostInfo["内核"],
		entry.CPUInfo[0]["型号"],
		entry.CPUInfo[0]["数量"],
		entry.CPUInfo[0]["使用率"],
		entry.DiskInfo,
		entry.DiskIOInfo,
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

	// 连接到数据库
	dsn := "root:SDgd@2023@tcp(192.168.1.229:3306)/go"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		http.Error(w, "Failed to connect to database", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// 保存日志条目到数据库
	err = saveLogEntryToDB(db, logEntry)
	if err != nil {
		http.Error(w, "Failed to save log entry to database", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func main() {
	http.HandleFunc("/log", reportHandler)
	fmt.Println("Server listening on port 8080...")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Error starting server:", err)
	}
}
