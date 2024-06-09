package sftp

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/goharbor/harbor/src/pkg/reg/adapter/storage/health"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	sftppkg "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"io"
	"net/url"
	"os"
	"path"
	"sync"
)

const (
	DriverName         = "sftp"
	defaultConcurrency = 10
)

type driver struct {
	regModel   *model.Registry
	client     *clientWrapper
	clientLock *sync.Mutex
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
}

type clientWrapper struct {
	*sftppkg.Client
	basePath string
}

func (c clientWrapper) normaliseBasePath(p string) string {
	return path.Join(c.basePath, p)
}

func (d *driver) GetContent(_ context.Context, path string) ([]byte, error) {
	client, err := d.getClient()
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

func (d *driver) PutContent(_ context.Context, p string, content []byte) error {

	client, err := d.getClient()
	if err != nil {
		return err
	}

	p = client.normaliseBasePath(p)

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

func (d *driver) Reader(_ context.Context, path string, offset int64) (io.ReadCloser, error) {

	if offset > 0 {
		return nil, fmt.Errorf("offset is not supported")
	}
	client, err := d.getClient()
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

func (d *driver) Writer(_ context.Context, path string, append bool) (storagedriver.FileWriter, error) {

	if append {
		return nil, fmt.Errorf("append is not supported")
	}

	client, err := d.getClient()
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

func (d *driver) Stat(_ context.Context, p string) (storagedriver.FileInfo, error) {

	client, err := d.getClient()
	if err != nil {
		return nil, err
	}

	p = client.normaliseBasePath(p)
	stat, err := client.Stat(p)
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

	client, err := d.getClient()
	if err != nil {
		return nil, fmt.Errorf("list error: %v", err)
	}
	p = client.normaliseBasePath(p)

	files, err := client.ReadDir(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{Path: p}
		}
		return nil, fmt.Errorf("read dir error: %v", err)
	}
	var result []string

	for _, file := range files {
		result = append(result, path.Join(p, file.Name()))
	}

	spew.Dump(result)

	return result, nil
}

func (d *driver) Move(_ context.Context, sourcePath string, destPath string) error {
	client, err := d.getClient()
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

func (d *driver) Delete(_ context.Context, path string) error {
	client, err := d.getClient()
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

func (d *driver) URLFor(_ context.Context, _ string, _ map[string]interface{}) (string, error) {
	return "", fmt.Errorf("URLFor is not implemented")
}

func (d *driver) Walk(ctx context.Context, path string, f storagedriver.WalkFn) error {
	return storagedriver.WalkFallback(ctx, d, path, f)
}

func (d *driver) resetClient() {
	d.clientLock.Lock()
	defer d.clientLock.Unlock()
	d.client = nil
}

func (d *driver) getClient() (*clientWrapper, error) {
	d.clientLock.Lock()
	defer d.clientLock.Unlock()

	if d.client != nil {
		return d.client, nil
	}

	fmt.Println("################# CONNECT!!!!! ##################")

	u, err := url.Parse(d.regModel.URL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse registry URL: %v", err)
	}

	port := u.Port()
	if port == "" {
		port = "22"
	}

	conf := &ssh.ClientConfig{}
	if d.regModel.Insecure {
		conf.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	}

	if d.regModel.Credential != nil {
		conf.User = d.regModel.Credential.AccessKey
		conf.Auth = append(conf.Auth, ssh.Password(d.regModel.Credential.AccessSecret))
	}
	hostname := fmt.Sprintf("%s:%s", u.Hostname(), port)

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
func (d *driver) Health(_ context.Context) error {
	fmt.Println("--------------- HEALTH CHECK ----------------")
	d.resetClient()
	client, err := d.getClient()
	if err != nil {
		return err
	}
	defer client.Close()
	return err
}

func New(regModel *model.Registry) storagedriver.StorageDriver {

	return &Driver{
		baseEmbed: baseEmbed{
			Base: base.Base{
				StorageDriver: base.NewRegulator(&driver{regModel: regModel, clientLock: &sync.Mutex{}}, defaultConcurrency),
			},
		},
	}
}

var _ health.Checker = (*driver)(nil)
