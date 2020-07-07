package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go_parse_nginx_logs/gtool"
	"go_parse_nginx_logs/parselog"
	"gopkg.in/gomail.v2"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

/*

旧
log_format zty_all_in_one_access_log_format '"$clientRealIp" | "$server_name" | "$remote_user" | "[$time_local]" | "$request" | '
		'"$status" | "$body_bytes_sent" | "$bytes_sent" | "$http_referer" | "$http_user_agent" | '
		'"$request_time" | "$upstream_response_time" | "$upstream_addr" | "$upstream_status" | "$uid_got" | "$request_length" | "$remote_addr"';







# 新
log_format  all_access_log_format  '"$remote_addr" | "$server_name" | "$remote_user" | "[$time_local]" | '
      '"$request" | "$status" | "$request_length" | "$request_time" | '
      '"$body_bytes_sent" | "$bytes_sent" | '
      '"$http_referer" | '
	  '"$http_user_agent" | '
      '"$upstream_addr" | "$upstream_status" | "$upstream_response_time" | '
      '"$http_x_forwarded_for" | '
      '"$http_cookie"';


0 $remote_addr : 客户端地址
1 $server_name : 服务器名称，如果你的server_name配置了多个，那么这里只显示排在第一个的server_name，即时使用其他server_name访问的。
2 $remote_user : 用于HTTP基础认证服务的用户名
3 $time_local : 服务器时间（LOG Format 格式

4 $request : 代表客户端的请求地址
5 $status : HTTP响应代码
6 $request_length : 请求的长度 (包括请求的地址，http请求头和请求主体)
7 $request_time : 处理客户端请求使用的时间; 从读取客户端的第一个字节开始计时

8 $body_bytes_sent : 传输给客户端的字节数，响应头不计算在内；这个变量和Apache的mod_log_config模块中的"%B"参数保持兼容
9 $bytes_sent : 传输给客户端的字节数


10 $http_referer : url跳转来源 （https://www.baidu.com/）
11 $http_user_agent : 用户终端浏览器等信息 （"Mozilla/4.0 (compatible; MSIE 8.0; Windows NT 5.1; Trident/4.0; SV1; GTB7.0; .NET4.0C;）

12 $upstream_addr : 后台upstream的地址，即真正提供服务的主机地址 （10.10.10.100:80）
13 $upstream_status : upstream状态 （200
14 $upstream_response_time : 请求过程中，upstream响应时间（0.002）

15 $http_x_forwarded_for : 获取到最原始用户IP，或者代理IP地址。这个能作为参考。如果nginx前面没有cdn，则不要使用该值来判断是否为客户端真实ip。如果nginx前方有放置cdn，则采用该值的最左边，也就是第一个值为真实ip。注意该值可能被恶意用户所伪造。$http_x_forwarded_for有多个值的时候，其多个值之间通过“逗号+空格”分割的。

16 $http_cookie : 获取客户端发过来的所有cookie，与$cookie_name不同的是，$cookie_name只是获取单个名为xxx的cookie，而$http_cookie获取所有cookie，每个cookie之间，通过“英文分号+空格”分割的


*/

func init() {
	gtool.InitGtool()
}

func main() {
	log.Info("Nginx日志分析开始运行......")
	startT := time.Now()     //计算当前时间
	re := gtool.RegexpIsIP() //判断是否为ip的正则
	var wg sync.WaitGroup
	//分析几天前的日志
	daysAgo := viper.GetInt("Common.daysAgo")
	yesterday := startT.AddDate(0, 0, daysAgo).Format(gtool.TimeDataFormat) //计算昨天的日期，因为当前统计的是昨天日志

	configNginx := parselog.UnmarshalConfigNginx() //解析日志分析配置

	for _, v := range configNginx {
		logName := v.LogName
		log.Debugf("配置参数:logName:%s", logName)

		if !v.Enable {
			log.Infof("日志: %s 分析被禁!", logName)
			continue
		}

		logOutPut := v.LogOutPath //日志分析结果的输出路径
		if ok := gtool.FileOrPathExists(logOutPut); !ok {
			err2 := os.MkdirAll(logOutPut, os.ModePerm) // 创建文件夹
			if err2 != nil {
				log.Errorf("输出日志文件夹不存在，且创建输出日志文件夹%s失败，报错:%s", logOutPut, err2)
			}
		}

		logSrcPath := v.LogSrcPath //日志文件路径
		log.Debugf("%s 的日志文件路径:%s", logName, logSrcPath)

		// 根据logSrcPath中的yyyy-mm-dd替换为真实的日期，则推测出此次分析的nginx日志的具体路径。
		logFileName := strings.Replace(path.Base(logSrcPath), "yyyy-mm-dd", yesterday, 1)
		logFilePath := filepath.Join(filepath.Dir(logSrcPath), logFileName)

		log.Debugf("%s 分析的Nginx日志文件是: %s", logName, logFilePath)

		if !gtool.FileOrPathExists(logFilePath) {
			log.Errorf("%s 分析日志文件: %s 不存在，请检查问题!", logName, logFilePath)
			continue
		}

		multiServerName := v.MultiServerName
		log.Debugf("配置参数:multiServerName:%v", multiServerName)
		MergeServerName := v.MergeServerName
		log.Debugf("配置参数:MergeServerName:%v", MergeServerName)
		ipTop := v.IpTop
		log.Debugf("配置参数:ipTop:%v", ipTop)
		uriTop := v.UriTop
		log.Debugf("配置参数:uriTop:%v", uriTop)
		cdn := v.CDN

		uvKeys := v.UVKey
		wg.Add(1)
		// 开始分析nginx日志
		go parselog.ParseNginxLog(&wg, logFilePath, logName, logOutPut, multiServerName, MergeServerName, ipTop, uriTop, re, cdn, uvKeys, yesterday)

	}
	wg.Wait()
	tc := time.Since(startT) //计算耗时
	log.Infof("分析日志并送出到Excel总耗时:%v", tc)
	if !gtool.EmailEnable {
		log.Info("发送邮件被禁用...,日志分析结束")
		return
	}
	// 发送邮件
	emailObj := gtool.NewEmailObj()
	sendEmail := gtool.NewSendEmail()

	for _, v := range parselog.MailAttachment {
		file_name := filepath.Base(v)
		log.Debugf("发送的附件名为:%s", file_name)
		emailObj.Attach(v,
			gomail.Rename(file_name),
			gomail.SetHeader(map[string][]string{
				"Content-Disposition": []string{
					fmt.Sprintf(`attachment; filename="%s"`, mime.QEncoding.Encode("UTF-8", file_name)),
				},
			}),
		)
		log.Debugf("邮件附件:%v", v)
	}
	if err := sendEmail.DialAndSend(emailObj); err != nil {
		log.Errorf("发送邮件失败:%", err)
	}
	log.Info("日志分析并发送邮件,结束...")

}
