package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/mem"
	"io/ioutil"
	"net/http"
	"time"
)

type Config struct {
	HostName       string `json:"host_name"`
	ReportURL      string `json:"report_url"`
	ReportInterval int    `json:"report_interval"`
}

type LogEntry struct {
	HostName   string `json:"主机名称"`
	HostInfo   string `json:"主机信息"`
	MemInfo    string `json:"内存信息"`
	CPUInfo    string `json:"CPU信息"`
	DiskInfo   string `json:"磁盘信息"`    // JSON string
	DiskIOInfo string `json:"磁盘I/O信息"` // JSON string
	ResultTime string `json:"当前时间"`
}

// 获取内存信息
func getMemInfo() map[string]string {
	memdata := make(map[string]string)
	v, _ := mem.VirtualMemory()

	total := handerUnit(v.Total, NUM_GB, "G")
	available := handerUnit(v.Available, NUM_GB, "G")
	used := handerUnit(v.Used, NUM_GB, "G")
	free := handerUnit(v.Free, NUM_GB, "G")
	userPercent := fmt.Sprintf("%.2f", v.UsedPercent)

	memdata["总量"] = total
	memdata["可用"] = available
	memdata["已使用"] = used
	memdata["空闲"] = free
	memdata["使用率"] = userPercent + "%"

	return memdata
}

// 获取CPU信息
func getCpuInfo(percent string) []map[string]string {
	cpudatas := []map[string]string{}

	infos, err := cpu.Info()
	if err != nil {
		fmt.Printf("get cpu info failed, err:%v", err)
	}
	for _, ci := range infos {
		cpudata := make(map[string]string)
		cpudata["型号"] = ci.ModelName
		cpudata["数量"] = fmt.Sprint(ci.Cores)
		cpudata["使用率"] = percent + "%"

		cpudatas = append(cpudatas, cpudata)
	}
	return cpudatas
}

// 获取主机信息
func getHostInfo(config Config) map[string]string {
	hostdata := make(map[string]string)

	hInfo, _ := host.Info()
	hostdata["主机名称"] = config.HostName
	hostdata["系统"] = hInfo.OS
	hostdata["平台"] = hInfo.Platform + "-" + hInfo.PlatformVersion + " " + hInfo.PlatformFamily
	hostdata["内核"] = hInfo.KernelArch

	return hostdata
}

// 获取磁盘信息
func getDiskInfo() []map[string]string {
	diskdatas := []map[string]string{}
	parts, err := disk.Partitions(true)
	if err != nil {
		fmt.Printf("get Partitions failed, err:%v\n", err)
		return diskdatas
	}
	for _, part := range parts {
		diskdata := make(map[string]string)
		diskInfo, _ := disk.Usage(part.Mountpoint)
		diskdata["挂载点"] = part.Mountpoint
		diskdata["总量"] = handerUnit(diskInfo.Total, NUM_GB, "G")
		diskdata["空闲"] = handerUnit(diskInfo.Free, NUM_GB, "G")
		diskdata["已使用"] = handerUnit(diskInfo.Used, NUM_GB, "G")
		diskdata["使用率"] = fmt.Sprintf("%.2f", diskInfo.UsedPercent) + "%"

		diskdatas = append(diskdatas, diskdata)
	}
	return diskdatas
}

// 获取磁盘I/O信息
func getDiskIOInfo() []map[string]string {
	ioStats := []map[string]string{}
	ioCounters, err := disk.IOCounters()
	if err != nil {
		fmt.Printf("get IOCounters failed, err:%v\n", err)
		return ioStats
	}
	for diskName, ioCounter := range ioCounters {
		ioStat := make(map[string]string)
		ioStat["磁盘"] = diskName
		ioStat["读次数"] = fmt.Sprintf("%d", ioCounter.ReadCount)
		ioStat["写次数"] = fmt.Sprintf("%d", ioCounter.WriteCount)
		ioStat["读字节数"] = fmt.Sprintf("%d", ioCounter.ReadBytes)
		ioStat["写字节数"] = fmt.Sprintf("%d", ioCounter.WriteBytes)
		ioStat["读时间"] = fmt.Sprintf("%d", ioCounter.ReadTime)
		ioStat["写时间"] = fmt.Sprintf("%d", ioCounter.WriteTime)

		ioStats = append(ioStats, ioStat)
	}
	return ioStats
}

// 单位转换
const NUM_GB = 1024 * 1024 * 1024

func handerUnit(value uint64, unit int, unitStr string) string {
	v := float64(value) / float64(unit)
	return fmt.Sprintf("%.2f%s", v, unitStr)
}

func main() {
	// 读取命令行参数
	configFile := flag.String("config", "client-config.json", "配置文件路径")
	flag.Parse()

	// 读取配置文件
	config, err := loadConfig(*configFile)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}

	for {
		resultTime := time.Now().Format("2006-01-02 15:04:05")

		// 获取内存信息
		memInfo, _ := json.Marshal(getMemInfo())
		// 获取主机信息
		hostInfo, _ := json.Marshal(getHostInfo(config))

		// 获取CPU使用率
		cpuPercents, _ := cpu.Percent(0, false)
		cpuInfo, _ := json.Marshal(getCpuInfo(fmt.Sprintf("%.2f", cpuPercents[0])))

		// 获取磁盘信息
		diskInfo, _ := json.Marshal(getDiskInfo())

		// 获取磁盘I/O信息
		diskIOInfo, _ := json.Marshal(getDiskIOInfo())

		logEntry := LogEntry{
			HostName:   config.HostName,
			HostInfo:   string(hostInfo),
			MemInfo:    string(memInfo),
			CPUInfo:    string(cpuInfo),
			DiskInfo:   string(diskInfo),
			DiskIOInfo: string(diskIOInfo),
			ResultTime: resultTime,
		}

		logEntryJSON, err := json.Marshal(logEntry)
		if err != nil {
			fmt.Printf("json marshal failed, err:%v\n", err)
			return
		}

		resp, err := http.Post(config.ReportURL, "application/json", bytes.NewBuffer(logEntryJSON))
		if err != nil {
			fmt.Printf("send post request failed, err:%v\n", err)
			return
		}
		defer resp.Body.Close()

		fmt.Println("Response status:", resp.Status)

		time.Sleep(time.Duration(config.ReportInterval) * time.Second)
	}
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
