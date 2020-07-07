package gtool

import (
	"fmt"
	"strconv"
)

//将float64的数值保留几位小数，然后再返回一个float64的小数。
//这里使用的是字符串拼接，性能略差，之后可以优化，研究下golang的字符串拼接。
func Float64Cut(value float64, cut int) float64 {
	cutStr := "%." + strconv.Itoa(cut) + "f"
	value, _ = strconv.ParseFloat(fmt.Sprintf(cutStr, value), 64)
	return value
}
