# 0.7.0 (2018-04-30)

## Features:
- Support global data monitoring (gold feeds...)
- Support stable exchange (the virtual exchange to handle tokens that are not listed on big cexs)

## Bug fixes:
- TODO

## Compatability:
- This version only works with KyberNetwork smart contracts version 0.3.0 or later

# 0.6.1 (2018-04-25)
## Features:
- Getting aws info from config file

## Bug fixes:
- Get correct filename from AWS's Item key for intergrity check

## Compatability:
- This version only works with KyberNetwork smart contracts version 0.3.0 or later

# 0.6.0 (2018-04-25)

## Features:
- support Huobi as backend centralized exchange
- API for analytic to submit notifications on exchange status
- API to enable/disable a particular exchange
- More graph data such as (timezone, heatmap, trade summary, trade summary for a wallet)
- Support more tokens (including DAI)
- Support multiple token mode (internal use, external use, unlisted tokens)
- API to support status server, dashboard notifications
- Analytic dev mode to support dev environment for analytic team
- API for analytic to submit pricing data

## Bug fixes:
- fix bunch of errors in order to ensure stat server will not miss any tradelogs (including making log aggregation and last log id persistence atomic)
- return log in correct order to ensure consistent stat results
- enable authentication in stat server (it was ignored before)

## Improvements:
- Fallback to other nodes when primary node is down with contract calls
- Massive refactor the codebase which is related to blockchain interaction
- Add more tests to make the whole component more stable
- Improve a lot log aggregration performance by process in group and reduce number of database transactions
- Using ethereum.Address type in database interfaces to ensure it's consistency
- Update bittrex constants and settings
- Persist error messages for all kind of activities (deposit/withdraw/trade)

## Compatability:
- This version only works with KyberNetwork smart contracts version 0.3.0 or later

# 0.5.0 (2018-03-08)

## Features:
- Add api to halt setrate
- Reorganize commandline to make core runable in different modes and configs
- Rotate logs for better log management
- Add API to get all core relevant addresses
- Add api to check core version
- Add a tool to monitor base/compact to detect bugs
- Add binance trade history API
## Bug fixes:
- Fix minors with deposit signer
- Fix order of pending activities in its API
- Add API timerange limit to all relevant apis to mitigate dos attack from key keepers
- Add sanity check to validate response from node
- Remove eth-eth pair in requesting to exchange
## Improvements:
- Update binance limits
- Reduce bianance api rate
- Organize configuration better to list/delist token more easily
- Wait sometime before fetching new rate to hopefully mitigrate reorg
- Added sanity check on deposit/trade/withdraw
- Improved gas limit estimation for deposit and setrate
- Removed duplicated records in get rate API
- Query rate at a specific block instead of relying on latest block

# 0.4.1 (2018-02-19)
## Features:
- Listed more 4 tokens (eng, salt, appc, rdn)
- Added more tools for monitoring and testing such as deposit/withdraw trigger, rate validator

## Bug fixes:
- Fixed submit empty setrate for the first one
- Fixed bug in rare case that panics when core couldn't get mined nonce
- Fixed incompatibility between geth and parity in tx receipt data
- Enable microsecond info in log

## Improvements:
- Separated cex token pairs to config
- Separated cex fee to config
- Added sanity checks on setrates, deposit, withdraw and trade
- Added env tag to sentry

## Compatability:
- This version only works with KyberNetwork smart contracts version 0.3.0 or later

# 0.4.0 (2018-02-08)

## Features:
- Support rebalance toggle, dynamic target qty with set/confirm key model
- Support multiple keys for different roles

## Bug fixes:
- Fixed minor bugs
- Detect throwing txs

## Improvements:
- Done sanity check in with setrate api
- Rebroadcasting tx to multiple node to improve tx propagation
- Replace staled/long mining set rate txs
- Made improvements to the code base
- Applied timeout to communication to nodes to ensure analytic doesn't have to wait for too long to set another rate

## Compatability:
- This version only works with KyberNetwork smart contracts version 0.3.0 or later

# 0.3.0 (2018-01-31)

## Features:
- Introduce various key permissions
- New API for getting KN rate historical data
- New API for getting trade history on cexs

## Bugfixes:
- Handle lost transactions

## Improvements:
- Using multiple nodes to broadcast tx
- Avoid storing redundant rate data
- More code refactoring

## Compatability:
- This version only works with KyberNetwork smart contracts version 0.3.0


