package core

import (
	"encoding/json"
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

type Config struct {
	ServerConfig     ServerConfig      `yaml:"server"`
	DHCPConfig       DHCPConfig        `yaml:"dhcp"`
	ResolverConfig   ResolverConfig    `yaml:"resolver"`
	TCPForwardConfig TCPForwardConfig  `yaml:"tcpforward"`
	UDPForwardConfig UDPForwardConfig  `yaml:"udpforward"`
	Plugins          map[string]string `yaml:"plugin"`
}

type ServerConfig struct {
	ListenAddr string `yaml:"listen"`
	AuthKey    string `yaml:"authKey"`
	Domain     string `yaml:"domain"`
}

type TCPForwardConfig struct {
	ListenAddr   string `yaml:"listen"`
	ReadTimeout  int    `yaml:"readTimeout"`
	WriteTimeout int    `yaml:"writeTimeout"`
}

type UDPForwardConfig struct {
	ListenAddr     string `yaml:"listen"`
	ReadTimeout    int    `yaml:"readTimeout"`
	WriteTimeout   int    `yaml:"writeTimeout"`
	SessionTimeout int    `yaml:"sessionTimeout"`
}

type DHCPConfig struct {
	Cidr string `yaml:"cidr"`
	IP   string `yaml:"ip"`
}

type ResolverConfig struct {
	EtcdEndpoints []string `yaml:"etcdEndpoints"`
}

func ParseConfig(path string) (*Config, error) {
	cnt, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	err = yaml.Unmarshal(cnt, &cfg)
	return &cfg, err
}

func (c *Config) String() string {
	cnt, _ := json.Marshal(c)
	return string(cnt)
}
