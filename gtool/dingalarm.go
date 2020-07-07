package gtool

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go_parse_nginx_logs/gtool/dingrobot"
)

var DingEnable bool
var dingWebHook string
var dingSecret string

func initDingConfig() {
	DingEnable = viper.GetBool("dingrobot.enable")
	dingWebHook = viper.GetString("dingrobot.webhook")
	dingSecret = viper.GetString("dingrobot.secret")
}

//钉钉报警
func DingAlarm(dingTitle, dingText string, atMobiles []string, isAtAll bool) {
	/*
		title := "SSL证书过期报警"
		text := "#### SSL证书过期报警 \n > - 报警域名：%s	\n > - 当前报警时间：%v\n > - 该域名证书到期时间：%v\n > - 该域名过期剩余时间：%v 天 \n\n > ![screenshot](https://i.loli.net/2020/01/14/OknowFXK1QBmu9f.jpg)\n"
		text = fmt.Sprintf(text, dName, gts.DateCommonFormat(alarmTime), gts.DateCommonFormat(noteAfter), gts.Float64Cut(expriationDays, 2))
		atMobiles := []string{"16637238865"}
		isAtAll := false
	*/
	robot := dingrobot.NewRobot(dingWebHook)
	robot.SetSecret(dingSecret)
	err := robot.SendMarkdown(dingTitle, dingText, atMobiles, isAtAll)
	if err != nil {
		log.Errorf("钉钉机器人报警出错，报错内容:%v", err)
	}
}
