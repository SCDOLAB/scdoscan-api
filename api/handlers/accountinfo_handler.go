package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/scdoproject/scan-api/database"
	"github.com/scdoproject/scan-api/log"
)

const (
	shardCount        = 4
	maxShowAccountNum = 10000
	txCount           = 25
	//exclude divide zero problem
	remianTotalBalance = 1
	MINERRANKSIZE      = 20
)

var errGetAccountsFromDB = errors.New("could not get miner data from db")

// AccountTbl represents an account list ordered by account balance
type AccountTbl struct {
	shardNumber  int
	DBClient     BlockInfoDB
	accountTbl   []*database.DBAccount
	accMutex     sync.RWMutex
	totalBalance int64
}

// ProcessGAccountTable process global account table
func (a *AccountTbl) ProcessGAccountTable() {
	temp, err := a.DBClient.GetAccountsByShardNumber(a.shardNumber, maxShowAccountNum)
	if err != nil {
		log.Error("[DB] err : %v", err)
	} else {
		a.accMutex.Lock()
		a.accountTbl = temp
		a.accMutex.Unlock()
	}
}

// GetAccountCnt get the length of account table
func (a *AccountTbl) GetAccountCnt() int {
	a.accMutex.RLock()
	size := len(a.accountTbl)
	a.accMutex.RUnlock()
	return size
}

func (a *AccountTbl) GetAccountByAddress(address string) *database.DBAccount {
	var retAccount *database.DBAccount
	a.accMutex.RLock()
	for i := 0; i < len(a.accountTbl); i++ {
		if a.accountTbl[i].Address == address {
			retAccount = a.accountTbl[i]
		}
	}
	a.accMutex.RUnlock()
	return retAccount
}

// GetAccountsByIdx get a transaction list from mongo by time period
func (a *AccountTbl) GetAccountsByIdx(begin uint64, end uint64) []*database.DBAccount {
	a.accMutex.RLock()
	if end > uint64(len(a.accountTbl)) {
		end = uint64(len(a.accountTbl)) - 1
	}

	retAccounts := a.accountTbl[begin:end]
	a.accMutex.RUnlock()
	return retAccounts
}

// getAccountsByBeginAndEnd
func (a *AccountTbl) getAccountsByBeginAndEnd(begin, end uint64) []*RetSimpleAccountInfo {
	var accounts []*RetSimpleAccountInfo

	dbAccounts := a.GetAccountsByIdx(begin, end)

	for i := 0; i < len(dbAccounts); i++ {
		data := dbAccounts[i]

		simpleAccount := createRetSimpleAccountInfo(data, a.totalBalance)
		simpleAccount.Rank = i + 1
		accounts = append(accounts, simpleAccount)
	}

	return accounts
}

// AccountHandler handle all account request
type AccountHandler struct {
	accTbls  []*AccountTbl
	DBClient BlockInfoDB
}

// NewAccHandler return an accounthandler to handler account request
func NewAccHandler(DBClient BlockInfoDB) *AccountHandler {
	var accTbls []*AccountTbl
	for i := 1; i <= shardCount; i++ {
		accTbl := &AccountTbl{shardNumber: i, DBClient: DBClient}
		accTbls = append(accTbls, accTbl)
	}

	ret := &AccountHandler{
		accTbls:  accTbls,
		DBClient: DBClient,
	}

	ret.updateImpl()
	return ret
}

func (h *AccountHandler) GetMinerAccounts() gin.HandlerFunc {
	return func(c *gin.Context) {
		miners, err := h.DBClient.GetMinerAccounts(MINERRANKSIZE)
		if err != nil {
			responseError(c, errGetAccountsFromDB, http.StatusInternalServerError, apiDBQueryError)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    apiOk,
			"message": "",
			"data":    miners,
		})

	}
}

func (h *AccountHandler) updateImpl() {

	for i := 1; i <= shardCount; i++ {
		h.accTbls[i-1].ProcessGAccountTable()
	}

	totalBalances, err := h.DBClient.GetTotalBalance()
	if err != nil {
		log.Error("[DB] err : %v", err)
	}

	for i := 0; i < shardCount; i++ {
		if v, exist := totalBalances[i+1]; exist != false {
			h.accTbls[i].totalBalance = v
		} else {
			h.accTbls[i].totalBalance = remianTotalBalance
		}
	}
}

