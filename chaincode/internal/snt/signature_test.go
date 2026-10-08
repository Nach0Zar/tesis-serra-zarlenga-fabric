package snt

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// contractDocPath es el contrato congelado, relativo a este paquete.
const contractDocPath = "../../../docs/api-contract.md"

// readContractDoc lee el contrato congelado. La ruta es una constante del
// paquete, no una entrada externa.
func readContractDoc(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.FromSlash(contractDocPath))
	if err != nil {
		t.Fatalf("no se pudo leer el contrato congelado: %v", err)
	}
	return string(raw)
}

// TestContractSignaturesMatchFrozenContract contrasta la firma REAL de cada
// operacion contra la firma que docs/api-contract.md declara.
//
// Por que no alcanza con los otros dos tests del paquete:
//
//   - TestContractSurfaceMatchesFrozenContract compara solo los NOMBRES. Una
//     operacion que cambiara el tipo de su request y conservara el nombre
//     seguiria pasando.
//   - contractapi.NewChaincode() valida que las firmas sean ADMISIBLES para la
//     ContractAPI, no que sean LAS del contrato. Cambiar
//     `req UnitRefRequest` por `req UnitEventRequest` produce una firma
//     igualmente admisible y romperia al cliente y a la baseline en silencio.
//
// El contrato esta congelado (docs/api-contract.md, "Politica de versionado y
// congelamiento"): cambiar una firma exige su propio PR con aprobacion
// explicita, y este test es lo que lo vuelve mecanico en lugar de convencional.
func TestContractSignaturesMatchFrozenContract(t *testing.T) {
	documented := parseDocumentedSignatures(t)

	if len(documented) != len(contractOperations) {
		t.Fatalf("el contrato documenta %d firmas y declara %d operaciones",
			len(documented), len(contractOperations))
	}

	contractType := reflect.TypeOf(&SNTContract{})
	for _, name := range contractOperations {
		t.Run(name, func(t *testing.T) {
			want, ok := documented[name]
			if !ok {
				t.Fatalf("docs/api-contract.md no documenta la firma de %s", name)
			}

			method, found := contractType.MethodByName(name)
			if !found {
				t.Fatalf("el chaincode no declara %s", name)
			}

			if got := normalizeReflectSignature(method); got != want {
				t.Fatalf("firma de %s distinta de la del contrato v%s\n  chaincode: %s\n  contrato:  %s",
					name, ContractVersion, got, want)
			}
		})
	}
}

// docSignatureRE captura las firmas de los bloques ```go del contrato. El
// nombre puede ser el de una operacion concreta o el marcador `<Nombre>` del
// bloque comun a los eventos extraordinarios.
var docSignatureRE = regexp.MustCompile(
	`(?m)^func \(c \*SNTContract\) (<Nombre>|[A-Za-z]\w*)\(([^)]*)\) \(([^)]*)\)$`)

// eventFunctionRE captura los nombres de la tabla de operaciones de eventos
// extraordinarios, que comparten la firma del bloque `<Nombre>`.
var eventFunctionRE = regexp.MustCompile("(?m)^\\| `([A-Z]\\w*)` \\| T\\d\\d")

// parseDocumentedSignatures devuelve, por operacion, su firma normalizada tal
// como la declara docs/api-contract.md.
func parseDocumentedSignatures(t *testing.T) map[string]string {
	t.Helper()

	doc := readContractDoc(t)

	// Las once operaciones de eventos extraordinarios y de resolucion no
	// repiten su firma: el contrato la escribe una vez con `<Nombre>` y las
	// enumera en la tabla que sigue.
	var eventFunctions []string
	for _, match := range eventFunctionRE.FindAllStringSubmatch(doc, -1) {
		eventFunctions = append(eventFunctions, match[1])
	}
	if len(eventFunctions) == 0 {
		t.Fatal("no se encontro la tabla de operaciones de eventos extraordinarios")
	}

	signatures := make(map[string]string)
	for _, match := range docSignatureRE.FindAllStringSubmatch(doc, -1) {
		name, params, results := match[1], match[2], match[3]
		normalized := normalizeSignature(params, results)

		if name != "<Nombre>" {
			signatures[name] = normalized
			continue
		}
		for _, eventName := range eventFunctions {
			signatures[eventName] = normalized
		}
	}
	return signatures
}

// normalizeSignature arma la representacion canonica `(tipos) (tipos)` de una
// firma documentada. Descarta los nombres de los parametros, que la reflexion
// no expone, y compara solo aridad y tipos.
func normalizeSignature(params, results string) string {
	return "(" + strings.Join(paramTypes(params), ", ") + ") (" +
		strings.Join(splitTypes(results), ", ") + ")"
}

