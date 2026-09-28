/**
*  @file
*  @copyright defined in scan-api/LICENSE
 */

package routers

// Multi-shard homepage endpoints (v2):
//   GET /api/v1/network/summary          -> per-shard height/status/tps/gas (5 shards)
//   GET /api/v1/blocks/latest/allshards  -> latest blocks grouped by shard
// Shards 1-4 (archive) are read from MongoDB (real data). Shard 0 (live mainnet)
// is read from the public shard0 RPC (parallel-node >= 0.4 serves real block
// headers: keccak hash, parentHash, timestamp, miner, gasUsed, transactions).
// If the RPC is unreachable shard0 reports its last known height with
// status "unreachable" and an empty block list (no simulated data).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	shardLiveRPC    = "https://scdoscan.io/rpc/0"
	shard0BlockSecs = 2 // parallel-node blockTime (2s) until observed rate is available
	// shard0 native currency: 18 decimals (1 SCDO = 1e18 wei) since the 2026-09-28 migration.
	// Before it, parallel-node used 8 decimals (1 unit = 1e-8 SCDO = 1e10 wei): tx values in
	// blocks < migration block are raw 8-decimal units and are scaled by 1e10 here.
	shard0Decimals        = 18
	shard0LegacyScale     = 1e10          // 8 -> 18 decimals
	shard0WeiPerGas       = 10000000000   // flat price: 21000 gas * 1e10 wei = 0.00021 SCDO per tx (same real fee before/after)
	shard0MigrationPreset = uint64(59754) // fallback if scdo_nativeCurrency is unavailable (migration block, 2026-09-28)
	shardSnapshotTTL      = 3 * time.Second
	shardBlocksPerShard   = 25
)

var shardDesc = map[int]string{
	0: "Active Mainnet - new EVM chain, live block production, MetaMask supported.",
	1: "Archive shard - historical SCDO mainnet blocks, accounts and transactions.",
	2: "Archive shard - historical SCDO shard 2 data.",
	3: "Archive shard - historical SCDO shard 3 data.",
	4: "Archive shard - historical SCDO shard 4 data.",
}

type shardSnapshot struct {
	at      time.Time
	summary []gin.H
	blocks  map[int][]gin.H
}

var (
	shardSnapMu   sync.Mutex
	shardSnap     *shardSnapshot
	shard0Mu      sync.Mutex
	shard0Height  uint64
	shard0At      time.Time
	shard0FromRPC bool
	shardHTTP     = &http.Client{Timeout: 2 * time.Second}
)

func shardAgeDesc(ts int64) string {
	d := time.Now().Unix() - ts
	switch {
	case d > 86400:
		return fmt.Sprintf("%d days ago", d/86400)
	case d > 3600:
		return fmt.Sprintf("%d hours ago", d/3600)
	case d > 60:
		return fmt.Sprintf("%d mins ago", d/60)
	default:
		if d <= 0 {
			d = 1
		}
		return fmt.Sprintf("%d secs ago", d)
	}
}

func shardRound(v float64, n int) float64 {
	p := math.Pow(10, float64(n))
	return math.Round(v*p) / p
}

// shard0CurrentHeight returns (height, fromRPC). Cached for 3s.
func shard0CurrentHeight() (uint64, bool) {
	shard0Mu.Lock()
	defer shard0Mu.Unlock()
	now := time.Now()
	if shard0Height > 0 && now.Sub(shard0At) < shardSnapshotTTL {
		return shard0Height, shard0FromRPC
	}
	body := []byte(`{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}`)
	resp, err := shardHTTP.Post(shardLiveRPC, "application/json", bytes.NewReader(body))
	if err == nil {
		var out struct {
			Result string `json:"result"`
		}
		derr := json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if derr == nil && strings.HasPrefix(out.Result, "0x") {
			if h, perr := strconv.ParseUint(out.Result[2:], 16, 64); perr == nil && h > 0 {
				shard0Height, shard0At, shard0FromRPC = h, now, true
				return h, true
			}
		}
	}
	// RPC unreachable: last known real height (0 if never reached), flagged not-from-RPC
	return shard0Height, false
}

