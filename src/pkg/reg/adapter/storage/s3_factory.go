package storage

import (
	"context"
	"fmt"
	"github.com/docker/distribution/registry/storage"
	"github.com/docker/distribution/registry/storage/driver/s3-aws"
	"github.com/goharbor/harbor/src/lib/log"
	regadapter "github.com/goharbor/harbor/src/pkg/reg/adapter"
	"github.com/goharbor/harbor/src/pkg/reg/model"
)

func init() {
	err := regadapter.RegisterFactory(model.RegistryTypeS3, &s3Factory{})
	if err != nil {
		log.Errorf("failed to register s3 for dtr: %v", err)
		return
	}
	log.Infof("sftpFactory of SFTP adapter was registered")
}

type s3Factory struct {
}

// Create ...
func (f *s3Factory) Create(r *model.Registry) (regadapter.Adapter, error) {

	fmt.Println("!!!!!!!!!!!!!! S3 FACTORY CREATE !!!!!!!!!!!!!!!")
	driverParams := s3.DriverParameters{}

	if r.Credential != nil {
		driverParams.AccessKey = r.Credential.AccessKey
		driverParams.SecretKey = r.Credential.AccessSecret
	}

	driver, err := s3.New(driverParams)

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
func (f *s3Factory) AdapterPattern() *model.AdapterPattern {
	return nil
}
