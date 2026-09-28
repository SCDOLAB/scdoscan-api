/**
*  @file
*  @copyright defined in scan-api/LICENSE
 */

package common

import (
	"os"
	"strings"
)

// Environment variables that override the DataBase section of the JSON config files.
// Keep database credentials in the environment, never in committed config files.
const (
	EnvMongoURLs     = "SCAN_MONGO_URLS"     // comma-separated host:port list, e.g. "127.0.0.1:27017"
	EnvMongoMode     = "SCAN_MONGO_MODE"     // "single" or "replset"
	EnvMongoReplset  = "SCAN_MONGO_REPLSET"  // replica set name (replset mode)
	EnvMongoDB       = "SCAN_MONGO_DB"       // database name, e.g. "scdo"
	EnvMongoUser     = "SCAN_MONGO_USER"     // username (single mode with authentication)
	EnvMongoPassword = "SCAN_MONGO_PASSWORD" // password (single mode with authentication)
	EnvMongoAuth     = "SCAN_MONGO_AUTH"     // "true"/"false"; defaults to true when user and password are set
)

// ApplyEnv overrides the database settings with any SCAN_MONGO_* environment variables that are set.
func (c *DataBaseConfig) ApplyEnv() {
	if c == nil {
		return
	}
	if v := strings.TrimSpace(os.Getenv(EnvMongoURLs)); v != "" {
		var urls []string
		for _, u := range strings.Split(v, ",") {
			if u = strings.TrimSpace(u); u != "" {
				urls = append(urls, u)
			}
		}
		c.DataBaseConnURLs = urls
	}
	if v := os.Getenv(EnvMongoMode); v != "" {
		c.DataBaseMode = v
	}
	if v := os.Getenv(EnvMongoReplset); v != "" {
		c.DataBaseReplsetName = v
	}
	if v := os.Getenv(EnvMongoDB); v != "" {
		c.DataBaseName = v
	}
	user, pwd := os.Getenv(EnvMongoUser), os.Getenv(EnvMongoPassword)
	if user != "" {
		c.User = user
	}
	if pwd != "" {
		c.Pwd = pwd
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvMongoAuth))) {
	case "1", "true", "yes":
		c.UseAuthentication = true
	case "0", "false", "no":
		c.UseAuthentication = false
	case "":
		if user != "" && pwd != "" {
			c.UseAuthentication = true
		}
	}
}

// EnvOr returns the value of the environment variable key, or def when it is unset or empty.
func EnvOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
