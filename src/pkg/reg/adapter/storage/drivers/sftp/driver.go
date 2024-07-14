package sftp

import (
	"bytes"
	"context"
	"fmt"
	"github.com/desops/sshpool"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/goharbor/harbor/src/pkg/reg/adapter/storage/health"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"io"
	"net/url"
	"os"
	"path"
)

const (
	DriverName         = "sftp"
	defaultConcurrency = 1
)

type driver struct {
	pool     *sshpool.Pool
	basePath string
	hostname string
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

func (d *driver) GetContent(ctx context.Context, path string) ([]byte, error) {

	fmt.Println("GetContent", path)

	session, err := d.getSFTP()
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			fmt.Printf("GetContent %s ERR: %v\n", path, err)
			session.Close()
		}
		session.Put()
	}()

	rc, err := d.Reader(ctx, path, 0)
	if err != nil {
		return nil, err
	}

	//defer rc.Close()

	return io.ReadAll(rc)
}

func (d *driver) PutContent(ctx context.Context, p string, contents []byte) error {

	fmt.Println("PutContent", p)

	writer, err := d.Writer(ctx, p, false)
	if err != nil {
		return err
	}

	defer writer.Close()
	_, err = io.Copy(writer, bytes.NewReader(contents))
	if err != nil {
		writer.Cancel()
		fmt.Printf("Putcontent %s ERR: %v\n", p, err)
		return err
	}
	return writer.Commit()
}

func (d *driver) Reader(_ context.Context, p string, offset int64) (io.ReadCloser, error) {

	fmt.Println("READER", p, "OFFSET", offset)
	var err error

	session, err := d.getSFTP()
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			fmt.Printf("Reader %s ERR: %v\n", p, err)
			session.Close()
		}
		session.Put()
	}()

	file, err := session.Open(d.normaliseBasePath(p))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{Path: p}
		}
		return nil, err
	}

	seekPos, err := file.Seek(offset, io.SeekStart)
	if err != nil {
		//file.Close()
		return nil, err
	} else if seekPos < offset {
		//file.Close()
		return nil, storagedriver.InvalidOffsetError{Path: p, Offset: offset}
	}

	return file, nil
}

func (d *driver) Writer(_ context.Context, p string, append bool) (storagedriver.FileWriter, error) {

	fmt.Println("Writer", p, append)

	session, err := d.getSFTP()
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			fmt.Printf("Writer %s ERR: %v\n", p, err)
			session.Close()
		}
		session.Put()
	}()

	p = d.normaliseBasePath(p)

	dir := path.Dir(p)

	if err = session.MkdirAll(dir); err != nil {
		return nil, fmt.Errorf("unable to create directory %s: %v", dir, err)
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
		return nil, err
	}

	return newFileWriter(file, offset), nil
}

func (d *driver) Stat(_ context.Context, p string) (storagedriver.FileInfo, error) {

	fmt.Println("Stat", p)

	session, err := d.getSFTP()
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			fmt.Printf("STAT %s ERR: %v\n", p, err)
			session.Close()
		}
		session.Put()
	}()

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
	fmt.Println("List", p)

	session, err := d.getSFTP()
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			fmt.Println("List ERR", err)
			session.Close()
		}
		session.Put()
	}()

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
	fmt.Println("Move", sourcePath, destPath)

	session, err := d.getSFTP()
	if err != nil {
		return err
	}

	defer session.Put()

	//
	sourcePath = d.normaliseBasePath(sourcePath)
	destPath = d.normaliseBasePath(destPath)

	if err := session.MkdirAll(path.Dir(destPath)); err != nil {
		return fmt.Errorf("unable to create destPath directory: %v", err)
	}

	return session.Rename(sourcePath, destPath)
}

func (d *driver) Delete(_ context.Context, path string) error {
	fmt.Println("Delete", path)

	session, err := d.getSFTP()
	if err != nil {
		return err
	}
	defer session.Put()
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
	fmt.Println("URLFor")

	return "", fmt.Errorf("URLFor is not implemented")
}

func (d *driver) Walk(ctx context.Context, path string, f storagedriver.WalkFn) error {
	return storagedriver.WalkFallback(ctx, d, path, f)
}

func (d *driver) Health(_ context.Context) error {
	session, err := d.getSFTP()
	if err != nil {
		return err
	}
	session.Put()

	return err
}

func New(regModel *model.Registry) (storagedriver.StorageDriver, error) {
	if regModel == nil {
		return nil, fmt.Errorf("internal error")
	}

	pool, err := poolFactory.Get(*regModel)
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(regModel.URL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse registry URL: %v", err)
	}

	var port = u.Port()
	if port == "" {
		port = "22"
	}

	return &Driver{
		baseEmbed: baseEmbed{
			Base: base.Base{
				StorageDriver: base.NewRegulator(&driver{
					pool:     pool,
					hostname: fmt.Sprintf("%s:%s", u.Hostname(), port),
					basePath: u.Path,
				}, defaultConcurrency),
			},
		},
	}, nil
}

func (d *driver) normaliseBasePath(p string) string {
	return path.Join(d.basePath, p)
}

func (d *driver) getSFTP() (*sshpool.SFTPSession, error) {
	return d.pool.GetSFTP(d.hostname)
}

var _ health.Checker = (*driver)(nil)