// shardRPC performs a JSON-RPC call against a public shard endpoint and returns the raw result.
func shardRPC(url, method, params string) (json.RawMessage, error) {
	body := []byte(fmt.Sprintf(`{"jsonrpc":"2.0","method":"%s","params":%s,"id":1}`, method, params))
	resp, err := shardHTTP.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, fmt.Errorf("rpc error: %s", out.Error.Message)
	}
	if len(out.Result) == 0 || string(out.Result) == "null" {
		return nil, fmt.Errorf("empty result")
	}
	return out.Result, nil
}

var (
	shard0MigMu    sync.Mutex
	shard0MigBlock uint64
	shard0MigKnown bool
	shard0MigTried time.Time
)

// shard0MigrationBlock: first block with 18-decimal values (from scdo_nativeCurrency, cached).
func shard0MigrationBlock() uint64 {
	shard0MigMu.Lock()
	defer shard0MigMu.Unlock()
	if shard0MigKnown || time.Since(shard0MigTried) < 30*time.Second {
		if !shard0MigKnown {
			return shard0MigrationPreset
		}
		return shard0MigBlock
	}
	shard0MigTried = time.Now()
	if raw, err := shardRPC(shardLiveRPC, "scdo_nativeCurrency", "[]"); err == nil {
		var info struct {
			Decimals               int    `json:"decimals"`
			DecimalsMigrationBlock uint64 `json:"decimalsMigrationBlock"`
		}
		if json.Unmarshal(raw, &info) == nil && info.Decimals == shard0Decimals {
			shard0MigBlock, shard0MigKnown = info.DecimalsMigrationBlock, true
			return shard0MigBlock
		}
	}
	return shard0MigrationPreset
}

// shard0ValueWei: tx value (hex) normalised to wei; pre-migration values are 8-decimal units.
func shard0ValueWei(v interface{}, height uint64) *big.Int {
	s, _ := v.(string)
	w := new(big.Int)
	if strings.HasPrefix(s, "0x") && len(s) > 2 {
		w.SetString(s[2:], 16)
	}
	if height < shard0MigrationBlock() {
		w.Mul(w, big.NewInt(shard0LegacyScale))
	}
	return w
}

// shard0FormatSCDO: exact decimal SCDO string for a wei amount (trailing zeros trimmed).
func shard0FormatSCDO(wei *big.Int) string {
	d := new(big.Int).Exp(big.NewInt(10), big.NewInt(shard0Decimals), nil)
	q, r := new(big.Int).QuoRem(wei, d, new(big.Int))
	f := strings.TrimRight(fmt.Sprintf("%018s", r.String()), "0")
	if f == "" {
		return q.String()
	}
	return q.String() + "." + f
}

// shard0GasPriceGwei: node-reported gas price (eth_gasPrice) converted to gwei (1e-9 SCDO).
// parallel-node (18 decimals) returns 1e10 wei/gas -> 10 gwei, i.e. the flat 0.00021 SCDO
// fee per 21000-gas transfer. A legacy 8-decimal node returned 1 unit (=1e10 wei) per gas;
// values below 1e6 are treated as such so the display stays right during a rollback.
func shard0GasPriceGwei() (float64, bool) {
	raw, err := shardRPC(shardLiveRPC, "eth_gasPrice", "[]")
	if err != nil {
		return 0, false
	}
	var hx string
	if json.Unmarshal(raw, &hx) != nil || !strings.HasPrefix(hx, "0x") {
		return 0, false
	}
	v, perr := strconv.ParseUint(hx[2:], 16, 64)
	if perr != nil {
		return 0, false
	}
	if v < 1000000 {
		return float64(v) * shard0LegacyScale / 1e9, true
	}
	return float64(v) / 1e9, true
}

