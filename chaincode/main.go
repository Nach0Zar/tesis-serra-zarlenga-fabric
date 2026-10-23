// Command snt es el punto de entrada del chaincode `snt` del prototipo del PFI.
//
// El nombre del chaincode (`snt`) y el del canal (`snt-channel`) los fija
// ADR-007, punto 4.
package main

import (
	"log"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/snt"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

func main() {
	contract := new(snt.SNTContract)
	contract.Info.Version = snt.ContractVersion
	contract.Info.Title = "Contrato de trazabilidad del SNT"

	chaincode, err := contractapi.NewChaincode(contract)
	if err != nil {
		log.Panicf("no se pudo crear el chaincode snt: %v", err)
	}

	if err := chaincode.Start(); err != nil {
		log.Panicf("no se pudo iniciar el chaincode snt: %v", err)
	}
}
