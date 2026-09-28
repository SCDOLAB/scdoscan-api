package syncer

import (
	"time"

	"github.com/scdoproject/scan-api/common"
)

// Config server config
type Config struct {
	RpcURL       string
	WriteLog     bool
	LogLevel     string
	LogFile      string
	DataBase     *common.DataBaseConfig
	SyncInterval time.Duration
	ShardNumber  int
}
