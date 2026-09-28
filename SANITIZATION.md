# What was changed for the public release

Source: the working copy on the production scdoscan.io API server, branch `ava-dev`
(HEAD `59ee26fa761655082c7f49222496b57b06e7f2c8`, 2026-09-26 12:54 UTC+8), copied
on 2026-09-28 together with its uncommitted changes. The server itself was not modified.

## Not included

| Excluded | Why |
|---|---|
| `.git/` | The history and remote configuration are not published. Start a fresh history |
| `build/` (916 MB) | Compiled binaries |
| `chart_service/`, `node_service/`, `scan-api/`, `scdo-syncer/` | Runtime log directories only (no source) |
| `*.log`, `*.logs`, `*.logs.*` | Logs (contain client IPs and request data) |
| `api/routers/router.go.bak2` | Local backup |

Included (uncommitted on the server): `api/routers/shards_live.go` (new) and the working-tree
versions of `api/handlers/accountinfo_handler.go`, `api/handlers/database.go`,
`api/routers/router.go`, `database/client.go`. `vendor/` is kept because the project builds in
GOPATH mode from it. Its 389 third-party `*_test.go` files and 4 `testdata/` directories
were removed (21 MB -> 15 MB). They held upstream test vectors, including go-ethereum
keystore test keys and a PEM private key in an InfluxDB client test, which GitHub push
protection may flag. The build does not need them.

## Credentials -> environment variables

1. **New file `common/env.go`.** `(*DataBaseConfig).ApplyEnv()` reads `SCAN_MONGO_URLS`,
   `SCAN_MONGO_MODE`, `SCAN_MONGO_REPLSET`, `SCAN_MONGO_DB`, `SCAN_MONGO_USER`,
   `SCAN_MONGO_PASSWORD`, `SCAN_MONGO_AUTH`. It also adds `EnvOr(key, def)`.
2. **`database/client.go`.** One line added at the top of `NewDBClient`:
   `cfg.ApplyEnv()`. Every binary (scan_server, scdo_syncer, chart_service, node_service,
   repair_receipt) and `server/server.go` gets its DB client through this function, so the
   environment now overrides every JSON config.
3. **Config files `cmd/*/cmd/server*.json`** (12 files: scan_server 1-2, node_service 1-2,
   chart_service 1-2, scdo_syncer 1, 1_node2, 1_rep, 2, 3, 4):
   `"User": "scan"` -> `"User": ""` and `"Pwd": "123456"` -> `"Pwd": ""`.
   (`UseAuthentication` was already `false` in all of them.)
4. **Test fixtures `cmd/*/cmd/testfile/*_test.json`** (8 files): `"Pwd": "123456"` ->
   `"Pwd": "changeme"`. **Tests `cmd/*/cmd/config_test.go`** (5 files) assert `"changeme"` to match.
5. **`api/routers/router.go`.** The node RPC URLs hard-coded in `EthCallProxy`
   (`http://74.208.207.184:8037`) and `DebugTraceProxy` (`http://74.208.207.184:8036`) are
   now the package variables `shard1NodeRPC` / `shard4NodeRPC`. They read `SCAN_SHARD1_RPC` /
   `SCAN_SHARD4_RPC` and default to the same URLs, so behaviour is unchanged. Adds the import
   `github.com/scdoproject/scan-api/common`.

No other source changes. No real MongoDB password appeared in the source tree (the
production credentials live outside the repository). Only the upstream sample
`scan` / `123456` values were found, and they are handled above.

## Left as is (public, not secret)

- 64-hex values in `api.md`, `rpc/*_test.go`, `rpc/scdo_rpc.go` comments,
  `api/handlers/handler_test.go`, `syncer/account_process_test.go`, and the skipped
  tx hash in `database/client.go`: these are public block/tx hashes, node IDs and addresses.
- `common/config.go` `MasterAccount`: public shard addresses.
- IPs in `api.md` (2019 example responses), `cmd/node_service/cmd/config_test.go`
  (`106.75.80.93`) and `cmd/scdo_syncer/cmd/server1.json` `RpcURL` (`198.251.69.156:8027`).
  These are public node addresses, not credentials.

## Verification

- `GO111MODULE=off go build` of all five commands succeeds with Go 1.24.4 (GOPATH layout).
- `go test` of `cmd/*/cmd`: scan_server passes. The failures in scdo_syncer, chart_service,
  node_service and repair_receipt are identical on the unmodified original (fixture
  RPC URLs vs. expectations).
