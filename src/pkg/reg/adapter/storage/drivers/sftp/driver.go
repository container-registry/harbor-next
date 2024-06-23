package sftp

import (
	"context"
	"fmt"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/goharbor/harbor/src/pkg/reg/adapter/storage/health"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	sftppkg "github.com/pkg/sftp"
	"github.com/silenceper/pool"
	"io"
	"os"
	"path"
)

const (
	DriverName         = "sftp"
	defaultConcurrency = 10
)

type driver struct {
	regModel *model.Registry
	pool     pool.Pool
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
	defer d.putClient(client)

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
	defer d.putClient(client)

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

	client, err := d.getClient()
	if err != nil {
		return nil, err
	}

	file, err := client.Open(client.normaliseBasePath(path))
	if err != nil {
		d.putClient(client)
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{}
		}
		return nil, err
	}

	seekPos, err := file.Seek(offset, io.SeekStart)
	if err != nil {
		file.Close()
		d.putClient(client)
		return nil, err
	} else if seekPos < offset {
		file.Close()
		d.putClient(client)
		return nil, storagedriver.InvalidOffsetError{Path: path, Offset: offset}
	}
	return newFileReader(file, d.pool, client), nil
}

func (d *driver) Writer(_ context.Context, path string, append bool) (storagedriver.FileWriter, error) {

	client, err := d.getClient()
	if err != nil {
		return nil, err
	}

	path = client.normaliseBasePath(path)
	file, err := client.Create(path)
	if err != nil {
		d.putClient(client)
		return nil, fmt.Errorf("client create sftp error: %v", err)
	}

	var offset int64

	if append {
		n, err := file.Seek(0, io.SeekEnd)
		if err != nil {
			_ = file.Close()
			d.putClient(client)
			return nil, err
		}
		offset = n
	} else {
		err := file.Truncate(0)
		if err != nil {
			_ = file.Close()
			d.putClient(client)
			return nil, err
		}
	}
	return newFileWriter(file, d.pool, client, offset), nil
}

func (d *driver) Stat(_ context.Context, p string) (storagedriver.FileInfo, error) {

	client, err := d.getClient()
	if err != nil {
		return nil, err
	}
	defer d.putClient(client)

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

	defer d.putClient(client)

	p = client.normaliseBasePath(p)

	files, err := client.ReadDir(p)
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
	client, err := d.getClient()
	if err != nil {
		return err
	}
	defer d.putClient(client)
	//
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
	defer d.putClient(client)
	//

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

func (d *driver) getClient() (*clientWrapper, error) {
	client, err := d.pool.Get()
	if err != nil {
		return nil, err
	}
	return client.(*clientWrapper), nil
}

func (d *driver) putClient(c *clientWrapper) {
	_ = d.pool.Put(c)
}

func (d *driver) Health(_ context.Context) error {
	client, err := d.getClient()
	if err != nil {
		return err
	}
	defer d.putClient(client)
	return err
}

func New(regModel *model.Registry) (storagedriver.StorageDriver, error) {
	pool, err := poolFactory.Get(regModel)
	if err != nil {
		return nil, err
	}
	return &Driver{
		baseEmbed: baseEmbed{
			Base: base.Base{
				StorageDriver: base.NewRegulator(&driver{
					regModel: regModel,
					pool:     pool,
				}, defaultConcurrency),
			},
		},
	}, nil
}

var _ health.Checker = (*driver)(nil)
