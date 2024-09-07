package storage

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/docker/distribution/registry/storage"
	"github.com/docker/distribution/registry/storage/driver/s3-aws"
	"github.com/docker/libtrust"
	"github.com/goharbor/harbor/src/lib/log"
	regadapter "github.com/goharbor/harbor/src/pkg/reg/adapter"
	s3driver "github.com/goharbor/harbor/src/pkg/reg/adapter/storage/drivers/s3"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"net/url"
	"strings"
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

	fmt.Println("s3 factory Create")
	spew.Dump(r)

	u, err := url.Parse(r.URL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse registry URL: %v", err)
	}

	pathParts := strings.Split(u.Path, "/")
	if len(pathParts) == 0 {
		return nil, fmt.Errorf("invalid registry URL: no folder defined which can be used as a bucket name")
	}

	driverParams := s3.DriverParameters{
		Bucket: pathParts[0],
		Region: "auto",
	}

	//if u.Query().Get("secure") == "false" {
	//	driverParams.Secure = false
	//}
	//
	//if u.Query().Get("region") == "" {
	//	return nil, fmt.Errorf("invalid registry URL: missing region param")
	//}

	if !strings.Contains(u.Hostname(), "s3.amazonaws.com") {
		driverParams.RegionEndpoint = u.Hostname()
	}

	if len(pathParts) > 1 {
		driverParams.RootDirectory = strings.Join(pathParts[1:], "/")
	}

	if r.Credential != nil {
		driverParams.AccessKey = r.Credential.AccessKey
		driverParams.SecretKey = r.Credential.AccessSecret
	}

	spew.Dump(driverParams)
	driverS3, err := s3.New(driverParams)
	if err != nil {
		return nil, err
	}

	trustKey, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		return nil, err
	}

	driver := &s3driver.Driver{Driver: driverS3}

	ns, err := storage.NewRegistry(context.Background(), driver,
		storage.EnableSchema1,
		storage.EnableDelete,
		storage.Schema1SigningKey(trustKey),
		storage.DisableDigestResumption)

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