// Update Update account list every 5 secs
func (h *AccountHandler) Update() {
	for {
		now := time.Now()
		// calcuate next zero hour
		next := now.Add(time.Second * 5)
		t := time.NewTimer(next.Sub(now))
		<-t.C

		h.updateImpl()
	}
}

// GetAccounts handler for get account list
func (h *AccountHandler) GetAccounts() gin.HandlerFunc {
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

		accTbl := h.accTbls[shardNumber-1]
		accCnt := accTbl.GetAccountCnt()

		page, begin, end := getAccountBeginAndEndByPage(uint64(accCnt), p, ps)
		accounts := accTbl.getAccountsByBeginAndEnd(begin, end)

		c.JSON(http.StatusOK, gin.H{
			"code":    apiOk,
			"message": "",
			"data": gin.H{
				"pageInfo": gin.H{
					"totalCount":   accCnt,
					"begin":        begin,
					"end":          end,
					"curPage":      page + 1,
					"totalBalance": accTbl.totalBalance,
				},
				"list": accounts,
			},
		})
	}
}

// GetHomeAccounts handler for get account list
func (h *AccountHandler) GetHomeAccounts() gin.HandlerFunc {
	return func(c *gin.Context) {
		var Account []*RetSimpleAccountHome
		Accounts := h.DBClient.GetAccountsByHome()
		for i := 0; i < len(Accounts); i++ {
			data := Accounts[i]
			simpleTx := createHomeRetSimpleAccountInfo(data)
			Account = append(Account, simpleTx)
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    apiOk,
			"message": "",
			"data":    Account,
		})
	}
}

// GetAccountByAddressImpl use account info, account tx list and account pending tx list to assembly account information
func (h *AccountHandler) GetAccountByAddressImpl(address string) *RetDetailAccountInfo {
	dbClient := h.DBClient
	begin := time.Now()
	data, err := dbClient.GetAccountByAddress(address)
	log.Debug("getAccountByAddress time:%d(s)", time.Since(begin))
	if err != nil {
		return nil
	}

	if data.AccType != 0 {
		return nil
	}
	begin = time.Now()
	txs, err := dbClient.GetTxsByAddresses(address, false, txCount, 0)
	log.Debug("getTxsByAddresss time:%d(s)", time.Since(begin))

	if err != nil {
		return nil
	}
	begin = time.Now()
	pengdingTxs, err := dbClient.GetPendingTxsByAddress(address)
	log.Debug("GetPendingTxsByAddress time:%d(s)", time.Since(begin))

	if err != nil {
		return nil
	}

	txs = append(pengdingTxs, txs...)

	begin = time.Now()
	var ttBalance int64
	if data.ShardNumber >= 1 && data.ShardNumber <= shardCount {
		ttBalance = h.accTbls[data.ShardNumber-1].totalBalance
	}
	log.Debug("get ttBalance time:%d(s)", time.Since(begin))

	begin = time.Now()
	log.Debug("txCount from data object:%d", data.TxCount)
	data.TxCount, err = dbClient.GetTxCntByShardNumberAndAddress(data.ShardNumber, address)
	log.Debug("txCount from GetTxCntByShardNumberAndAddress:%d", data.TxCount)
	log.Debug("get data.TxCount time:%d(s)", time.Since(begin))

	if err != nil {
		return nil
	}

	begin = time.Now()
	if data.ShardNumber >= 1 && data.ShardNumber <= shardCount {
		account := h.accTbls[data.ShardNumber-1].GetAccountByAddress(data.Address)
		if account != nil && account.TxCount != data.TxCount {
			account.TxCount = data.TxCount
		}
	}
	log.Debug("update TxCount time:%d(s)", time.Since(begin))

	src20Accounts, err := h.DBClient.GetAccountSrc20ByAddress(address)
	if err != nil {
		return nil
	}

	detailAccount := createRetDetailAccountInfo(data, txs, ttBalance)

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

// GetAccountByAddress get account detail info by address
func (h *AccountHandler) GetAccountByAddress() gin.HandlerFunc {
	return func(c *gin.Context) {
		address := c.Query("address")
		// if len(address) != addressLength {
		// 	responseError(c, errParamInvalid, http.StatusBadRequest, apiParmaInvalid)
		// 	return
		// }

		detailAccount := h.GetAccountByAddressImpl(address)

		c.JSON(http.StatusOK, gin.H{
			"code":    apiOk,
			"message": "",
			"data":    detailAccount,
		})

	}
}

func (h *AccountHandler) GetAccountSrc20TransactionByAddress() gin.HandlerFunc {
	return func(c *gin.Context) {
		address := c.Query("address")
		if address == "" {
			responseError(c, errParamInvalid, http.StatusBadRequest, apiParmaInvalid)
			return
		}
		src20Transactions, err := h.DBClient.GetSrc20TxsByAddresses(address, false, txCount, 0)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code":    apiOk,
				"message": "",
				"data":    nil,
			})
		}

		for i := range src20Transactions {
			contract, err := h.DBClient.GetContractByAddress(src20Transactions[i].ContractAddress)
			if err != nil {
				log.Error(err)
				continue
			}
			src20Transactions[i].Contract = *contract
		}

		detailSrc20Transactions := createRetDetailAccountSrc20TxsInfo(address, src20Transactions)
		c.JSON(http.StatusOK, gin.H{
			"code":    apiOk,
			"message": "",
			"data":    detailSrc20Transactions,
		})
	}
}

