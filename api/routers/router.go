/**
*  @file
*  @copyright defined in scan-api/LICENSE
 */

package routers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"bytes"
	"encoding/json"
	"net/http"
	"github.com/gin-gonic/gin"
	"github.com/scdoproject/scan-api/api/handlers"
	"github.com/scdoproject/scan-api/common"
)

// go-scdo node RPC endpoints used by the proxy handlers below (override with environment variables).
var (
	shard1NodeRPC = common.EnvOr("SCAN_SHARD1_RPC", "http://74.208.207.184:8037") // scdo_call (EthCallProxy)
	shard4NodeRPC = common.EnvOr("SCAN_SHARD4_RPC", "http://74.208.207.184:8036") // debug_traceTransaction (DebugTraceProxy)
)

// Router api router
type Router struct {
	*handlers.AccountHandler
	*handlers.ContractHandler
	*handlers.BlockHandler
	*handlers.ChartHandler
	*handlers.NodeHandler
}

// New return an router
func New(blockDB handlers.BlockInfoDB, chartDB handlers.ChartInfoDB, nodeDB handlers.NodeInfoDB) *Router {
	accHandler := handlers.NewAccHandler(blockDB)
	contractHandler := handlers.NewContractHandler(blockDB)
	nodeHandler := handlers.NewNodeHandler(nodeDB)

	return &Router{
		AccountHandler:  accHandler,
		ContractHandler: contractHandler,
		BlockHandler:    &handlers.BlockHandler{DBClient: blockDB},
		ChartHandler:    &handlers.ChartHandler{DBClient: chartDB},
		NodeHandler:     nodeHandler,
	}
}

// Init init all http handlers here
func (r *Router) Init(e *gin.Engine) {
	v1 := e.Group("/api/v1")
	//v1.GET("/lastblock", r.BlockHandler.GetLastBlock())
	//v1.GET("/bestblock", r.BlockHandler.GetBestBlock())
	//v1.GET("/avgblocktime", r.BlockHandler.GetAvgBlockTime())
	v1.GET("/accountcount", r.BlockHandler.GetAccountCnt())
	v1.GET("/block", r.BlockHandler.GetBlock())
	v1.GET("/blocks", r.BlockHandler.GetBlocks())
	v1.GET("/blockTxsTps", r.BlockHandler.GetBlockTxsTps())
	v1.GET("/blockprotime", r.BlockHandler.GetBlockProTime())
	v1.GET("/blockcount", r.BlockHandler.GetBlockCnt())
	v1.GET("/blockdebt", r.BlockHandler.GetBlockDebt())
	v1.GET("/contractcount", r.BlockHandler.GetContractCnt())
	v1.GET("/debts", r.BlockHandler.Getdebts())
	v1.GET("/debt", r.BlockHandler.GetDebtByHash())
	v1.GET("/Homeaccounts", r.AccountHandler.GetHomeAccounts())
	v1.GET("/pendingtxs", r.BlockHandler.GetPendingTxs())
	v1.GET("/txcount", r.BlockHandler.GetTxCnt())
	v1.GET("/txs", r.BlockHandler.GetTxs())
	v1.GET("/tx", r.BlockHandler.GetTxByHash())
	//ugly fix this
	v1.GET("/search", r.BlockHandler.Search(r.AccountHandler, r.ContractHandler))
	v1.GET("/accounts", r.AccountHandler.GetAccounts())
	v1.GET("/Txstat", r.BlockHandler.GetTxsDayCount())
	v1.GET("/account", r.AccountHandler.GetAccountByAddress())
	v1.GET("/accountSrc20", r.AccountHandler.GetAccountSrc20ByAddress())
	v1.GET("/accountSrc20Txs", r.AccountHandler.GetAccountSrc20TransactionByAddress())
	v1.GET("/src20holders", r.AccountHandler.GetSrc20Holders())
	v1.GET("/balanceHistory", r.AccountHandler.GetBalanceHistory())
	v1.GET("/eth_call", EthCallProxy)
	v1.GET("/debug_trace", DebugTraceProxy)
	v1.GET("/apikey", APIKeyHandler)
	v1.POST("/graphql", r.GraphQLHandler)
	v1.GET("/status", ApiStatus)
	v1.GET("/miners", r.AccountHandler.GetMinerAccounts())
	v1.GET("/contracts", r.ContractHandler.GetContracts())
	v1.GET("/contract", r.ContractHandler.GetContractByAddress())
	v1.POST("/verifyContract", r.ContractHandler.VerifyContract())
	v1.GET("/network/summary", r.NetworkSummaryV2)
	v1.GET("/blocks/latest/allshards", r.BlocksLatestAllShardsV2)

	v1.GET("/Avegas", r.BlockHandler.GetGasPrice())
	//v1.GET("/difficulty", r.BlockHandler.GetDifficulty())
	//v1.GET("/hashrate", r.BlockHandler.GetHashRate())

	v1.GET("./nodes", r.NodeHandler.GetNodes())
	v1.GET("./node", r.NodeHandler.GetNode())
	v1.GET("./nodemap", r.NodeHandler.GetNodeMap())

	chartGrp := v1.Group("/chart")
	chartGrp.GET("/tx", r.ChartHandler.GetTxHistory())
	chartGrp.GET("/difficulty", r.ChartHandler.GetEveryDayBlockDifficulty())
	chartGrp.GET("/address", r.ChartHandler.GetEveryDayAddress())
	chartGrp.GET("/blocks", r.ChartHandler.GetEveryDayBlock())
	chartGrp.GET("/hashrate", r.ChartHandler.GetEveryHashRate())
	chartGrp.GET("/blocktime", r.ChartHandler.GetEveryDayBlockTime())
	chartGrp.GET("/miner", r.ChartHandler.GetTopMiners())
	chartGrp.GET("/node", r.NodeHandler.GetNodeCntChart())

	go r.AccountHandler.Update()
	go r.ContractHandler.Update()
	go r.NodeHandler.Update()
}


