package logger

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFileSweeperFactory
func TestFileSweeperFactory(t *testing.T) {
	is := make([]OptionItem, 0)
	is = append(is, OptionItem{"work_dir", "/tmp"})
	is = append(is, OptionItem{"duration", 2})

	_, err := FileSweeperFactory(is...)
	require.Nil(t, err)
}

// TestFileSweeperFactoryErr
func TestFileSweeperFactoryErr(t *testing.T) {
	is := make([]OptionItem, 0)
	is = append(is, OptionItem{"duration", 2})

	_, err := FileSweeperFactory(is...)
	require.NotNil(t, err)
}

// TestDBSweeperFactory
func TestDBSweeperFactory(t *testing.T) {
	is := make([]OptionItem, 0)
	is = append(is, OptionItem{"duration", 2})

	_, err := DBSweeperFactory(is...)
	require.Nil(t, err)
}
