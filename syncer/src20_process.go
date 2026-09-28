package syncer

import (
	"encoding/hex"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"

	common2 "github.com/ethereum/go-ethereum/common"
	"github.com/scdoproject/scan-api/common"
	"github.com/scdoproject/scan-api/database"
	"github.com/scdoproject/scan-api/log"
	"github.com/scdoproject/scan-api/rpc"
)

// txSync insert the transactions into database
func (s *Syncer) src20Sync(block *rpc.BlockInfo) error {
	// init abi
	tokenAbi, err := abi.JSON(strings.NewReader(common.SRC20AbiJsonString))
	if err != nil {
		return err
	}
	timeBegin := time.Now().Unix()
	transIdx, _ := s.db.GetSrc20TxCntByShardNumber(s.shardNumber)
	log.Debug("scdo_syncer tx_process GetTxCntByShardNumber time:%d(s)", time.Now().Unix()-timeBegin)

	src20Txs := []interface{}{}
	var dbSrc20Txs []*database.DBSrc20Tx
	timeBegin = time.Now().Unix()
	for _, trans := range block.Txs {
		if common.GetAccountType(trans.To) != 1 {
			continue
		}
		trans.Block = block.Height
		trans.Timestamp = block.Timestamp.Uint64()
		transIdx++
		trans.Idx = transIdx
		dbSrc20Tx := database.CreateDbSrc20Tx(trans)
		dbSrc20Tx.Pending = false
		dbSrc20Tx.ShardNumber = s.shardNumber
		dbSrc20Tx.ContractAddress = trans.To
		tokenContract, err := s.db.GetContractByAddress(trans.To)
		if err != nil {
			log.Error("scdo_syncer src20Sync GetContractByAddress error:%v", err)
			continue
		}
		if tokenContract.ABI == "" {
			continue
		}
		dbSrc20Tx.Contract = *tokenContract

		inputData, err := hex.DecodeString(trans.Payload[2:])
		if err != nil {
			log.Error(err)
			continue
		}

		method, err := tokenAbi.MethodById(inputData[:4])
		if err != nil {
			log.Error(err)
			continue
		}
		dbSrc20Tx.Method = method.Name

		args, err := method.Inputs.UnpackValues(inputData[4:])
		if err != nil {
			log.Error(err)
			continue
		}

		switch method.Name {
		case "transfer":
			toAddress, ok := args[0].(common2.Address)
			if !ok {
				continue
			}
			toAddressString := common.HexAddrToSAddr(toAddress.String())
			amount, ok := args[1].(*big.Int)
			if !ok {
				continue
			}
			var decimals *big.Int
			decimalsBigInt, ok := new(big.Int).SetString(tokenContract.Decimals, 10)
			if !ok {
				// Use default decimals
				decimals = new(big.Int).Exp(big.NewInt(10), big.NewInt(8), nil)
			} else {
				decimals = new(big.Int).Exp(big.NewInt(10), decimalsBigInt, nil)
			}
			amountFloat := new(big.Float).Quo(new(big.Float).SetInt(amount), new(big.Float).SetInt(decimals))
			dbSrc20Tx.To = toAddressString
			dbSrc20Tx.ActionAmount = amountFloat.String()
		default:
			continue
		}

		if receipt, err := s.rpc.GetReceiptByTxHash(trans.Hash, ""); err == nil {
			dbSrc20Tx.Fee = receipt.TotalFee
			dbSrc20Tx.UsedGas = receipt.UsedGas
			dbSrc20Tx.Receipt = *receipt
		}

		dbSrc20Txs = append(dbSrc20Txs, dbSrc20Tx)
		src20Txs = append(src20Txs, dbSrc20Tx)

		// update from address token balance
		go s.accountSrc20Sync(tokenContract, dbSrc20Tx.From)
		// update to address token balance
		go s.accountSrc20Sync(tokenContract, dbSrc20Tx.To)
	}
	log.Debug("scdo_syncer src20_process prepareTxs time:%d(s)", time.Now().Unix()-timeBegin)

	if len(dbSrc20Txs) == 0 {
		return nil
	}
	timeBegin = time.Now().Unix()
	if err := s.db.AddSrc20Txs(src20Txs...); err != nil {
		return err
	}
	log.Debug("scdo_syncer src20_process AddSrc20Txs time:%d(s)", time.Now().Unix()-timeBegin)

	return nil
}
