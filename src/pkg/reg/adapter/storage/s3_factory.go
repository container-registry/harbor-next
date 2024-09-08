package storage

import (
	"context"
	"fmt"
	awss3 "github.com/aws/aws-sdk-go/service/s3"
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

const minChunkSize = 5 << 20

const defaultChunkSize = 2 * minChunkSize

const defaultMultipartCopyChunkSize = 32 << 20

// defaultMultipartCopyMaxConcurrency defines the default maximum number
// of concurrent Upload Part - Copy operations for a multipart copy.
const defaultMultipartCopyMaxConcurrency = 100

// defaultMultipartCopyThresholdSize defines the default object size
// above which multipart copy will be used. (PUT Object - Copy is used
// for objects at or below this size.)  Empirically, 32 MB is optimal.
const defaultMultipartCopyThresholdSize = 32 << 20

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

	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(pathParts) == 0 {
		return nil, fmt.Errorf("invalid registry URL: no folder defined which can be used as a bucket name")
	}

	driverParams := s3.DriverParameters{
		Bucket:                      pathParts[0],
		Region:                      "auto",
		MultipartCopyChunkSize:      defaultMultipartCopyChunkSize,
		MultipartCopyMaxConcurrency: defaultMultipartCopyMaxConcurrency,
		MultipartCopyThresholdSize:  defaultMultipartCopyThresholdSize,
		ChunkSize:                   defaultChunkSize,
	}

	// @todo does not work
	if u.Query().Get("secure") == "false" {
		driverParams.Secure = false
	}

	//  @todo does not work
	if u.Query().Get("region") != "" {
		driverParams.Region = u.Query().Get("region")
	}

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
	// default ACL
	driverParams.ObjectACL = awss3.ObjectCannedACLPrivate

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
