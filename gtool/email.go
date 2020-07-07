package gtool

import (
	"crypto/tls"
	"github.com/spf13/viper"
	"gopkg.in/gomail.v2"
)

var emailFrom string
var emailToSlice []string
var subject string
var smtp string
var smtpPort int
var smtpAccount string
var smtpPassWord string
var EmailEnable bool

var UnifyEmailObj *gomail.Message
var UnifySendEmail *gomail.Dialer
var UnifyEmailBody string
var UnifySendEmailFlag = false

func initEmailConfig() {
	emailFrom = viper.GetString("email.from")
	emailToSlice = viper.GetStringSlice("email.to")
	subject = viper.GetString("email.subject")
	smtp = viper.GetString("email.smtp")
	smtpPort = viper.GetInt("email.smtpPort")
	smtpAccount = viper.GetString("email.smtpAccount")
	smtpPassWord = viper.GetString("email.smtpPassWord")
	EmailEnable = viper.GetBool("email.enable")
}

func NewEmailObj() (m *gomail.Message) {
	m = gomail.NewMessage()
	m.SetHeader("From", emailFrom)
	m.SetHeader("To", emailToSlice...)
	m.SetHeader("Subject", subject)
	return m

}

func NewSendEmail() (d *gomail.Dialer) {
	d = gomail.NewDialer(smtp, smtpPort, smtpAccount, smtpPassWord)
	d.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	return d

}
