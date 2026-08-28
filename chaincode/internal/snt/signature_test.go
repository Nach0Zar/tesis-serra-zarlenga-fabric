package snt

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// contractDocPath es el contrato congelado, relativo a este paquete.
const contractDocPath = "../../../docs/api-contract.md"

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

	raw, err := os.ReadFile(filepath.FromSlash(contractDocPath))
	if err != nil {
		t.Fatalf("no se pudo leer el contrato congelado: %v", err)
	}
	doc := string(raw)

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
