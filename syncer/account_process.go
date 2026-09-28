package syncer

import (
	"sync"
	"time"

	"github.com/scdoproject/scan-api/common"
	"github.com/scdoproject/scan-api/database"
	"github.com/scdoproject/scan-api/log"
	"github.com/scdoproject/scan-api/rpc"
)

const (
	nullAddress = "0S0000000000000000000000000000000000000000"
)

func (s *Syncer) accountUpdateSync() {
	var wg sync.WaitGroup
	wg.Add(len(s.updateAccount) + len(s.updateMinerAccount))

	for _, v := range s.updateAccount {

		//txCnt, err := s.db.GetTxCntByShardNumberAndAddress(s.shardNumber, v.Address)
		txCnt, err := s.db.GetTxCntByAddressFromAccount(v.Address)
		if err != nil {
			log.Error(err)
			txCnt = 0
		}

		v.TxCount = txCnt + 1

		v := v

		s.workerpool.Submit(func() {

			account := v
			balance, err := s.rpc.GetBalance(common.HexAddrToSAddr(account.Address))
			if err != nil {
				log.Error(err)
				balance = 0
			}

			account.Balance = balance

			s.db.UpdateAccount(account)

			wg.Done()
		})
	}

	for _, m := range s.updateMinerAccount {
		s.workerpool.Submit(func() {
			s.db.UpdateMinerAccount(m)
			wg.Done()
		})
	}

	wg.Wait()

	s.updateAccount = make(map[string]*database.DBAccount)
	s.updateMinerAccount = make(map[string]*database.DBMiner)
}

func (s *Syncer) accountSync(b *rpc.BlockInfo) error {
	var address string
	// var AccType int
	// var SourceCode string
	// var ABI string
	// var ContractName string
	// var CompilerVersion string
	txDebtsTo := map[string]int{} // get all the txDebts in block
	for i := 0; i < len(b.TxDebts); i++ {
		txDebtsTo[b.TxDebts[i].To] = 1
	}
	s.mu.Lock()
	for i := 0; i < len(b.Txs); i++ {
		tx := b.Txs[i]
		if tx.From != nullAddress {
			if common.GetAccountType(tx.To) == 0 {
				address = tx.From
				balance, err := s.rpc.GetBalance(common.HexAddrToSAddr(address))
				if err != nil {
					log.Error(err)
					balance = 0
				}
				txCnt, accType, err := s.db.GetTxCntAndAccTypeByAddressFromAccount(address)
				if err != nil {
					log.Error(err)
					txCnt = 0
				}
				accounts := &database.DBAccount{
					AccType:     accType,
					ShardNumber: s.shardNumber,
					Address:     address,
					TxCount:     txCnt + 1,
					Balance:     balance,
					TimeStamp:   b.Timestamp.Int64(),
				}
				s.db.UpdateAccount(accounts)
			}
		}

		if tx.To == "" {
			//create contract transaction
			//Get contract address from receipt
			// receipt, err := s.rpc.GetReceiptByTxHash(tx.Hash)
			// if err == nil {
			// 	contractAddress := receipt.ContractAddress
			// 	address = contractAddress
			// 	AccType = 1
			// }
			continue
		} else if common.GetAccountType(tx.To) == 1 {
			// contractAccount, err := s.db.GetAccountByAddress(tx.To)
			// if err != nil {
			// 	// get this contract account from database failed
			// 	address = tx.To
			// } else {
			// 	// get this contract account from database success
			// 	SourceCode = contractAccount.SourceCode
			// 	ABI = contractAccount.ABI
			// 	ContractName = contractAccount.ContractName
			// 	CompilerVersion = contractAccount.CompilerVersion
			// }
			continue
		} else {
			address = tx.To                      // To might be another shard account for cross-shard transaction
			if _, ok := txDebtsTo[address]; ok { // to is debt account
				continue
			}
		}
		balance, err := s.rpc.GetBalance(common.HexAddrToSAddr(address))
		if err != nil {
			log.Error(err)
			balance = 0
		}
		txCnt, accType, err := s.db.GetTxCntAndAccTypeByAddressFromAccount(address)
		if err != nil {
			if err.Error() == "not found" { // new address will use the default AccType: 0 for normal address, 1 for contract
				txCnt = 0
				// accType = AccType
			} else {
				log.Error(err)
				return err
			}
		}
		accounts := &database.DBAccount{
			AccType:     accType,
			ShardNumber: s.shardNumber,
			Address:     address,
			TxCount:     txCnt + 1,
			Balance:     balance,
			TimeStamp:   b.Timestamp.Int64(),
			// SourceCode:      SourceCode,
			// ABI:             ABI,
			// ContractName:    ContractName,
			// CompilerVersion: CompilerVersion,
		}
		s.db.UpdateAccount(accounts)
	}

	for i := 0; i < len(b.Debts); i++ {
		debts := b.Debts[i]
		address := common.HexAddrToSAddr(debts.To)
		balance, err := s.rpc.GetBalance(common.HexAddrToSAddr(address))
		if err != nil {
			log.Error(err)
			balance = 0
		}
		txCnt, accType, err := s.db.GetTxCntAndAccTypeByAddressFromAccount(address)
		if err != nil {
			log.Error(err)
			txCnt = 0
		}
		accounts := &database.DBAccount{
			AccType:     accType,
			ShardNumber: s.shardNumber,
			Address:     address,
			TxCount:     txCnt + 1,
			Balance:     balance,
			TimeStamp:   b.Timestamp.Int64(),
		}
		s.db.UpdateAccount(accounts)
	}
	defer s.mu.Unlock()
	return nil

}

