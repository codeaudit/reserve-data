package stat

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/KyberNetwork/reserve-data/common"
	"github.com/KyberNetwork/reserve-data/stat/util"
	ethereum "github.com/ethereum/go-ethereum/common"
)

const (
	REORG_BLOCK_SAFE       uint64 = 7
	TIMEZONE_BUCKET_PREFIX string = "utc"
	START_TIMEZONE         int64  = -11
	END_TIMEZONE           int64  = 14
)

type Fetcher struct {
	statStorage            StatStorage
	userStorage            UserStorage
	logStorage             LogStorage
	rateStorage            RateStorage
	blockchain             Blockchain
	runner                 FetcherRunner
	currentBlock           uint64
	currentBlockUpdateTime uint64
	deployBlock            uint64
	reserveAddress         ethereum.Address
	thirdPartyReserves     []ethereum.Address
}

func NewFetcher(
	statStorage StatStorage,
	logStorage LogStorage,
	rateStorage RateStorage,
	userStorage UserStorage,
	runner FetcherRunner,
	deployBlock uint64,
	reserve ethereum.Address,
	thirdPartyReserves []ethereum.Address) *Fetcher {
	return &Fetcher{
		statStorage:        statStorage,
		logStorage:         logStorage,
		rateStorage:        rateStorage,
		userStorage:        userStorage,
		blockchain:         nil,
		runner:             runner,
		deployBlock:        deployBlock,
		reserveAddress:     reserve,
		thirdPartyReserves: thirdPartyReserves,
	}
}

func (self *Fetcher) Stop() error {
	return self.runner.Stop()
}

func (self *Fetcher) SetBlockchain(blockchain Blockchain) {
	self.blockchain = blockchain
	self.FetchCurrentBlock()
}

func (self *Fetcher) Run() error {
	log.Printf("Fetcher runner is starting...")
	self.runner.Start()
	go self.RunBlockFetcher()
	go self.RunLogFetcher()
	go self.RunReserveRatesFetcher()
	go self.RunTradeLogProcessor()
	go self.RunCatLogProcessor()
	log.Printf("Fetcher runner is running...")
	return nil
}

func (self *Fetcher) RunCatLogProcessor() {
	for {
		t := <-self.runner.GetCatLogProcessorTicker()
		// get trade log from db
		fromTime, err := self.userStorage.GetLastProcessedCatLogTimepoint()
		if err != nil {
			log.Printf("get last processor state from db failed: %v", err)
			continue
		}
		fromTime += 1
		if fromTime == 1 {
			// there is no cat log being processed before
			// load the first log we have and set the fromTime to it's timestamp
			l, err := self.logStorage.GetFirstCatLog()
			if err != nil {
				log.Printf("can't get first cat log: err(%s)", err)
				continue
			} else {
				fromTime = l.Timestamp - 1
			}
		}
		toTime := common.TimeToTimepoint(t) * 1000000
		maxRange := self.logStorage.MaxRange()
		if toTime-fromTime > maxRange {
			toTime = fromTime + maxRange
		}
		catLogs, err := self.logStorage.GetCatLogs(fromTime, toTime)
		if err != nil {
			log.Printf("get cat log from db failed: %v", err)
			continue
		}
		log.Printf("PROCESS %d cat logs from %d to %d", len(catLogs), fromTime, toTime)
		if len(catLogs) > 0 {
			var last uint64
			for _, l := range catLogs {
				err := self.userStorage.UpdateAddressCategory(
					strings.ToLower(l.Address.Hex()),
					l.Category,
				)
				if err != nil {
					log.Printf("updating address and category failed: err(%s)", err)
				} else {
					if l.Timestamp > last {
						last = l.Timestamp
					}
				}
			}
			self.userStorage.SetLastProcessedCatLogTimepoint(last)
		} else {
			l, err := self.logStorage.GetLastCatLog()
			if err != nil {
				log.Printf("LogFetcher - can't get last cat log: err(%s)", err)
				continue
			} else {
				// log.Printf("LogFetcher - got last cat log: %+v", l)
				if toTime < l.Timestamp {
					// if we are querying on past logs, store toTime as the last
					// processed trade log timepoint
					self.userStorage.SetLastProcessedCatLogTimepoint(toTime)
				}
			}
		}

		log.Println("processed cat logs")
	}
}

