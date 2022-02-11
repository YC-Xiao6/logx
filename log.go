/*******************************************************************

    Author: Xiao
    Date: 2021/5/12 16:52

*******************************************************************/
package logx

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// 日志等级
type logLevel int

const (
	DEBUG logLevel = iota
	INFO
	WARN
	ERROR
	FATAL
	maxStorageDay = 60
	maxSize       = 1024 * 1024 * 256 	// 256 MB
	bufferSize    = 1024 * 256        	// 256 KB
	digits        = "0123456789"		// 定数
	flushInterval = 30 * time.Second	// 日志写入刷新间隔
	logShort      = "[DEBUG],[INFO],[WARN],[ERROR],[FATAL]"
	colorShort	  = "37,34,33,31,35"
)

// 字符串等级
func (lv logLevel) Str() string {
	if lv >= DEBUG && lv <= FATAL {
		levels := strings.Split(logShort,",")
		colors := strings.Split(colorShort,",")
		return "\x1b[1;"+colors[lv]+";40m"+levels[lv]+"\x1b[0m"
	}
	return "[NONE]"
}

type LoggerObj struct {
	config			*Config			// 基础配置
	size     		int64			// 累计大小 无后缀
	lpath    		string			// 文件目录 完整路径 lpath=lname+lsuffix
	lname    		string			// 文件名
	lsuffix  		string			// 文件后缀名 默认 .log
	created  		string			// 文件创建日期
	level    		logLevel		// 输出的最低日志等级 默认 DEBUG
	pool     		sync.Pool		// Pool
	lock     		sync.Mutex		// logger锁
	writer   		*bufio.Writer	// 缓存io 缓存到文件
	file     		*os.File		// 日志文件
}

// 日志的基础配置
type Config struct {
	NewOutObj		bool			// 是否使用外部新定义的日志对象 默认 false
	LogFileClose	bool			// 是否关闭日志文件的写入 默认 false
	ConsoleClose    bool			// 控制台输出  默认 false
	CallInfo 		bool			// 是否输出行号和文件名 默认 false
	ShortPath 		bool			// 短路径	默认 false
	MaxStorageDay   int				// 最大保留天数 默认 60天
	FlushInterval  	time.Duration	// 日志写入刷新间隔 默认 30s
	MaxSize  		int64			// 单个日志最大容量 默认 256MB
	Path    		string			// 文件目录 完整路径 默认 logs/log.log
	MaiOpen			bool			// 是否开启日志邮件功能
	Mail			MailConfig		// 日志邮件服务端信息
}

// 日志实例对象
var lo *LoggerObj

// 根据配置创建实例
func NewLoggerObj(c Config) *LoggerObj{
	if c.MaiOpen && !c.LogFileClose{
		checkMailInfo(&c.Mail)
	}
	if c.Path == "" {
		c.Path = "logs/log.log"
	}
	if c.FlushInterval == 0{
		c.FlushInterval = flushInterval
	}
	if c.MaxStorageDay == 0 {
		c.MaxStorageDay = maxStorageDay
	}
	if c.MaxSize == 0{
		c.MaxSize = maxSize
	}
	if c.NewOutObj{
		nlo := new(LoggerObj)
		nlo.config = &c
		nlo.newLogger()
		return nlo
	}
	lo = new(LoggerObj)
	lo.config = &c
	lo.newLogger()
	return lo
}

// 以默认值初始化实例
func InitLogger() {
	lpath := "logs/log.log"
	c := Config{
		MaxSize: maxSize,
		MaxStorageDay: maxStorageDay,
		FlushInterval: flushInterval,
		Path: lpath,
	}
	lo = new(LoggerObj)
	lo.config = &c
	lo.newLogger()
}

// 判断logger对象是否存在,不存在默认创建
func checkLoggerObj() {
	if lo == nil{
		InitLogger()
	}
}

