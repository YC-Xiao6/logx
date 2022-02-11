/*******************************************************************

    Author: Xiao
    Date: 2021/5/12 16:52

*******************************************************************/
package logx

import (
	"gopkg.in/gomail.v2"
	"time"
)

type MailConfig struct {
	Host			string			// smtp接口地址
	Port			int				// 端口号
	User			string			// 邮件帐号
	Password 		string			// 邮件帐号对应smtp服务的授权码
	Nickname		string			// 发件人名称 需要全英文
	Subject			string			// 邮件主题
	MailSendObjs	[]string		// 日志发送对象
}

func SendMail(mc *MailConfig, fileName,body string) error {
	m := gomail.NewMessage()
	// 这种方式可以添加别名，即 nickname， 也可以直接用<code>m.SetHeader("From", MAIL_USER)</code>
	m.SetHeader("From",mc.Nickname + "<" + mc.User + ">")
	// 发送给多个用户
	m.SetHeader("To", mc.MailSendObjs...)
	// 设置邮件主题
	m.SetHeader("Subject", mc.Subject)
	// 设置邮件正文
	m.SetBody("text/html", body)
	// 发送附件
	m.Attach(fileName,gomail.Rename(time.Now().Format("2006/01/02 ")+fileName))
	d := gomail.NewDialer(mc.Host, mc.Port, mc.User, mc.Password)
	// 发送邮件
	err := d.DialAndSend(m)
	return err
}