func (self *Fetcher) RunTradeLogProcessor() {
	for {
		log.Printf("TradeLogProcessor - waiting for signal from trade log processor channel")
		t := <-self.runner.GetTradeLogProcessorTicker()
		// get trade log from db
		fromTime, err := self.statStorage.GetLastProcessedTradeLogTimepoint()
		if err != nil {
			log.Printf("get trade log processor state from db failed: %v", err)
			continue
		}
		fromTime += 1
		if fromTime == 1 {
			// there is no trade log being processed before
			// load the first log we have and set the fromTime to it's timestamp
			l, err := self.logStorage.GetFirstTradeLog()
			if err != nil {
				log.Printf("can't get first trade log: err(%s)", err)
				continue
			} else {
				log.Printf("got first trade: %+v", l)
				fromTime = l.Timestamp - 1
			}
		}
		toTime := common.TimeToTimepoint(t) * 1000000
		maxRange := self.logStorage.MaxRange()
		if toTime-fromTime > maxRange {
			toTime = fromTime + maxRange
		}
		tradeLogs, err := self.logStorage.GetTradeLogs(fromTime, toTime)
		if err != nil {
			log.Printf("get trade log from db failed: %v", err)
			continue
		}
		log.Printf("AGGREGATE %d trades from %d to %d", len(tradeLogs), fromTime, toTime)
		if len(tradeLogs) > 0 {
			var last uint64
			userTradeList := map[string]uint64{} // map of user address and fist trade timestamp
			for _, trade := range tradeLogs {
				userAddr := common.AddrToString(trade.UserAddress)
				key := fmt.Sprintf("%s_%d", userAddr, trade.Timestamp)
				userTradeList[key] = trade.Timestamp
			}
			self.statStorage.SetFirstTradeEver(userTradeList)
			self.statStorage.SetFirstTradeInDay(userTradeList)

			countryStats := map[string]common.CountryStatsTimeZone{}
			walletStats := map[string]common.WalletStatsTimeZone{}
			tradeSummary := common.TradeSummaryTimeZone{}
			for _, trade := range tradeLogs {
				if err := self.aggregateTradeLog(trade, countryStats, walletStats, tradeSummary); err == nil {
					if trade.Timestamp > last {
						last = trade.Timestamp
					}
				}
			}
			self.statStorage.SetCountryStat(countryStats)
			self.statStorage.SetWalletStat(walletStats)
			self.statStorage.SetTradeSummary(tradeSummary)
			self.statStorage.SetLastProcessedTradeLogTimepoint(last)
			log.Printf("AGGREGATE finished all logs in the batch")
		} else {
			l, err := self.logStorage.GetLastTradeLog()
			if err != nil {
				log.Printf("can't get last trade log: err(%s)", err)
				continue
			} else {
				if toTime < l.Timestamp {
					// if we are querying on past logs, store toTime as the last
					// processed trade log timepoint
					self.statStorage.SetLastProcessedTradeLogTimepoint(toTime)
				}
			}
		}
		log.Println("aggregated trade stats")
	}
}

func (self *Fetcher) RunReserveRatesFetcher() {
	for {
		log.Printf("waiting for signal from reserve rate channel")
		t := <-self.runner.GetReserveRatesTicker()
		log.Printf("got signal in reserve rate channel with timstamp %d", common.GetTimepoint())
		timepoint := common.TimeToTimepoint(t)
		self.FetchReserveRates(timepoint)
		log.Printf("fetched reserve rate from blockchain")
	}
}