func (lo *LoggerObj)newLogger() {
	lpath := lo.config.Path
	lo.lpath = lpath									// logs/app.log
	lo.lsuffix = filepath.Ext(lpath)					// .log
	lo.lname = strings.TrimSuffix(lpath, lo.lsuffix)	// logs/app
	if lo.lsuffix == "" {
		lo.lsuffix = ".log"
	}
	lo.level = DEBUG
	lo.pool = sync.Pool{
		New: func() interface{} {
			return new(buffer)
		},
	}
	if !lo.config.LogFileClose{
		os.MkdirAll(filepath.Dir(lpath), 0755)
		go lo.daemon()
		go lo.writeLogByRetreatSafely()
	}
}

// 监听退出信号，实现退出写入日志文件
func (lo *LoggerObj) writeLogByRetreatSafely() {
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT)
	lo.Warnf("Exit By %s ", <-ch)
	lo.Flush()
	os.Exit(0)
}

// 判定邮件配置所需信息
func checkMailInfo(mc *MailConfig){
	if mc.User == "" || mc.Subject == "" || mc.Nickname == "" || mc.MailSendObjs == nil ||mc.Password == ""{
		fmt.Fprintf(os.Stderr, "logs: exiting because of error: The mail function is turned on, but the configuration is incomplete!\n")
		os.Exit(0)
	}
}

// 开启邮件功能
func SetOpenMail(b bool) {
	checkLoggerObj()
	lo.SetOpenMail(b)
}

// 开启邮件功能
func (lo *LoggerObj) SetOpenMail(b bool) {
	if b {
		checkMailInfo(&lo.config.Mail)
	}
	lo.lock.Lock()
	lo.config.MaiOpen = b
	lo.lock.Unlock()
}

// 设置邮件信息
func SetMailConfig(mc MailConfig) {
	checkLoggerObj()
	lo.SetMailConfig(mc)
}

// 设置邮件信息
func (lo *LoggerObj) SetMailConfig(mc MailConfig) {
	lo.lock.Lock()
	lo.config.Mail = mc
	lo.lock.Unlock()
}

// 设置邮件发送人
func SetSendObjs(objs []string) {
	checkLoggerObj()
	lo.SetSendObjs(objs)
}

// 设置邮件信息
func (lo *LoggerObj) SetSendObjs(objs []string) {
	lo.lock.Lock()
	lo.config.Mail.MailSendObjs = objs
	lo.lock.Unlock()
}

// 设置是否关闭日志文件输出
func SetLogFileClose(b bool) {
	checkLoggerObj()
	lo.SetLogFileClose(b)
}

// 设置是否关闭日志文件输出
func (lo *LoggerObj) SetLogFileClose(b bool) {
	lo.lock.Lock()
	lo.config.LogFileClose = b
	lo.lock.Unlock()
}

// 设置实例等级
func SetLevel(lv logLevel) {
	checkLoggerObj()
	lo.SetLevel(lv)
}

// 设置输出等级
func (lo *LoggerObj) SetLevel(lv logLevel) {
	if lv < DEBUG || lv > FATAL {
		panic("非法的日志等级")
	}
	lo.lock.Lock()
	lo.level = lv
	lo.lock.Unlock()
}

// 设置最大保存天数
func SetMaxStorageDay(msd int) {
	checkLoggerObj()
	lo.SetStorageDay(msd)
}

// 设置最大保存天数
func (lo *LoggerObj) SetStorageDay(msd int) {
	lo.lock.Lock()
	lo.config.MaxStorageDay = msd
	lo.lock.Unlock()
}

// 设置最大容量
func SetMaxSize(s int64) {
	checkLoggerObj()
	lo.SetMaxSize(s)
}

// 设置最大容量
func (lo *LoggerObj) SetMaxSize(s int64) {
	lo.lock.Lock()
	lo.config.MaxSize = s
	lo.lock.Unlock()
}

