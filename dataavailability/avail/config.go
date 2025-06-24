// SPDX-License-Identifier: Apache-2.0
package avail

import (
	"encoding/json"
	"io"
	"os"

	s3_storage_service "github.com/0xPolygon/cdk/dataavailability/avail/s3StorageService"
)

type Config struct {
	Seed       string `mapstructure:"Seed"`
	AppID      int    `mapstructure:"AppID"`
	WsApiUrl   string `mapstructure:"WsApiUrl"`
	HttpApiUrl string `mapstructure:"HttpApiUrl"`

	BridgeEnabled bool   `mapstructure:"BridgeEnabled"`
	BridgeApiUrl  string `mapstructure:"BridgeApiUrl"`
	BridgeTimeout int    `mapstructure:"BridgeTimeout"`
	// Fallback
	FallbackS3ServiceConfig s3_storage_service.S3StorageServiceConfig `mapstructure:"FallbackS3ServiceConfig"`
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
