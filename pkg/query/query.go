package query

import (
	"encoding/hex"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	utils "github.com/primevprotocol/validator-registry/pkg/utils"
	vr "github.com/primevprotocol/validator-registry/pkg/validatorregistry"
)

func GetAllStakedValsFromNewRegistry() ([]string, error) {
	client, err := ethclient.Dial("https://ethereum-holesky-rpc.publicnode.com")
	if err != nil {
		log.Fatalf("Failed to create eth client: %v", err)
	}

	contractAddress := common.HexToAddress("0x5d4fC7B5Aeea4CF4F0Ca6Be09A2F5AaDAd2F2803") // Holesky validator registry 6/13

	vrc, err := vr.NewValidatorregistryCaller(contractAddress, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Registry caller: %v", err)
	}

	fmt.Println("-------------------")
	fmt.Println("Querying full set of validators BLS pubkeys staked with the registry contract...")
	fmt.Println("-------------------")

	numStakedVals, valsetVersion, err := vrc.GetNumberOfStakedValidators(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get number of staked validators: %v", err)
	}
	aggregatedValset := utils.GetStakedValidators(vrc, numStakedVals, valsetVersion)

	vals := make([]string, len(aggregatedValset))
	for i, val := range aggregatedValset {
		vals[i] = hex.EncodeToString(val[:])
	}

	if numStakedVals.Cmp(big.NewInt(int64(len(aggregatedValset)))) != 0 {
		return nil, fmt.Errorf("number of staked validators does not match aggregated validator set length")
	}
	fmt.Println("Number of staked validators: ", len(aggregatedValset))

	return vals, nil
}
