// SPDX-License-Identifier: Apache-2.0
package avail

import (
	"encoding/json"
	"io"
	"os"

	s3_storage_service "github.com/0xPolygon/cdk/dataavailability/avail/s3StorageService"
)

type Config struct {
	Seed       string `mapstructure:"seed"`
	AppID      int    `mapstructure:"app_id"`
	WsApiUrl   string `mapstructure:"ws_api_url"`
	HttpApiUrl string `mapstructure:"http_api_url"`

	BridgeEnabled bool   `mapstructure:"bridge_enabled"`
	BridgeApiUrl  string `mapstructure:"bridge_api_url"`
	BridgeTimeout int    `mapstructure:"bridge_timeout"`

	// Fallback
	FallbackS3ServiceConfig s3_storage_service.S3StorageServiceConfig `koanf:"fallback-s3-service-config"`
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