// 设置日志写入刷新间隔
func SetFlushInterval(d time.Duration) {
	checkLoggerObj()
	lo.SetFlushInterval(d)
}

// 设置日志写入刷新间隔
func (lo *LoggerObj) SetFlushInterval(d time.Duration) {
	lo.lock.Lock()
	lo.config.FlushInterval = d
	lo.lock.Unlock()
}

// 设置调用信息
func SetCallInfo(b bool) {
	checkLoggerObj()
	lo.SetCallInfo(b)
}

// 设置调用信息
func (lo *LoggerObj) SetCallInfo(b bool) {
	lo.lock.Lock()
	lo.config.CallInfo = b
	lo.lock.Unlock()
}

// 设置短路径
func SetShortPath(b bool) {
	checkLoggerObj()
	lo.SetCallInfo(b)
}
// 设置短路径
func (lo *LoggerObj) SetShortPath(b bool) {
	lo.lock.Lock()
	lo.config.ShortPath = b
	lo.lock.Unlock()
}

// 关闭控制台输出
func ConsoleClose(b bool) {
	checkLoggerObj()
	lo.ConsoleClose(b)
}

// 设置控制台输出是否关闭
func (lo *LoggerObj) ConsoleClose(b bool) {
	lo.lock.Lock()
	lo.config.ConsoleClose = b
	lo.lock.Unlock()
}

// 写入文件
func Flush() {
	checkLoggerObj()
	lo.Flush()
}

// 写入文件
func (lo *LoggerObj) Flush() {
	if lo.config.LogFileClose {
		return
	}
	lo.lock.Lock()
	lo.flushSync()
	lo.lock.Unlock()
}

type buffer struct {
	temp [64]byte
	bytes.Buffer
}

// 将两位数写入buf
func (buf *buffer) write2(i, d int) {
	buf.temp[i+1] = digits[d%10]
	d /= 10
	buf.temp[i] = digits[d%10]
}

// 将四位数写入buf
func (buf *buffer) write4(i, d int) {
	buf.temp[i+3] = digits[d%10]
	d /= 10
	buf.temp[i+2] = digits[d%10]
	d /= 10
	buf.temp[i+1] = digits[d%10]
	d /= 10
	buf.temp[i] = digits[d%10]
}

// 将n位数写入buf
func (buf *buffer) writeN(i, d int) int {
	j := len(buf.temp)
	for d > 0 {
		j--
		buf.temp[j] = digits[d%10]
		d /= 10
	}
	return copy(buf.temp[i:], buf.temp[j:])
}

// 生成日志头信息
// format yyyy/mm/dd hh:mm:ss [Level] [file:line] msg
func (lo *LoggerObj) header(lv logLevel, depth int) *buffer {
	now := time.Now()
	buf := lo.pool.Get().(*buffer)
	year, month, day := now.Date()
	hour, minute, second := now.Clock()
	buf.write4(0, year)
	buf.temp[4] = '/'
	buf.write2(5, int(month))
	buf.temp[7] = '/'
	buf.write2(8, day)
	buf.temp[10] = ' '
	buf.write2(11, hour)
	buf.temp[13] = ':'
	buf.write2(14, minute)
	buf.temp[16] = ':'
	buf.write2(17, second)
	buf.temp[19] = ' '
	lvNum := 20+len(lv.Str())
	copy(buf.temp[20:lvNum], lv.Str())
	buf.temp[lvNum] = ' '
	buf.Write(buf.temp[:lvNum+1])
	// 调用行号信息
	if lo.config.CallInfo {
		_, file, line, ok := runtime.Caller(3 + depth)
		if !ok {
			file = "###"
			line = 1
		}
		if lo.config.ShortPath{
			short := file
			one := false
			for i := len(file) - 1; i > 0; i-- {
				if file[i] == '/'{
					if one{
						short = file[i+1:]
						break
					}else {
						one = true
					}
				}
			}
			file = short
		}
		buf.WriteString("[ "+file)
		buf.temp[0] = ':'
		n := buf.writeN(1, line)
		buf.temp[n+1] = ' '
		buf.temp[n+2] = ']'
		buf.temp[n+3] = ':'
		buf.Write(buf.temp[:n+4])
	}
	return buf
}

