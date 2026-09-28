package handlers

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/gin-gonic/gin"
	"github.com/scdoproject/scan-api/common"
	"github.com/scdoproject/scan-api/database"
	"github.com/scdoproject/scan-api/log"
)

// ContractTbl describe
type ContractTbl struct {
	shardNumber   int
	DBClient      BlockInfoDB
	contractTbl   []*database.DBContract
	contractMutex sync.RWMutex
	totalBalance  int64
}

// ProcessGContractTable process global account table
func (a *ContractTbl) ProcessGContractTable() {
	temp, err := a.DBClient.GetContractsByShardNumber(a.shardNumber, maxShowAccountNum)
	if err != nil {
		log.Error("[DB] err : %v", err)
	} else {
		a.contractMutex.Lock()
		a.contractTbl = temp
		a.contractMutex.Unlock()
	}
}

// GetContractCnt get the length of account table
func (a *ContractTbl) GetContractCnt() int {
	a.contractMutex.RLock()
	size := len(a.contractTbl)
	a.contractMutex.RUnlock()
	return size
}

// GetContractsByIdx get a transaction list from mongo by time period
func (a *ContractTbl) GetContractsByIdx(begin uint64, end uint64) []*database.DBContract {
	a.contractMutex.RLock()
	if end > uint64(len(a.contractTbl)) {
		end = uint64(len(a.contractTbl)) - 1
	}

	retContracts := a.contractTbl[begin:end]
	a.contractMutex.RUnlock()
	return retContracts
}

func (a *ContractTbl) GetAccountByAddress(address string) *database.DBContract {
	var retContract *database.DBContract
	a.contractMutex.RLock()
	for i := 0; i < len(a.contractTbl); i++ {
		if a.contractTbl[i].Address == address {
			retContract = a.contractTbl[i]
		}
	}
	a.contractMutex.RUnlock()
	return retContract
}

// getAccountsByBeginAndEnd
func (a *ContractTbl) getContractsByBeginAndEnd(begin, end uint64) []*RetSimpleContractInfo {
	var contracts []*RetSimpleContractInfo

	dbAccounts := a.GetContractsByIdx(begin, end)

	for i := 0; i < len(dbAccounts); i++ {
		data := dbAccounts[i]

		simpleContract := createRetSimpleContractInfo(data, a.totalBalance)
		simpleContract.Rank = i + 1
		contracts = append(contracts, simpleContract)
	}

	return contracts
}

// ContractHandler handle all contract request
type ContractHandler struct {
	contractTbls []*ContractTbl
	DBClient     BlockInfoDB
}

// NewContractHandler return an contractHandler to handler account request
func NewContractHandler(DBClient BlockInfoDB) *ContractHandler {
	var contractTbls []*ContractTbl
	for i := 1; i <= shardCount; i++ {
		contractTbl := &ContractTbl{shardNumber: i, DBClient: DBClient}
		contractTbls = append(contractTbls, contractTbl)
	}

	ret := &ContractHandler{
		contractTbls: contractTbls,
		DBClient:     DBClient,
	}
	ret.updateImpl()
	return ret
}

func (h *ContractHandler) updateImpl() {
	for i := 1; i <= shardCount; i++ {
		h.contractTbls[i-1].ProcessGContractTable()
	}

	totalBalances, err := h.DBClient.GetTotalBalance()
	if err != nil {
		log.Error("[DB] err : %v", err)
	}

	for i := 0; i < shardCount; i++ {
		if v, exist := totalBalances[i+1]; exist {
			h.contractTbls[i].totalBalance = v
		} else {
			h.contractTbls[i].totalBalance = remianTotalBalance
		}
	}
}

// Update Update account list every 5 secs
func (h *ContractHandler) Update() {
	for {
		now := time.Now()
		// calcuate next zero hour
		next := now.Add(time.Second * 5)
		t := time.NewTimer(next.Sub(now))
		<-t.C

		h.updateImpl()
	}
}

