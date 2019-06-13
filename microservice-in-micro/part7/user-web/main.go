package main

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/afex/hystrix-go/hystrix"
	"github.com/micro-in-cn/tutorials/microservice-in-micro/part7/basic"
	"github.com/micro-in-cn/tutorials/microservice-in-micro/part7/basic/common"
	"github.com/micro-in-cn/tutorials/microservice-in-micro/part7/basic/config"
	"github.com/micro-in-cn/tutorials/microservice-in-micro/part7/plugins/breaker"
	tracer "github.com/micro-in-cn/tutorials/microservice-in-micro/part7/plugins/tracer/jaeger"
	"github.com/micro-in-cn/tutorials/microservice-in-micro/part7/plugins/tracer/opentracing/std2micro"
	"github.com/micro-in-cn/tutorials/microservice-in-micro/part7/user-web/handler"
	"github.com/micro/cli"
	"github.com/micro/go-config/source/grpc"
	"github.com/micro/go-log"
	"github.com/micro/go-micro/registry"
	"github.com/micro/go-micro/registry/consul"
	"github.com/micro/go-web"
	"github.com/opentracing/opentracing-go"
)

var (
	appName = "user_web"
	cfg     = &userCfg{}
)

type userCfg struct {
	common.AppCfg
}

func main() {
	// 初始化配置
	initCfg()

	// 使用consul注册
	micReg := consul.NewRegistry(registryOptions)

	t, io, err := tracer.NewTracer(cfg.Name, "")
	if err != nil {
		log.Fatal(err)
	}
	defer io.Close()
	opentracing.SetGlobalTracer(t)

	// 创建新服务
	service := web.NewService(
		web.Name(cfg.Name),
		web.Version(cfg.Version),
		web.RegisterTTL(time.Second*15),
		web.RegisterInterval(time.Second*10),
		web.Registry(micReg),
		web.Address(cfg.Addr()),
	)

	// 初始化服务
	if err := service.Init(
		web.Action(
			func(c *cli.Context) {
				// 初始化handler
				handler.Init()
			}),
	); err != nil {
		log.Fatal(err)
	}

	//设置采样率
	std2micro.SetSamplingFrequency(50)
	// 注册登录接口
	handlerLogin := http.HandlerFunc(handler.Login)
	service.Handle("/user/login", std2micro.TracerWrapper(breaker.BreakerWrapper(handlerLogin)))
	// 注册退出接口
	service.Handle("/user/logout", std2micro.TracerWrapper(http.HandlerFunc(handler.Logout)))
	service.Handle("/user/test", std2micro.TracerWrapper(http.HandlerFunc(handler.TestSession)))

	hystrixStreamHandler := hystrix.NewStreamHandler()
	hystrixStreamHandler.Start()
	go http.ListenAndServe(net.JoinHostPort("", "81"), hystrixStreamHandler)

	// 运行服务
	if err := service.Run(); err != nil {
		log.Fatal(err)
	}
}

func registryOptions(ops *registry.Options) {
	consulCfg := &common.Consul{}
	err := config.C().App("consul", consulCfg)
	if err != nil {
		panic(err)
	}

	ops.Timeout = time.Second * 5
	ops.Addrs = []string{fmt.Sprintf("%s:%d", consulCfg.Host, consulCfg.Port)}
}

func initCfg() {
	source := grpc.NewSource(
		grpc.WithAddress("127.0.0.1:9600"),
		grpc.WithPath("micro"),
	)

	basic.Init(config.WithSource(source))

	err := config.C().App(appName, cfg)
	if err != nil {
		panic(err)
	}

	log.Logf("[initCfg] 配置，cfg：%v", cfg)

	return
}
