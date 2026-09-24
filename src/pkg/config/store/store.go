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

// Package store is only used in the internal implement of manager, not a public api.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/goharbor/harbor/src/common/utils"
	"github.com/goharbor/harbor/src/lib/config/metadata"
	"github.com/goharbor/harbor/src/lib/log"
)

type valueMap = map[string]metadata.ConfigureValue

// ConfigStore - the config data store
//
// Values are held in an immutable map that is replaced as a whole, so a reload
// never exposes a mix of old and new values for one key set.
type ConfigStore struct {
	cfgDriver Driver
	mu        sync.Mutex // serializes writers
	cfgValues atomic.Pointer[valueMap]
	// driverVersion is the Versioned.Version() the values were last loaded at.
	driverVersion atomic.Uint64
}

// NewConfigStore create config store
func NewConfigStore(cfgDriver Driver) *ConfigStore {
	return &ConfigStore{cfgDriver: cfgDriver}
}

func (c *ConfigStore) values() valueMap {
	if m := c.cfgValues.Load(); m != nil {
		return *m
	}
	return nil
}

// update applies fn to a copy of the current values and publishes the copy. Callers must hold c.mu.
func (c *ConfigStore) update(fn func(next valueMap)) {
	next := maps.Clone(c.values())
	if next == nil {
		next = valueMap{}
	}
	fn(next)
	c.cfgValues.Store(&next)
}

// Get - Get config data from current store
func (c *ConfigStore) Get(key string) (*metadata.ConfigureValue, error) {
	if value, ok := c.values()[key]; ok {
		return &value, nil
	}
	return nil, metadata.ErrValueNotSet
}

// GetFromDriver ...
func (c *ConfigStore) GetFromDriver(ctx context.Context, key string) (map[string]any, error) {
	if c.cfgDriver == nil {
		return nil, errors.New("failed to load store, cfgDriver is nil")
	}
	cfgs, err := c.cfgDriver.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return cfgs, nil
}

// GetAnyType get any type for config items
func (c *ConfigStore) GetAnyType(key string) (any, error) {
	if value, ok := c.values()[key]; ok {
		return value.GetAnyType()
	}
	return nil, metadata.ErrValueNotSet
}

// Set - Set configure value in store, not saved to config driver
func (c *ConfigStore) Set(key string, value metadata.ConfigureValue) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.update(func(next valueMap) { next[key] = value })
	return nil
}

// Load - Load data from driver, all user config in the store will be refreshed
func (c *ConfigStore) Load(ctx context.Context) error {
	if c.cfgDriver == nil {
		return errors.New("failed to load store, cfgDriver is nil")
	}
	var version uint64
	if v, ok := c.cfgDriver.(Versioned); ok {
		version = v.Version()
		// Skipping unchanged reloads also keeps values set locally by Update
		// until the driver reports the change as committed.
		if version != 0 && version == c.driverVersion.Load() {
			return nil
		}
	}
	cfgs, err := c.cfgDriver.Load(ctx)
	if err != nil {
		return err
	}
	loaded := make(valueMap, len(cfgs))
	for key, value := range cfgs {
		strValue, err := ToString(value)
		if err != nil {
			log.Errorf("failed to transform the value from driver to string, key: %s, value: %v, error: %v", key, value, err)
			continue
		}
		cfgValue := metadata.ConfigureValue{}
		err = cfgValue.Set(key, strValue)
		if err != nil {
			log.Errorf("error when loading data item, key %v, value %v, error %v", key, value, err)
			continue
		}
		loaded[key] = cfgValue
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if version != 0 && version < c.driverVersion.Load() {
		// a concurrent Load already published newer values
		return nil
	}
	c.update(func(next valueMap) { maps.Copy(next, loaded) })
	c.driverVersion.Store(version)
	return nil
}

// Save - Save all data in current store
func (c *ConfigStore) Save(ctx context.Context) error {
	cfgMap := map[string]any{}
	for keyStr, configValue := range c.values() {
		if _, ok := metadata.Instance().GetByName(keyStr); ok {
			cfgMap[keyStr] = configValue.Value
		} else {
			log.Errorf("failed to get metadata for key %v", keyStr)
		}
	}

	if c.cfgDriver == nil {
		return errors.New("failed to save store, cfgDriver is nil")
	}

	return c.cfgDriver.Save(ctx, cfgMap)
}

// Update - Only update specified settings in cfgMap in store and driver
func (c *ConfigStore) Update(ctx context.Context, cfgMap map[string]any) error {
	// Update to store
	updated := valueMap{}
	for key, value := range cfgMap {
		configValue, err := metadata.NewCfgValue(key, utils.GetStrValueOfAnyType(value))
		if err != nil {
			log.Warningf("error %v, skip to update configure item, key:%v ", err, key)
			delete(cfgMap, key)
			continue
		}
		updated[key] = *configValue
	}
	c.mu.Lock()
	c.update(func(next valueMap) { maps.Copy(next, updated) })
	c.mu.Unlock()
	// Update to driver
	err := c.cfgDriver.Save(ctx, cfgMap)
	// The save may still fail or be rolled back with the caller's transaction, in which
	// case the driver version never changes. Forcing the next Load to re-merge makes
	// readers follow the committed state instead of keeping the local values.
	c.driverVersion.Store(0)
	return err
}

// ToString ...
func ToString(value any) (string, error) {
	if value == nil {
		return "nil", nil
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Map, reflect.Array, reflect.Slice, reflect.Struct:
		d, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		return string(d), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), nil
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil
	case reflect.String:
		return value.(string), nil
	default:
		return fmt.Sprintf("%v", value), nil
	}
}
