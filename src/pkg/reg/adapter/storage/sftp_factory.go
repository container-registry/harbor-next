package storage

import (
	"context"
	"fmt"
	"github.com/docker/distribution/registry/storage"
	"github.com/goharbor/harbor/src/lib/log"
	regadapter "github.com/goharbor/harbor/src/pkg/reg/adapter"
	sftpdriver "github.com/goharbor/harbor/src/pkg/reg/adapter/storage/drivers/sftp"
	"github.com/goharbor/harbor/src/pkg/reg/model"
)

func init() {
	err := regadapter.RegisterFactory(model.RegistryTypeSFTP, &sftpFactory{})
	if err != nil {
		log.Errorf("failed to register sftpFactory for dtr: %v", err)
		return
	}
	log.Infof("sftpFactory of SFTP adapter was registered")
}

type sftpFactory struct {
}

// Create ...
func (f *sftpFactory) Create(r *model.Registry) (regadapter.Adapter, error) {

	fmt.Println("1!!!!!!!!!!!!! SFTP FACTORY CREATE !!!!!!!!!!!!!!!")
	driver, err := sftpdriver.New(r)
	if err != nil {
		return nil, err
	}

	ns, err := storage.NewRegistry(context.TODO(), driver)
	if err != nil {
		return nil, err
	}
	return &adapter{
		regModel: r,
		driver:   driver,
		registry: ns,
	}, nil
}

// AdapterPattern ...
func (f *sftpFactory) AdapterPattern() *model.AdapterPattern {
	return nil
}