// GetAccountSrc20ByAddress get src20 token holdings by address
func (h *AccountHandler) GetAccountSrc20ByAddress() gin.HandlerFunc {
	return func(c *gin.Context) {
		address := c.Query("address")
		if address == "" {
			responseError(c, errParamInvalid, http.StatusBadRequest, apiParmaInvalid)
			return
		}
		src20Accounts, err := h.DBClient.GetAccountSrc20ByAddress(address)
		if err != nil {
			responseError(c, err, http.StatusInternalServerError, apiInternalError)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    apiOk,
			"message": "",
			"data":    src20Accounts,
		})
	}
}



// GetSrc20Holders returns holders of a token (lightweight)
func (h *AccountHandler) GetSrc20Holders() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenAddr := c.Query("address")
		if tokenAddr == "" {
			c.JSON(200, gin.H{"code": -1, "msg": "address required"})
			return
		}
		holders, err := h.DBClient.GetSrc20Holders(tokenAddr)
		if err != nil {
			c.JSON(200, gin.H{"code": -1, "msg": err.Error()})
			return
		}
		type holder struct {
			Address     string  `json:"address"`
			Balance     float64 `json:"balance"`
			ShardNumber int     `json:"shardNumber"`
		}
		result := make([]holder, 0, len(holders))
		for _, h := range holders {
			result = append(result, holder{h.Address, h.Balance, h.ShardNumber})
		}
		c.JSON(200, gin.H{"code": 0, "data": result})
	}
}


// GetBalanceHistory returns balance history for an address
func (h *AccountHandler) GetBalanceHistory() gin.HandlerFunc {
	return func(c *gin.Context) {
		addr := c.Query("address")
		if addr == "" {
			c.JSON(200, gin.H{"code": -1, "msg": "address required"})
			return
		}
		// Get current balance
		acct, err := h.DBClient.GetAccountByAddress(addr)
		if err != nil || acct == nil {
			c.JSON(200, gin.H{"code": 0, "data": []interface{}{}})
			return
		}
		currentBal := acct.Balance
		// Get last 500 txs ascending
		txs, err := h.DBClient.GetTxsByAddresses(addr, true, 500, 0)
		if err != nil || len(txs) == 0 {
			c.JSON(200, gin.H{"code": 0, "data": []interface{}{}})
			return
		}
		// Walk backwards to reconstruct balance
		bal := currentBal
		type point struct {
			Timestamp string `json:"ts"`
			Balance   int64  `json:"balance"`
		}
		var result []point
		for i := len(txs) - 1; i >= 0; i-- {
			tx := txs[i]
			if tx.To == addr {
				bal -= tx.Amount
				if tx.From != addr {
					bal -= tx.Fee
				}
			} else if tx.From == addr {
				bal += tx.Amount
			}
			result = append(result, point{tx.Timestamp, bal})
		}
		// Reverse to chronological order
		for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
			result[i], result[j] = result[j], result[i]
		}
		c.JSON(200, gin.H{"code": 0, "data": result})
	}
}
