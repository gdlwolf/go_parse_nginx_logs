package parselog

import (
	"bufio"
	"fmt"
	"github.com/360EntSecGroup-Skylar/excelize/v2"
	"github.com/emirpasic/gods/sets/hashset"
	"github.com/lionsoul2014/ip2region/binding/golang/ip2region"
	"github.com/mholt/archiver/v3"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go_parse_nginx_logs/gtool"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 要发送的邮件附件
var MailAttachment []string

type IPInfo struct {
	Count       int    //ip访问次数
	UpTraffic   int    // 上传流量
	DownTraffic int    //下载流量
	Zone        string //地区
}

type URIInfo struct {
	Count       int // uri访问次数
	Code2xx     int // code 200的次数
	Code3xx     int
	Code4xx     int
	Code5xx     int
	UpCode2xx   int // 上游服务器响应码200的次数
	UpCode3xx   int
	UpCode4xx   int
	UpCode5xx   int
	ReqTimeMin  float64 // 请求响应“最小"时间
	ReqTimeAvg  float64 // 请求响应“平均"时间  ，这里存放的其实不是平均值，而是时间总和，之后除以次数，就求出平均值了。
	ReqTimeMax  float64 // 请求响应“最大"时间
	UpTimeMin   float64 // 上游服务器响应“最小”时间
	UpTimeAvg   float64
	UpTimeMax   float64
	UpTraffic   int // 上传流量
	DownTraffic int // 下载流量

}

type BandwidthCount struct {
	UpTraffic    int // 上传带宽
	DownTraffic  int // 下载带宽
	TotalTraffic int // 总带宽
}
type ServerInfo struct {
	IPsInfoMap       map[string]*IPInfo
	URISInfo         map[string]*URIInfo
	UpTrafficTotal   int                        // 上传总流量
	DownTrafficTotal int                        // 下载总流量
	PV               int                        // pv
	UV               int                        // UV
	Bandwidth        map[string]*BandwidthCount //带宽
}

type ServersInfo struct {
	ServersInfo map[string]*ServerInfo
}

type MergeServerName struct {
	MergeName    string
	MergeMembers []string
}

type ConfigNginxLog struct {
	LogName         string
	LogSrcPath      string
	LogOutPath      string
	CDN             bool
	IpTop           int
	UriTop          int
	UVKey           []string
	Enable          bool // 是否启用
	MultiServerName bool //是否为all in one 多server_name日志
	MergeServerName []MergeServerName
}

//解析配置文件
func UnmarshalConfigNginx() (c []ConfigNginxLog) {

	err := viper.UnmarshalKey("NginxLog", &c)
	if err != nil {
		log.Fatalf("关于NginxLog的配置有误,报错:", err)
	}
	return c

}

//分析Nginx日志
func ParseNginxLog(wg *sync.WaitGroup, logFilePath, logName, outPath string, multiServerName bool, mergeServerName []MergeServerName, ipTop, uriTop int, re *regexp.Regexp, cdn bool, uvKeys []string, yesterday string) {
	defer wg.Done()
	now := time.Now()

	region := gtool.GetIp2Region() //加载ip2region，ip2region是线程不安全的。
	defer region.Close()

	log.Infof("%s 开始日志分析......", logName)
	f, err := os.Open(logFilePath)
	defer f.Close()
	if err != nil {
		log.Errorf("%s 日志文件: %s 无法读取", logName, logFilePath)
		return
	}

	r := bufio.NewReaderSize(f, 4096)
	serversInfo := new(ServersInfo)                        // 将分析统计的信息存储到该结构对象中
	serversInfo.ServersInfo = make(map[string]*ServerInfo) // 凡是map都需要通过make来初始化，否则无法使用
	// 循环读取nginx日志到serversInfo struct中
	ReadNginxLog(r, multiServerName, mergeServerName, logName, serversInfo, region, re, cdn, uvKeys, yesterday)

	tc := time.Since(now) //计算耗时

	log.Infof("%s 仅日志分析不包含输出，总共耗时:%v", logName, tc)

	// 输出结果到Excel中
	var ipInfosSheet = []string{"ip_by_count", "ip_by_up_traffic", "ip_by_down_traffic"}
	var uriInfosSheet = []string{"uri_by_count", "uri_by_request_time", "uri_by_up_traffic", "uri_by_down_traffic"}
	var bandwidth_InfosSheet = []string{"bandwidth_count"}
	var tmpExcelSlice []string

	log.Debugf("log_name:%s 开始...输出到Excel....", logName)

	for i, v := range serversInfo.ServersInfo {
		tmpExcelSlice = OutPutExcel(i, v, ipInfosSheet, uriInfosSheet, ipTop, uriTop, outPath, tmpExcelSlice, bandwidth_InfosSheet, yesterday)

	}

	log.Debugf("log_name:%s 结束...输出到Excel....", logName)

	//multiServerName为真时，将所有的excel文件打包为xxx.tar.gz文件
	if multiServerName {

		//yesterday := now.AddDate(0, 0, daysAgo).Format(gtool.TimeDataFormat)

		tarGz := logName + "_" + yesterday + ".tar.gz"
		tarGz = filepath.Join(outPath, tarGz)

		if gtool.FileOrPathExists(tarGz) {
			err := os.Remove(tarGz)
			if err != nil {
				log.Errorf("判断包 %v 已经存在，重新打包前删除，然删除失败，报错:%v", tarGz, err)
			}
		}

		err := archiver.Archive(tmpExcelSlice, tarGz)
		if err != nil {
			log.Errorf("多server_name结果，打包tar.gz报错:%s", err)
		}

		MailAttachment = append(MailAttachment, tarGz)
	} else {
		if tmpExcelSlice == nil {
			log.Error("tmpExcelSlice is nil")
		} else {
			MailAttachment = append(MailAttachment, tmpExcelSlice[0])
		}
	}

}

/*
   循环读取日志，并记录到struct中
   形参multiServerName判断nginx日志是否按多server_name来处理
*/
func ReadNginxLog(r *bufio.Reader, multiServerName bool, mergeServerName []MergeServerName, logName string, serversInfo *ServersInfo, region *ip2region.Ip2Region, re *regexp.Regexp, cdn bool, uvKeys []string, yesterday string) {

	logSplit := make([]string, 17) //将单条日志拆后，放到切片中。
	uvKeysSet := hashset.New()     // 统计uv时，将用户唯一标识放入uvKeysSet中。

	for {
		line, err := gtool.ReadLine(r)
		if err == io.EOF {
			break
		}

		logSplit = strings.Split(line, " | ")

		//获取时间
		timeStr := HandleTimeFiled(logSplit[3])
		logTime := gtool.Str2Time(gtool.TimeNginxFormat, timeStr)
		logDataStr := logTime.Format(gtool.TimeDataFormat)
		// 如果日志中的时间不等于我们需要分析几天前日期，则跳过，这条日志不是我们想要分析的目标。
		if logDataStr != yesterday {
			continue
		}

		// 统计当前日志时间中的 “分钟”
		logTimeMinuteStr := logTime.Format(gtool.TimeMinuteFormat)
		logTimeMinuteInt, err := strconv.Atoi(logTimeMinuteStr)
		gtool.SimpleCheckError(err)
		// 我们这次假设统计每5分钟的带宽
		timePlaceholderInt := (logTimeMinuteInt / 5) * 5
		var timePlaceholderStr string
		if timePlaceholderInt < 10 {
			timePlaceholderStr = "0" + strconv.Itoa(timePlaceholderInt)
		} else {
			timePlaceholderStr = strconv.Itoa(timePlaceholderInt)
		}
		// 该条日志应该算作哪个时间点或者时间段的带宽呢？
		bandwidthMapKey := logTime.Format(gtool.TimeFormatSpecial1)
		bandwidthMapKey = strings.Replace(bandwidthMapKey, "placeholder", timePlaceholderStr, 1)

		//获取IP地址
		var ipAddr string

		if !cdn { // 如果没有cdn，则直接使用$remote_addr作为ip地址
			ipAddr = HandleFiled(logSplit[0])
		} else {
			ipAddr = GetIP(logSplit[0], logSplit[15], re)
		}

		//获取主机名，默认取日志的server_name变量，如果multiServerName为false ，则将日志当做一个站点的日志来分析
		serverName := GetServerName(logSplit[1])

		if !multiServerName {
			serverName = logName
		} else {
			if mergeServerName != nil {
				for _, v := range mergeServerName {
					if contain, _ := gtool.Contain(serverName, v.MergeMembers); contain {
						serverName = v.MergeName
					}
				}
			}
		}

		//获取下载流量,即服务器的上传流量
		downTraffic, ok := GetDownTraffic(logSplit[9])
		if !ok {
			continue
		}
		//获取上传流量，即服务器的下载流量
		upTraffic, ok := GetUpTraffic(logSplit[6])
		if !ok {
			continue
		}
		//获取uri/api
		uri, args := GetURI(logSplit[4])

		// 获取$status
		statusCodeInt, err := GetStatus(logSplit[5])
		if err != nil {
			log.Error(err)
		}

		//获取$upstream_status
		upStatusCodeInt, err := GetUpStatus(logSplit[13])
		if err != nil {
			log.Error(err)
		}

		//获取$request_time
		requestTimeFloat64, err := GetRequestTime(logSplit[7])
		if err != nil {
			log.Error(err)
		}

		//获取$upstream_response_time
		upstreamRespTimeFloat64, err := GetUpstreamTime(logSplit[14])
		if err != nil {
			log.Error(err)
		}
		// 获取$http_cookie
		httpCookie := HandleFiled(logSplit[16])

		// 开始统计------------------------------------------------------------
		if _, ok := serversInfo.ServersInfo[serverName]; !ok {
			serverInfo := new(ServerInfo)
			serversInfo.ServersInfo[serverName] = serverInfo
			serversInfo.ServersInfo[serverName].IPsInfoMap = make(map[string]*IPInfo)
			serversInfo.ServersInfo[serverName].URISInfo = make(map[string]*URIInfo)
			serversInfo.ServersInfo[serverName].Bandwidth = make(map[string]*BandwidthCount)
		}

		if _, ok := serversInfo.ServersInfo[serverName].IPsInfoMap[ipAddr]; !ok {
			ipInfo := new(IPInfo)
			serversInfo.ServersInfo[serverName].IPsInfoMap[ipAddr] = ipInfo

			ipZone, err := region.BtreeSearch(ipAddr)
			if err != nil {
				log.Errorf("ip2region没有找到ip所在区域，ip是:%v;log_name:%v", ipAddr, logName)
			}

			serversInfo.ServersInfo[serverName].IPsInfoMap[ipAddr].Zone = ipZone.String()
		}

		if _, ok := serversInfo.ServersInfo[serverName].URISInfo[uri]; !ok {
			uriInfo := new(URIInfo)
			serversInfo.ServersInfo[serverName].URISInfo[uri] = uriInfo
		}

		if _, ok := serversInfo.ServersInfo[serverName].Bandwidth[bandwidthMapKey]; !ok {
			bandwidth_count := new(BandwidthCount)
			serversInfo.ServersInfo[serverName].Bandwidth[bandwidthMapKey] = bandwidth_count
		}

		// 填充BandwidthCount
		serversInfo.ServersInfo[serverName].Bandwidth[bandwidthMapKey].UpTraffic += upTraffic
		serversInfo.ServersInfo[serverName].Bandwidth[bandwidthMapKey].DownTraffic += downTraffic
		tmpTotalTraffic := upTraffic + downTraffic
		serversInfo.ServersInfo[serverName].Bandwidth[bandwidthMapKey].TotalTraffic += tmpTotalTraffic

		// 填充ipInfo数据
		serversInfo.ServersInfo[serverName].IPsInfoMap[ipAddr].Count += 1                 // ip总数统计
		serversInfo.ServersInfo[serverName].IPsInfoMap[ipAddr].UpTraffic += upTraffic     // 某个ip的上传流量统计
		serversInfo.ServersInfo[serverName].IPsInfoMap[ipAddr].DownTraffic += downTraffic // 某个ip的下载流量统计

		serversInfo.ServersInfo[serverName].UpTrafficTotal += upTraffic     // 整个站点的上传流量统计
		serversInfo.ServersInfo[serverName].DownTrafficTotal += downTraffic // 整个站点的下载流量统计
		serversInfo.ServersInfo[serverName].PV += 1                         // 整个站点的PV统计

		// 统计UV，统计UV需要几个数据，IP，args,cookie,uvKeys []string
		uvId := GetUVId(ipAddr, args, httpCookie, uvKeys)
		if !uvKeysSet.Contains(uvId) {
			serversInfo.ServersInfo[serverName].UV += 1
			uvKeysSet.Add(uvId)
		}

		// 填充uriInfo数据
		serversInfo.ServersInfo[serverName].URISInfo[uri].Count += 1
		// code统计
		HandleCode(statusCodeInt, upStatusCodeInt, serversInfo, serverName, uri)

		if requestTimeFloat64 < serversInfo.ServersInfo[serverName].URISInfo[uri].ReqTimeMin {
			serversInfo.ServersInfo[serverName].URISInfo[uri].ReqTimeMin = requestTimeFloat64
		}

		//求平均数，这里其实求的总数，最后让总数除以pv数就是平均数了。
		serversInfo.ServersInfo[serverName].URISInfo[uri].ReqTimeAvg += requestTimeFloat64

		if requestTimeFloat64 > serversInfo.ServersInfo[serverName].URISInfo[uri].ReqTimeMax {
			serversInfo.ServersInfo[serverName].URISInfo[uri].ReqTimeMax = requestTimeFloat64
		}
		if upstreamRespTimeFloat64 < serversInfo.ServersInfo[serverName].URISInfo[uri].UpTimeMin {
			serversInfo.ServersInfo[serverName].URISInfo[uri].UpTimeMin = upstreamRespTimeFloat64
		}
		serversInfo.ServersInfo[serverName].URISInfo[uri].UpTimeAvg += upstreamRespTimeFloat64
		if upstreamRespTimeFloat64 > serversInfo.ServersInfo[serverName].URISInfo[uri].UpTimeMax {
			serversInfo.ServersInfo[serverName].URISInfo[uri].UpTimeMax = upstreamRespTimeFloat64
		}

		serversInfo.ServersInfo[serverName].URISInfo[uri].UpTraffic += upTraffic     // 某个URI的上传流量统计
		serversInfo.ServersInfo[serverName].URISInfo[uri].DownTraffic += downTraffic // 某个ip的下载流量统计

	}

}

/*
统计UV时使用的id
*/
func GetUVId(ipAddr, args, httpCookie string, uvKeys []string) (uvId string) {

	//log.Debugf("func GetUVId 的参数 ipAddr: %v", ipAddr)
	//log.Debugf("func GetUVId 的参数 args: %v", args)
	//log.Debugf("func GetUVId 的参数 httpCookie: %v", httpCookie)
	//log.Debugf("func GetUVId 的参数 uvKeys: %v", uvKeys)

	//检查uvKeys
	if uvKeys == nil {
		uvId = ipAddr
		return uvId
	}

	if httpCookie == "-" && args == "" {
		uvId = ipAddr
		return uvId
	}

	var cookieSplit []string
	if httpCookie != "-" && httpCookie != "" {
		cookieSplit = strings.Split(httpCookie, "; ")
	}

	var argsSplit []string
	if args != "" {
		argsSplit = strings.Split(args, "&")
	}

	// 遍历uvKeys
	for _, v := range uvKeys {

		//判断是cookie_还是 uri_
		if strings.HasPrefix(v, "cookie_") {
			if httpCookie == "-" || httpCookie == "" {
				continue
			}

			cookieId := v[7:]
			//	判断该条日志中有没有想要的cookie？
			for _, vv := range cookieSplit {
				if strings.HasPrefix(vv, cookieId) {
					uvId = vv
					return uvId
				}
			}

			continue
		}

		if strings.HasPrefix(v, "uri_") {
			if args == "" {
				continue
			}
			argId := v[4:]
			for _, vv := range argsSplit {
				if strings.HasPrefix(vv, argId) {
					uvId = vv
					return uvId
				}
			}

		}
	}

	uvId = ipAddr
	return uvId
}

// 处理code统计
func HandleCode(statusInt, upStatusInt int, serversInfo *ServersInfo, serverName, uri string) {
	if statusInt >= 200 && statusInt < 300 {
		serversInfo.ServersInfo[serverName].URISInfo[uri].Code2xx += 1
	} else if statusInt >= 300 && statusInt < 400 {
		serversInfo.ServersInfo[serverName].URISInfo[uri].Code3xx += 1
	} else if statusInt >= 400 && statusInt < 500 {
		serversInfo.ServersInfo[serverName].URISInfo[uri].Code4xx += 1
	} else if statusInt >= 500 && statusInt < 600 {
		serversInfo.ServersInfo[serverName].URISInfo[uri].Code5xx += 1
	}

	if upStatusInt >= 200 && upStatusInt < 300 {
		serversInfo.ServersInfo[serverName].URISInfo[uri].UpCode2xx += 1
	} else if upStatusInt >= 300 && upStatusInt < 400 {
		serversInfo.ServersInfo[serverName].URISInfo[uri].UpCode3xx += 1
	} else if upStatusInt >= 400 && upStatusInt < 500 {
		serversInfo.ServersInfo[serverName].URISInfo[uri].UpCode4xx += 1
	} else if upStatusInt >= 500 && upStatusInt < 600 {
		serversInfo.ServersInfo[serverName].URISInfo[uri].UpCode5xx += 1
	}
}

func HandleTimeFiled(filed string) (timeStr string) {
	timeStr = strings.TrimSpace(filed)
	timeStr = strings.Trim(timeStr, "\"")
	timeStr = strings.Trim(timeStr, "[")
	timeStr = strings.Trim(timeStr, "]")
	timeStr = strings.Split(timeStr, " ")[0]
	return timeStr

}

//切割后的字符串处理
func HandleFiled(filed string) string {
	return strings.Trim(strings.TrimSpace(filed), "\"")
}

//日志分析结果输出到excel中
func OutPutExcel(serverName string, serverInfo *ServerInfo, ipInfoSheet, uriInfoSheet []string, ipTop, uriTop int, outPath string, tmpExcelSlice []string, bandwidth_InfosSheet []string, yesterday string) []string {

	//创建excel
	fileExcel := excelize.NewFile()
	// 总流量
	totalTraffic, unit := gtool.TrafficUnitConv(serverInfo.DownTrafficTotal + serverInfo.UpTrafficTotal)
	totalTraffic = gtool.Float64get3(totalTraffic)

	// 上传总流量
	upTrafficTotal, upTrafficUnit := gtool.TrafficUnitConv(serverInfo.UpTrafficTotal)
	upTrafficTotal = gtool.Float64get3(upTrafficTotal)

	// 下载总流量
	downTrafficTotal, downTrafficUnit := gtool.TrafficUnitConv(serverInfo.DownTrafficTotal)
	downTrafficTotal = gtool.Float64get3(downTrafficTotal)

	// IP总数
	ipTotalCount := len(serverInfo.IPsInfoMap)

	// PV总数
	pv := serverInfo.PV

	// UV总数
	uv := serverInfo.UV

	// 输出IP_info
	for _, v := range ipInfoSheet {
		fileExcel.NewSheet(v)
		excelWriter, err := fileExcel.NewStreamWriter(v)
		gtool.SimpleCheckError(err)

		err = excelWriter.SetRow("A1", []interface{}{"网站 : ", serverName})
		gtool.SimpleCheckError(err)

		err = excelWriter.SetRow("A2", []interface{}{"IP总数:", ipTotalCount, "PV总数:", pv, "UV总数:", uv})
		gtool.SimpleCheckError(err)

		err = excelWriter.SetRow("A3", []interface{}{"总流量:", totalTraffic, unit, "上传总流量", upTrafficTotal, upTrafficUnit,
			"下载总流量", downTrafficTotal, downTrafficUnit})
		gtool.SimpleCheckError(err)

		if ipTop != 0 {
			err = excelWriter.SetRow("A5", []interface{}{"Top IP :", ipTop})
			gtool.SimpleCheckError(err)
		}

		err = excelWriter.SetRow("A6", []interface{}{"IP", "访问次数", "上传流量", "单位", "下载流量", "单位", "地区"})
		gtool.SimpleCheckError(err)

		SortIPInfo(v, excelWriter, serverInfo, ipTop)

	}

	//输出URI_info
	for _, v := range uriInfoSheet {
		fileExcel.NewSheet(v)
		excelWriter, err := fileExcel.NewStreamWriter(v)
		gtool.SimpleCheckError(err)
		err = excelWriter.SetRow("A1", []interface{}{"网站 : ", serverName})
		gtool.SimpleCheckError(err)

		if uriTop != 0 {
			err = excelWriter.SetRow("A4", []interface{}{"Top URI :", uriTop})
			gtool.SimpleCheckError(err)
		}

		err = excelWriter.SetRow("A5", []interface{}{"URI",
			"访问次数",
			"上传流量",
			"单位",
			"下载流量",
			"单位",
			"code_2xx",
			"code_3xx",
			"code_4xx",
			"code_5xx",
			"up_code_2xx",
			"up_code_3xx",
			"up_code_4xx",
			"up_code_5xx",
			"req_time_min",
			"req_time_avg",
			"req_time_max",
			"upstream_time_min",
			"upstream_time_avg",
			"upstream_time_max",
		})
		gtool.SimpleCheckError(err)

		SortURIInfo(v, serverInfo, excelWriter, uriTop)
	}

	//输出带宽
	for _, v := range bandwidth_InfosSheet {
		fileExcel.NewSheet(v)
		excelWriter, err := fileExcel.NewStreamWriter(v)
		gtool.SimpleCheckError(err)
		err = excelWriter.SetRow("A1", []interface{}{"网站 : ", serverName})
		gtool.SimpleCheckError(err)
		err = excelWriter.SetRow("A2", []interface{}{"说明 : ", "带宽单位为:KB。统计的是每5分钟的带宽流量，可初略估算带宽峰值。"})
		gtool.SimpleCheckError(err)

		err = excelWriter.SetRow("A4", []interface{}{"时间",
			"上传带宽",
			"下载带宽",
			"总带宽",
		})
		gtool.SimpleCheckError(err)

		SortBindWidthInfo(v, serverInfo, excelWriter, fileExcel)
	}

	// 保存excel
	//now := time.Now()
	//yesterday := now.AddDate(0, 0, daysAgo).Format(gtool.TimeDataFormat)
	fileExcel.DeleteSheet("Sheet1")
	excelName := serverName + "_" + yesterday + ".xlsx"
	excelName = filepath.Join(outPath, excelName)
	if err2 := fileExcel.SaveAs(excelName); err2 != nil {
		log.Error(err2)
	}
	tmpExcelSlice = append(tmpExcelSlice, excelName)
	return tmpExcelSlice
}



/*
这个函数中输出到excel时使用了折线图，然而目前的360 excelize 输出的折线图是带数据标签的，这样看上去的效果不好，而且excelize现在不支持去掉这个数据标签(2020-07-07,excelize:v2.2.0)。
参考github上网友的分享：https://github.com/360EntSecGroup-Skylar/excelize/issues/657
修改了https://github.com/360EntSecGroup-Skylar/excelize/issues/657的源代码后，去掉了折线图中的数据标签。
*/
func SortBindWidthInfo(bindwidth_InfoSheet string, serverInfo *ServerInfo, excelWriter *excelize.StreamWriter, fileExcel *excelize.File) {
	var ss []gtool.SortKV
	for i, v := range serverInfo.Bandwidth {
		ss = append(ss, gtool.SortKV{Key: i, Value: v})
	}

	sort.Slice(ss, func(i, j int) bool {
		return ss[i].Key < ss[j].Key
	})

	for ii, vv := range ss {
		//	时间
		timeStr := vv.Key
		//	上传带宽
		upTraffic := float64(serverInfo.Bandwidth[vv.Key].UpTraffic) / 1024
		// 下载带宽
		downTraffic := float64(serverInfo.Bandwidth[vv.Key].DownTraffic) / 1024
		// 总带宽
		totalTraffic := float64(serverInfo.Bandwidth[vv.Key].TotalTraffic) / 1024

		err := excelWriter.SetRow("A"+strconv.Itoa(ii+5), []interface{}{
			timeStr,
			//upTraffic,
			gtool.Float64get3(upTraffic),
			//downTraffic,
			gtool.Float64get3(downTraffic),
			//totalTraffic,
			gtool.Float64get3(totalTraffic),
		})
		gtool.SimpleCheckError(err)

	}
	tmp1 := len(ss) +4

	// 插入折线图
	//	excel_line_format := `{
	//	"type": "line",
	//	"series": [{
	//		"name": "bandwidth_count!$B$4",
	//		"categories": "bandwidth_count!$A$5:$A$292",
	//		"values": "bandwidth_count!$B$5:$B$292"
	//	}, {
	//		"name": "bandwidth_count!$C$4",
	//		"categories": "bandwidth_count!$A$5:$A$292",
	//		"values": "bandwidth_count!$C$5:$C$292"
	//	}, {
	//		"name": "bandwidth_count!$D$4",
	//		"categories": "bandwidth_count!$A$5:$A$292",
	//		"values": "bandwidth_count!$D$5:$D$292"
	//	}],
	//	"legend": {
	//		"position": "top",
	//		"show_legend_key": false
	//	},
	//	"title": {
	//		"name": "每5分钟带宽峰值统计"
	//	},
	//	"plotarea": {
	//		"show_bubble_size": false,
	//		"show_cat_name": false,
	//		"show_leader_lines": false,
	//		"show_percent": false,
	//		"show_series_name": false,
	//		"show_val": false
	//	}
	//}`

	excel_line_format := `{
	"type": "line",
	"series": [{
		"name": "bandwidth_count!$B$4",
		"categories": "bandwidth_count!$A$5:$A$%d",
		"values": "bandwidth_count!$B$5:$B$%d"
	}, {
		"name": "bandwidth_count!$C$4",
		"categories": "bandwidth_count!$A$5:$A$%d",
		"values": "bandwidth_count!$C$5:$C$%d"
	}, {
		"name": "bandwidth_count!$D$4",
		"categories": "bandwidth_count!$A$5:$A$%d",
		"values": "bandwidth_count!$D$5:$D$%d"
	}],
	"legend": {
		"position": "top",
		"show_legend_key": false
	},
	"title": {
		"name": "每5分钟带宽峰值统计"
	},
	"plotarea": {
		"show_bubble_size": false,
		"show_cat_name": false,
		"show_leader_lines": false,
		"show_percent": false,
		"show_series_name": false,
		"show_val": false
	},
	"dimension": {
		"width": 1920,
		"height": 600
	},
	"y_axis": {
		"minimum" : 0
	}
}`
	excel_line_format_str := fmt.Sprintf(excel_line_format, tmp1, tmp1, tmp1, tmp1, tmp1, tmp1)
	log.Debugf("-------------- Begin --------------")
	log.Debugf(excel_line_format_str)
	log.Debugf("-------------- End --------------")

	err := fileExcel.AddChart(bindwidth_InfoSheet, "E2", excel_line_format_str)
	gtool.SimpleCheckError(err)

	if err := excelWriter.Flush(); err != nil {
		log.Error(err)
	}

}

//排序输出IP_info
func SortIPInfo(ipInfoSheet string, excelWriter *excelize.StreamWriter, serverInfo *ServerInfo, ipTop int) {

	var ss []gtool.SortKVint
	switch ipInfoSheet {
	case "ip_by_count":
		for ip, v := range serverInfo.IPsInfoMap {
			ss = append(ss, gtool.SortKVint{Key: ip, Value: v.Count})
		}

	case "ip_by_up_traffic":
		for ip, v := range serverInfo.IPsInfoMap {
			ss = append(ss, gtool.SortKVint{Key: ip, Value: v.UpTraffic})
		}

	case "ip_by_down_traffic":
		for ip, v := range serverInfo.IPsInfoMap {
			ss = append(ss, gtool.SortKVint{Key: ip, Value: v.DownTraffic})
		}

	}
	sort.Slice(ss, func(i, j int) bool {
		return ss[i].Value > ss[j].Value
	})

	var count = 0
	for ii, vv := range ss {
		// 单个ip的计数
		ipCount := serverInfo.IPsInfoMap[vv.Key].Count
		// 单个ip的上传流量
		ipUpTraffic := serverInfo.IPsInfoMap[vv.Key].UpTraffic
		ipUpTrafficFloat, ipUpTrafficFloatUnit := gtool.TrafficUnitConv(ipUpTraffic)
		ipUpTrafficFloat = gtool.Float64get3(ipUpTrafficFloat)

		// 单个ip的下载流量
		ipDownTraffic := serverInfo.IPsInfoMap[vv.Key].DownTraffic
		ipDownTrafficFloat, ipDownTrafficFloatUnit := gtool.TrafficUnitConv(ipDownTraffic)
		ipDownTrafficFloat = gtool.Float64get3(ipDownTrafficFloat)

		// 单个ip的地区
		ip_zone := serverInfo.IPsInfoMap[vv.Key].Zone

		err := excelWriter.SetRow("A"+strconv.Itoa(ii+7), []interface{}{
			vv.Key,
			ipCount,
			ipUpTrafficFloat,
			ipUpTrafficFloatUnit,
			ipDownTrafficFloat,
			ipDownTrafficFloatUnit,
			ip_zone,
		})
		gtool.SimpleCheckError(err)

		if ipTop > 0 {
			count++
			if count >= ipTop {
				break
			}
		}
	}
	if err := excelWriter.Flush(); err != nil {
		log.Error(err)
	}
}

//排序输出URI_info
func SortURIInfo(uriInfoSheet string, serverInfo *ServerInfo, excelWriter *excelize.StreamWriter, uriTop int) {
	var ss []gtool.SortKVint
	var ss1 []gtool.SortKVFloat
	var count = 0
	switch uriInfoSheet {
	case "uri_by_count":
		for uri, v := range serverInfo.URISInfo {
			ss = append(ss, gtool.SortKVint{Key: uri, Value: v.Count})
		}
		sort.Slice(ss, func(i, j int) bool {
			return ss[i].Value > ss[j].Value
		})

		for ii, vv := range ss {
			// 单个uri的计数
			uri_count := serverInfo.URISInfo[vv.Key].Count
			//	单个uri的code_2xx计数
			code_2xx := serverInfo.URISInfo[vv.Key].Code2xx
			code_3xx := serverInfo.URISInfo[vv.Key].Code3xx
			code_4xx := serverInfo.URISInfo[vv.Key].Code4xx
			code_5xx := serverInfo.URISInfo[vv.Key].Code5xx
			//	 单个uri的up_code_2xx计数
			up_code_2xx := serverInfo.URISInfo[vv.Key].UpCode2xx
			up_code_3xx := serverInfo.URISInfo[vv.Key].UpCode3xx
			up_code_4xx := serverInfo.URISInfo[vv.Key].UpCode4xx
			up_code_5xx := serverInfo.URISInfo[vv.Key].UpCode5xx

			//	单个uri的request_time
			req_time_min := serverInfo.URISInfo[vv.Key].ReqTimeMin
			req_time_avg := gtool.Float64get3(serverInfo.URISInfo[vv.Key].ReqTimeAvg / float64(uri_count))
			req_time_max := serverInfo.URISInfo[vv.Key].ReqTimeMax

			//	单个uri的upstreamt_time
			upstream_time_min := serverInfo.URISInfo[vv.Key].UpTimeMin
			upstream_time_avg := gtool.Float64get3(serverInfo.URISInfo[vv.Key].UpTimeAvg / float64(uri_count))
			upstream_time_max := serverInfo.URISInfo[vv.Key].UpTimeMax

			// 单个URI的上传流量
			uri_up_traffic := serverInfo.URISInfo[vv.Key].UpTraffic
			uri_up_traffic_float64, uri_up_traffic_unit := gtool.TrafficUnitConv(uri_up_traffic)
			uri_up_traffic_float64 = gtool.Float64get3(uri_up_traffic_float64)

			// 单个URI的下载流量
			uri_down_traffic := serverInfo.URISInfo[vv.Key].DownTraffic
			uri_down_traffic_float64, uri_down_traffic_unit := gtool.TrafficUnitConv(uri_down_traffic)
			uri_down_traffic_float64 = gtool.Float64get3(uri_down_traffic_float64)

			err := excelWriter.SetRow("A"+strconv.Itoa(ii+6), []interface{}{
				vv.Key,
				uri_count,
				uri_up_traffic_float64,
				uri_up_traffic_unit,
				uri_down_traffic_float64,
				uri_down_traffic_unit,
				code_2xx,
				code_3xx,
				code_4xx,
				code_5xx,
				up_code_2xx,
				up_code_3xx,
				up_code_4xx,
				up_code_5xx,
				req_time_min,
				req_time_avg,
				req_time_max,
				upstream_time_min,
				upstream_time_avg,
				upstream_time_max,
			})
			gtool.SimpleCheckError(err)

			if uriTop > 0 {
				count++
				if count >= uriTop {
					break
				}
			}
		}

	case "uri_by_request_time":
		for uri, v := range serverInfo.URISInfo {
			ss1 = append(ss1, gtool.SortKVFloat{Key: uri, Value: v.ReqTimeMax})
		}
		sort.Slice(ss1, func(i, j int) bool {
			return ss1[i].Value > ss1[j].Value
		})

		for ii, vv := range ss1 {
			// 单个uri的计数
			uri_count := serverInfo.URISInfo[vv.Key].Count
			//	单个uri的code_2xx计数
			code_2xx := serverInfo.URISInfo[vv.Key].Code2xx
			code_3xx := serverInfo.URISInfo[vv.Key].Code3xx
			code_4xx := serverInfo.URISInfo[vv.Key].Code4xx
			code_5xx := serverInfo.URISInfo[vv.Key].Code5xx
			//	 单个uri的up_code_2xx计数
			up_code_2xx := serverInfo.URISInfo[vv.Key].UpCode2xx
			up_code_3xx := serverInfo.URISInfo[vv.Key].UpCode3xx
			up_code_4xx := serverInfo.URISInfo[vv.Key].UpCode4xx
			up_code_5xx := serverInfo.URISInfo[vv.Key].UpCode5xx

			//	单个uri的request_time
			req_time_min := serverInfo.URISInfo[vv.Key].ReqTimeMin
			req_time_avg := gtool.Float64get3(serverInfo.URISInfo[vv.Key].ReqTimeAvg / float64(uri_count))
			req_time_max := serverInfo.URISInfo[vv.Key].ReqTimeMax

			//	单个uri的upstreamt_time
			upstream_time_min := serverInfo.URISInfo[vv.Key].UpTimeMin
			upstream_time_avg := gtool.Float64get3(serverInfo.URISInfo[vv.Key].UpTimeAvg / float64(uri_count))
			upstream_time_max := serverInfo.URISInfo[vv.Key].UpTimeMax

			// 单个URI的上传流量
			uri_up_traffic := serverInfo.URISInfo[vv.Key].UpTraffic
			uri_up_traffic_float64, uri_up_traffic_unit := gtool.TrafficUnitConv(uri_up_traffic)
			uri_up_traffic_float64 = gtool.Float64get3(uri_up_traffic_float64)

			// 单个URI的下载流量
			uri_down_traffic := serverInfo.URISInfo[vv.Key].DownTraffic
			uri_down_traffic_float64, uri_down_traffic_unit := gtool.TrafficUnitConv(uri_down_traffic)
			uri_down_traffic_float64 = gtool.Float64get3(uri_down_traffic_float64)

			err := excelWriter.SetRow("A"+strconv.Itoa(ii+6), []interface{}{
				vv.Key,
				uri_count,
				uri_up_traffic_float64,
				uri_up_traffic_unit,
				uri_down_traffic_float64,
				uri_down_traffic_unit,
				code_2xx,
				code_3xx,
				code_4xx,
				code_5xx,
				up_code_2xx,
				up_code_3xx,
				up_code_4xx,
				up_code_5xx,
				req_time_min,
				req_time_avg,
				req_time_max,
				upstream_time_min,
				upstream_time_avg,
				upstream_time_max,
			})
			gtool.SimpleCheckError(err)

			if uriTop > 0 {
				count++
				if count >= uriTop {
					break
				}
			}
		}

	case "uri_by_up_traffic":
		for uri, v := range serverInfo.URISInfo {
			ss = append(ss, gtool.SortKVint{Key: uri, Value: v.UpTraffic})
		}
		sort.Slice(ss, func(i, j int) bool {
			return ss[i].Value > ss[j].Value
		})

		for ii, vv := range ss {
			// 单个uri的计数
			uri_count := serverInfo.URISInfo[vv.Key].Count
			//	单个uri的code_2xx计数
			code_2xx := serverInfo.URISInfo[vv.Key].Code2xx
			code_3xx := serverInfo.URISInfo[vv.Key].Code3xx
			code_4xx := serverInfo.URISInfo[vv.Key].Code4xx
			code_5xx := serverInfo.URISInfo[vv.Key].Code5xx
			//	 单个uri的up_code_2xx计数
			up_code_2xx := serverInfo.URISInfo[vv.Key].UpCode2xx
			up_code_3xx := serverInfo.URISInfo[vv.Key].UpCode3xx
			up_code_4xx := serverInfo.URISInfo[vv.Key].UpCode4xx
			up_code_5xx := serverInfo.URISInfo[vv.Key].UpCode5xx

			//	单个uri的request_time
			req_time_min := serverInfo.URISInfo[vv.Key].ReqTimeMin
			req_time_avg := gtool.Float64get3(serverInfo.URISInfo[vv.Key].ReqTimeAvg / float64(uri_count))
			req_time_max := serverInfo.URISInfo[vv.Key].ReqTimeMax

			//	单个uri的upstreamt_time
			upstream_time_min := serverInfo.URISInfo[vv.Key].UpTimeMin
			upstream_time_avg := gtool.Float64get3(serverInfo.URISInfo[vv.Key].UpTimeAvg / float64(uri_count))
			upstream_time_max := serverInfo.URISInfo[vv.Key].UpTimeMax

			// 单个URI的上传流量
			uri_up_traffic := serverInfo.URISInfo[vv.Key].UpTraffic
			uri_up_traffic_float64, uri_up_traffic_unit := gtool.TrafficUnitConv(uri_up_traffic)
			uri_up_traffic_float64 = gtool.Float64get3(uri_up_traffic_float64)

			// 单个URI的下载流量
			uri_down_traffic := serverInfo.URISInfo[vv.Key].DownTraffic
			uri_down_traffic_float64, uri_down_traffic_unit := gtool.TrafficUnitConv(uri_down_traffic)
			uri_down_traffic_float64 = gtool.Float64get3(uri_down_traffic_float64)

			err := excelWriter.SetRow("A"+strconv.Itoa(ii+6), []interface{}{
				vv.Key,
				uri_count,
				uri_up_traffic_float64,
				uri_up_traffic_unit,
				uri_down_traffic_float64,
				uri_down_traffic_unit,
				code_2xx,
				code_3xx,
				code_4xx,
				code_5xx,
				up_code_2xx,
				up_code_3xx,
				up_code_4xx,
				up_code_5xx,
				req_time_min,
				req_time_avg,
				req_time_max,
				upstream_time_min,
				upstream_time_avg,
				upstream_time_max,
			})
			gtool.SimpleCheckError(err)

			if uriTop > 0 {
				count++
				if count >= uriTop {
					break
				}
			}
		}

	case "uri_by_down_traffic":
		for uri, v := range serverInfo.URISInfo {
			ss = append(ss, gtool.SortKVint{Key: uri, Value: v.DownTraffic})
		}
		sort.Slice(ss, func(i, j int) bool {
			return ss[i].Value > ss[j].Value
		})

		for ii, vv := range ss {
			// 单个uri的计数
			uri_count := serverInfo.URISInfo[vv.Key].Count
			//	单个uri的code_2xx计数
			code_2xx := serverInfo.URISInfo[vv.Key].Code2xx
			code_3xx := serverInfo.URISInfo[vv.Key].Code3xx
			code_4xx := serverInfo.URISInfo[vv.Key].Code4xx
			code_5xx := serverInfo.URISInfo[vv.Key].Code5xx
			//	 单个uri的up_code_2xx计数
			up_code_2xx := serverInfo.URISInfo[vv.Key].UpCode2xx
			up_code_3xx := serverInfo.URISInfo[vv.Key].UpCode3xx
			up_code_4xx := serverInfo.URISInfo[vv.Key].UpCode4xx
			up_code_5xx := serverInfo.URISInfo[vv.Key].UpCode5xx

			//	单个uri的request_time
			req_time_min := serverInfo.URISInfo[vv.Key].ReqTimeMin
			req_time_avg := gtool.Float64get3(serverInfo.URISInfo[vv.Key].ReqTimeAvg / float64(uri_count))
			req_time_max := serverInfo.URISInfo[vv.Key].ReqTimeMax

			//	单个uri的upstreamt_time
			upstream_time_min := serverInfo.URISInfo[vv.Key].UpTimeMin
			upstream_time_avg := gtool.Float64get3(serverInfo.URISInfo[vv.Key].UpTimeAvg / float64(uri_count))
			upstream_time_max := serverInfo.URISInfo[vv.Key].UpTimeMax

			// 单个URI的上传流量
			uri_up_traffic := serverInfo.URISInfo[vv.Key].UpTraffic
			uri_up_traffic_float64, uri_up_traffic_unit := gtool.TrafficUnitConv(uri_up_traffic)
			uri_up_traffic_float64 = gtool.Float64get3(uri_up_traffic_float64)

			// 单个URI的下载流量
			uri_down_traffic := serverInfo.URISInfo[vv.Key].DownTraffic
			uri_down_traffic_float64, uri_down_traffic_unit := gtool.TrafficUnitConv(uri_down_traffic)
			uri_down_traffic_float64 = gtool.Float64get3(uri_down_traffic_float64)

			err := excelWriter.SetRow("A"+strconv.Itoa(ii+6), []interface{}{
				vv.Key,
				uri_count,
				uri_up_traffic_float64,
				uri_up_traffic_unit,
				uri_down_traffic_float64,
				uri_down_traffic_unit,
				code_2xx,
				code_3xx,
				code_4xx,
				code_5xx,
				up_code_2xx,
				up_code_3xx,
				up_code_4xx,
				up_code_5xx,
				req_time_min,
				req_time_avg,
				req_time_max,
				upstream_time_min,
				upstream_time_avg,
				upstream_time_max,
			})
			gtool.SimpleCheckError(err)

			if uriTop > 0 {
				count++
				if count >= uriTop {
					break
				}
			}
		}
	}

	if err := excelWriter.Flush(); err != nil {
		log.Error(err)
	}

}

//获取ip地址
func GetIP(remoteAddr, xFFAddr string, re *regexp.Regexp) string {
	xFFAddr = HandleFiled(xFFAddr)
	remoteAddr = HandleFiled(remoteAddr)

	// 如果x-forwarded-for的ip是空，或者-，说明x-forwarded-for不正确，使用$remote_addr
	if xFFAddr == "-" || xFFAddr == "" {
		return remoteAddr
	}

	xFFAddrSplit := strings.Split(xFFAddr, ",")
	ip := strings.TrimSpace(xFFAddrSplit[0])

	// 判断x-forwarded-for的第一个ip地址是否为一个正常的ip地址，如果是则使用，否则则使用$remote_addr
	if re.MatchString(ip) {
		return ip
	}

	return remoteAddr

}

//获取主机名server_name
func GetServerName(serverName string) string {
	serverName = HandleFiled(serverName)
	if serverName != "" && serverName != "-" {
		return serverName
	}
	return "default"
}

//$bytes_sent 获取下载流量
func GetDownTraffic(downTraffic string) (int, bool) {
	downTraffic = HandleFiled(downTraffic)
	if downTraffic != "" && downTraffic != "-" {
		downTrafficInt, err := strconv.Atoi(downTraffic)
		if err != nil {
			log.Errorf("下载流量字符串转int报错:%s", err)
			return 0, false
		}
		return downTrafficInt, true
	}
	return 0, false
}

//$request_length 获取上传流量
func GetUpTraffic(upTraffic string) (int, bool) {
	upTraffic = HandleFiled(upTraffic)
	if upTraffic != "" && upTraffic != "-" {
		upTrafficInt, err := strconv.Atoi(upTraffic)
		if err != nil {
			return 0, false
		}
		return upTrafficInt, true
	}
	return 0, false

}

//获取api/uri地址
/*
注意，nginx日志中出现过"GET HTTP/1.1"的日志
*/
func GetURI(url string) (uri, args string) {
	url = HandleFiled(url)

	url_slice := strings.Split(url, " ")
	if len(url_slice) < 3 {
		return "/", ""
	}

	tmpSplit := strings.Split(url_slice[1], "?")

	uri = tmpSplit[0]
	if len(tmpSplit) == 2 {
		args = tmpSplit[1]
	} else {
		args = ""
	}

	return uri, args

}

//获取状态吗$status
func GetStatus(status string) (int, error) {
	status = HandleFiled(status)
	status_int, err := strconv.Atoi(status)
	if err != nil {
		return status_int, err
	}
	return status_int, nil
}

// 获取上游服务器返回的状态码
func GetUpStatus(upStatus string) (int, error) {
	upStatus = HandleFiled(upStatus)
	if upStatus != "-" && upStatus != "" {
		upStatus_int, err := strconv.Atoi(upStatus)
		if err != nil {
			return 0, err
		}
		return upStatus_int, nil

	}
	return 0, nil

}

//获取$request_time
func GetRequestTime(request_time string) (float64, error) {
	request_time = HandleFiled(request_time)
	float, err := strconv.ParseFloat(request_time, 64)
	if err != nil {
		return 0, err
	}
	return float, nil

}

//获取$upstream_response_time
func GetUpstreamTime(upstream_time string) (float64, error) {
	upstream_time = HandleFiled(upstream_time)
	if upstream_time != "" && upstream_time != "-" {
		float, err := strconv.ParseFloat(upstream_time, 64)
		if err != nil {
			return 0, err
		}
		return float, nil
	}
	return 0, nil
}