func (self *Fetcher) GetReserveRates(
	currentBlock uint64, reserveAddr ethereum.Address,
	tokens []common.Token, data *sync.Map, wg *sync.WaitGroup) {
	defer wg.Done()
	rates, err := self.blockchain.GetReserveRates(currentBlock-1, currentBlock, reserveAddr, tokens)
	if err != nil {
		log.Println(err.Error())
	}
	data.Store(string(reserveAddr.Hex()), rates)
}

func (self *Fetcher) FetchReserveRates(timepoint uint64) {
	log.Printf("Fetching reserve and sanity rate from blockchain")
	tokens := []common.Token{}
	for _, token := range common.SupportedTokens {
		if token.ID != "ETH" {
			tokens = append(tokens, token)
		}
	}
	supportedReserves := append(self.thirdPartyReserves, self.reserveAddress)
	data := sync.Map{}
	wg := sync.WaitGroup{}
	// get current block to use to fetch all reserve rates.
	// dont use self.currentBlock directly with self.GetReserveRates
	// because otherwise, rates from different reserves will not
	// be synced with block no
	block := self.currentBlock
	for _, reserveAddr := range supportedReserves {
		wg.Add(1)
		go self.GetReserveRates(block, reserveAddr, tokens, &data, &wg)
	}
	wg.Wait()
	data.Range(func(key, value interface{}) bool {
		reserveAddr := key.(string)
		rates := value.(common.ReserveRates)
		log.Printf("Storing reserve rates to db...")
		self.rateStorage.StoreReserveRates(reserveAddr, rates, common.GetTimepoint())
		return true
	})
}

func (self *Fetcher) RunLogFetcher() {
	for {
		log.Printf("LogFetcher - waiting for signal from log channel")
		t := <-self.runner.GetLogTicker()
		timepoint := common.TimeToTimepoint(t)
		log.Printf("LogFetcher - got signal in log channel with timestamp %d", timepoint)
		lastBlock, err := self.logStorage.LastBlock()
		if lastBlock == 0 {
			lastBlock = self.deployBlock
		}
		if err == nil {
			toBlock := lastBlock + 1 + 1440 // 1440 is considered as 6 hours
			if toBlock > self.currentBlock-REORG_BLOCK_SAFE {
				toBlock = self.currentBlock - REORG_BLOCK_SAFE
			}
			if lastBlock+1 > toBlock {
				continue
			}
			nextBlock, err := self.FetchLogs(lastBlock+1, toBlock, timepoint)
			if err != nil {
				// in case there is error, we roll back and try it again.
				// dont have to do anything here. just continute with the loop.
				log.Printf("LogFetcher - continue with the loop to try it again")
			} else {
				if nextBlock == lastBlock && toBlock != 0 {
					// in case that we are querying old blocks (6 hours in the past)
					// and got no logs. we will still continue with next block
					// It is not the case if toBlock == 0, means we are querying
					// best window, we should keep querying it in order not to
					// miss any logs due to node inconsistency
					nextBlock = toBlock
				}
				log.Printf("LogFetcher - update log block: %d", nextBlock)
				self.logStorage.UpdateLogBlock(nextBlock, timepoint)
				log.Printf("LogFetcher - nextBlock: %d", nextBlock)
			}
		} else {
			log.Printf("LogFetcher - failed to get last fetched log block, err: %+v", err)
		}
	}
}

func (self *Fetcher) RunBlockFetcher() {
	for {
		log.Printf("waiting for signal from block channel")
		t := <-self.runner.GetBlockTicker()
		timepoint := common.TimeToTimepoint(t)
		log.Printf("got signal in block channel with timestamp %d", timepoint)
		self.FetchCurrentBlock()
		log.Printf("fetched block from blockchain")
	}
}

func (self *Fetcher) GetTradeGeo(txHash string) (string, string, error) {
	url := fmt.Sprintf("https://broadcast.kyber.network/get-tx-info/%s", txHash)

	resp, err := http.Get(url)
	if err != nil {
		return "", "", err
	}
	response := common.TradeLogGeoInfoResp{}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return "", "", err
	}
	if response.Success {
		if response.Data.Country != "" {
			return response.Data.IP, response.Data.Country, err
		}
		country, err := util.IPToCountry(response.Data.IP)
		if err != nil {
			return "", "", err
		}
		return response.Data.IP, country, err
	}
	return "", "unknown", err
}

