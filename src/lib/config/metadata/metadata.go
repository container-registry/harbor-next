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

package metadata

import (
	"sync"
)

var metaDataOnce sync.Once
var metaDataInstance *CfgMetaData

// Instance - Get Instance, make it singleton because there is only one copy of metadata in an env
func Instance() *CfgMetaData {
	metaDataOnce.Do(func() {
		metaDataInstance = newCfgMetaData()
		metaDataInstance.init()
	})
	return metaDataInstance
}

func newCfgMetaData() *CfgMetaData {
	return &CfgMetaData{metaMap: make(map[string]Item)}
}

// CfgMetaData ...
type CfgMetaData struct {
	mu      sync.RWMutex
	metaMap map[string]Item
}

// Register adds a configuration item after package initialization, when
// appending to ConfigList is too late because Instance() has already been
// built by the config managers. It returns the existing item and false when
// an item with the same name is already defined.
func Register(item Item) (Item, bool) {
	inst := Instance()
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if existing, ok := inst.metaMap[item.Name]; ok {
		return existing, false
	}
	inst.metaMap[item.Name] = item
	ConfigList = append(ConfigList, item)
	return item, true
}

// init ...
func (c *CfgMetaData) init() {
	c.initFromArray(ConfigList)
}

// initFromArray - Initial metadata from an array
func (c *CfgMetaData) initFromArray(items []Item) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metaMap = make(map[string]Item)
	for _, item := range items {
		c.metaMap[item.Name] = item
	}
}

// GetByName - Get current metadata of current name, if not defined, return false in second params
func (c *CfgMetaData) GetByName(name string) (*Item, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if item, ok := c.metaMap[name]; ok {
		return &item, true
	}
	return nil, false
}

// GetAll - Get all metadata in current env
func (c *CfgMetaData) GetAll() []Item {
	c.mu.RLock()
	defer c.mu.RUnlock()
	metaDataList := make([]Item, 0)
	for _, value := range c.metaMap {
		metaDataList = append(metaDataList, value)
	}
	return metaDataList
}