// GetContractByAddressImpl  use account info, account tx list and account pending tx list to assembly contract information
func (h *ContractHandler) GetContractByAddressImpl(address string) *RetDetailContractInfo {
	dbClinet := h.DBClient

	data, err := dbClinet.GetContractByAddress(address)
	if err != nil {
		return nil
	}

	// if data.AccType != 1 {
	// 	return nil
	// }

	//txs, err := dbClinet.GetTxsByAddresss(address, txCount, false)
	txs, err := dbClinet.GetTxsByAddresses(address, false, txCount, -1)
	if err != nil {
		return nil
	}

	pengdingTxs, err := dbClinet.GetPendingTxsByAddress(address)
	if err != nil {
		return nil
	}

	// var ttBalance int64
	// if data.ShardNumber >= 1 && data.ShardNumber <= shardCount {
	// 	ttBalance = h.contractTbls[data.ShardNumber-1].totalBalance
	// }

	txs = append(txs, pengdingTxs...)

	data.TxCount, err = dbClinet.GetTxCntByShardNumberAndAddress(data.ShardNumber, address)
	if err != nil {
		return nil
	}

	if data.ShardNumber >= 1 && data.ShardNumber <= shardCount {
		account := h.contractTbls[data.ShardNumber-1].GetAccountByAddress(data.Address)
		if account != nil && account.TxCount != data.TxCount {
			account.TxCount = data.TxCount
		}
	}

	src20Accounts, err := h.DBClient.GetAccountSrc20ByAddress(address)
	if err != nil {
		return nil
	}

	// detailAccount := createRetDetailAccountInfo(data, txs, ttBalance)
	detailAccount := createRetDetailContractInfo(data, txs)

	for i := 0; i < len(src20Accounts); i++ {
		var src20Token RetDetailAccountSrc20TokenInfo
		src20Token.AccType = src20Accounts[i].AccType
		src20Token.Address = src20Accounts[i].Address
		src20Token.TokenAddress = src20Accounts[i].TokenAddress
		src20Token.Balance = src20Accounts[i].Balance
		src20Token.ShardNumber = src20Accounts[i].ShardNumber
		src20Token.TimeStamp = src20Accounts[i].TimeStamp

		if contract, err := h.DBClient.GetContractByAddress(src20Accounts[i].TokenAddress); err == nil {
			src20Token.Contract = RetDetailContractInfo{
				Address:              contract.Address,
				Balance:              contract.Balance,
				ShardNumber:          contract.ShardNumber,
				TimeStamp:            contract.TimeStamp,
				SourceCode:           contract.SourceCode,
				ABI:                  contract.ABI,
				ContractName:         contract.ContractName,
				CompilerVersion:      contract.CompilerVersion,
				ContractCreationCode: contract.ContractCreationCode,
				DeployedBytecode:     contract.DeployedBytecode,
				AbiInput:             contract.AbiInput,
				ConstructorArguments: contract.ConstructorArguments,
				Name:                 contract.Name,
				Symbol:               contract.Symbol,
				Decimals:             contract.Decimals,
				TotalSupply:          contract.TotalSupply,
				Logo:                 contract.Logo,
			}
		}

		detailAccount.Src20Tokens = append(detailAccount.Src20Tokens, src20Token)
	}

	return detailAccount
}

// GetContractByAddress get contract detail info by address
func (h *ContractHandler) GetContractByAddress() gin.HandlerFunc {
	return func(c *gin.Context) {
		address := c.Query("address")
		// if len(address) != addressLength {
		// 	responseError(c, errParamInvalid, http.StatusBadRequest, apiParmaInvalid)
		// 	return
		// }

		detailAccount := h.GetContractByAddressImpl(address)

		c.JSON(http.StatusOK, gin.H{
			"code":    apiOk,
			"message": "",
			"data":    detailAccount,
		})

	}
}

