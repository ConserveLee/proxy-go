package server

import (
	"context"
	"fmt"
	"github.com/go-ping/ping"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"
	"os"
	"sync"
	"time"
)

type HeartbeatConfig struct {
	interval         time.Duration         `yaml:"interval"`
	timeout          time.Duration         `yaml:"timeout"`
	maxMissed        int                   `yaml:"max_missed_heartbeats"`
	maxMissedTimeout time.Duration         `yaml:"max_missed_heartbeats_timeout"`
	Heartbeats       map[string]*Heartbeat `yaml:"heartbeats"`
}

type Heartbeat struct {
	IP    string `yaml:"ip"`
	State uint8
}

const (
	Offline = iota
	Active
	Reconnecting
	configFile = "heartbeats.yaml"
)

var (
	Instance   *HeartbeatConfig
	loadErr    error
	configOnce sync.Once
)

func init() {
	Instance = &HeartbeatConfig{
		interval:         30 * time.Second,
		timeout:          10 * time.Second,
		maxMissedTimeout: 15 * time.Second,
		Heartbeats:       make(map[string]*Heartbeat),
	}
	loadErr = Instance.LoadConfig()
	if loadErr != nil {
		panic(loadErr)
	}
	fmt.Println(Instance.Heartbeats)
	fmt.Printf("loaderr: %+v\n", loadErr)
}

func StartHeartbeatChecker() (errs []error) {
	ticker := time.NewTicker(Instance.interval)
	defer ticker.Stop()

	for range ticker.C {
		go func() {
			// 创建带超时的上下文
			ctx, cancel := context.WithTimeout(context.Background(), Instance.maxMissedTimeout+Instance.timeout)
			defer cancel()
			// 执行任务
			if innerErrs := checkHeartbeat(ctx); len(errs) > 0 {
				//每一轮的错误
				errs = innerErrs
			}
			for s, heartbeat := range Instance.Heartbeats {
				fmt.Println(s, heartbeat)
			}
		}()
	}
	return
}

func (c *HeartbeatConfig) LoadConfig() (loadErr error) {
	configOnce.Do(func() {
		// 初始化默认配置
		file, err := os.Open(configFile)
		if err != nil {
			loadErr = err
			return
		}
		defer func(file *os.File) {
			_ = file.Close()
		}(file)

		// 尝试解码配置文件
		if err := yaml.NewDecoder(file).Decode(*c); err != nil {
			loadErr = fmt.Errorf("config decode error: %w", err)
			return
		}
	})
	return
}

func Ping(ip string) (bool, error) {
	p, err := ping.NewPinger(ip)
	if err != nil {
		return false, fmt.Errorf("pinger creation failed: %w", err)
	}

	p.SetPrivileged(false)
	// 配置参数
	p.Count = Instance.maxMissed
	p.Timeout = Instance.timeout
	p.Interval = 500 * time.Millisecond

	err = p.Run()
	if err != nil {
		return false, fmt.Errorf("ping failed: %w", err)
	}

	return p.Statistics().PacketsRecv > 0, nil
}

func updateHeartbeatStatus(index string, ok bool) {
	if ok {
		Instance.Heartbeats[index].State = Active
		return
	}
	Instance.Heartbeats[index].State = Offline
}

func checkHeartbeat(ctx context.Context) (errs []error) {
	innerCtx, cancel := context.WithTimeout(ctx, Instance.maxMissedTimeout)
	defer cancel()
	g, innerCtx := errgroup.WithContext(innerCtx)

	for index, heartbeat := range Instance.Heartbeats {
		ip, index := heartbeat.IP, index
		g.Go(func() error {
			ok, err := Ping(ip)
			if ok {
				updateHeartbeatStatus(index, ok)
				return nil
			}
			errs = append(errs, fmt.Errorf("ping %s: %w", ip, err))
			return err
		})
	}
	if err := g.Wait(); err != nil {
		errs = append(errs, err)
	}
	return
}
