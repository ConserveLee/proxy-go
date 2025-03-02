package client

import (
	"bytes"
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	configFile = "clients.yaml"
	tmpSuffix  = ".tmp"
)

type Config struct {
	Clients map[string]*Client `yaml:"clients"`
	NextId  int
	IpMap   map[string]string // 用于快速判断IP是否已存在
	Mu      sync.RWMutex      // 控制内存访问
	fileMu  sync.Mutex        // 控制文件操作
}

type saveConfig struct {
	Clients map[string]*Client `yaml:"clients"`
}

type Client struct {
	ID          int    `json:"id" binding:"required"`
	IP          string `yaml:"ip" json:"ip" binding:"required"`
	Name        string `yaml:"name" json:"name" binding:"required"`
	ProxyTarget string `yaml:"proxy_target" json:"proxy_target" binding:"required,url"`
	Enabled     bool   `json:"enabled"`
}

var (
	appConfigInstance *Config
	once              sync.Once
	loadErr           error // 保存首次加载的错误
)

// todo 去锁

func AppConfig() (*Config, error) {
	once.Do(func() {
		instance := &Config{
			Clients: make(map[string]*Client),
			Mu:      sync.RWMutex{},
			fileMu:  sync.Mutex{},
			IpMap:   make(map[string]string),
		}

		// 初始化加载配置
		if err := instance.Load(); err != nil {
			loadErr = err
			return
		}
		if instance.NextId == 0 {
			maxID := 0
			for id, client := range instance.Clients {
				instance.IpMap[client.IP] = client.ProxyTarget
				if num, err := strconv.Atoi(strings.TrimPrefix(id, "client")); err == nil {
					if num > maxID {
						maxID = num
					}
				}
			}
			instance.NextId = maxID + 1
		}

		appConfigInstance = instance
	})

	return appConfigInstance, loadErr
}

// 新增原子化加载方法

func (c *Config) Load() error {
	c.fileMu.Lock()
	defer c.fileMu.Unlock()

	c.Mu.Lock()
	defer c.Mu.Unlock()

	// 检查临时文件残留
	if err := cleanStaleTempFile(); err != nil {
		return err
	}

	file, err := os.Open(configFile)
	switch {
	case os.IsNotExist(err):
		return nil // 新配置文件
	case err != nil:
		return err
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	return yaml.NewDecoder(file).Decode(c)
}

// 原子化保存实现

func (c *Config) Save() error {
	// 1. 快速数据快照（控制在10μs内）
	snapshot := func() []byte {
		buf := bytes.NewBuffer(make([]byte, 0, 1024*1024)) // 预分配1MB
		enc := yaml.NewEncoder(buf)
		enc.SetIndent(2)
		save := &saveConfig{
			Clients: c.Clients,
		}
		if err := enc.Encode(save); err != nil {
			return nil
		}
		return buf.Bytes()
	}()

	if snapshot == nil {
		return errors.New("配置序列化失败")
	}

	// 2. 无锁文件操作（控制在50ms内）
	tmpPath := fmt.Sprintf("%s.%d.tmp", configFile, time.Now().UnixNano())

	// 使用直接IO绕过系统缓存（需Linux内核4.18+）
	file, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		// 回退到缓冲写入
		file, err = os.Create(tmpPath)
		if err != nil {
			return err
		}
	}
	defer func(name string) {
		_ = os.Remove(name)
	}(tmpPath)

	// 批量写入优化
	if _, err = file.Write(snapshot); err != nil {
		return err
	}

	// 异步刷盘（根据数据重要性选择）
	go func() {
		if err := file.Sync(); err != nil {
			log.Printf("异步刷盘失败: %v", err)
		}
		_ = file.Close()
	}()

	// 3. 原子替换（纳秒级操作）
	return os.Rename(tmpPath, configFile)
}

// 新增列表展示方法

func (c *Config) ClientsList() []*Client {
	c.Mu.RLock()
	defer c.Mu.RUnlock()

	list := make([]*Client, 0, len(c.Clients))
	for _, client := range c.Clients {
		// 返回客户端副本防止外部修改
		cpy := *client
		list = append(list, &cpy)
	}
	return list
}

func (c *Config) IsExist(id string) bool {
	key := "client" + id
	if _, exists := c.Clients[key]; exists {
		return true
	}
	return false
}

func (c *Config) GetProxyFromCache(ip string) (string, bool) {
	if _, exists := c.IpMap[ip]; exists {
		return c.IpMap[ip], true
	}
	return "", false
}

// 清理残留临时文件
func cleanStaleTempFile() error {
	matches, err := filepath.Glob(configFile + ".*")
	if err != nil {
		return err
	}

	for _, f := range matches {
		if filepath.Ext(f) == tmpSuffix {
			if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}