// paramTypes extrae el tipo de cada parametro `nombre tipo` de la firma
// documentada.
func paramTypes(params string) []string {
	var out []string
	for _, param := range splitTypes(params) {
		fields := strings.Fields(param)
		out = append(out, canonicalType(fields[len(fields)-1]))
	}
	return out
}

func splitTypes(list string) []string {
	var out []string
	for _, item := range strings.Split(list, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, canonicalType(item))
		}
	}
	return out
}

// normalizeReflectSignature arma la misma representacion canonica a partir del
// metodo real. El primer parametro del reflect.Method de un tipo puntero es el
// receptor y se descarta.
func normalizeReflectSignature(method reflect.Method) string {
	fn := method.Type

	var params []string
	for i := 1; i < fn.NumIn(); i++ {
		params = append(params, canonicalType(fn.In(i).String()))
	}

	var results []string
	for i := 0; i < fn.NumOut(); i++ {
		results = append(results, canonicalType(fn.Out(i).String()))
	}

	return "(" + strings.Join(params, ", ") + ") (" + strings.Join(results, ", ") + ")"
}

// packageQualifierRE captura el prefijo `paquete.` de un tipo calificado.
var packageQualifierRE = regexp.MustCompile(`\b[a-z]\w*\.`)

// typeAliases resuelve los alias del contrato hacia su tipo subyacente. La
// reflexion nunca ve el alias -- `MedicationUnitView = MedicationUnit` es un
// alias, no un tipo nuevo --, de modo que sin esta tabla toda operacion que
// devuelva la vista publica pareceria divergir del contrato.
var typeAliases = map[string]string{
	"MedicationUnitView": "MedicationUnit",
	"OrganizationView":   "OrganizationRecord",
}

// canonicalType deja un tipo en la forma que ambos lados comparten: sin
// calificador de paquete y con los alias del contrato resueltos.
func canonicalType(t string) string {
	t = packageQualifierRE.ReplaceAllString(t, "")

	prefix := ""
	for {
		switch {
		case strings.HasPrefix(t, "*"):
			prefix, t = prefix+"*", t[1:]
		case strings.HasPrefix(t, "[]"):
			prefix, t = prefix+"[]", t[2:]
		default:
			if alias, ok := typeAliases[t]; ok {
				t = alias
			}
			return prefix + t
		}
	}
}

// TestDocumentedOperationsMatchDeclaredSurface cierra el otro sentido: que la
// lista de operaciones contra la que se contrasta la superficie sea exactamente
// la que el contrato documenta, y no una copia que quedo atras.
func TestDocumentedOperationsMatchDeclaredSurface(t *testing.T) {
	documented := parseDocumentedSignatures(t)

	names := make([]string, 0, len(documented))
	for name := range documented {
		names = append(names, name)
	}
	expected := append([]string(nil), contractOperations...)
	sort.Strings(names)
	sort.Strings(expected)

	if !reflect.DeepEqual(expected, names) {
		t.Fatalf("operaciones documentadas distintas de las declaradas\n  documentadas: %v\n  declaradas:   %v",
			names, expected)
	}
}

// --- Version declarada y errores declarados ---------------------------------

// TestContractVersionMatchesFrozenContract impide que la constante del paquete
// y el encabezado del contrato se separen. ContractVersion viaja al peer como
// Info.Version del chaincode y aparece en los mensajes de los tests de firma:
// si dijera una version que el documento ya no tiene, todo el andamiaje de
// congelamiento estaria citando un contrato inexistente.
func TestContractVersionMatchesFrozenContract(t *testing.T) {
	raw := readContractDoc(t)

	match := regexp.MustCompile(
		`(?m)^- \*\*Versi.n del contrato\*\*: ` + "`" + `([0-9]+\.[0-9]+\.[0-9]+)` + "`").FindStringSubmatch(raw)
	if match == nil {
		t.Fatal("no se pudo leer la version declarada en el encabezado de docs/api-contract.md")
	}
	if match[1] != ContractVersion {
		t.Fatalf("el documento declara la version %s y el paquete %s", match[1], ContractVersion)
	}
}

// docErrorsRE captura la lista de errores que el contrato declara para cada
// operacion. La seccion de una operacion abre con `### ` + su nombre entre
// backticks, y su linea de errores es un item `- **Errores**:` con los codigos
// entre backticks.
var docErrorsRE = regexp.MustCompile(
	`(?m)^### ` + "`" + `([A-Za-z]\w*)` + "`" + `$|^- \*\*Errores\*\*: (.+)$`)

// codeRE captura cada codigo del catalogo dentro de una linea de errores.
var codeRE = regexp.MustCompile("`([A-Z][A-Z_]+)`")

