package logger

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFileGetterFactory
func TestFileGetterFactory(t *testing.T) {
	is := make([]OptionItem, 0)
	is = append(is, OptionItem{"other_key1", 11})
	is = append(is, OptionItem{"base_dir", "/tmp"})
	is = append(is, OptionItem{"other_key2", ""})

	_, err := FileGetterFactory(is...)
	require.Nil(t, err)
}

// TestFileGetterFactoryErr1
func TestFileGetterFactoryErr1(t *testing.T) {
	is := make([]OptionItem, 0)
	is = append(is, OptionItem{"other_key1", 11})

	_, err := FileGetterFactory(is...)
	require.NotNil(t, err)
}

// TestDBGetterFactory
func TestDBGetterFactory(t *testing.T) {
	is := make([]OptionItem, 0)

	_, err := DBGetterFactory(is...)
	require.Nil(t, err)
}
