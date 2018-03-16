package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/big"
	"os"

	"github.com/KyberNetwork/reserve-data/cmd/configuration"
	"github.com/KyberNetwork/reserve-data/common"
)

var noAuthEnable bool
var endpointOW string

func validateArgs() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: ./verification ACTION FLAGS")
	fmt.Println(" verify -base_url BASE_URL, base_url default is http://localhost:8000 - verify deposit and withdraw all supported exchange")
	fmt.Println(" deposit -exchange EXCHANGE -amount AMOUNT -token TOKEN -base_url BASE_URL, base_url default is http://localhost:8000")
	fmt.Println(" withdraw -exchange EXCHANGE -amount AMOUNT -token TOKEN -base_url BASE_URL, base_url default is http://localhost:8000")
	fmt.Println(" getauthdata")
	fmt.Println(" getpendingactivities")
}

func getTokenAmount(amount float64, token common.Token) string {
	amountInt := amount * math.Pow10(int(token.Decimal))
	return fmt.Sprintf("0x%0x", uint64(amountInt))
}

func floatToBigInt(val float64, token common.Token) string {
	bigVal := new(big.Float)
	bigVal.SetFloat64(val)

	decimal := new(big.Float)
	decimal.SetInt(big.NewInt(int64(math.Pow10(int(token.Decimal)))))

	bigVal.Mul(bigVal, decimal)
	result := new(big.Int)
	bigVal.Int(result)

	return fmt.Sprintf("0x%0x", result)
}

func run(verify *Verification) {
	depositCmd := flag.NewFlagSet("deposit", flag.ExitOnError)
	withdrawCmd := flag.NewFlagSet("withdraw", flag.ExitOnError)

	depositAmount := depositCmd.Float64("amount", 0, "Amount to deposit")
	depositToken := depositCmd.String("token", "", "Token to deposit")
	depositExchange := depositCmd.String("exchange", "", "Exchange to deposit to")
	depositBaseUrl := depositCmd.String("base_url", "", "Core host")

	withdrawAmount := withdrawCmd.Float64("amount", 0, "Amount to withdraw")
	withdrawToken := withdrawCmd.String("token", "", "Token to withdraw")
	withdrawExchange := withdrawCmd.String("exchange", "", "Exchange to withdraw from")
	withdrawBaseUrl := withdrawCmd.String("base_url", "", "Core host")

	switch os.Args[1] {
	case "verify":
		verify.RunVerification()
	case "deposit":
		err := depositCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err.Error())
		}
	case "withdraw":
		err := withdrawCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err.Error())
		}
	case "getauthdata":
		_, err := verify.GetAuthData(common.GetTimepoint())
		if err != nil {
			log.Println(err.Error())
			os.Exit(1)
		}
	case "getpendingactivity":
		_, err := verify.GetPendingActivities(common.GetTimepoint())
		if err != nil {
			log.Println(err.Error())
			os.Exit(1)
		}
	case "getactivities":
		_, err := verify.GetActivities(common.GetTimepoint())
		if err != nil {
			fmt.Println(err.Error())
		}
	default:
		printUsage()
		os.Exit(1)
	}

	if depositCmd.Parsed() {
		var amountStr string
		var token common.Token
		var err error
		if *depositBaseUrl != "" {
			verify.UpdateBaseUrl(*depositBaseUrl)
		}
		if *depositToken != "" {
			token, err = common.GetToken(*depositToken)
			if err != nil {
				log.Println(err.Error)
				os.Exit(1)
			}
		} else {
			log.Println("Token cannot be empty")
			os.Exit(1)
		}

		if *depositAmount > 0 {
			amountStr = floatToBigInt(*depositAmount, token)
		} else {
			log.Println("Amount must bigger than 0")
		}

		if *depositExchange == "" {
			log.Println("Exchange cannot be empty")
		}
		timepoint := common.GetTimepoint()
		result, err := verify.Deposit(*depositExchange, *depositToken, amountStr, timepoint)
		if err != nil {
			log.Panic(err.Error())
		}
		log.Printf("Deposit result: %v", result)
	}

	if withdrawCmd.Parsed() {
		var amountStr string
		var token common.Token
		var err error
		if *withdrawBaseUrl != "" {
			verify.UpdateBaseUrl(*withdrawBaseUrl)
		}
		if *withdrawToken != "" {
			token, err = common.GetToken(*withdrawToken)
			if err != nil {
				log.Println(err.Error)
				os.Exit(1)
			}
		} else {
			log.Println("Token cannot be empty")
			os.Exit(1)
		}

		if *withdrawAmount > 0 {
			amountStr = floatToBigInt(*withdrawAmount, token)
		} else {
			log.Println("Amount must bigger than 0")
		}

		if *withdrawExchange == "" {
			log.Println("Exchange cannot be empty")
		}
		timepoint := common.GetTimepoint()
		result, err := verify.Withdraw(*withdrawExchange, *withdrawToken, amountStr, timepoint)
		if err != nil {
			log.Panic(err.Error())
		}
		log.Printf("Withdraw result: %v", result)
	}
}

func main() {
	InitLogger(ioutil.Discard, os.Stdout, os.Stdout, os.Stderr)
	var config *configuration.Config
	kyberENV := os.Getenv("KYBER_ENV")
	if kyberENV == "" {
		kyberENV = "dev"
	}
	config = configuration.GetConfig(kyberENV,
		!noAuthEnable,
		endpointOW)
	if config.AuthEngine == nil {
		Warning.Println("Current environment setting does not enable authentication. Please check again!!!")
	}
	verify := NewVerification(config.AuthEngine)
	validateArgs()

	run(verify)
}