// GetContracts handler for get contract list
func (h *ContractHandler) GetContracts() gin.HandlerFunc {
	return func(c *gin.Context) {
		p, _ := strconv.ParseUint(c.Query("p"), 10, 64)
		ps, _ := strconv.ParseUint(c.Query("ps"), 10, 64)
		s, _ := strconv.ParseInt(c.Query("s"), 10, 64)
		if ps == 0 {
			ps = blockItemNumsPrePage
		} else if ps > maxItemNumsPrePage {
			ps = maxItemNumsPrePage
		}

		if p >= 1 {
			p--
		}

		if s <= 0 {
			s = 1
		}
		shardNumber := int(s)
		if shardNumber < 1 || shardNumber > 20 {
			responseError(c, errParamInvalid, http.StatusBadRequest, apiParmaInvalid)
			return
		}

		contractTbl := h.contractTbls[shardNumber-1]
		contractCnt := contractTbl.GetContractCnt()

		page, begin, end := getBeginAndEndByPage(uint64(contractCnt), p, ps)
		contracts := contractTbl.getContractsByBeginAndEnd(begin, end)

		c.JSON(http.StatusOK, gin.H{
			"code":    apiOk,
			"message": "",
			"data": gin.H{
				"pageInfo": gin.H{
					"totalCount": contractCnt,
					"begin":      begin,
					"end":        end,
					"curPage":    page + 1,
				},
				"list": contracts,
			},
		})
	}
}

type Inputs struct {
	Indexed      bool   `json:"indexed"`
	InternalType string `json:"internalType"`
	Name         string `json:"name"`
	Type         string `json:"type"`
}
type Outputs struct {
	InternalType string `json:"internalType"`
	Name         string `json:"name"`
	Type         string `json:"type"`
}
type Abi struct {
	Anonymous       bool      `json:"anonymous,omitempty"`
	Inputs          []Inputs  `json:"inputs"`
	Name            string    `json:"name"`
	Type            string    `json:"type"`
	Constant        bool      `json:"constant,omitempty"`
	Outputs         []Outputs `json:"outputs,omitempty"`
	Payable         bool      `json:"payable,omitempty"`
	StateMutability string    `json:"stateMutability,omitempty"`
}
type Bytecode struct {
	Object    string `json:"object"`
	Opcodes   string `json:"opcodes"`
	SourceMap string `json:"sourceMap"`
}
type Evm struct {
	Bytecode Bytecode `json:"bytecode"`
}
type Data struct {
	Abi []Abi
	Evm Evm
}
type Errors struct {
	Component        string `json:"component"`
	FormattedMessage string `json:"formattedMessage"`
	Message          string `json:"message"`
	Severity         string `json:"severity"`
	Type             string `json:"type"`
}
type Contracts struct {
	Errors    []Errors                   `json:"errors"`
	Contracts map[string]map[string]Data `json:"contracts"`
}