// return block number that we just fetched the logs
func (self *Fetcher) FetchLogs(fromBlock uint64, toBlock uint64, timepoint uint64) (uint64, error) {
	logs, err := self.blockchain.GetLogs(fromBlock, toBlock)
	if err != nil {
		log.Printf("LogFetcher - fetching logs data from block %d failed, error: %v", fromBlock, err)
		if fromBlock == 0 {
			return 0, err
		} else {
			return fromBlock - 1, err
		}
	} else {
		if len(logs) > 0 {
			for _, il := range logs {
				if il.Type() == "TradeLog" {
					l := il.(common.TradeLog)
					txHash := il.TxHash()
					ip, country, err := self.GetTradeGeo(txHash.Hex())
					l.IP = ip
					l.Country = country
					err = self.statStorage.SetCountry(country)
					if err != nil {
						log.Printf("Cannot store country: %s", err.Error())
					}

					err = self.logStorage.StoreTradeLog(l, timepoint)
					if err != nil {
						log.Printf("LogFetcher - storing trade log failed, ignore that log and proceed with remaining logs, err: %+v", err)
					}
				} else if il.Type() == "SetCatLog" {
					l := il.(common.SetCatLog)
					err = self.logStorage.StoreCatLog(l)
					if err != nil {
						log.Printf("LogFetcher - storing cat log failed, ignore that log and proceed with remaining logs, err: %+v", err)
					}
				}
			}
			var max uint64 = 0
			for _, l := range logs {
				if l.BlockNo() > max {
					max = l.BlockNo()
				}
			}
			return max, nil
		} else {
			return fromBlock - 1, nil
		}
	}
}

func (self *Fetcher) updateTimeZoneBuckets(timestamp uint64, updates common.TradeStats) (err error) {
	//update to timezone buckets
	for i := START_TIMEZONE; i <= END_TIMEZONE; i++ {
		//log.Printf("AGGREGATE updaing timezone buckets: %d", i)
		freq := fmt.Sprintf("%s%d", TIMEZONE_BUCKET_PREFIX, i)
		err = self.statStorage.SetTradeStats(freq, timestamp, updates)
		if err != nil {
			return err
		}
	}
	return nil
}

func checkWalletAddress(wallet string) bool {
	walletAddr := ethereum.HexToAddress(wallet)
	cap := big.NewInt(0)
	cap.Exp(big.NewInt(2), big.NewInt(128), big.NewInt(0))
	if walletAddr.Big().Cmp(cap) < 0 {
		return false
	}
	return true
}

func getTimestampFromTimeZone(timepoint uint64, timezone int64) uint64 {
	ui64offset := uint64(int64(time.Hour) * timezone)
	ui64Day := uint64(time.Hour * 24)
	var result uint64
	if timezone > 0 {
		result = (timepoint+ui64offset)/ui64Day*ui64Day + ui64offset
	} else {
		timezone = 0 - timezone
		result = (timepoint-ui64offset)/ui64Day*ui64Day - ui64offset
	}
	return result
}

