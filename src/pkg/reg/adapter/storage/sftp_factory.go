// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"context"

	"github.com/docker/distribution/registry/storage"
	"github.com/docker/libtrust"

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

	driver, err := sftpdriver.New(r)
	if err != nil {
		return nil, err
	}

	trustKey, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		return nil, err
	}

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
func (f *sftpFactory) AdapterPattern() *model.AdapterPattern {
	return nil
}
