package driver

import (
	"context"
	"fmt"
	"github.com/docker/distribution/registry/storage/driver"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	sftppkg "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
)

type Driver struct {
	regModel   *model.Registry
	client     *clientWrapper
	clientLock *sync.Mutex
}

func (d *Driver) Name() string {
	return "sftp"
}

type clientWrapper struct {
	*sftppkg.Client
	basePath string
}

func (c clientWrapper) normaliseBasePath(path string) string {
	return normalisePath(c.basePath, path)
}

func normalisePath(base, path string) string {
	return fmt.Sprintf("%s/%s", strings.TrimRight(base, "/"), strings.TrimLeft(path, "/"))
}

func (d *Driver) GetContent(_ context.Context, path string) ([]byte, error) {
	client, err := d.getClient(d.regModel)
	if err != nil {
		return nil, err
	}

	file, err := client.Open(client.normaliseBasePath(path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{}
		}
		return nil, fmt.Errorf("unable to open file: %v", err)
	}
	return io.ReadAll(file)
}

func (d *Driver) PutContent(_ context.Context, p string, content []byte) error {

	client, err := d.getClient(d.regModel)
	if err != nil {
		return err
	}
	client.normaliseBasePath(p)

	if err := client.MkdirAll(path.Dir(p)); err != nil {
		return fmt.Errorf("unable to create directory: %v", err)
	}

	file, err := client.Create(p)
	if err != nil {
		return fmt.Errorf("put content file create error: %v", err)
	}
	defer file.Close()
	_, err = file.Write(content)
	if err != nil {
		return fmt.Errorf("file write error: %v", err)
	}
	return err
}

func (d *Driver) Reader(_ context.Context, path string, offset int64) (io.ReadCloser, error) {
	if offset > 0 {
		return nil, fmt.Errorf("offset is not supported")
	}
	client, err := d.getClient(d.regModel)
	if err != nil {
		return nil, err
	}

	file, err := client.Open(client.normaliseBasePath(path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{}
		}
		return nil, err
	}

	seekPos, err := file.Seek(offset, io.SeekStart)
	if err != nil {
		file.Close()
		return nil, err
	} else if seekPos < offset {
		file.Close()
		return nil, storagedriver.InvalidOffsetError{Path: path, Offset: offset}
	}
	return file, nil
}

func (d *Driver) Writer(_ context.Context, path string, append bool) (driver.FileWriter, error) {
	if append {
		return nil, fmt.Errorf("append is not supported")
	}

	client, err := d.getClient(d.regModel)
	if err != nil {
		return nil, err
	}

	path = client.normaliseBasePath(path)

	file, err := client.Create(path)
	if err != nil {
		return nil, fmt.Errorf("client create sftp error: %v", err)
	}

	var offset int64

	if !append {
		err := file.Truncate(0)
		if err != nil {
			file.Close()
			return nil, err
		}
	} else {
		n, err := file.Seek(0, io.SeekEnd)
		if err != nil {
			file.Close()
			return nil, err
		}
		offset = n
	}
	return newFileWriter(file, client, offset), nil
}

func (d *Driver) Stat(_ context.Context, path string) (driver.FileInfo, error) {
	client, err := d.getClient(d.regModel)
	if err != nil {
		return nil, err
	}

	path = client.normaliseBasePath(path)
	stat, err := client.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{Path: path}
		}
		return nil, err
	}

	return fileInfo{
		FileInfo: stat,
		path:     path,
	}, nil
}

func (d *Driver) List(_ context.Context, path string) ([]string, error) {
	client, err := d.getClient(d.regModel)
	if err != nil {
		return nil, fmt.Errorf("list error: %v", err)
	}
	path = client.normaliseBasePath(path)

	files, err := client.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{Path: path}
		}
		return nil, fmt.Errorf("read dir error: %v", err)
	}
	var result []string

	for _, file := range files {
		result = append(result, normalisePath(path, file.Name()))
	}
	return result, nil
}

func (d *Driver) Move(_ context.Context, sourcePath string, destPath string) error {
	client, err := d.getClient(d.regModel)
	if err != nil {
		return err
	}

	sourcePath = client.normaliseBasePath(sourcePath)
	destPath = client.normaliseBasePath(destPath)

	if err := client.MkdirAll(path.Dir(destPath)); err != nil {
		return fmt.Errorf("unable to create destPath directory: %v", err)
	}

	return client.Rename(sourcePath, destPath)
}

func (d *Driver) Delete(_ context.Context, path string) error {
	client, err := d.getClient(d.regModel)
	if err != nil {
		return err
	}

	path = client.normaliseBasePath(path)
	if err := client.RemoveAll(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("unable to remove all %s: %v", path, err)
	}

	if err = client.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s error: %v", path, err)
	}
	return nil
}

func (d *Driver) URLFor(_ context.Context, path string, options map[string]interface{}) (string, error) {

	return "", fmt.Errorf("URLFor is not implemented")
}

func (d *Driver) Walk(_ context.Context, path string, f driver.WalkFn) error {
	client, err := d.getClient(d.regModel)
	if err != nil {
		return err
	}
	path = client.normaliseBasePath(path)
	walker := client.Walk(path)

	for walker.Step() {
		if walker.Err() != nil {
			continue
		}
		if err = f(&fileInfo{
			FileInfo: walker.Stat(),
			path:     normalisePath(path, walker.Path()),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (d *Driver) getClient(registry *model.Registry) (*clientWrapper, error) {
	d.clientLock.Lock()
	defer d.clientLock.Unlock()

	if d.client != nil {
		return d.client, nil
	}

	u, err := url.Parse(registry.URL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse registry URL: %v", err)
	}

	port := u.Port()
	if port == "" {
		port = "22"
	}

	conf := &ssh.ClientConfig{}
	if registry.Insecure {
		conf.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	}

	if registry.Credential != nil {
		conf.User = registry.Credential.AccessKey
		conf.Auth = append(conf.Auth, ssh.Password(registry.Credential.AccessSecret))
	}
	hostname := fmt.Sprintf("%s:%s", u.Hostname(), u.Port())

	conn, err := ssh.Dial("tcp", hostname, conf)
	if err != nil {
		return nil, fmt.Errorf("dial %s error: %v", hostname, err)
	}

	c, err := sftppkg.NewClient(conn)
	if err != nil {
		return nil, err
	}

	d.client = &clientWrapper{
		Client:   c,
		basePath: u.Path,
	}
	return d.client, nil
}

func NewDriver(regModel *model.Registry) *Driver {
	return &Driver{regModel: regModel, clientLock: &sync.Mutex{}}
}
