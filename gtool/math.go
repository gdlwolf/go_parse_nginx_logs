package gtool

import "math"

// 流量单位转换
func TrafficUnitConv(a int) (res float64, unit string) {
	unitsList := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}
	unitsIndex := 0
	res = float64(a)
	for {
		if res > 1024 {
			unitsIndex += 1
			res = res / 1024
		} else {
			return res, unitsList[unitsIndex]
		}
	}

}

//float64 保留2位小数

func Float64get3(value float64) float64 {
	return math.Trunc(value*1e3+0.5) * 1e-3
}
