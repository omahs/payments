package gas

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mysteriumnetwork/payments/units"
	"github.com/rs/zerolog/log"
)

// DefaultEtherscanEndpointURI the default etherscan api endpoint.
const DefaultEtherscanEndpointURI = "https://api.etherscan.io/"

// EtherscanStation represents the etherscan api to retrive gas prices.
type EtherscanStation struct {
	apiKey      string
	endpointURI string
	upperBound  *big.Int

	client *http.Client
}

// NewEtherscanStation returns a new instance of etherscan api for gas price checks.
func NewEtherscanStation(apiKey, endpointURI string, upperBound *big.Int) *EtherscanStation {
	endpoint := endpointURI
	if !strings.HasSuffix(endpoint, "/") {
		endpoint += "/"
	}

	return &EtherscanStation{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		endpointURI: endpointURI,
		upperBound:  upperBound,
		apiKey:      apiKey,
	}
}

func (esa *EtherscanStation) GetGasPrices() (*GasPrices, error) {
	res, err := esa.request()
	if err != nil {
		return nil, err
	}
	average, err := strconv.ParseFloat(res.Result.ProposeGasPrice, 64)
	if err != nil {
		return nil, err
	}
	safeLow, err := strconv.ParseFloat(res.Result.SafeGasPrice, 64)
	if err != nil {
		return nil, err
	}
	fast, err := strconv.ParseFloat(res.Result.FastGasPrice, 64)
	if err != nil {
		return nil, err
	}
	prices := GasPrices{
		SafeLow: esa.result(safeLow),
		Average: esa.result(average),
		Fast:    esa.result(fast),
	}
	return &prices, nil
}

func (esa *EtherscanStation) request() (*etherscanGasPriceResponse, error) {
	if esa.apiKey == "" {
		log.Warn().Msg("no API key set, rate is limited")
	}

	response, err := esa.client.Get(fmt.Sprintf("%v%v%v", esa.endpointURI, "api?module=gastracker&action=gasoracle&apikey=", esa.apiKey))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	resp, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var res etherscanGasPriceResponse
	err = json.Unmarshal(resp, &res)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body from etherscan with an error: %w and body: %s", err, string(resp))
	}

	if res.Status != "1" {
		return nil, fmt.Errorf("etherscan api failed with message: %s", res.Message)
	}

	return &res, nil
}

func (esa *EtherscanStation) result(price float64) *big.Int {
	bp := units.FloatGweiToBigIntWei(price)
	return priceMaxUpperBound(bp, esa.upperBound)
}

// etherscanGasPriceResponse returns the gas station response.
type etherscanGasPriceResponse struct {
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Result  gasPriceResult `json:"result"`
}

type etherscanGasPriceResponseFail struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Result  string `json:"result"`
}

// gasPriceResult the gas prices for the last block.
type gasPriceResult struct {
	LastBlock       string `json:"LastBlock"`
	SafeGasPrice    string `json:"SafeGasPrice"`
	ProposeGasPrice string `json:"ProposeGasPrice"`
	FastGasPrice    string `json:"FastGasPrice"`
	SuggestBaseFee  string `json:"suggestBaseFee"`
	GasUsedRatio    string `json:"gasUsedRatio"`
}