// archivePendingCounts: real tx-pool sizes of the go-scdo shards 1-4 (txpool_getTxPoolTxCount), fetched in parallel.
func archivePendingCounts() map[int]int64 {
	res := map[int]int64{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 1; i <= 4; i++ {
		wg.Add(1)
		go func(sh int) {
			defer wg.Done()
			raw, err := shardRPC(fmt.Sprintf("https://scdoscan.io/rpc/%d", sh), "txpool_getTxPoolTxCount", "[]")
			if err != nil {
				return
			}
			var n int64
			if json.Unmarshal(raw, &n) == nil {
				mu.Lock()
				res[sh] = n
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	return res
}

// ---- shard0 real block data via eth_getBlockByNumber (cached by height; blocks are immutable) ----
var (
	shard0BlkMu  sync.Mutex
	shard0Blocks = map[uint64]gin.H{}
)

func shard0RPCBlock(h uint64) (gin.H, error) {
	body := []byte(fmt.Sprintf(`{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0x%x",true],"id":1}`, h))
	resp, err := shardHTTP.Post(shardLiveRPC, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Result map[string]interface{} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Result == nil {
		return nil, fmt.Errorf("bad block response")
	}
	hexv := func(k string) int64 {
		v, _ := out.Result[k].(string)
		if strings.HasPrefix(v, "0x") {
			n, _ := strconv.ParseInt(v[2:], 16, 64)
			return n
		}
		return 0
	}
	str := func(k string) string { v, _ := out.Result[k].(string); return v }
	txn := 0
	txs := []gin.H{}
	if arr, ok := out.Result["transactions"].([]interface{}); ok {
		txn = len(arr)
		for _, t := range arr {
			if m, ok := t.(map[string]interface{}); ok {
				vw := shard0ValueWei(m["value"], h)
				txs = append(txs, gin.H{"hash": m["hash"], "from": m["from"], "to": m["to"],
					"value": m["value"], "valueWei": vw.String(), "valueScdo": shard0FormatSCDO(vw),
					"nonce": m["nonce"], "index": m["transactionIndex"]})
			}
		}
	}
	used := hexv("gasUsed")
	// fee: parallel-node charges a flat price per gas (1e10 wei = 1e-8 SCDO; identical real value
	// before and after the 18-decimal migration); block gasUsed only counts successful txs, so
	// fee (wei) == gasUsed * 1e10. No block reward.
	feeWei := new(big.Int).Mul(big.NewInt(used), big.NewInt(shard0WeiPerGas))
	return gin.H{"shard": 0, "shardnumber": 0, "height": h, "hash": str("hash"), "parentHash": str("parentHash"),
		"stateRoot": str("stateRoot"), "miner": str("miner"), "nonce": str("nonce"),
		"txs": txn, "txn": txn, "transactions": txs, "reward": 0, "fee": feeWei.String(), "feeUnit": "wei (1e-18 SCDO)",
		"feeScdo": shard0FormatSCDO(feeWei), "decimals": shard0Decimals,
		"usedGas": used, "gasLimit": hexv("gasLimit"), "gasprice": shard0WeiPerGas, "gaspriceUnit": "wei", "difficulty": hexv("difficulty"),
		"nodeTime": hexv("timestamp"), "simulated": false}, nil
}

// shard0RealBlocks returns up to n latest shard0 blocks from RPC (nil on RPC error)
func shard0RealBlocks(head uint64, n int, blockSecs float64) []gin.H {
	shard0BlkMu.Lock()
	defer shard0BlkMu.Unlock()
	now := time.Now().Unix()
	var list []gin.H
	fetched := 0
	for i := 0; i < n && uint64(i) < head; i++ {
		h := head - uint64(i)
		b, ok := shard0Blocks[h]
		if !ok {
			if fetched >= 12 { // bound RPC calls per refresh
				break
			}
			var err error
			b, err = shard0RPCBlock(h)
			fetched++
			if err != nil {
				if len(list) == 0 {
					return nil
				}
				break
			}
			shard0Blocks[h] = b
		}
		// real node timestamp (parallel-node >= 0.4); only a clearly bogus value is estimated
		ts := b["nodeTime"].(int64)
		c := gin.H{}
		for k, v := range b {
			c[k] = v
		}
		if ts <= 0 || ts > now+600 {
			ts = now - int64(float64(head-h)*blockSecs)
			c["timeEstimated"] = true
		}
		c["time"] = ts
		c["age"] = shardAgeDesc(ts)
		list = append(list, c)
	}
	for h := range shard0Blocks { // prune cache
		if h+200 < head {
			delete(shard0Blocks, h)
		}
	}
	return list
}

func (r *Router) buildShardSnapshot() *shardSnapshot {
	now := time.Now()
	nowTs := now.Unix()
	snap := &shardSnapshot{at: now, blocks: map[int][]gin.H{}}

	// ---- shard 0 (live, real data from parallel-node RPC) ----
	h0, fromRPC := shard0CurrentHeight()
	bt0 := float64(shard0BlockSecs)
	var b0 []gin.H
	if fromRPC {
		b0 = shard0RealBlocks(h0, shardBlocksPerShard, bt0)
	}
	blocksReal := len(b0) > 0
	if b0 == nil {
		b0 = []gin.H{}
	}
	snap.blocks[0] = b0
	var tps0 interface{}
	var avgBt0 interface{}
	used0 := int64(0)
	txTot := 0
	var tMin, tMax int64
	for k, b := range b0 {
		used0 += b["usedGas"].(int64)
		ts := b["time"].(int64)
		if k == 0 || ts > tMax {
			tMax = ts
		}
		if k == 0 || ts < tMin {
			tMin = ts
		}
		if k > 0 { // txs of the newest block are excluded so the span covers counted blocks
			txTot += b["txn"].(int)
		}
	}
	if len(b0) > 0 {
		used0 /= int64(len(b0))
	}
	if len(b0) > 1 && tMax > tMin {
		span := float64(tMax - tMin)
		tps0 = shardRound(float64(txTot)/span, 4)
		avgBt0 = shardRound(span/float64(len(b0)-1), 2)
	}
	var gas0 interface{}
	gasSrc := "unavailable"
	if g, ok := shard0GasPriceGwei(); ok {
		gas0, gasSrc = g, "rpc"
	}
	var gasRaw0 interface{}
	if g, ok := gas0.(float64); ok {
		gasRaw0 = int64(math.Round(g * 1e9)) // wei per gas
	}
	status0 := "live"
	if !fromRPC {
		status0 = "unreachable"
	} else if blocksReal && nowTs-tMax > 60 {
		status0 = "stale"
	}
	src := func(real bool) string {
		if real {
			return "rpc"
		}
		return "unavailable"
	}
	var last0 interface{}
	if blocksReal {
		last0 = tMax
	}
	// pending: the parallel node exposes no tx-pool size API (pool is drained every 2s block) -> null.
	snap.summary = append(snap.summary, gin.H{
		"id": 0, "name": "shard0", "type": "live", "status": status0, "height": h0,
		"tps": tps0, "gasPrice": gas0, "gasPriceUnit": "gwei", "gasPriceRaw": gasRaw0, "gasPriceRawUnit": "wei per gas (1e-18 SCDO)", "decimals": shard0Decimals,
		"gasUsed": used0, "gasLimit": 30000000, "pendingTxs": nil,
		"avgBlockTime": avgBt0, "lastBlockTime": last0, "chainId": 5680,
		"desc": shardDesc[0],
		"sources": gin.H{"height": src(fromRPC), "blocks": src(blocksReal), "tps": src(blocksReal),
			"gasUsed": src(blocksReal), "avgBlockTime": src(blocksReal), "gasPrice": gasSrc, "pendingTxs": "unavailable"},
		"heightSource": src(fromRPC),
		"simulated":    false,
	})

	// ---- shards 1-4 (archive, MongoDB; tx-pool size from node RPC) ----
	pendArch := archivePendingCounts()
	for i := 1; i <= 4; i++ {
		item := gin.H{"id": i, "name": fmt.Sprintf("shard%d", i), "type": "archive",
			"height": uint64(0), "status": "syncing", "tps": 0.0, "gasPrice": 0.0,
			"gasPriceUnit": "gwei", "gasUsed": int64(0), "pendingTxs": 0,
			"desc": shardDesc[i], "simulated": false}
		h, err := r.BlockHandler.DBClient.GetBlockHeight(i)
		if err == nil && h > 0 {
			item["height"] = h
			item["status"] = "ok"
			begin := uint64(0)
			if h+1 > shardBlocksPerShard {
				begin = h + 1 - shardBlocksPerShard
			}
			dbBlocks, berr := r.BlockHandler.DBClient.GetBlocksByHeight(i, begin, h+1)
			var list []gin.H
			if berr == nil {
				var txTotal int
				var gpSum, gpCnt, usedSum int64
				var tMin, tMax int64
				for k, b := range dbBlocks {
					var fee, gp int64
					for _, tx := range b.Txs {
						fee += tx.Fee
						gp += tx.GasPrice
					}
					n := len(b.Txs)
					if n > 0 {
						gpSum += gp
						gpCnt += int64(n)
						gp = gp / int64(n)
					}
					txTotal += n
					usedSum += b.UsedGas
					if k == 0 || b.Timestamp > tMax {
						tMax = b.Timestamp
					}
					if k == 0 || b.Timestamp < tMin {
						tMin = b.Timestamp
					}
					list = append(list, gin.H{
						"shard": i, "shardnumber": i, "height": b.Height, "hash": b.HeadHash,
						"miner": b.Creator, "txs": n, "txn": n, "time": b.Timestamp,
						"age": shardAgeDesc(b.Timestamp), "reward": b.Reward, "fee": fee,
						"usedGas": b.UsedGas, "gasprice": gp, "difficulty": b.Difficulty,
						"simulated": false,
					})
				}
				if len(dbBlocks) > 1 && tMax > tMin {
					span := float64(tMax - tMin)
					item["tps"] = shardRound(float64(txTotal)/span, 4)
					item["avgBlockTime"] = shardRound(span/float64(len(dbBlocks)-1), 1)
				}
				if gpCnt > 0 {
					item["gasPrice"] = shardRound(float64(gpSum)/float64(gpCnt)/1e9, 4)
				}
				item["gasPriceSamples"] = gpCnt
				if len(dbBlocks) > 0 {
					item["gasUsed"] = usedSum / int64(len(dbBlocks))
					item["lastBlockTime"] = tMax
					if nowTs-tMax > 3600 { // no new block indexed for >1h: node/syncer down
						item["status"] = "stale"
						item["stale"] = true
					}
				}
				pendSrc := "unavailable"
				if pc, ok := pendArch[i]; ok {
					item["pendingTxs"] = pc
					pendSrc = "rpc"
				} else {
					item["pendingTxs"] = nil
				}
				item["sources"] = gin.H{"height": "mongodb", "blocks": "mongodb", "tps": "mongodb", "gasPrice": "mongodb", "avgBlockTime": "mongodb", "pendingTxs": pendSrc}
			}
			snap.blocks[i] = list
		} else {
			snap.blocks[i] = []gin.H{}
		}
		snap.summary = append(snap.summary, item)
	}
	return snap
}

func (r *Router) shardSnapshot() *shardSnapshot {
	shardSnapMu.Lock()
	defer shardSnapMu.Unlock()
	if shardSnap == nil || time.Since(shardSnap.at) > shardSnapshotTTL {
		shardSnap = r.buildShardSnapshot()
	}
	return shardSnap
}

// NetworkSummaryV2 returns heights, status, TPS and gas for all 5 shards.
func (r *Router) NetworkSummaryV2(c *gin.Context) {
	s := r.shardSnapshot()
	var totalTps float64
	for _, it := range s.summary {
		if v, ok := it["tps"].(float64); ok {
			totalTps += v
		}
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": s.summary,
		"totalTps": shardRound(totalTps, 2), "activeShard": 0, "updatedAt": s.at.Unix()})
}

// BlocksLatestAllShardsV2 returns latest blocks grouped by shard.
// data: newest block per shard (backward compatible); byShard: {"0":[...],...}
// query: size (1-25, default 10), shard (optional, limit byShard to one shard)
func (r *Router) BlocksLatestAllShardsV2(c *gin.Context) {
	s := r.shardSnapshot()
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	if size <= 0 {
		size = 10
	}
	if size > shardBlocksPerShard {
		size = shardBlocksPerShard
	}
	only := -1
	if q := c.Query("shard"); q != "" {
		if v, err := strconv.Atoi(q); err == nil {
			only = v
		}
	}
	latest := []gin.H{}
	by := gin.H{}
	for sh := 0; sh <= 4; sh++ {
		list := s.blocks[sh]
		if len(list) > 0 {
			latest = append(latest, list[0])
		} else {
			latest = append(latest, gin.H{"shard": sh, "height": 0})
		}
		if only >= 0 && only != sh {
			continue
		}
		n := size
		if n > len(list) {
			n = len(list)
		}
		out := list[:n]
		if out == nil {
			out = []gin.H{}
		}
		by[strconv.Itoa(sh)] = out
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": latest, "byShard": by, "updatedAt": s.at.Unix()})
}
