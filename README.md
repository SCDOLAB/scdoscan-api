# scdoscan-api

Backend of the [scdoscan.io](https://scdoscan.io) explorer: a MongoDB-backed indexer and
REST API for the SCDO go-scdo shards 1-4, plus live shard 0 (EVM) data read from the
public shard 0 RPC.

> **Operator / compliance.** scdoscan.io is operated by **9Y9 PTY LTD** (Melbourne, Australia;
> ACN 600 445 118, ABN 19 600 445 118). 9Y9 PTY LTD is registered with [AUSTRAC](https://online.apps.austrac.gov.au/vaspr) as a
> Digital Currency Exchange provider (**DCE100714503-001**, registration valid until
> 14 March 2029) and is a member of the Australian Financial Complaints Authority
> (**AFCA member 124589**). See the [SCDO compliance page](https://scdoscan.io/compliance.html). AUSTRAC registration is not an endorsement or licence of the
> business, its products or SCDO by AUSTRAC. This repository is software only. It is not an
> offer of any financial product or service.

Source: branch `ava-dev` of the scan-api project (last commit `59ee26f`, 2026-09-26 12:54 UTC+8)
plus uncommitted work that runs in production (`api/routers/shards_live.go` and changes to
`router.go`, `accountinfo_handler.go`, `api/handlers/database.go`, `database/client.go`).
Sanitized for publication: see [SANITIZATION.md](SANITIZATION.md).

## Components

| Binary | Entry point | Role |
|---|---|---|
| `scan_server` | `cmd/scan_server` | REST API (gin) under `/api/v1` |
| `scdo_syncer` | `cmd/scdo_syncer` | Pulls blocks, txs, receipts, debts and SRC20 transfers from a go-scdo node (one process per shard) into MongoDB |
| `chart_service` | `cmd/chart_service` | Computes daily chart series (tx history, difficulty, hashrate, block time, top miners, addresses) |
| `node_service` | `cmd/node_service` | Crawls go-scdo P2P nodes for the node list and map |
| `repair_receipt` | `cmd/repair_receipt` | One-off tool that re-fetches missing receipts |

```text
api/        handlers (per resource) and routers (router.go, shards_live.go)
chart/      chart processors
cmd/        entry points and sample JSON configs
common/     shared config types, env overrides (env.go)
database/   MongoDB client (mgo)
deploy/     enhance.js: front-end enhancement script served with the explorer UI
node/ rpc/ server/ syncer/ log/
vendor/     vendored Go dependencies (govendor, GOPATH layout)
```

## Build

GOPATH layout with vendored dependencies (no `go.mod`). Verified to build with Go 1.24.4 in GOPATH mode:

```bash
mkdir -p "$GOPATH/src/github.com/scdoproject"
git clone https://github.com/SCDOLAB/scdoscan-api "$GOPATH/src/github.com/scdoproject/scan-api"
cd "$GOPATH/src/github.com/scdoproject/scan-api"
GO111MODULE=off make          # outputs to ./build/{server,syncer,chart,node}/
```

Requirements: MongoDB (production runs 3.6.18; the unmaintained `mgo` driver is not
expected to work with MongoDB 6.0+), and a reachable go-scdo node RPC for each shard you sync.

## Run

```bash
export SCAN_MONGO_URLS=127.0.0.1:27017 SCAN_MONGO_DB=scdo
export SCAN_MONGO_USER=scan SCAN_MONGO_PASSWORD='<from your secret store>'   # optional
./build/syncer/scdo_syncer -c cmd/scdo_syncer/cmd/server1.json   # shard 1 (repeat per shard)
./build/server/scan_server -c cmd/scan_server/cmd/server1.json   # API on :3003
./build/chart/chart_service -c cmd/chart_service/cmd/server1.json
./build/node/node_service  -c cmd/node_service/cmd/server1.json
```

### Configuration

The JSON files in `cmd/*/cmd/` hold non-secret settings (listen address, log level, node
RPC URL, shard number). The environment variables below override the `DataBase` section
(see `common/env.go`), and two more set the node RPCs used by the proxy endpoints:

| Variable | Overrides | Example |
|---|---|---|
| `SCAN_MONGO_URLS` | `DataBase.DataBaseConnUrls` (comma-separated) | `127.0.0.1:27017` |
| `SCAN_MONGO_MODE` | `DataBase.DataBaseMode` | `single` or `replset` |
| `SCAN_MONGO_REPLSET` | `DataBase.DataBaseReplsetName` | `rs0` |
| `SCAN_MONGO_DB` | `DataBase.DataBaseName` | `scdo` |
| `SCAN_MONGO_USER` | `DataBase.User` | |
| `SCAN_MONGO_PASSWORD` | `DataBase.Pwd` | |
| `SCAN_MONGO_AUTH` | `DataBase.UseAuthentication` (`true`/`false`; defaults to `true` when user and password are both set) | |
| `SCAN_SHARD1_RPC` | node used by `/eth_call` (`scdo_call`) | `http://127.0.0.1:8037` |
| `SCAN_SHARD4_RPC` | node used by `/debug_trace` | `http://127.0.0.1:8036` |

Authentication is only applied in `single` mode (replset mode connects without
credentials, as in the original code).

## REST API

Base URL in production: `https://api.scdoscan.io/api/v1` (the explorer also proxies some
routes under `https://scdoscan.io/api/v1`, e.g. `/network/summary`).
Common query parameters: `p` = page (default 1), `ps` = page size (default 25),
`s` = shard number (1-4). Responses are `{"code":0,"data":...}`, non-zero `code` on error.
Detailed request/response examples (Chinese): [api.md](api.md).

### Blocks and transactions

| Method | Path | Parameters | Response |
|---|---|---|---|
| GET | `/blocks` | `p`, `ps`, `s` | Block list, newest first |
| GET | `/block` | `height` + `s`, or `hash` | Block detail |
| GET | `/blockcount` | | Latest block height per shard |
| GET | `/blockprotime` | | Latest block time |
| GET | `/blockTxsTps` | | Tx count / TPS of recent blocks |
| GET | `/blockdebt` | `block`, `p`, `ps`, `s` | Cross-shard debts in a block |
| GET | `/txs` | `p`, `ps`, `s`, optional `block` or `address` | Transaction list |
| GET | `/tx` | `txhash` | Transaction detail |
| GET | `/txcount` | | Total transactions |
| GET | `/Txstat` | | Transactions per day |
| GET | `/pendingtxs` | `p`, `ps`, `s` | Pending transactions |
| GET | `/debts` | `p`, `ps`, `s` | Cross-shard debt list |
| GET | `/debt` | `debtHash` | Debt detail |
| GET | `/Avegas` | | Average gas fee per gas unit |
| GET | `/search` | `content` | Block, tx, account or contract matching `content` |

### Accounts, contracts, tokens

| Method | Path | Parameters | Response |
|---|---|---|---|
| GET | `/accounts` | `p`, `ps`, `s` | Accounts ranked by balance |
| GET | `/Homeaccounts` | | Top accounts for the home page |
| GET | `/account` | `address` | Account detail (balance, txs, SRC20 holdings) |
| GET | `/accountcount` | | Number of accounts |
| GET | `/balanceHistory` | `address` | Balance history |
| GET | `/miners` | | Miner accounts |
| GET | `/accountSrc20` | `address` | SRC20 token holdings |
| GET | `/accountSrc20Txs` | `address` | SRC20 transfers |
| GET | `/src20holders` | `address` (token contract) | Holders of a token |
| GET | `/contracts` | `p`, `ps`, `s` | Contract list |
| GET | `/contract` | `address` | Contract detail |
| GET | `/contractcount` | | Number of contracts |
| POST | `/verifyContract` | form: `mode`, `address`, `compilerVersion`, `abiInput`, `logo`, `standardJSON` or (`contractName`, source file, `optimizer`, `runs`) | Verifies source with a local `solc-bin/` compiler |

### Nodes and charts

| Method | Path | Parameters | Response |
|---|---|---|---|
| GET | `/nodes` | `p`, `ps`, `s` | P2P node list |
| GET | `/node` | `id` | Node detail |
| GET | `/nodemap` | | Node locations |
| GET | `/chart/tx` | `s` | Daily tx history |
| GET | `/chart/difficulty` | `s` | Daily difficulty |
| GET | `/chart/address` | `s` | Daily new addresses |
| GET | `/chart/blocks` | `s` | Daily blocks and rewards |
| GET | `/chart/hashrate` | `s` | Daily hashrate |
| GET | `/chart/blocktime` | `s` | Daily average block time |
| GET | `/chart/miner` | `s` | Top miners |
| GET | `/chart/node` | | Node count history |

The node routes are registered in `router.go` as `./nodes`, `./node`, `./nodemap`. gin
normalizes them to `/api/v1/nodes` and so on.

### Multi-shard (shard 0 + shards 1-4), from `shards_live.go`

| Method | Path | Response |
|---|---|---|
| GET | `/network/summary` | Per-shard height, status, TPS, gas fee per gas unit, average block time for shards 0-4. Shard 0 is read live from `https://scdoscan.io/rpc/0` (chainId 5680, 18 decimals); shards 1-4 come from MongoDB. Cached for 3 s |
| GET | `/blocks/latest/allshards` | Latest blocks grouped by shard |

### Proxies and helpers

| Method | Path | Notes |
|---|---|---|
| GET | `/eth_call` | `to`, `data`. Forwards `scdo_call` to `SCAN_SHARD1_RPC` at a fixed height (9240000) |
| GET | `/debug_trace` | `height`, `hash`. Forwards `debug_traceTransaction` to `SCAN_SHARD4_RPC` |
| GET | `/status` | API version and a list of endpoints |
| GET | `/apikey` | **Stub.** Gives a random `scdo_...` key. Keys are not stored or enforced, and no rate limit is applied |
| POST | `/graphql` | **Stub.** Keyword-matches the query. Only `blockcount` gives real data; `token` gives a fixed sample (`TEST1`) |

## Known issues (not fixed in this release)

- `/debug_trace` puts the `height` query value into the JSON-RPC body without escaping, so it
  should be validated before this endpoint is exposed.
- `/verifyContract` builds the `solc` path from the user-supplied `compilerVersion` without
  validation. Restrict it to a known version list before exposing this endpoint.
- Some `cmd/*/cmd/config_test.go` tests fail on the original code as well: the fixtures'
  RPC URLs no longer match the test expectations.

## License

GNU Lesser General Public License v3.0: see [LICENSE](LICENSE) (unchanged from the original project).
