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

package sftp

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"

	"github.com/goharbor/harbor/src/lib/log"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"golang.org/x/crypto/ssh"

	sshpool "github.com/goharbor/harbor/src/pkg/reg/adapter/storage/drivers/sftp/pool"
	"github.com/goharbor/harbor/src/pkg/reg/adapter/storage/health"
	"github.com/goharbor/harbor/src/pkg/reg/model"
)

const (
	DriverName = "sftp"
)

var sshPool = sshpool.NewPool(&sshpool.PoolConfig{
	GCInterval: time.Minute,
})

type driver struct {
	basePath  string
	sshConfig *sshpool.SSHConfig
}

func (d *driver) Name() string {
	return DriverName
}

type baseEmbed struct {
	base.Base
}

// Driver is a storagedriver.StorageDriver implementation backed by a local
// filesystem. All provided paths will be subpaths of the RootDirectory.
type Driver struct {
	baseEmbed
	driver *driver
}

func (d *driver) GetContent(_ context.Context, p string) ([]byte, error) {
	log.Debugf("get content of %s", p)

	var err error
	session, release, err := d.getSFTP()
	if err != nil {
		return nil, fmt.Errorf("reader %s sftp session failed: %v", p, err)
	}

	defer release()
	file, err := session.Open(d.addBasePath(p))

	if err != nil {
		_ = session.Close()
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{Path: p, DriverName: DriverName}
		}
		return nil, err
	}

	defer func() {
		// session closes by file
		_ = file.Close()
	}()

	data, err := io.ReadAll(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{Path: p, DriverName: DriverName}
		}
		return nil, err
	}
	return data, err
}

func (d *driver) PutContent(_ context.Context, p string, contents []byte) error {
	log.Debugf("put content for %s with length %d", p, len(contents))

	session, release, err := d.getSFTP()
	if err != nil {
		return fmt.Errorf("putcontent %s get sftp session failed: %v", p, err)
	}

	defer release()
	p = d.addBasePath(p)

	dir := path.Dir(p)
	if err = session.MkdirAll(dir); err != nil {
		_ = session.Close()

		return fmt.Errorf("putcontent: unable to create directory %s: %v", dir, err)
	}

	file, err := session.Create(p)
	if err != nil {
		_ = session.Close()
		return fmt.Errorf("putcontent: file create %s error: %v", p, err)
	}

	// session closes by file
	defer func() {
		_ = file.Close()
	}()
	_, err = file.Write(contents)
	return err
}

func (d *driver) Reader(_ context.Context, p string, offset int64) (io.ReadCloser, error) {
	log.Debugf("reader for %s with offset %d", p, offset)

	var err error
	session, release, err := d.getSFTP()
	if err != nil {
		return nil, fmt.Errorf("reader %s sftp session failed: %v", p, err)
	}

	file, err := session.Open(d.addBasePath(p))
	if err != nil {
		_ = session.Close()
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{Path: p, DriverName: DriverName}
		}
		return nil, fmt.Errorf("reader open %s: %v", p, err)
	}

	seekPos, err := file.Seek(offset, io.SeekStart)
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	if seekPos < offset {
		_ = file.Close()
		return nil, storagedriver.InvalidOffsetError{Path: p, Offset: offset, DriverName: DriverName}
	}

	//
	return sshpool.NewCloser(file, release), nil
}

func (d *driver) Writer(_ context.Context, p string, a bool) (storagedriver.FileWriter, error) {
	log.Debugf("writer for %s with append %v", p, a)

	session, release, err := d.getSFTP()
	if err != nil {
		return nil, fmt.Errorf("writer %s get sftp session failed: %v", p, err)
	}

	p = d.addBasePath(p)
	dir := path.Dir(p)

	if err = session.MkdirAll(dir); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("unable to create directory %s: %v", dir, err)
	}

	var (
		offset int64
		file   *sftp.File
	)

	if a {
		file, err = session.OpenFile(p, os.O_RDWR|os.O_APPEND)
		if err != nil {
			_ = session.Close()
			return nil, fmt.Errorf("append open %s: %v", p, err)
		}

		n, err := file.Seek(0, io.SeekEnd)
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		offset = n
	} else {

		file, err = session.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_TRUNC)
		if err != nil {
			_ = session.Close()
			return nil, fmt.Errorf("open %s: %v", p, err)
		}

		err = file.Truncate(0)
		if err != nil {
			_ = file.Close()
			return nil, err
		}
	}

	// connection closes with the file
	return newFileWriter(file, offset, release), nil
}

