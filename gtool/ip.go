package gtool

import "github.com/lionsoul2014/ip2region/binding/golang/ip2region"
import log "github.com/sirupsen/logrus"

//获取ip2region,ip2region是非线程安全的。
func GetIp2Region() *ip2region.Ip2Region {
	absPath := GetAbsPath("ip2region.db")
	region, err := ip2region.New(absPath)
	if err != nil {
		log.Fatalf("ip2region加载失败,报错:%s", err)
	}
	log.Debug("ip2region加载成功......")
	return region
}