func (self *Fetcher) aggregateTradeLog(trade common.TradeLog,
	countryStats map[string]common.CountryStatsTimeZone,
	walletStats map[string]common.WalletStatsTimeZone,
	tradeSummary common.TradeSummaryTimeZone) (err error) {

	srcAddr := common.AddrToString(trade.SrcAddress)
	dstAddr := common.AddrToString(trade.DestAddress)
	reserveAddr := common.AddrToString(trade.ReserveAddress)
	walletAddr := common.AddrToString(trade.WalletAddress)
	userAddr := common.AddrToString(trade.UserAddress)

	if checkWalletAddress(walletAddr) {
		self.statStorage.SetWalletAddress(walletAddr)
	}

	var srcAmount, destAmount, ethAmount, burnFee, walletFee float64
	var tokenAddr string
	for _, token := range common.SupportedTokens {
		if strings.ToLower(token.Address) == srcAddr {
			srcAmount = common.BigToFloat(trade.SrcAmount, token.Decimal)
			if token.IsETH() {
				ethAmount = srcAmount
			} else {
				tokenAddr = strings.ToLower(token.Address)
			}
		}

		if strings.ToLower(token.Address) == dstAddr {
			destAmount = common.BigToFloat(trade.DestAmount, token.Decimal)
			if token.IsETH() {
				ethAmount = destAmount
			} else {
				tokenAddr = strings.ToLower(token.Address)
			}
		}
	}

	eth := common.SupportedTokens["ETH"]
	if trade.BurnFee != nil {
		burnFee = common.BigToFloat(trade.BurnFee, eth.Decimal)
	}
	if trade.WalletFee != nil {
		walletFee = common.BigToFloat(trade.WalletFee, eth.Decimal)
	}

	updates := common.TradeStats{
		fmt.Sprintf("assets_volume_%s", srcAddr):                 srcAmount,
		fmt.Sprintf("assets_volume_%s", dstAddr):                 destAmount,
		fmt.Sprintf("assets_eth_amount_%s", tokenAddr):           ethAmount,
		fmt.Sprintf("assets_usd_amount_%s", srcAddr):             trade.FiatAmount,
		fmt.Sprintf("assets_usd_amount_%s", dstAddr):             trade.FiatAmount,
		fmt.Sprintf("burn_fee_%s", reserveAddr):                  burnFee,
		fmt.Sprintf("wallet_fee_%s_%s", reserveAddr, walletAddr): walletFee,
		fmt.Sprintf("user_volume_%s", userAddr):                  trade.FiatAmount,
	}

	log.Printf("AGGREGATE step 4")
	for _, freq := range []string{"M", "H", "D"} {
		err = self.statStorage.SetTradeStats(freq, trade.Timestamp, updates)
		if err != nil {
			return
		}
	}
	log.Printf("AGGREGATE step 5")
	if err = self.updateTimeZoneBuckets(trade.Timestamp, updates); err != nil {
		return
	}

	// stats on user
	userAddr = strings.ToLower(trade.UserAddress.String())
	email, regTime, err := self.userStorage.GetUserOfAddress(userAddr)
	if err != nil {
		return
	}

	var kycEd bool
	if email != "" && email != userAddr && trade.Timestamp > regTime {
		kycEd = true
	}
	self.aggregateTradeSummary(trade, ethAmount, burnFee, tradeSummary, kycEd)
	self.aggregateWalletStat(trade, ethAmount, burnFee, walletStats, kycEd)
	self.aggregateCountryStat(trade, ethAmount, burnFee, countryStats, kycEd)

	return
}

func (self *Fetcher) aggregateTradeSummary(trade common.TradeLog, ethAmount, burnFee float64,
	tradeSummary common.TradeSummaryTimeZone, kycEd bool) {
	for i := START_TIMEZONE; i <= END_TIMEZONE; i++ {
		timestamp := getTimestampFromTimeZone(trade.Timestamp, i)
		userAddr := common.AddrToString(trade.UserAddress)

		dataTimeZone, exist := tradeSummary[i]
		if !exist {
			dataTimeZone = map[uint64]common.TradeSummary{}
		}
		data, exist := dataTimeZone[timestamp]
		if !exist {
			data = common.TradeSummary{}
		}
		timeFirstTrade := self.statStorage.GetFirstTradeEver(userAddr)
		if timeFirstTrade == trade.Timestamp {
			data.NewUniqueAddresses++
			data.UniqueAddr++
			if kycEd {
				data.KYCEd++
			}
		} else {
			firstTradeInday := self.statStorage.GetFirstTradeInDay(userAddr, trade.Timestamp, i)
			if firstTradeInday == trade.Timestamp {
				data.UniqueAddr++
				if kycEd {
					data.KYCEd++
				}
			}
		}
		data.ETHVolume += ethAmount
		data.BurnFee += burnFee
		data.TradeCount++
		data.USDVolume += trade.FiatAmount
		dataTimeZone[timestamp] = data
		tradeSummary[i] = dataTimeZone
	}
	return
}