func (h *ContractHandler) VerifyContract() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Receive contract info submitted by users
		// sourceCode := c.PostForm("sourceCode")
		mode := c.PostForm("mode")
		address := c.PostForm("address")
		compilerVersion := c.PostForm("compilerVersion")
		abiInput := c.PostForm("abiInput")
		logo := c.PostForm("logo")
		switch mode {
		case "standard-json":
			standardJSON := c.PostForm("standardJSON")
			if err := h.verifyContractByJSONImpl(address, standardJSON, abiInput, compilerVersion, logo); err != nil {
				log.Error(err.Error())
				c.JSON(http.StatusOK, gin.H{
					"code":    apiOk,
					"message": err.Error(),
					"data":    false,
				})
			} else {
				c.JSON(http.StatusOK, gin.H{
					"code":    apiOk,
					"message": "",
					"data":    true,
				})
			}
		default:
			contractName := c.PostForm("contractName")
			optimizer, err := strconv.ParseBool(c.PostForm("optimizer"))
			if err != nil {
				log.Error(err.Error())
				c.JSON(http.StatusOK, gin.H{
					"code":    apiOk,
					"message": "Invalid boolean value",
					"data":    false,
				})
				return
			}
			runs, err := strconv.Atoi(c.PostForm("runs"))
			if err != nil {
				log.Error(err.Error())
				c.JSON(http.StatusOK, gin.H{
					"code":    apiOk,
					"message": "Invalid number value",
					"data":    false,
				})
				return
			}
			uploadFile, err := c.FormFile("sourceCode")
			if err != nil {
				log.Error(err.Error())
				c.JSON(http.StatusOK, gin.H{
					"code":    apiOk,
					"message": err.Error(),
					"data":    false,
				})
				return
			}

			// Check file suffix
			if filepath.Ext(uploadFile.Filename) != ".sol" && filepath.Ext(uploadFile.Filename) != ".zip" {
				c.JSON(http.StatusOK, gin.H{
					"code":    apiOk,
					"message": "Incorrect file type",
					"data":    false,
				})
				return
			}

			// Get temp dir path
			// tempDir, err := os.MkdirTemp("", "scdo_contract_verify")
			tempDir, err := ioutil.TempDir("", "scdo_contract_verify")
			if err != nil {
				log.Error(err.Error())
				c.JSON(http.StatusOK, gin.H{
					"code":    apiOk,
					"message": err.Error(),
					"data":    false,
				})
				return
			}
			// Save upload file
			uploadFilePath := filepath.Join(tempDir, uploadFile.Filename)
			if err = c.SaveUploadedFile(uploadFile, uploadFilePath); err != nil {
				log.Error(err.Error())
				c.JSON(http.StatusOK, gin.H{
					"code":    apiOk,
					"message": err.Error(),
					"data":    false,
				})
				return
			}

			// The directory returned by os.MkdirTemp will not be automatically deleted after use. You need to ensure manual deletion here.
			defer os.RemoveAll(tempDir)

			if err := h.verifyContractImpl(address, uploadFilePath, abiInput, contractName, compilerVersion, optimizer, runs, logo); err != nil {
				log.Error(err.Error())
				c.JSON(http.StatusOK, gin.H{
					"code":    apiOk,
					"message": err.Error(),
					"data":    false,
				})
			} else {
				c.JSON(http.StatusOK, gin.H{
					"code":    apiOk,
					"message": "",
					"data":    true,
				})
			}
		}
	}
}