func (s *Syncer) minersaccountSync(b *rpc.BlockInfo) error {
	//exclude genesis block
	timeBegin := time.Now().Unix()
	if b.Creator != nullAddress {
		s.mu.Lock()
		defer s.mu.Unlock()
		blockCnt, blockFee, blockAmount, err := s.db.GetMinedBlocksByShardNumberAndAddress(s.shardNumber, b.Creator) // previous mined block info
		log.Debug("scdo_syncer account_process mineraccount GetMinedBlocksByShardNumberAndAddress time %d(s)", time.Now().Unix()-timeBegin)
		if err != nil {
			log.Error(err)
			blockCnt = 0
		}
		// add current mined block info
		dbBlock := database.CreateDbBlock(b)
		txDebtsTo := map[string]int{} // get all the txDebts in block
		for i := 0; i < len(b.TxDebts); i++ {
			txDebtsTo[b.TxDebts[i].To] = 1
		}
		for i := 0; i < len(dbBlock.Txs); i++ {
			trans := dbBlock.Txs[i]
			receipt, err := s.rpc.GetReceiptByTxHash(trans.Hash)
			if err == nil {
				dbBlock.Txs[i].Fee = receipt.TotalFee
			} else {
				return err
			}
		}
		for i := 0; i < len(dbBlock.Debts); i++ {
		getDebt:
			dbTx, err := s.db.GetTxByHash(dbBlock.Debts[i].TxHash) // transaction should be synchronized before this debt block is processed
			if err == nil {
				dbBlock.Debts[i].Fee = dbTx.Fee
			} else {
				time.Sleep(10 * time.Second)
				log.Info("Try again to get debt's fee from transaction hash:%s", dbBlock.Debts[i].TxHash)
				goto getDebt
				return err
			}
		}
		for j := 0; j < len(dbBlock.Txs); j++ {
			data := dbBlock.Txs[j]
			if txDebtsTo[data.To] > 0 {
				blockFee += data.Fee / 3 // cross shard txs
			} else {
				blockFee += data.Fee
			}
		}
		for j := 0; j < len(dbBlock.Debts); j++ {
			data := dbBlock.Debts[j]
			blockFee += data.Fee * 2 / 3 //block fee for cross shard destination
		}
		blockAmount += dbBlock.Reward
		blockCnt += 1
		miners := &database.DBMiner{
			ShardNumber: s.shardNumber,
			Address:     b.Creator,
			Reward:      blockAmount,
			TxFee:       blockFee,
			Revenue:     blockAmount + blockFee,
			TimeStamp:   b.Timestamp.Int64(),
			Mined:       blockCnt,
		}
		timeBegin = time.Now().Unix()
		s.db.UpdateMinerAccount(miners)
		log.Debug("scdo_syncer account_process mineraccount UpdateMinerAccount time %d(s)", time.Now().Unix()-timeBegin)
	}
	return nil

}
