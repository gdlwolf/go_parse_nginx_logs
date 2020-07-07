package gtool

func InitGtool() {
	// 初始化配置
	LoadViperConfig()
	// 初始化日志
	initLogrus()
	// 初始化Email配置
	initEmailConfig()
	//	初始化钉钉报警配置
	initDingConfig()
}
