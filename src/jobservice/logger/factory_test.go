package logger

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFileFactory
func TestFileFactory(t *testing.T) {
	is := make([]OptionItem, 0)
	is = append(is, OptionItem{"level", "DEBUG"})
	is = append(is, OptionItem{"base_dir", "/tmp"})
	is = append(is, OptionItem{"filename", "test.out"})
	is = append(is, OptionItem{"depth", 5})

	ff, err := FileFactory(is...)
	require.Nil(t, err)

	if closer, ok := ff.(Closer); ok {
		_ = closer.Close()
	}
}

// TestFileFactoryErr1
func TestFileFactoryErr1(t *testing.T) {
	is := make([]OptionItem, 0)
	is = append(is, OptionItem{"level", "DEBUG"})
	is = append(is, OptionItem{"filename", "test.out"})

	_, err := FileFactory(is...)
	require.NotNil(t, err)
}

// TestFileFactoryErr2
func TestFileFactoryErr2(t *testing.T) {
	is := make([]OptionItem, 0)
	is = append(is, OptionItem{"level", "DEBUG"})
	is = append(is, OptionItem{"base_dir", "/tmp"})

	_, err := FileFactory(is...)
	require.NotNil(t, err)
}

// TestStdFactory
func TestStdFactory(t *testing.T) {
	is := make([]OptionItem, 0)
	is = append(is, OptionItem{"level", "DEBUG"})
	is = append(is, OptionItem{"output", "std_out"})
	is = append(is, OptionItem{"depth", 5})

	_, err := StdFactory(is...)
	require.Nil(t, err)
}

// TestDBFactory
func TestDBFactory(t *testing.T) {
	is := make([]OptionItem, 0)
	is = append(is, OptionItem{"level", "DEBUG"})
	is = append(is, OptionItem{"key", "key_db_logger_unit_text"})
	is = append(is, OptionItem{"depth", 5})

	_, err := DBFactory(is...)
	require.Nil(t, err)
}

// TestDBFactoryErr1
func TestDBFactoryErr1(t *testing.T) {
	is := make([]OptionItem, 0)
	is = append(is, OptionItem{"level", "DEBUG"})
	is = append(is, OptionItem{"depth", 5})

	_, err := DBFactory(is...)
	require.NotNil(t, err)
}
