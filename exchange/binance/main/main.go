package main

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/KyberNetwork/reserve-data/common"
	"github.com/KyberNetwork/reserve-data/exchange"
	"github.com/KyberNetwork/reserve-data/exchange/binance"
)

func getBinanceInterface() binance.Interface {
	return binance.NewDevInterface()
}

func AsyncUpdateDepositAddress(ex common.Exchange, tokenID, addr string, wait *sync.WaitGroup) {
	defer wait.Done()
	ex.UpdateDepositAddress(common.MustGetInternalToken(tokenID), addr)
}

func main() {
	secretPath := "/go/src/github.com/KyberNetwork/reserve-data/cmd/config.json"
	filePath := "/go/src/github.com/KyberNetwork/reserve-data/cmd/dev_setting.json"
	feePath := "/go/src/github.com/KyberNetwork/reserve-data/cmd/fee.json"
	binanceSigner := binance.NewSignerFromFile(secretPath)
	endpoint := binance.NewBinanceEndpoint(binanceSigner, getBinanceInterface())
	addressConfig, _ := common.GetAddressConfigFromFile(filePath)
	feeConfig, _ := common.GetFeeFromFile(feePath)
	minDepositPath := "/go/src/github.com/KyberNetwork/reserve-data/cmd/min_deposit.json"
	for id, t := range addressConfig.Tokens {
		tok := common.Token{
			id, t.Address, t.Decimals,
		}
		if t.Active {
			if t.KNReserveSupport {
				common.RegisterInternalActiveToken(tok)
			} else {
				common.RegisterExternalActiveToken(tok)
			}
		} else {
			common.RegisterInactiveToken(tok)
		}
	}
	minDeposit, _ := common.GetMinDepositFromFile(minDepositPath)
	bin := exchange.NewBinance(addressConfig.Exchanges["binance"], feeConfig.Exchanges["binance"], endpoint, minDeposit.Exchanges["binance"])
	wait := sync.WaitGroup{}
	for tokenID, addr := range addressConfig.Exchanges["binance"] {
		wait.Add(1)
		go AsyncUpdateDepositAddress(bin, tokenID, addr, &wait)
	}
	wait.Wait()
	bin.UpdatePairsPrecision()
	base, _ := common.GetInternalToken("EOS")
	quote, _ := common.GetInternalToken("ETH")
	tradeHistory, _ := endpoint.GetAccountTradeHistory(base, quote, 0)
	jsonlog, _ := json.Marshal(tradeHistory)
	log.Printf("trade history: %s", jsonlog)
}
