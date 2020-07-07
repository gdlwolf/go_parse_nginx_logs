package gtool

import "time"

type JSTime time.Time

const (
	TimeApiFormat    = "2006-01-02 15:04:05"
	TimeDataFormat   = "2006-01-02"
	TimeJsISOFormat  = "2006-01-02T15:04:05.999Z07:00"
	TimeNginxFormat  = "02/Jan/2006:15:04:05"
	TimeOnlyFormat   = "15:04:05"
	TimeMinuteFormat = "04"

	// Nginx日志分析时间用的特殊格式化
	TimeFormatSpecial1      = "15:placeholder:00"




	/*


		str1 := "03/Jul/2020:08:39:21"
		s := strings.Split(str1, ":")[0]
		fmt.Println(s)

		timeFormat1 := "02/Jan/2006:15:04:05"
		timeFormat2 := "2006-01-02"
		timeFormat3 := "15:04:05"
		timeFormat4 := "04"

		local, _ := time.LoadLocation("Local")
		t, _ := time.ParseInLocation(timeFormat1, str1, local)
		fmt.Println(t)
		fmt.Println(t.Format(timeFormat2))
		fmt.Println(t.Format(timeFormat3))
		fmt.Println(t.Format(timeFormat4))

	*/

)

func (t *JSTime) UnmarshalJSON(data []byte) (err error) {
	now, err := time.ParseInLocation(`"`+TimeApiFormat+`"`, string(data), time.Local)
	*t = JSTime(now)
	return
}

func (t JSTime) MarshalJSON() ([]byte, error) {
	b := make([]byte, 0, len(TimeApiFormat)+2)
	b = append(b, '"')
	b = time.Time(t).AppendFormat(b, TimeApiFormat)
	b = append(b, '"')
	return b, nil
}

func (t JSTime) String() string {
	return time.Time(t).Format(TimeApiFormat)
}

/*
格式化时间
*/
func DateCommonFormat(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// 时间字符串 转 Time.time
func Str2Time(timeFormat, timeStr string) time.Time {
	local, err := time.LoadLocation("Local")
	if err != nil {
		SimpleCheckError(err)
	}
	t, err := time.ParseInLocation(timeFormat, timeStr, local)
	if err != nil {
		SimpleCheckError(err)
	}
	return t
}
