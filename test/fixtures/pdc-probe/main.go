package main

import (
	"fmt"
	"log"

	"github.com/hyperledger/fabric-chaincode-go/v2/shim"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
)

const transientPrivateKey = "private"

type probeChaincode struct{}

func (p *probeChaincode) Init(shim.ChaincodeStubInterface) *pb.Response {
	return shim.Success(nil)
}

func (p *probeChaincode) Invoke(stub shim.ChaincodeStubInterface) *pb.Response {
	function, args := stub.GetFunctionAndParameters()
	switch function {
	case "Put":
		return put(stub, args)
	case "GetPublic":
		return getPublic(stub, args)
	case "GetPrivate":
		return getPrivate(stub, args)
	case "PutImplicit":
		return putImplicit(stub, args)
	case "GetImplicit":
		return getImplicit(stub, args)
	default:
		return shim.Error(fmt.Sprintf("unknown probe function %q", function))
	}
}

func put(stub shim.ChaincodeStubInterface, args []string) *pb.Response {
	if len(args) != 3 {
		return shim.Error("Put requires collection, key and public value")
	}
	privateValue, err := transientValue(stub)
	if err != nil {
		return shim.Error(err.Error())
	}
	if err := stub.PutState(args[1], []byte(args[2])); err != nil {
		return shim.Error(fmt.Sprintf("put public state: %v", err))
	}
	if err := stub.PutPrivateData(args[0], args[1], privateValue); err != nil {
		return shim.Error(fmt.Sprintf("put private state: %v", err))
	}
	return shim.Success([]byte(args[1]))
}

func getPublic(stub shim.ChaincodeStubInterface, args []string) *pb.Response {
	if len(args) != 1 {
		return shim.Error("GetPublic requires key")
	}
	value, err := stub.GetState(args[0])
	return queryResult(value, err)
}

func getPrivate(stub shim.ChaincodeStubInterface, args []string) *pb.Response {
	if len(args) != 2 {
		return shim.Error("GetPrivate requires collection and key")
	}
	value, err := stub.GetPrivateData(args[0], args[1])
	return queryResult(value, err)
}

func putImplicit(stub shim.ChaincodeStubInterface, args []string) *pb.Response {
	if len(args) != 2 {
		return shim.Error("PutImplicit requires owner MSP and key")
	}
	privateValue, err := transientValue(stub)
	if err != nil {
		return shim.Error(err.Error())
	}
	collection := "_implicit_org_" + args[0]
	if err := stub.PutPrivateData(collection, args[1], privateValue); err != nil {
		return shim.Error(fmt.Sprintf("put implicit state: %v", err))
	}
	return shim.Success([]byte(args[1]))
}

func getImplicit(stub shim.ChaincodeStubInterface, args []string) *pb.Response {
	if len(args) != 2 {
		return shim.Error("GetImplicit requires owner MSP and key")
	}
	value, err := stub.GetPrivateData("_implicit_org_"+args[0], args[1])
	return queryResult(value, err)
}

func transientValue(stub shim.ChaincodeStubInterface) ([]byte, error) {
	transient, err := stub.GetTransient()
	if err != nil {
		return nil, fmt.Errorf("read transient map: %w", err)
	}
	value, ok := transient[transientPrivateKey]
	if !ok || len(value) == 0 {
		return nil, fmt.Errorf("transient key %q is required", transientPrivateKey)
	}
	return value, nil
}

func queryResult(value []byte, err error) *pb.Response {
	if err != nil {
		return shim.Error(err.Error())
	}
	if value == nil {
		return shim.Error("not found")
	}
	return shim.Success(value)
}

func main() {
	if err := shim.Start(&probeChaincode{}); err != nil {
		log.Fatalf("start pdc probe: %v", err)
	}
}