func (h *ContractHandler) verifyContractImpl(address string, uploadFilePath string, abiInput string, contractName string, compilerVersion string, optimizer bool, runs int, logo string) (err error) {
	if address == "" || uploadFilePath == "" || contractName == "" || compilerVersion == "" {
		return errors.New("missing one or more query parameters")
	}

	contractData := h.GetContractByAddressImpl(address)
	if contractData == nil {
		return errors.New("This address is empty data")
	}

	uploadFileExt := filepath.Ext(uploadFilePath)
	var sourceFile string
	var sourceCode string
	if uploadFileExt == ".zip" {
		common.UnZip(uploadFilePath, filepath.Dir(uploadFilePath))
		sourceFile = contractName + ".sol"

		sourceCodeMap := make(map[string]string)
		err := filepath.Walk(filepath.Dir(uploadFilePath), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Checks if a file has a specified extension
			if info.IsDir() || filepath.Ext(info.Name()) != ".sol" {
				return nil
			}

			// get sourceCode file string
			sourceCodeByte, err := ioutil.ReadFile(path)
			if err != nil {
				return errors.New(fmt.Sprintln("Error read file:", err))
			}
			sourceCode = string(sourceCodeByte)

			sourceCodePath, err := filepath.Rel(filepath.Dir(uploadFilePath), path)
			if err != nil {
				return err
			}

			sourceCodeMap[sourceCodePath] = sourceCode
			return nil
		})
		if err != nil {
			return err
		}
		sourceCodeByte, err := json.Marshal(sourceCodeMap)
		if err != nil {
			return err
		}
		sourceCode = string(sourceCodeByte)

	} else if uploadFileExt == ".sol" {
		sourceFile = contractName + ".sol"

		// get sourceCode file string
		sourceCodeByte, err := ioutil.ReadFile(uploadFilePath)
		if err != nil {
			return errors.New(fmt.Sprintln("Error read file:", err))
		}
		sourceCode = string(sourceCodeByte)
	} else if uploadFileExt != ".zip" && uploadFileExt != ".sol" {
		return errors.New("Invalid file type")
	}

	executablePath, err := os.Executable()
	if err != nil {
		log.Error(err)
		return err
	}
	var solcFilePath string
	if runtime.GOOS == "windows" {
		solcFilePath = filepath.Join(filepath.Dir(executablePath), "solc-bin", "solc-"+runtime.GOOS+"-"+runtime.GOARCH+"-"+compilerVersion, "solc.exe")
	} else if runtime.GOOS == "linux" {
		solcFilePath = filepath.Join(filepath.Dir(executablePath), "solc-bin", "solc-"+runtime.GOOS+"-"+runtime.GOARCH+"-"+compilerVersion)
	}

	cmd := exec.Command(
		solcFilePath, "--standard-json",
		"--allow-paths", "",
	)
	cmd.Dir = filepath.Dir(uploadFilePath)
	solcInputByte, err := json.Marshal(map[string]interface{}{
		"language": "Solidity",
		"sources": map[string]interface{}{
			contractName + ".sol": map[string]interface{}{
				"urls": []string{
					sourceFile,
				},
			},
		},
		"settings": map[string]interface{}{
			"optimizer": map[string]interface{}{
				"enabled": optimizer,
				"runs":    runs,
			},
			"outputSelection": map[string]interface{}{
				"*": map[string]interface{}{
					"*": []string{
						"abi",
						"metadata",
						"evm.bytecode",
						"evm.bytecode.sourceMap",
					},
				},
			},
			"evmVersion": "istanbul",
		},
	})
	if err != nil {
		return err
	}
	cmd.Stdin = bytes.NewReader(solcInputByte)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout // Standard Output
	cmd.Stderr = &stderr // Error Output
	if err := cmd.Run(); err != nil {
		return errors.New(fmt.Sprintf("cmd.Run() failed with %s\n", err))
	}
	outStr, errStr := stdout.String(), stderr.String()
	log.Debug("Standard Output:\n%s\nError Output:\n%s\n", outStr, errStr)

	var solcOutputJson Contracts
	if err = json.Unmarshal([]byte(stdout.Bytes()), &solcOutputJson); err != nil {
		return errors.New(fmt.Sprintln("Error decoding JSON:", err))
	}

	if len(solcOutputJson.Errors) > 0 {
		return errors.New(fmt.Sprintln("Error build solidity:", solcOutputJson.Errors[0].Message))
	}

	// Compare contract codes
	bytecode := "0x" + solcOutputJson.Contracts[contractName+".sol"][contractName].Evm.Bytecode.Object
	log.Debug("build result:", bytecode+abiInput)
	log.Debug("payload data:", contractData.ContractCreationCode)
	if bytecode+abiInput != contractData.ContractCreationCode {
		return errors.New("contract code does not match sourceCode")
	}

	// Get ABI JSON
	abiJsonStruct := solcOutputJson.Contracts[contractName+".sol"][contractName].Abi
	abiJsonByte, err := json.Marshal(abiJsonStruct)
	if err != nil {
		return errors.New(fmt.Sprintln("Error decoding JSON:", err))
	}

	abiCodec, err := abi.JSON(bytes.NewReader(abiJsonByte))
	if err != nil {
		log.Error(err)
		return
	}

	constructorInput, err := hex.DecodeString(abiInput)
	if err != nil {
		log.Error(err)
		return
	}
	var abiUnpackMap = make(map[string]interface{})
	err = abiCodec.Constructor.Inputs.UnpackIntoMap(abiUnpackMap, constructorInput)
	if err != nil {
		log.Error(err)
		return
	}
	var dataArrary = map[string]string{}
	for index, value := range abiUnpackMap {
		if dataString, ok := value.(string); ok {
			dataArrary[index+"(string)"] = dataString
		}
		if dataBigInt, ok := value.(*big.Int); ok {
			dataArrary[index+"(uint256)"] = dataBigInt.String()
		}
	}
	name, ok := abiUnpackMap["_name"].(string)
	if !ok {
		return errors.New(fmt.Sprintln("_name not found"))
	}
	symbol, ok := abiUnpackMap["_symbol"].(string)
	if !ok {
		return errors.New(fmt.Sprintln("_symbol not found"))
	}
	decimals, ok := abiUnpackMap["_decimals"].(*big.Int)
	if !ok {
		return errors.New(fmt.Sprintln("_decimals not found"))
	}
	totalSupply, ok := abiUnpackMap["_initialSupply"].(*big.Int)
	if !ok {
		return errors.New(fmt.Sprintln("_initialSupply not found"))
	}

	contract := &database.DBContract{
		Address:              address,
		SourceCode:           string(sourceCode),
		ABI:                  string(abiJsonByte),
		ContractName:         contractName,
		CompilerVersion:      compilerVersion,
		DeployedBytecode:     bytecode,
		AbiInput:             abiInput,
		ConstructorArguments: dataArrary,
		Name:                 name,
		Symbol:               symbol,
		Decimals:             decimals.String(),
		TotalSupply:          totalSupply.String(),
		Logo:                 logo,
	}
	err = h.DBClient.UpdateContract(contract)
	if err != nil {
		log.Error("save contract verification info failed, address:%s", address)
		return err
	}
	return nil
}

