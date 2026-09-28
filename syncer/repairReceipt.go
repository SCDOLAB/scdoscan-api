package syncer

import (
	"fmt"
	"time"

	"github.com/scdoproject/scan-api/common"
	"github.com/scdoproject/scan-api/log"
	"gopkg.in/mgo.v2"
)

// StartSync start an timer to sync block data from scdo node
func (s *Syncer) StartSyncRepairReceipt(interval time.Duration, blockHeight uint64) {
	// ticks := time.NewTicker(interval * time.Second)
	// tick := ticks.C
	lowBlockHeight := uint64(2979594)
	go func() {
		for blockHeight >= lowBlockHeight {
			log.Info("syncRepairReceipt blockHeight: %d", blockHeight)
			if err := s.syncRepairReceipt(blockHeight); err != nil {
				log.Error("%d syncRepairReceipt error: %s", blockHeight, err.Error())
				blockHeight--
				continue
			}
			blockHeight--
			// _, ok := <-tick
			// if !ok {
			// 	break
			// }
		}
	}()
}

// StartSync start an timer to sync block data from scdo node
func (s *Syncer) StartSyncRepairBlock(interval time.Duration, blockHeight uint64) {
	// ticks := time.NewTicker(interval * time.Second)
	// tick := ticks.C
	lowBlockHeight := uint64(2979594)
	go func() {
		for blockHeight >= lowBlockHeight {
			log.Info("syncRepairBlock blockHeight: %d", blockHeight)
			_, err := s.db.GetBlockByHeight(s.shardNumber, blockHeight)
			if err != nil && err != mgo.ErrNotFound {
				log.Error("%d syncRepairBlock GetBlockByHeight error: %s", blockHeight, err.Error())
				blockHeight--
				continue
			}
			if err == nil {
				log.Info("%d syncRepairBlock skip this block", blockHeight)
				blockHeight--
				continue
			}
			if isOk := s.SyncHandle(blockHeight); isOk {
				blockHeight--
				continue
			}
			blockHeight--
			// _, ok := <-tick
			// if !ok {
			// 	break
			// }
		}
	}()
}

func (s *Syncer) syncRepairReceipt(blockHeight uint64) error {
	block, err := s.db.GetBlockByHeight(s.shardNumber, blockHeight)
	if err != nil {
		return err
	}
	for _, txData := range block.Txs {
		tx, err := s.db.GetTxByHash(txData.Hash)
		if err != nil {
			log.Error("hash:%s err:%s", err.Error())
			continue
		}
		dbTx := tx
		var abi = ""
		if common.GetAccountType(tx.To) == 1 {
			//contract function call
			//find if the contract is verified, thus having abi info
			// contractAccount, err := s.db.GetAccountByAddress(tx.To)
			// if err == nil && contractAccount.ABI != "" {
			// abi = contractAccount.ABI
			// }
		}
		// transaction fee is in the receipt
		if receipt, err := s.rpc.GetReceiptByTxHash(dbTx.Hash, abi); err == nil {
			dbTx.Fee = receipt.TotalFee
			dbTx.UsedGas = receipt.UsedGas
			if dbTx.To == "" {
				dbTx.TxType = 1
				dbTx.ContractAddress = receipt.Contract
				dbTx.Receipt = *receipt
			} else if common.GetAccountType(dbTx.To) == 1 {
				dbTx.ContractAddress = dbTx.To
				dbTx.Receipt = *receipt
			} else {
				dbTx.Receipt = *receipt
			}
		}

		// update database
		if err = s.db.UpdateTxByHash(s.shardNumber, tx.Hash, dbTx); err != nil {
			return fmt.Errorf("processing %s transaction error for %d block: %s", tx.Hash, blockHeight, err.Error())
		}

	}

	return nil
}
