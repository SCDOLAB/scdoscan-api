/**
*  @file
*  @copyright defined in scan-api/LICENSE
 */

package cmd

import (
	"encoding/json"
	"github.com/scdoproject/scan-api/common"
	"io/ioutil"

	"github.com/scdoproject/scan-api/server"
)

// LoadConfigFromFile unmarshal config from a file
func LoadConfigFromFile(filepath string) (server.Config, error) {
	var config server.Config
	buff, err := ioutil.ReadFile(filepath)
	if err != nil {
		return config, err
	}

	err = json.Unmarshal(buff, &config)

	return config, err
}

// LoadFilterConfigFromFile unmarshal config from a file
func LoadFilterConfigFromFile(filepath string) (common.FilterConfig, error) {
	var config common.FilterConfig
	buff, err := ioutil.ReadFile(filepath)
	if err != nil {
		return config, err
	}

	err = json.Unmarshal(buff, &config)

	return config, err
}
