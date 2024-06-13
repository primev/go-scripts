package utils

import (
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/ethclient"
	vr "github.com/primevprotocol/validator-registry/pkg/validatorregistry"
)

func InitClient() *ethclient.Client {
	client, err := ethclient.Dial("https://chainrpc.testnet.mev-commit.xyz")
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}
	return client
}

func GetStakedValidators(vrc *vr.ValidatorregistryCaller, numStakedVals *big.Int, valsetVersion *big.Int) [][]byte {
	queryBatchSize := 1000
	aggregatedValset := make([][]byte, 0)
	numStakedValsInt := int(numStakedVals.Int64())
	for i := 0; i < numStakedValsInt; i += queryBatchSize {
		end := i + queryBatchSize
		if end > numStakedValsInt {
			end = numStakedValsInt
		}
		vals, valsetVer, err := vrc.GetStakedValidators(nil, big.NewInt(int64(i)), big.NewInt(int64(end)))
		if err != nil {
			log.Fatalf("Failed to get staked validators: %v", err)
		}
		if valsetVer.Cmp(valsetVersion) != 0 {
			log.Fatalf("Valset version mismatch from len query: %v != %v", valsetVer, valsetVersion)
		}
		aggregatedValset = append(aggregatedValset, vals...)
	}
	return aggregatedValset
}