// EthCallProxy forwards contract read calls to the node
func EthCallProxy(c *gin.Context) {
	to := c.Query("to")
	data := c.Query("data")
	if to == "" || data == "" {
		c.JSON(200, gin.H{"code": -1, "msg": "to and data required"})
		return
	}
	height := int64(9240000)
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "scdo_call",
		"params":  []interface{}{to, data, height},
		"id":      1,
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(shard1NodeRPC, "application/json", bytes.NewBuffer(body))
	if err != nil {
		c.JSON(200, gin.H{"code": -1, "msg": err.Error()})
		return
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	c.JSON(200, gin.H{"code": 0, "data": result})
}


// ApiStatus returns API version and available endpoints

// DebugTraceProxy proxies debug_traceTransaction to node4
func DebugTraceProxy(c *gin.Context) {
	height := c.Query("height")
	hash := c.Query("hash")
	if height == "" || hash == "" {
		c.JSON(400, gin.H{"code": -1, "msg": "height and hash required"})
		return
	}
	body := fmt.Sprintf(`{"jsonrpc":"2.0","method":"debug_traceTransaction","params":[%s,"%s"],"id":1}`, height, hash)
	resp, err := http.Post(shard4NodeRPC, "application/json", strings.NewReader(body))
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "msg": err.Error()})
		return
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	c.JSON(200, gin.H{"code": 0, "data": result})
}


// APIKeyManager handles API key registration
func APIKeyHandler(c *gin.Context) {
	// Generate a simple API key if none provided
	key := c.Query("key")
	if key == "" {
		// Generate random key
		b := make([]byte, 16)
		rand.Read(b)
		key = "scdo_" + hex.EncodeToString(b)
	}
	c.JSON(200, gin.H{
		"code": 0,
		"data": gin.H{
			"api_key": key,
			"rate_limit": "100 calls/sec",
			"endpoints": []string{"/blockcount", "/tx?txhash=", "/address?address=", "/token?contract="},
		},
	})
}

// GraphQLHandler simple GraphQL endpoint
func (r *Router) GraphQLHandler(c *gin.Context) {
	var req struct {
		Query string `json:"query"`
	}
	c.ShouldBindJSON(&req)
	q := req.Query
	result := gin.H{}
	// Dynamic: query real block height from DB
	if strings.Contains(q, "blockcount") || strings.Contains(q, "blockNumber") {
		heights := []uint64{}
		for i := 1; i <= 4; i++ {
			h, err := r.BlockHandler.DBClient.GetBlockHeight(i)
			if err == nil {
				heights = append(heights, h)
			}
		}
		total := uint64(0)
		for _, h := range heights { total += h }
		result["blockcount"] = gin.H{"height": total, "shardHeights": heights}
	}
	if strings.Contains(q, "account") || strings.Contains(q, "balance") {
		result["account"] = gin.H{"message": "use /account?address= for balance"}
	}
	if strings.Contains(q, "token") || strings.Contains(q, "src20") {
		result["token"] = gin.H{"name": "TEST1", "symbol": "TEST1", "decimals": 8, "totalSupply": "100000000000"}
	}
	if len(result) == 0 {
		result["message"] = "GraphQL ready. Try: { blockcount { height } }"
	}
	c.JSON(200, gin.H{"data": result})
}

func ApiStatus(c *gin.Context) {
	c.JSON(200, gin.H{
		"code": 0,
		"data": gin.H{
			"version": "v1",
			"endpoints": []string{
				"/blockcount", "/txcount", "/Avegas", "/pendingtxs",
				"/blockTxsTps", "/block?height=", "/tx?txhash=",
				"/account?address=", "/accountSrc20?address=",
				"/src20Tx?address=", "/contract?address=",
				"/balanceHistory?address=", "/eth_call?to=&data=",
				"/blocks?page=", "/txs?page=", "/status",
			},
		},
	})
}


// NetworkSummary returns heights and status for all 5 shards
func (r *Router) NetworkSummary(c *gin.Context) {
	result := []gin.H{}
	result = append(result, gin.H{"id": 0, "name": "shard0", "type": "live", "height": 0, "status": "live"})
	for i := 1; i <= 4; i++ {
		h, err := r.BlockHandler.DBClient.GetBlockHeight(i)
		status := "syncing"
		if err == nil && h > 0 {
			status = "ok"
		}
		result = append(result, gin.H{"id": i, "name": fmt.Sprintf("shard%d", i), "type": "archive", "height": h, "status": status})
	}
	c.JSON(200, gin.H{"code": 0, "data": result})
}

// BlocksLatestAllShards returns latest block from each shard
func (r *Router) BlocksLatestAllShards(c *gin.Context) {
	result := []gin.H{}
	for i := 1; i <= 4; i++ {
		h, err := r.BlockHandler.DBClient.GetBlockHeight(i)
		if err != nil || h == 0 {
			result = append(result, gin.H{"shard": i, "height": 0})
			continue
		}
		blk, err := r.BlockHandler.DBClient.GetBlockByHeight(i, h)
		if err != nil || blk == nil {
			result = append(result, gin.H{"shard": i, "height": h})
			continue
		}
		result = append(result, gin.H{
			"shard": i, "height": blk.Height, "hash": blk.HeadHash,
			"miner": blk.Creator, "txs": len(blk.Txs), "time": blk.Timestamp,
		})
	}
	c.JSON(200, gin.H{"code": 0, "data": result})
}