func (h *ContractHandler) verifyContractByJSONImpl(address string, standardJSON string, abiInput string, compilerVersion string, logo string) (err error) {
	if address == "" || standardJSON == "" || compilerVersion == "" {
		return errors.New("missing one or more query parameters")
	}

	contractData := h.GetContractByAddressImpl(address)
	if contractData == nil {
		return errors.New("this address is empty data")
	}

	var standardJSONData map[string]interface{}
	if err := json.Unmarshal([]byte(standardJSON), &standardJSONData); err != nil {
		return err
	}

	executablePath, err := os.Executable()
	if err != nil {
		log.Error(err)
		return err
	}
	var solcFilePath string
	if runtime.GOOS == "windows" {
		solcFilePath = filepath.Join(filepath.Dir(executablePath), "solc-bin", "solc-"+runtime.GOOS+"-"+runtime.GOARCH+"-"+compilerVersion, "solc.exe")
	} else if runtime.GOOS == "linux" {
		solcFilePath = filepath.Join(filepath.Dir(executablePath), "solc-bin", "solc-"+runtime.GOOS+"-"+runtime.GOARCH+"-"+compilerVersion)
	}

	cmd := exec.Command(
		solcFilePath, "--standard-json",
		"--allow-paths", "",
	)

	standardJSONByte, err := json.Marshal(standardJSONData)
	if err != nil {
		return err
	}
	cmd.Dir = filepath.Dir(filepath.Dir(solcFilePath))
	solcInputByte := standardJSONByte
	cmd.Stdin = bytes.NewReader(solcInputByte)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout // Standard Output
	cmd.Stderr = &stderr // Error Output
	if err := cmd.Run(); err != nil {
		return errors.New(fmt.Sprintf("cmd.Run() failed with %s\n", err))
	}
	outStr, errStr := stdout.String(), stderr.String()
	log.Debug("Standard Output:\n%s\nError Output:\n%s\n", outStr, errStr)

	var solcOutputJson Contracts
	if err = json.Unmarshal([]byte(stdout.Bytes()), &solcOutputJson); err != nil {
		return errors.New(fmt.Sprintln("Error decoding JSON:", err))
	}

	if len(solcOutputJson.Errors) > 0 {
		return errors.New(fmt.Sprintln("Error build solidity:", solcOutputJson.Errors[0].Message))
	}

	// Compare contract codes
	var Found bool
	var SolcContractFile string
	var SolcContractName string
	var Bytecode string
	for solcContractFile, solcContractDatas := range solcOutputJson.Contracts {
		for solcContractName, solcContractData := range solcContractDatas {
			// Add source code
			bytecode := "0x" + solcContractData.Evm.Bytecode.Object
			log.Debug("build result:", bytecode+abiInput)
			log.Debug("payload data:", contractData.ContractCreationCode)
			if bytecode+abiInput == contractData.ContractCreationCode {
				// found
				Found = true
				SolcContractFile = solcContractFile
				SolcContractName = solcContractName
				Bytecode = bytecode
				break
			}
		}
		if Found {
			break
		}
	}
	if !Found {
		return errors.New("contract code does not match sourceCode")
	}

	// Get ABI JSON
	abiJsonStruct := solcOutputJson.Contracts[SolcContractFile][SolcContractName].Abi
	abiJsonByte, err := json.Marshal(abiJsonStruct)
	if err != nil {
		return errors.New(fmt.Sprintln("Error decoding JSON:", err))
	}

	abiCodec, err := abi.JSON(bytes.NewReader(abiJsonByte))
	if err != nil {
		log.Error(err)
		return
	}

	constructorInput, err := hex.DecodeString(abiInput)
	if err != nil {
		log.Error(err)
		return
	}
	var abiUnpackMap = make(map[string]interface{})
	err = abiCodec.Constructor.Inputs.UnpackIntoMap(abiUnpackMap, constructorInput)
	if err != nil {
		log.Error(err)
		return
	}
	var dataArrary = map[string]string{}
	for index, value := range abiUnpackMap {
		if dataString, ok := value.(string); ok {
			dataArrary[index+"(string)"] = dataString
		}
		if dataBigInt, ok := value.(*big.Int); ok {
			dataArrary[index+"(uint256)"] = dataBigInt.String()
		}
	}
	name, ok := abiUnpackMap["_name"].(string)
	if !ok {
		return errors.New(fmt.Sprintln("_name not found"))
	}
	symbol, ok := abiUnpackMap["_symbol"].(string)
	if !ok {
		return errors.New(fmt.Sprintln("_symbol not found"))
	}
	decimals, ok := abiUnpackMap["_decimals"].(*big.Int)
	if !ok {
		return errors.New(fmt.Sprintln("_decimals not found"))
	}
	totalSupply, ok := abiUnpackMap["_initialSupply"].(*big.Int)
	if !ok {
		return errors.New(fmt.Sprintln("_initialSupply not found"))
	}

	sourceCodeMap := make(map[string]string)
	if sources, ok := standardJSONData["sources"].(map[string]interface{}); ok {
		for sourceFileName, sourceInterface := range sources {
			if source, ok := sourceInterface.(map[string]interface{}); ok {
				if sourceContent := source["content"].(string); ok {
					sourceCodeMap[sourceFileName] = sourceContent
				} else {
					return errors.New("the standardJSON data does not contain the source code content")
				}
			} else {
				return errors.New("the standardJSON data does not contain the source code content")
			}
		}
	} else {
		return errors.New("the standardJSON data does not contain the source code content")
	}

	sourceCodeString, err := json.Marshal(sourceCodeMap)
	if err != nil {
		return err
	}
	contract := &database.DBContract{
		Address:              address,
		SourceCode:           string(sourceCodeString),
		ABI:                  string(abiJsonByte),
		ContractName:         SolcContractName,
		CompilerVersion:      compilerVersion,
		ContractCreationCode: contractData.ContractCreationCode,
		DeployedBytecode:     Bytecode,
		AbiInput:             abiInput,
		ConstructorArguments: dataArrary,
		Name:                 name,
		Symbol:               symbol,
		Decimals:             decimals.String(),
		TotalSupply:          totalSupply.String(),
		Logo:                 logo,
	}
	err = h.DBClient.UpdateContract(contract)
	if err != nil {
		log.Error("save contract verification info failed, address:%s", address)
		return err
	}
	return nil
}