// 换行输出
func (lo *LoggerObj) println(lv logLevel, args ...interface{}) {
	if lv < lo.level {
		return
	}
	buf := lo.header(lv, 0)
	fmt.Fprintln(buf, args...)
	lo.Write(buf.Bytes())
	buf.Reset()
	lo.pool.Put(buf)
}

// 格式输出
func (lo *LoggerObj) printf(lv logLevel, format string, args ...interface{}) {
	if lv < lo.level {
		return
	}
	buf := lo.header(lv, 0)
	fmt.Fprintf(buf, format, args...)
	if buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
	lo.Write(buf.Bytes())
	buf.Reset()
	lo.pool.Put(buf)
}

// 写入数据
func (lo *LoggerObj) Write(buf []byte) (n int, err error) {
	lo.lock.Lock()
	defer lo.lock.Unlock()
	if !lo.config.ConsoleClose {
		os.Stderr.Write(buf)
	}
	if lo.config.LogFileClose{
		return
	}
	if lo.file == nil {
		if err := lo.rotate(); err != nil {
			os.Stderr.Write(buf)
			lo.exit(err)
		}
	}
	// 按天切割
	if lo.created != string(buf[0:10]) {
		go lo.delete() // 每天检测一次旧文件
		if err := lo.rotate(); err != nil {
			lo.exit(err)
		}
	}
	// 按大小切割
	if lo.size+int64(len(buf)) >= lo.config.MaxSize {
		if err := lo.rotate(); err != nil {
			lo.exit(err)
		}
	}
	n, err = lo.writer.Write(buf)
	lo.size += int64(n)
	if err != nil {
		lo.exit(err)
	}
	return
}

// 删除旧日志
func (lo *LoggerObj) delete() {
	if lo.config.MaxStorageDay < 0 {
		return
	}
	dir := filepath.Dir(lo.lpath)
	fakeNow := time.Now().AddDate(0, 0, -lo.config.MaxStorageDay)
	filepath.Walk(dir, func(fpath string, info os.FileInfo, err error) error {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "logs: unable to delete old file '%s', error: %v\n", fpath, r)
			}
		}()
		if info == nil {
			return nil
		}
		// 防止误删
		if !info.IsDir() && info.ModTime().Before(fakeNow) && strings.HasSuffix(info.Name(), lo.lsuffix) {
			os.Remove(fpath)
		}
		return nil
	})
}

