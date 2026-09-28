package syncer

import (
	"github.com/scdoproject/scan-api/common"
	"github.com/scdoproject/scan-api/database"
	"github.com/scdoproject/scan-api/log"
	"github.com/scdoproject/scan-api/rpc"
)

func (s *Syncer) contractSync(b *rpc.BlockInfo) error {
	// var SourceCode string
	// var ABI string
	// var ContractName string
	// var CompilerVersion string
	txDebtsTo := map[string]int{} // get all the txDebts in block
	for i := 0; i < len(b.TxDebts); i++ {
		txDebtsTo[b.TxDebts[i].To] = 1
	}
	for i := 0; i < len(b.Txs); i++ {
		tx := b.Txs[i]

		// Create contract transactions
		if tx.To == "" {
			log.Debug("Contract deployment transaction found")
			receipt, err := s.rpc.GetReceiptByTxHash(tx.Hash)
			if err != nil {
				log.Error(err)
				continue
			}
			if receipt.Failed {
				log.Debug("Contract deployment transaction failed")
				continue
			}
			if receipt.Contract == "" {
				log.Debug("Contract deployment transaction failed, contract is empty")
				continue
			}
			address := receipt.Contract
			balance, err := s.rpc.GetBalance(common.HexAddrToSAddr(address))
			if err != nil {
				log.Error(err)
				balance = 0
			}
			contract := &database.DBContract{
				ShardNumber:          s.shardNumber,
				Address:              address,
				Balance:              balance,
				TxCount:              0 + 1,
				TimeStamp:            b.Timestamp.Int64(),
				ContractCreationCode: tx.Payload,
			}
			err = s.db.UpdateContract(contract)
			if err != nil {
				log.Error("Update contract error: %s", err.Error())
			}
		}

		// Contract transactions via fromAddress
		if common.GetAccountType(tx.From) == 1 {
			address := tx.From
			balance, err := s.rpc.GetBalance(common.HexAddrToSAddr(address))
			if err != nil {
				log.Error(err)
				balance = 0
			}
			txCnt, _, err := s.db.GetTxCntAndAccTypeByAddressFromAccount(address)
			if err != nil {
				log.Error(err)
				txCnt = 0
			}
			contract := &database.DBContract{
				Address:   address,
				Balance:   balance,
				TxCount:   txCnt + 1,
				TimeStamp: b.Timestamp.Int64(),
			}
			s.db.UpdateContract(contract)
			if err != nil {
				log.Error("Update contract error: %s", err.Error())
			}
		}

		// Contract transactions via toAddress
		if common.GetAccountType(tx.To) == 1 {
			address := tx.To
			balance, err := s.rpc.GetBalance(common.HexAddrToSAddr(address))
			if err != nil {
				log.Error(err)
				balance = 0
			}
			txCnt, _, err := s.db.GetTxCntAndAccTypeByAddressFromAccount(address)
			if err != nil {
				log.Error(err)
				txCnt = 0
			}
			contract := &database.DBContract{
				Address:   address,
				Balance:   balance,
				TxCount:   txCnt + 1,
				TimeStamp: b.Timestamp.Int64(),
			}
			s.db.UpdateContract(contract)
			if err != nil {
				log.Error("Update contract error: %s", err.Error())
			}
		}
	}

	return nil
}
