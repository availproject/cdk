// SPDX-License-Identifier: Apache-2.0
package avail

import (
	"encoding/json"
	"io"
	"os"
)

type Config struct {
	Seed         string `mapstructure:"seed"`
	WsApiUrl     string `mapstructure:"ws_api_url"`
	HttpApiUrl   string `mapstructure:"http_api_url"`
	BridgeApiUrl string `mapstructure:"bridge_api_url"`
	AppID        int    `mapstructure:"app_id"`
	Timeout      int    `mapstructure:"timeout"`
}

func (c *Config) GetConfig(configFileName string) error {
	jsonFile, err := os.Open(configFileName)
	if err != nil {
		return err
	}
	defer jsonFile.Close()

	byteValue, err := io.ReadAll(jsonFile)
	if err != nil {
		return err
	}

	err = json.Unmarshal(byteValue, c)
	if err != nil {
		return err
	}

	return nil
}
