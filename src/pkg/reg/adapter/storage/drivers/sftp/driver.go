package sftp

import (
	"bytes"
	"context"
	"fmt"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	sshpool "github.com/goharbor/harbor/src/pkg/reg/adapter/storage/drivers/sftp/pool"
	"github.com/goharbor/harbor/src/pkg/reg/adapter/storage/health"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"io"
	"net/url"
	"os"
	"path"
	"strconv"
	"time"
)

const (
	DriverName         = "sftp"
	defaultConcurrency = 1
)

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

var sshPool = sshpool.NewPool(&sshpool.PoolConfig{
	GCInterval: 10 * time.Second,
	MaxConns:   10,
})

func (d *driver) GetContent(ctx context.Context, path string) ([]byte, error) {

	fmt.Println("GET CONTENT", path)
	rc, err := d.Reader(ctx, path, 0)
	if err != nil {
		return nil, err
	}

	defer rc.Close()
	return io.ReadAll(rc)
}

func (d *driver) PutContent(ctx context.Context, p string, contents []byte) error {
	fmt.Println("PUT CONTENT", p)

	writer, err := d.Writer(ctx, p, false)
	if err != nil {
		return fmt.Errorf("writer error: %v", err)
	}

	defer writer.Close()
	_, err = io.Copy(writer, bytes.NewReader(contents))
	if err != nil {
		_ = writer.Cancel()
		return fmt.Errorf("writer error: %v", err)
	}

	err = writer.Commit()
	if err != nil {
		return fmt.Errorf("commit error: %v", err)
	}
	return nil
}

func (d *driver) Reader(_ context.Context, path string, offset int64) (io.ReadCloser, error) {
	fmt.Println("GET READER", path)
	var err error

	session, cl, err := d.getSFTP()
	if err != nil {
		return nil, err
	}

	file, err := session.Open(d.normaliseBasePath(path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{Path: path}
		}
		return nil, err
	}

	seekPos, err := file.Seek(offset, io.SeekStart)
	if err != nil {
		//file.Close()
		return nil, err
	} else if seekPos < offset {
		//file.Close()
		return nil, storagedriver.InvalidOffsetError{Path: path, Offset: offset}
	}

	r := reader{
		File:  file,
		close: cl,
	}
	return r, nil
}

func (d *driver) Writer(_ context.Context, p string, append bool) (storagedriver.FileWriter, error) {

	fmt.Println("GET WRITER", p)

	session, _, err := d.getSFTP()
	if err != nil {
		return nil, err
	}

	p = d.normaliseBasePath(p)

	if err = session.MkdirAll(path.Dir(p)); err != nil {
		return nil, fmt.Errorf("unable to create directory %s: %v", path.Dir(p), err)
	}

	file, err := session.Create(p)
	if err != nil {
		return nil, fmt.Errorf("file create error: %v", err)
	}

	var offset int64

	if append {
		offset, err = file.Seek(0, io.SeekEnd)
	} else {
		err = file.Truncate(0)
	}
	if err != nil {
		return nil, fmt.Errorf("file seek/truncate error: %v", err)
	}

	return newFileWriter(file, offset), nil
}

func (d *driver) Stat(_ context.Context, p string) (storagedriver.FileInfo, error) {
	fmt.Println("GET STAT", p)

	session, cl, err := d.getSFTP()
	if err != nil {
		return nil, err
	}

	defer cl()

	p = d.normaliseBasePath(p)
	stat, err := session.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{Path: p}
		}
		return nil, err
	}

	return fileInfo{
		FileInfo: stat,
		path:     p,
	}, nil
}

func (d *driver) List(_ context.Context, p string) ([]string, error) {

	fmt.Println("GET LIST", p)

	session, cl, err := d.getSFTP()
	if err != nil {
		return nil, err
	}

	defer cl()

	p = d.normaliseBasePath(p)

	files, err := session.ReadDir(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{Path: p}
		}
		return nil, fmt.Errorf("read dir %s error: %v", p, err)
	}
	var result []string

	for _, file := range files {
		result = append(result, path.Join(p, file.Name()))
	}

	return result, nil
}

func (d *driver) Move(_ context.Context, sourcePath string, destPath string) error {

	fmt.Println("MOVE", sourcePath)

	session, cl, err := d.getSFTP()
	if err != nil {
		return err
	}

	defer cl()

	//
	sourcePath = d.normaliseBasePath(sourcePath)
	destPath = d.normaliseBasePath(destPath)

	if err := session.MkdirAll(path.Dir(destPath)); err != nil {
		return fmt.Errorf("unable to create destPath directory: %v", err)
	}

	return session.Rename(sourcePath, destPath)
}

func (d *driver) Delete(_ context.Context, path string) error {
	fmt.Println("DELETE", path)

	session, cl, err := d.getSFTP()
	if err != nil {
		return err
	}
	defer cl()
	//

	path = d.normaliseBasePath(path)
	if err := session.RemoveAll(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("unable to remove all %s: %v", path, err)
	}

	if err = session.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s error: %v", path, err)
	}
	return nil
}

func (d *driver) URLFor(_ context.Context, _ string, _ map[string]interface{}) (string, error) {
	return "", fmt.Errorf("URLFor is not implemented")
}

func (d *driver) Walk(ctx context.Context, path string, f storagedriver.WalkFn) error {
	fmt.Println("WALK", path)

	return storagedriver.WalkFallback(ctx, d, path, f)
}

func (d *Driver) Health(_ context.Context) error {
	client, cl, err := d.driver.getSFTP()
	if err != nil {
		return err
	}
	defer cl()
	_, err = client.Getwd()
	return err
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
		Host: u.Hostname(),
	}
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

	portInt, err := strconv.Atoi(port)
	if err == nil {
		config.Port = portInt
	}

	d := &driver{
		sshConfig: config,
		basePath:  u.Path,
	}

	return &Driver{
		driver: d,
		baseEmbed: baseEmbed{
			Base: base.Base{
				StorageDriver: base.NewRegulator(d, defaultConcurrency),
			},
		},
	}, nil
}

func (d *driver) getSFTP() (*sftp.Client, func(), error) {
	return sshPool.NewSFTPSession(d.sshConfig)
}

func (d *driver) normaliseBasePath(p string) string {
	return path.Join(d.basePath, p)
}

var _ health.Checker = (*Driver)(nil)
