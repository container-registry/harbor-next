package sftp

import (
	"context"
	"fmt"
	"github.com/pkg/sftp"
	"io"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"golang.org/x/crypto/ssh"

	sshpool "github.com/goharbor/harbor/src/pkg/reg/adapter/storage/drivers/sftp/pool"
	"github.com/goharbor/harbor/src/pkg/reg/adapter/storage/health"
	"github.com/goharbor/harbor/src/pkg/reg/model"
)

const (
	DriverName = "sftp"
	//defaultConcurrency = 1
)

var sshPool = sshpool.NewPool(&sshpool.PoolConfig{
	GCInterval: 0,
	MaxConns:   10,
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

func (d *driver) GetContent(ctx context.Context, p string) ([]byte, error) {
	var err error
	session, err := d.getSFTP()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("reader %s sftp session failed: %v", p, err)
	}

	file, err := session.Open(d.addBasePath(p))

	if err != nil {
		_ = session.Close()
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{Path: p, DriverName: DriverName}
		}
		return nil, err
	}

	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{Path: p, DriverName: DriverName}
		}
		return nil, err
	}

	return data, err
}

func (d *driver) PutContent(ctx context.Context, p string, contents []byte) error {
	session, err := d.getSFTP()
	if err != nil {
		return fmt.Errorf("putcontent %s get sftp session failed: %v", p, err)
	}

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

	defer file.Close()
	_, err = file.Write(contents)
	return err
}

func (d *driver) Reader(_ context.Context, p string, offset int64) (io.ReadCloser, error) {

	var err error
	session, err := d.getSFTP()
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

	return file, nil
}

func (d *driver) Writer(_ context.Context, p string, append bool) (storagedriver.FileWriter, error) {

	session, err := d.getSFTP()
	if err != nil {
		return nil, fmt.Errorf("writer %s get sftp session failed: %v", p, err)
	}

	p = d.addBasePath(p)
	dir := path.Dir(p)

	if err = session.MkdirAll(dir); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("unable to create directory %s: %v", dir, err)
	}

	var offset int64

	var file *sftp.File

	if !append {

		file, err = session.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_TRUNC)
		if err != nil {
			session.Close()
			return nil, fmt.Errorf("open %s: %v", p, err)
		}

		err := file.Truncate(0)
		if err != nil {
			_ = file.Close()
			return nil, err
		}
	} else {

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
	}

	// connection closes with the file
	return newFileWriter(file, offset), nil
}

func (d *driver) Stat(_ context.Context, p string) (storagedriver.FileInfo, error) {

	session, err := d.getSFTP()
	if err != nil {
		return nil, fmt.Errorf("stat %s get sftp session failed: %v", p, err)
	}

	defer session.Close()

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

	session, err := d.getSFTP()
	if err != nil {
		return nil, fmt.Errorf("list %s get sftp session failed: %v", p, err)
	}

	defer session.Close()

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

	session, err := d.getSFTP()
	if err != nil {
		return fmt.Errorf("move %s get sftp session failed: %v", sourcePath, err)
	}

	defer session.Close()
	//
	sourcePath = d.addBasePath(sourcePath)
	destPath = d.addBasePath(destPath)

	if err := session.MkdirAll(path.Dir(destPath)); err != nil {
		return fmt.Errorf("unable to create destPath directory: %v", err)
	}

	return session.Rename(sourcePath, destPath)
}

func (d *driver) Delete(_ context.Context, p string) error {

	session, err := d.getSFTP()
	if err != nil {
		return fmt.Errorf("delete %s get sftp session failed: %v", p, err)
	}
	defer session.Close()
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

func (d *driver) URLFor(_ context.Context, _ string, _ map[string]interface{}) (string, error) {
	return "", fmt.Errorf("URLFor is not implemented")
}

func (d *driver) Walk(ctx context.Context, p string, f storagedriver.WalkFn) error {
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

	//return &Driver{
	//	driver: d,
	//	baseEmbed: baseEmbed{
	//		Base: base.Base{
	//			StorageDriver: base.NewRegulator(d, 1),
	//		},
	//	},
	//}, nil

	return d, nil
}

func (d *driver) Health(ctx context.Context) error {
	session, err := d.getSFTP()
	if err != nil {
		return err
	}
	defer session.Close()
	_, err = session.Getwd()
	return err
}

func (d *driver) getSFTP() (*sftp.Client, error) {
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