func (d *driver) Stat(_ context.Context, p string) (storagedriver.FileInfo, error) {
	log.Debugf("stat of %s", p)
	defer func() {
		log.Debugf("stat of %s acquired", p)
	}()

	session, release, err := d.getSFTP()
	if err != nil {
		return nil, fmt.Errorf("stat %s get sftp session failed: %v", p, err)
	}

	defer func() {
		release()
		_ = session.Close()
	}()

	p = d.addBasePath(p)

	stat, err := session.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{Path: p}
		}
		return nil, fmt.Errorf("stat %s: %v", p, err)
	}

	return fileInfo{
		FileInfo: stat,
		path:     p,
	}, nil
}

func (d *driver) List(_ context.Context, p string) ([]string, error) {
	log.Debugf("list of %s", p)

	session, release, err := d.getSFTP()
	if err != nil {
		return nil, fmt.Errorf("list %s get sftp session failed: %v", p, err)
	}

	defer func() {
		release()
		_ = session.Close()
	}()

	pn := d.addBasePath(p)
	files, err := session.ReadDir(pn)

	if err != nil {
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{Path: p}
		}
		return nil, fmt.Errorf("read dir %s error: %v", p, err)
	}
	var result []string

	for _, file := range files {
		// trim base path
		result = append(result, path.Join(p, file.Name()))
	}
	return result, nil
}

func (d *driver) Move(_ context.Context, sourcePath string, destPath string) error {
	log.Debugf("move %s to %s", sourcePath, destPath)

	session, release, err := d.getSFTP()
	if err != nil {
		return fmt.Errorf("move %s get sftp session failed: %v", sourcePath, err)
	}

	defer func() {
		release()
		_ = session.Close()
	}()
	//
	sourcePath = d.addBasePath(sourcePath)
	destPath = d.addBasePath(destPath)

	if err := session.MkdirAll(path.Dir(destPath)); err != nil {
		return fmt.Errorf("unable to create destPath directory: %v", err)
	}

	return session.Rename(sourcePath, destPath)
}

func (d *driver) Delete(_ context.Context, p string) error {
	log.Debugf("delete %s", p)

	session, release, err := d.getSFTP()
	if err != nil {
		return fmt.Errorf("delete %s get sftp session failed: %v", p, err)
	}

	defer func() {
		release()
		_ = session.Close()
	}()
	//

	p = d.addBasePath(p)
	if err := session.RemoveAll(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("unable to remove all %s: %v", p, err)
	}

	if err = session.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s error: %v", p, err)
	}
	return nil
}

func (d *driver) URLFor(_ context.Context, p string, _ map[string]interface{}) (string, error) {
	log.Debugf("URL for %s", p)
	return "", fmt.Errorf("URLFor is not implemented")
}

func (d *driver) Walk(ctx context.Context, p string, f storagedriver.WalkFn) error {
	log.Debugf("walk %s", p)
	return storagedriver.WalkFallback(ctx, d, p, func(fi storagedriver.FileInfo) error {
		// manipulate file info to trim base path, harbor should know nothing about it
		return f(fileInfoMock{
			path:    d.trimBasePath(fi.Path()),
			isDir:   fi.IsDir(),
			size:    fi.Size(),
			modTime: fi.ModTime(),
		})
	})
}

func (d *Driver) Health(ctx context.Context) error {
	return d.driver.Health(ctx)
}

func New(regModel *model.Registry) (storagedriver.StorageDriver, error) {
	if regModel == nil {
		return nil, fmt.Errorf("internal error")
	}

	u, err := url.Parse(regModel.URL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse registry URL: %v", err)
	}

	config := &sshpool.SSHConfig{
		Host:               u.Hostname(),
		TCPKeepAlive:       true,
		TCPKeepAlivePeriod: time.Minute,
		Timeout:            30 * time.Minute,
	}
	if regModel.Insecure {
		config.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	} else {
		return nil, fmt.Errorf("verifying remove certificate is not implemented")
	}

	if regModel.Credential != nil {
		config.User = regModel.Credential.AccessKey
		config.Auth = append(config.Auth, ssh.Password(regModel.Credential.AccessSecret))
	}

	port := u.Port()
	if port == "" {
		port = "22"
	}

	portInt, err := strconv.Atoi(port)
	if err == nil {
		config.Port = portInt
	}

	d := &driver{
		sshConfig: config,
		basePath:  u.Path,
	}

	return d, nil
}

func (d *driver) Health(_ context.Context) error {
	session, release, err := d.getSFTP()
	if err != nil {
		return err
	}
	defer func() {
		release()
		_ = session.Close()
	}()
	_, err = session.Getwd()
	return err
}

func (d *driver) getSFTP() (*sftp.Client, func(), error) {
	return sshPool.NewSFTPSession(d.sshConfig)
}

func (d *driver) addBasePath(p string) string {
	return path.Join(d.basePath, p)
}

func (d *driver) trimBasePath(p string) string {
	return strings.TrimPrefix(p, d.basePath)
}

var _ health.Checker = (*driver)(nil)
var _ health.Checker = (*Driver)(nil)
