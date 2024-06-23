package sftp

import (
	"fmt"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	sftppkg "github.com/pkg/sftp"
	"github.com/silenceper/pool"
	"golang.org/x/crypto/ssh"
	"net/url"
	"time"
)

var poolFactory = NewPoolFactory()

type PoolFactory struct {
	registry map[string]pool.Pool
}

func NewPoolFactory() *PoolFactory {
	return &PoolFactory{registry: make(map[string]pool.Pool)}
}

func (f *PoolFactory) Get(regModel *model.Registry) (pool.Pool, error) {
	if p, ok := f.registry[regModel.URL]; ok {
		fmt.Println("USE POOL FROM REGISTRY")
		return p, nil
	}

	//Create a connection pool: Initialize the number of connections to 5, the maximum idle connection is 20, and the maximum concurrent connection is 30
	poolConfig := &pool.Config{
		InitialCap: 1,
		MaxIdle:    1,
		MaxCap:     2,
		Factory: func() (interface{}, error) {

			fmt.Println("CONNECTING TO ", regModel.URL)
			u, err := url.Parse(regModel.URL)
			if err != nil {
				return nil, fmt.Errorf("unable to parse registry URL: %v", err)
			}

			port := u.Port()
			if port == "" {
				port = "22"
			}

			conf := &ssh.ClientConfig{}
			if regModel.Insecure {
				conf.HostKeyCallback = ssh.InsecureIgnoreHostKey()
			}

			if regModel.Credential != nil {
				conf.User = regModel.Credential.AccessKey
				conf.Auth = append(conf.Auth, ssh.Password(regModel.Credential.AccessSecret))
			}
			hostname := fmt.Sprintf("%s:%s", u.Hostname(), port)

			conn, err := ssh.Dial("tcp", hostname, conf)
			if err != nil {
				return nil, fmt.Errorf("dial %s error: %v", hostname, err)
			}
			c, err := sftppkg.NewClient(conn)
			if err != nil {
				return nil, err
			}
			return &clientWrapper{
				Client:   c,
				basePath: u.Path,
			}, nil
		},
		Close: func(v interface{}) error {
			return v.(*clientWrapper).Close()
		},
		Ping: func(v interface{}) error {
			return nil
		},
		//The maximum idle time of the connection, the connection exceeding this time will be closed, which can avoid the problem of automatic failure when connecting to EOF when idle
		IdleTimeout: 15 * time.Second,
	}

	p, err := pool.NewChannelPool(poolConfig)
	if err != nil {
		return nil, err
	}

	f.registry[regModel.URL] = p
	return p, nil
}