// parseDocumentedErrors devuelve, por operacion, el conjunto de codigos que el
// contrato declara para ella.
func parseDocumentedErrors(t *testing.T) map[string]map[cerr.Code]bool {
	t.Helper()

	documented := map[string]map[cerr.Code]bool{}
	current := ""
	for _, match := range docErrorsRE.FindAllStringSubmatch(readContractDoc(t), -1) {
		if match[1] != "" {
			current = match[1]
			continue
		}
		if current == "" {
			continue
		}
		codes := map[cerr.Code]bool{}
		for _, code := range codeRE.FindAllStringSubmatch(match[2], -1) {
			codes[cerr.Code(code[1])] = true
		}
		documented[current] = codes
	}
	return documented
}

// TestProducedErrorsAreDeclaredByTheContract cierra el hueco entre lo que una
// operacion PUEDE devolver y lo que su contrato DECLARA que devuelve.
//
// El resto de la bateria comprueba las dos mitades por separado y ninguna
// detecta que se separen: TestErrorCatalogIsCovered exige que cada codigo del
// catalogo tenga algun escenario, sin mirar de que operacion sale, y
// TestContractSignaturesMatchFrozenContract compara firmas, no errores. Una
// operacion podia asi devolver de forma estable un codigo que su seccion del
// contrato no nombraba -- que es exactamente lo que le pasaba a `Dispense` y a
// `RejectTransfer` con ORG_NOT_REGISTERED y ORG_INACTIVE hasta la v2.6.2.
//
// El caso cubierto es el de las dos condiciones TRANSVERSALES de DES-6: toda
// operacion que resuelve la identidad del invocador contra el registro de
// ADR-003 puede rechazar porque la organizacion no tiene entrada o porque no
// esta habilitada. Son las que se olvidan, precisamente porque no son propias
// de ninguna operacion.
//
// INTERNAL_ERROR queda deliberadamente fuera: el catalogo lo define como el
// error no clasificable de cualquier operacion y el contrato no lo repite en
// cada lista.
func TestProducedErrorsAreDeclaredByTheContract(t *testing.T) {
	documented := parseDocumentedErrors(t)

	// Cada operacion custodial, con la invocacion que la alcanza sobre una
	// unidad en el estado del que parte. El mspId lo elige el caso.
	operations := map[string]func(contract *SNTContract, ctx contractapi.TransactionContextInterface) error{
		"RegisterUnit": func(c *SNTContract, ctx contractapi.TransactionContextInterface) error {
			_, err := c.RegisterUnit(ctx, validRegisterUnitRequest())
			return err
		},
		"DispatchTransfer": func(c *SNTContract, ctx contractapi.TransactionContextInterface) error {
			_, err := c.DispatchTransfer(ctx, DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			return err
		},
		"ReceiveTransfer": func(c *SNTContract, ctx contractapi.TransactionContextInterface) error {
			_, err := c.ReceiveTransfer(ctx, UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			return err
		},
		"RejectTransfer": func(c *SNTContract, ctx contractapi.TransactionContextInterface) error {
			_, err := c.RejectTransfer(ctx,
				UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "Motivo documentado."})
			return err
		},
		"Dispense": func(c *SNTContract, ctx contractapi.TransactionContextInterface) error {
			_, err := c.Dispense(ctx, UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			return err
		},
	}

	for name, invoke := range operations {
		t.Run(name, func(t *testing.T) {
			declared, ok := documented[name]
			if !ok {
				t.Fatalf("docs/api-contract.md no declara errores para %s", name)
			}

			t.Run(string(cerr.OrgNotRegistered), func(t *testing.T) {
				stub, contract := transferFixture(t)
				seedUnit(t, stub, domain.StateEnCustodia, "GLN:"+farmaciaGLN)
				err := invoke(contract, testContext(stub, "OrgFantasmaMSP", RoleOperator))
				requireCode(t, err, cerr.OrgNotRegistered)
				if !declared[cerr.OrgNotRegistered] {
					t.Fatalf("%s devuelve %s y el contrato v%s no lo declara para esa operacion",
						name, cerr.OrgNotRegistered, ContractVersion)
				}
			})

			t.Run(string(cerr.OrgInactive), func(t *testing.T) {
				stub, contract := transferFixture(t)
				seedUnit(t, stub, domain.StateEnCustodia, "GLN:"+farmaciaGLN)
				if _, err := contract.SetOrganizationActive(
					testContext(stub, anmatMSP, RoleRegulatoryAdmin),
					SetOrganizationActiveRequest{MSPID: farmaciaMSP, Active: false}); err != nil {
					t.Fatalf("SetOrganizationActive: %v", err)
				}
				err := invoke(contract, testContext(stub, farmaciaMSP, RoleOperator))
				requireCode(t, err, cerr.OrgInactive)
				if !declared[cerr.OrgInactive] {
					t.Fatalf("%s devuelve %s y el contrato v%s no lo declara para esa operacion",
						name, cerr.OrgInactive, ContractVersion)
				}
			})
		})
	}
}
