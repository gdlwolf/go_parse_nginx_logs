package gtool

import (
	"github.com/spf13/viper"
	"path/filepath"
)
import log "github.com/sirupsen/logrus"

//加载配置
func LoadViperConfig() {
	currentDir := GetCurrentDir()
	absConfigPath := filepath.Join(currentDir, "./configs")

	viper.SetConfigName("viper_config.yml") //指定配置文件的文件名称(不需要制定配置文件的扩展名)
	//viper.SetConfigName("config")
	//viper.AddConfigPath(".")    // 设置配置文件和可执行二进制文件在用一个目录
	viper.SetConfigType("yaml")
	//viper.SetConfigType("toml")
	viper.AddConfigPath(absConfigPath)
	err := viper.ReadInConfig() // 根据以上配置读取加载配置文件
	if err != nil {
		log.Fatal(err) // 读取配置文件失败致命错误
	}

}
