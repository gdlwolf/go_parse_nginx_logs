package gtool

import (
	"fmt"
	"github.com/natefinch/lumberjack"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
)

//日志
func initLogrus() {

	logFile := viper.GetString("log.logfile")
	logMaxSize := viper.GetInt("log.maxSize")
	logMaxBackups := viper.GetInt("log.maxBackups")
	logMaxAge := viper.GetInt("log.maxAge")
	logCompress := viper.GetBool("log.compress")

	//格式化日志
	formatter := &log.TextFormatter{
		// 不需要彩色日志
		DisableColors: false,
		// 定义时间戳格式
		TimestampFormat: "2006-01-02 15:04:05",
		//ForceColors: true,
		FullTimestamp: true,
	}
	log.SetFormatter(formatter)

	//日志级别
	logLevel := strings.ToLower(viper.GetString(`log.level`))
	//日志级别
	if logLevel == "info" {
		//log.SetFormatter(new(MyFormatter))
		log.SetLevel(log.InfoLevel)
	} else if "debug" == logLevel {
		log.SetLevel(log.DebugLevel)
		log.AddHook(NewContextHook())
	}

	//	输出到控制台还是文件

	//配合lumberjack输出到文件
	writer1 := &lumberjack.Logger{
		// 日志输出文件路径
		//Filename: "foo.log",
		Filename: logFile,
		// 日志文件最大 size, 单位是 MB
		//MaxSize: 500, // megabytes
		MaxSize: logMaxSize, // megabytes
		// 最大过期日志保留的个数
		//MaxBackups: 3,
		MaxBackups: logMaxBackups,
		// 保留过期文件的最大时间间隔,单位是天
		//MaxAge: 28, //days
		MaxAge: logMaxAge, //days
		// 是否需要压缩滚动日志, 使用的 gzip 压缩
		//Compress: false, // disabled by default
		Compress: logCompress, // disabled by default
	}

	logger2 := os.Stdout
	//配置文件中日志输出到那里
	log_out := strings.ToLower(viper.GetString(`log.out`))
	if "console" == log_out {
		log.SetOutput(logger2)
	} else if "file" == log_out {
		log.SetOutput(writer1)
	} else {
		multi_writer := io.MultiWriter(writer1, logger2)
		log.SetOutput(multi_writer)
	}

}

// ContextHook for log the call context
type contextHook struct {
	Field  string
	Skip   int
	levels []log.Level
}

// NewContextHook use to make an hook
// 根据上面的推断, 我们递归深度可以设置到5即可.
func NewContextHook(levels ...log.Level) log.Hook {
	hook := contextHook{
		Field:  "line",
		Skip:   5,
		levels: levels,
	}

	if len(hook.levels) == 0 {
		hook.levels = log.AllLevels
	}

	return &hook
}

// Levels implement levels
func (hook contextHook) Levels() []log.Level {
	return log.AllLevels
}

// Fire implement fire
func (hook contextHook) Fire(entry *log.Entry) error {
	entry.Data[hook.Field] = findCaller(hook.Skip)
	return nil
}

// 对caller进行递归查询, 直到找到非logrus包产生的第一个调用.
// 因为filename我获取到了上层目录名, 因此所有logrus包的调用的文件名都是 logrus/...
// 因此通过排除logrus开头的文件名, 就可以排除所有logrus包的自己的函数调用
func findCaller(skip int) string {
	file := ""
	line := 0
	for i := 0; i < 10; i++ {
		file, line = getCaller(skip + i)
		if !strings.HasPrefix(file, "logrus") {
			break
		}
	}
	return fmt.Sprintf("%s:%d", file, line)
}

// 这里其实可以获取函数名称的: fnName := runtime.FuncForPC(pc).Name()
// 但是我觉得有 文件名和行号就够定位问题, 因此忽略了caller返回的第一个值:pc
// 在标准库log里面我们可以选择记录文件的全路径或者文件名, 但是在使用过程成并发最合适的,
// 因为文件的全路径往往很长, 而文件名在多个包中往往有重复, 因此这里选择多取一层, 取到文件所在的上层目录那层.
func getCaller(skip int) (string, int) {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "", 0
	}
	n := 0
	for i := len(file) - 1; i > 0; i-- {
		if file[i] == '/' {
			n++
			if n >= 2 {
				file = file[i+1:]
				break
			}
		}
	}
	return file, line
}

type MyFormatter struct{}

func (s *MyFormatter) Format(entry *log.Entry) ([]byte, error) {
	timestamp := time.Now().Local().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf("%s [ %5s ] %s \n", timestamp, strings.ToUpper(entry.Level.String()), entry.Message)
	return []byte(msg), nil
}
