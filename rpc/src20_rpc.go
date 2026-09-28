package rpc

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	common2 "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/scdoproject/scan-api/common"
)

// GeneratePayload generate payload
func (rpc *ScdoRPC) GeneratePayload(abiJSON string, methodName string, args []interface{}) (string, error) {
	var result interface{}
	var request []interface{}
	request = append(request, abiJSON)
	request = append(request, methodName)
	request = append(request, args)
	err := rpc.call("scdo_generatePayload", request, &result)
	if err != nil {
		return "", err
	}
	payload, ok := result.(string)
	if !ok {
		return "", fmt.Errorf("invalid result type, expected string, got %T", result)
	}
	return payload, nil
}

func (rpc *ScdoRPC) Decimals(tokenAddress string) (*big.Int, error) {
	// 解析ABI
	src20ABI, err := abi.JSON(strings.NewReader(common.SRC20AbiJsonString))
	if err != nil {
		return big.NewInt(0), err
	}
	payloadByte, err := src20ABI.Pack("decimals")
	if err != nil {
		return big.NewInt(0), err
	}
	payload := hexutil.Encode(payloadByte)
	decimalsResult, err := rpc.Call(tokenAddress, payload, -1)
	if err != nil {
		return big.NewInt(0), err
	}
	// 模拟一个返回值
	returnValue := hexutil.MustDecode(decimalsResult.Result)

	var decimals *big.Int
	// 解码返回值
	err = src20ABI.Unpack(&decimals, "decimals", returnValue)
	if err != nil {
		return big.NewInt(0), err
	}
	return decimals, nil
}

func (rpc *ScdoRPC) BalanceOf(tokenAddress string, spender string) (*big.Float, error) {
	// 解析ABI
	src20ABI, err := abi.JSON(strings.NewReader(common.SRC20AbiJsonString))
	if err != nil {
		return big.NewFloat(0), err
	}
	payloadByte, err := src20ABI.Pack("balanceOf", common2.HexToAddress("0x"+spender[2:]))
	if err != nil {
		return big.NewFloat(0), err
	}
	payload := hexutil.Encode(payloadByte)
	balanceOfResult, err := rpc.Call(tokenAddress, payload, -1)
	if err != nil {
		return big.NewFloat(0), err
	}

	returnValue := hexutil.MustDecode(balanceOfResult.Result)

	var balanceOf *big.Int
	// 解码返回值
	err = src20ABI.Unpack(&balanceOf, "balanceOf", returnValue)
	if err != nil {
		return big.NewFloat(0), err
	}

	decimals, err := rpc.Decimals(tokenAddress)
	if err != nil {
		return big.NewFloat(0), err
	}

	// 取小数位
	return big.NewFloat(0).Quo(new(big.Float).SetInt(balanceOf), new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), decimals, nil))), nil
}
