package syncer

import (
	"time"

	"github.com/scdoproject/scan-api/common"
	"github.com/scdoproject/scan-api/database"
	"github.com/scdoproject/scan-api/log"
)

func (s *Syncer) accountSrc20Sync(tokenContract *database.DBContract, accountAddress string) {
	// TODO: sync token balance
	accType := common.GetAccountType(accountAddress)

	balance, err := s.rpc.BalanceOf(tokenContract.Address, accountAddress)
	if err != nil {
		log.Error(err)
		return
	}

	balanceFloat64, _ := balance.Float64()
	if err := s.db.UpdateAccountSrc20(&database.DBAccountSrc20{
		AccType:      accType,
		Address:      accountAddress,
		TokenAddress: tokenContract.Address,
		Balance:      balanceFloat64,
		TimeStamp:    time.Now().Unix(),
		ShardNumber:  s.shardNumber,
	}); err != nil {
		log.Error(err)
		return
	}
}
