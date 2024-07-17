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
	ResultTime string `json:"当前时间"`
	HostInfo   string `json:"主机信息"`
	MemInfo    string `json:"内存信息"`
	CPUInfo    string `json:"CPU信息"`
	DiskInfo   string `json:"磁盘信息"`    // JSON string
	DiskIOInfo string `json:"磁盘I/O信息"` // JSON string
}

func saveLogEntryToDB(db *sql.DB, entry LogEntry) error {
	query := `INSERT INTO alarm (
		result_time, host_info, mem_info, 
		cpu_info, disk_info, disk_io_info
	) VALUES (?, ?, ?, ?, ?, ?)`

	// 打印执行的SQL语句
	fullQuery := fmt.Sprintf("INSERT INTO alarm (result_time, host_info, mem_info, cpu_info, disk_info, disk_io_info) VALUES ('%s', '%s', '%s', '%s', '%s', '%s')",
		entry.ResultTime,
		entry.HostInfo,
		entry.MemInfo,
		entry.CPUInfo,
		entry.DiskInfo,
		entry.DiskIOInfo,
	)
	fmt.Println("Executing SQL: ", fullQuery)

	_, err := db.Exec(query,
		entry.ResultTime,
		entry.HostInfo,
		entry.MemInfo,
		entry.CPUInfo,
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
	http.HandleFunc("/alarm", reportHandler)
	fmt.Println("Server listening on port 8080...")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Error starting server:", err)
	}
}
