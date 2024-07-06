package sftp

import (
	"fmt"
	"github.com/desops/sshpool"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"golang.org/x/crypto/ssh"
	"net/url"
)

var poolFactory = NewPoolFactory()

type PoolFactory struct {
	registry map[model.Registry]*sshpool.Pool
}

func NewPoolFactory() *PoolFactory {
	return &PoolFactory{registry: make(map[model.Registry]*sshpool.Pool)}
}

func (f *PoolFactory) Get(regModel model.Registry) (*sshpool.Pool, error) {
	if p, ok := f.registry[regModel]; ok {
		fmt.Println("USE POOL FROM REGISTRY")
		return p, nil
	}

	fmt.Println("CREATING POOL ", regModel.URL)

	u, err := url.Parse(regModel.URL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse registry URL: %v", err)
	}

	config := &ssh.ClientConfig{}
	if regModel.Insecure {
		config.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	}

	if regModel.Credential != nil {
		config.User = regModel.Credential.AccessKey
		config.Auth = append(config.Auth, ssh.Password(regModel.Credential.AccessSecret))
	}

	port := u.Port()
	if port == "" {
		port = "22"
	}

	pool := sshpool.New(config, &sshpool.PoolConfig{
		Debug:          true,
		MaxConnections: 5,
		MaxSessions:    5,
	})
	f.registry[regModel] = pool

	return pool, nil
}