func (self *Fetcher) aggregateWalletStat(trade common.TradeLog, ethAmount, burnFee float64,
	walletStats map[string]common.WalletStatsTimeZone,
	kycEd bool) {
	for i := START_TIMEZONE; i <= END_TIMEZONE; i++ {
		timestamp := getTimestampFromTimeZone(trade.Timestamp, i)
		walletAddr := common.AddrToString(trade.WalletAddress)
		userAddr := common.AddrToString(trade.UserAddress)

		currentWalletData, exist := walletStats[walletAddr]
		if !exist {
			currentWalletData = common.WalletStatsTimeZone{}
		}
		dataTimeZone, exist := currentWalletData[i]
		if !exist {
			dataTimeZone = map[uint64]common.WalletStats{}
		}
		data, exist := dataTimeZone[timestamp]
		if !exist {
			data = common.WalletStats{}
		}
		timeFirstTrade := self.statStorage.GetFirstTradeEver(userAddr)
		if timeFirstTrade == trade.Timestamp {
			data.NewUniqueAddresses++
			data.UniqueAddr++
			if kycEd {
				data.KYCEd++
			}
		} else {
			firstTradeInday := self.statStorage.GetFirstTradeInDay(userAddr, trade.Timestamp, i)
			if firstTradeInday == trade.Timestamp {
				data.UniqueAddr++
				if kycEd {
					data.KYCEd++
				}
			}
		}
		data.ETHVolume += ethAmount
		data.BurnFee += burnFee
		data.TradeCount++
		data.USDVolume += trade.FiatAmount
		dataTimeZone[timestamp] = data
		currentWalletData[i] = dataTimeZone
		walletStats[walletAddr] = currentWalletData
	}
	return
}

func (self *Fetcher) aggregateCountryStat(trade common.TradeLog, ethAmount, burnFee float64,
	countryStats map[string]common.CountryStatsTimeZone,
	kycEd bool) {
	userAddr := common.AddrToString(trade.UserAddress)

	for i := START_TIMEZONE; i <= END_TIMEZONE; i++ {
		timestamp := getTimestampFromTimeZone(trade.Timestamp, i)
		currentCountryData, exist := countryStats[trade.Country]
		if !exist {
			currentCountryData = common.CountryStatsTimeZone{}
		}
		dataTimeZone, exist := currentCountryData[i]
		if !exist {
			dataTimeZone = map[uint64]common.CountryStats{}
		}
		data, exist := dataTimeZone[timestamp]
		if !exist {
			data = common.CountryStats{}
		}
		timeFirstTrade := self.statStorage.GetFirstTradeEver(userAddr)
		if timeFirstTrade == trade.Timestamp {
			data.NewUniqueAddresses++
			data.UniqueAddr++
			if kycEd {
				data.KYCEd++
			}
		} else {
			firstTradeInday := self.statStorage.GetFirstTradeInDay(userAddr, trade.Timestamp, i)
			if firstTradeInday == trade.Timestamp {
				data.UniqueAddr++
				if kycEd {
					data.KYCEd++
				}
			}
		}

		data.ETHVolume += ethAmount
		data.BurnFee += burnFee
		data.TradeCount++
		data.USDVolume += trade.FiatAmount
		dataTimeZone[timestamp] = data
		currentCountryData[i] = dataTimeZone
		countryStats[trade.Country] = currentCountryData
	}
	return
}

func (self *Fetcher) FetchCurrentBlock() {
	block, err := self.blockchain.CurrentBlock()
	if err != nil {
		log.Printf("Fetching current block failed: %v. Ignored.", err)
	} else {
		// update currentBlockUpdateTime first to avoid race condition
		// where fetcher is trying to fetch new rate
		self.currentBlockUpdateTime = common.GetTimepoint()
		self.currentBlock = block
	}
}