// rotate 切割文件
func (lo *LoggerObj) rotate() error {
	if lo.config.LogFileClose{
		return nil
	}
	now := time.Now()
	if lo.file != nil {
		lo.writer.Flush()
		lo.file.Sync()
		lo.sendmail()
		lo.file.Close()
		// 保存
		fbak := filepath.Join(lo.lname + now.Format("_2006-01-02") + lo.lsuffix)
		for i := 0; i < 1000; i++ {
			_, err := os.Stat(fbak)
			if err == nil {
				d := fmt.Sprintf("%s_%d", now.Format("_2006-01-02") ,i)
				fbak = filepath.Join(lo.lname +d+ lo.lsuffix)
			}else {
				break
			}
		}
		os.Rename(lo.lpath, fbak)
		lo.size = 0
	}
	finfo, err := os.Stat(lo.lpath)
	lo.created = now.Format("2006/01/02")
	if err == nil {
		lo.size = finfo.Size()
		lo.created = finfo.ModTime().Format("2006/01/02")
	}
	fout, err := os.OpenFile(lo.lpath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	lo.file = fout
	lo.writer = bufio.NewWriterSize(lo.file, bufferSize)
	return nil
}

// 发送邮件日志
func SendLogMail()  {
	checkLoggerObj()
	lo.sendmail()
}

// 邮件日志
func (lo *LoggerObj) sendmail() {
	if lo.config.MaiOpen && lo.file != nil{
		b,err := ioutil.ReadAll(lo.file)
		if err != nil{
			fmt.Fprintf(os.Stderr, "logs: log sendmail error: %s\n", err)
			return
		}
		err = SendMail(&lo.config.Mail,lo.file.Name(),string(b))
		if err != nil{
			fmt.Fprintf(os.Stderr, "logs: log sendmail error: %s\n", err)
		}
	}
}

// 定时写入日志文件
func (lo *LoggerObj) daemon() {
	for range time.NewTicker(lo.config.FlushInterval).C {
		lo.Flush()
	}
}

// 不能锁
func (lo *LoggerObj) flushSync() {
	if lo.file != nil {
		lo.writer.Flush() // 写入底层数据
		lo.file.Sync()    // 同步到磁盘
	}
}

func (lo *LoggerObj) exit(err error) {
	fmt.Fprintf(os.Stderr, "logs: exiting because of error: %s\n", err)
	lo.flushSync()
	lo.sendmail()
	os.Exit(0)
}


// -------- 实例 loggerObj对象

func Debug(args ...interface{}) {
	checkLoggerObj()
	lo.println(DEBUG, args...)
}

func Debugf(format string, args ...interface{}) {
	checkLoggerObj()
	lo.printf(DEBUG, format, args...)
}
func Info(args ...interface{}) {
	checkLoggerObj()
	lo.println(INFO, args...)
}

func Infof(format string, args ...interface{}) {
	checkLoggerObj()
	lo.printf(INFO, format, args...)
}

func Warn(args ...interface{}) {
	checkLoggerObj()
	lo.println(WARN, args...)
}

func Warnf(format string, args ...interface{}) {
	checkLoggerObj()
	lo.printf(WARN, format, args...)
}

func Error(args ...interface{}) {
	checkLoggerObj()
	lo.println(ERROR, args...)
}

func Errorf(format string, args ...interface{}) {
	checkLoggerObj()
	lo.printf(ERROR, format, args...)
}

func Fatal(args ...interface{}) {
	checkLoggerObj()
	lo.println(FATAL, args...)
	os.Exit(0)
}
func Fatalf(format string, args ...interface{}) {
	checkLoggerObj()
	lo.printf(FATAL, format, args...)
	os.Exit(0)
}
func Writer() io.Writer {
	checkLoggerObj()
	return lo
}

// -------- 实例 方法

func (lo *LoggerObj) Debug(args ...interface{}) {
	lo.println(DEBUG, args...)
}

func (lo *LoggerObj) Debugf(format string, args ...interface{}) {
	lo.printf(DEBUG, format, args...)
}
func (lo *LoggerObj) Info(args ...interface{}) {
	lo.println(INFO, args...)
}

func (lo *LoggerObj) Infof(format string, args ...interface{}) {
	lo.printf(INFO, format, args...)
}

func (lo *LoggerObj) Warn(args ...interface{}) {
	lo.println(WARN, args...)
}

func (lo *LoggerObj) Warnf(format string, args ...interface{}) {
	lo.printf(WARN, format, args...)
}

func (lo *LoggerObj) Error(args ...interface{}) {
	lo.println(ERROR, args...)
}

func (lo *LoggerObj) Errorf(format string, args ...interface{}) {
	lo.printf(ERROR, format, args...)
}

func (lo *LoggerObj) Fatal(args ...interface{}) {
	lo.println(FATAL, args...)
	lo.Flush()
	os.Exit(0)
}

func (lo *LoggerObj) Fatalf(format string, args ...interface{}) {
	lo.printf(FATAL, format, args...)
	lo.Flush()
	os.Exit(0)
}

func (lo *LoggerObj) Writer() io.Writer {
	return lo
}