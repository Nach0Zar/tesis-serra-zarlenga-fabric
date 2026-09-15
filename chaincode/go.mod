module github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode

go 1.23

// Las versiones de las librerias de Fabric estan FIJADAS a proposito, no
// tomadas de la ultima disponible: fabric-contract-api-go v2.2.1 en adelante
// declara `go 1.24`/`go 1.25`, lo que obligaria al builder de chaincode del
// peer (Fabric 2.5.x) a disponer de esa toolchain. v2.2.0 + v2.3.0 declaran
// `go 1.21`/`go 1.22` y compilan con la toolchain del entorno de desarrollo del
// proyecto. Subirlas exige verificar antes el builder de la red (NET-3/NET-4).
require (
	github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain v0.0.0
	github.com/hyperledger/fabric-chaincode-go/v2 v2.3.0
	github.com/hyperledger/fabric-contract-api-go/v2 v2.2.0
)

require (
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/spec v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/hyperledger/fabric-protos-go-apiv2 v0.3.6 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	golang.org/x/net v0.34.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250115164207-1a7da9e5054f // indirect
	google.golang.org/grpc v1.70.0 // indirect
	google.golang.org/protobuf v1.36.5 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// El paquete compartido de reglas (ADR-008 punto 2, ADR-012 seccion 1) vive en
// el mismo repositorio. Ver chaincode/README.md, "Layout del workspace Go",
// para por que el empaquetado del chaincode exige `go mod vendor`.
replace github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain => ../domain
