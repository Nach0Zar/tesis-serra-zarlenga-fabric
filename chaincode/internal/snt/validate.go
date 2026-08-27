package snt

import (
	"strings"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// Longitudes de los identificadores GS1 que usa el prototipo.
const (
	gtinLength         = 14 // GTIN-14 con digito verificador (modelo-datos.md §2.2)
	glnLength          = 13 // GLN/CUFE de 13 digitos (ADR-003)
	serialMaxLength    = 20
	serialForbidden20  = "779" // un serie de 20 caracteres no puede empezar con "779"
	expirationDateForm = "2006-01-02"
)

// txTimestamp devuelve el timestamp de la transaccion en ISO 8601 UTC.
//
// Debe salir SIEMPRE de GetTxTimestamp() y nunca de time.Now(): el modelo de
// endoso de Fabric exige que la ejecucion sea determinística, y el reloj local
// da un valor distinto en cada peer endosante, rompiendo el write-set
// (modelo-datos.md §3.5).
func txTimestamp(ctx contractapi.TransactionContextInterface) (string, error) {
	ts, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return "", cerr.Internal(err, "no se pudo obtener el timestamp de la transaccion")
	}
	return ts.AsTime().UTC().Format(time.RFC3339), nil
}

// txTime devuelve el timestamp de la transaccion como time.Time, para las
// comparaciones de vigencia que ADR-007 punto 6.f deja en la logica.
func txTime(ctx contractapi.TransactionContextInterface) (time.Time, error) {
	ts, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return time.Time{}, cerr.Internal(err, "no se pudo obtener el timestamp de la transaccion")
	}
	return ts.AsTime().UTC(), nil
}

func invalidRequest(format string, args ...any) *cerr.ContractError {
	return cerr.New(cerr.InvalidRequest, format, args...)
}

// validateGTIN aplica la validacion de formato GS1 al GTIN: 14 digitos y
// digito verificador correcto.
func validateGTIN(gtin string) error {
	if len(gtin) != gtinLength || !isAllDigits(gtin) {
		return invalidRequest("el GTIN debe tener %d digitos", gtinLength).
			WithDetails(map[string]any{"gtin": gtin})
	}
	if !hasValidGS1CheckDigit(gtin) {
		return invalidRequest("el digito verificador GS1 del GTIN es invalido").
			WithDetails(map[string]any{"gtin": gtin})
	}
	return nil
}

// validateSerialNumber aplica el formato de numero de serie que releva el marco
// teorico del proyecto para GS1: hasta 20 caracteres que no pueden empezar con
// "779" si ocupan los 20 (modelo-datos.md §2.2).
//
// "Alfanumerico" se interpreta como el conjunto de caracteres 82 que GS1 define
// para el AI(21), y no como [A-Za-z0-9] estricto: el propio contrato usa
// `SN-0001-ABCD` como ejemplo canonico, que un alfanumerico estricto
// rechazaria.
func validateSerialNumber(serial string) error {
	if serial == "" {
		return invalidRequest("el numero de serie es obligatorio")
	}
	if len(serial) > serialMaxLength {
		return invalidRequest("el numero de serie no puede superar los %d caracteres", serialMaxLength).
			WithDetails(map[string]any{"numeroSerie": serial})
	}
	if !isGS1CharacterSet82(serial) {
		return invalidRequest("el numero de serie contiene caracteres fuera del conjunto GS1 82").
			WithDetails(map[string]any{"numeroSerie": serial})
	}
	if len(serial) == serialMaxLength && strings.HasPrefix(serial, serialForbidden20) {
		return invalidRequest("un numero de serie de %d caracteres no puede comenzar con %q",
			serialMaxLength, serialForbidden20).
			WithDetails(map[string]any{"numeroSerie": serial})
	}
	return nil
}

// validateExpirationDate exige el formato persistido YYYY-MM-DD que fija
// modelo-datos.md §3.2, no el AAMMDD ambiguo del codigo 2D GS1.
func validateExpirationDate(value string) error {
	if value == "" {
		return invalidRequest("la fecha de vencimiento es obligatoria")
	}
	if _, err := time.Parse(expirationDateForm, value); err != nil {
		return invalidRequest("la fecha de vencimiento debe estar en formato ISO 8601 (YYYY-MM-DD)").
			WithDetails(map[string]any{"fechaVencimiento": value})
	}
	return nil
}

// validateUnitRef valida la referencia a una unidad comun a casi todas las
// operaciones del contrato.
func validateUnitRef(gtin, numeroSerie string) error {
	if err := validateGTIN(gtin); err != nil {
		return err
	}
	return validateSerialNumber(numeroSerie)
}

// validateOrganizationID valida el identificador regulatorio de una entrada del
// registro segun su idType: 13 digitos con verificador GS1 para GLN/CUFE, o un
// slug estable no vacio para REG (ADR-003; ADR-010, punto 1).
func validateOrganizationID(idType, id string) error {
	switch idType {
	case IDTypeGLN, IDTypeCUFE:
		if len(id) != glnLength || !isAllDigits(id) {
			return invalidRequest("un identificador %s debe tener %d digitos", idType, glnLength).
				WithDetails(map[string]any{"id": id, "idType": idType})
		}
		if !hasValidGS1CheckDigit(id) {
			return invalidRequest("el digito verificador GS1 del identificador %s es invalido", idType).
				WithDetails(map[string]any{"id": id, "idType": idType})
		}
		return nil
	case IDTypeREG:
		if id == "" || !isRegSlug(id) {
			return invalidRequest("un identificador REG debe ser un slug estable del organismo").
				WithDetails(map[string]any{"id": id})
		}
		return nil
	default:
		return invalidRequest("idType %q fuera del catalogo (%s, %s, %s)", idType, IDTypeGLN, IDTypeCUFE, IDTypeREG).
			WithDetails(map[string]any{"idType": idType})
	}
}

// parseCanonicalID descompone un identificador canonico `<idType>:<id>` y
// valida su forma. Es la validacion 1 del receptor declarado en una devolucion
// (ADR-009, punto 2) y la del destino declarado en un despacho.
func parseCanonicalID(value string) (idType string, id string, err error) {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return "", "", invalidRequest("el identificador canonico debe tener la forma <idType>:<id>").
			WithDetails(map[string]any{"identificador": value})
	}
	idType, id = parts[0], parts[1]
	if idType != IDTypeGLN && idType != IDTypeCUFE {
		return "", "", invalidRequest("el identificador canonico de un establecimiento debe ser %s: o %s:",
			IDTypeGLN, IDTypeCUFE).
			WithDetails(map[string]any{"identificador": value})
	}
	if err := validateOrganizationID(idType, id); err != nil {
		return "", "", err
	}
	return idType, id, nil
}

// hasValidGS1CheckDigit aplica el calculo de digito verificador de GS1: desde
// el digito de datos mas a la derecha, los pesos alternan 3 y 1; el verificador
// es el complemento a la decena superior de la suma ponderada.
func hasValidGS1CheckDigit(value string) bool {
	if len(value) < 2 || !isAllDigits(value) {
		return false
	}

	data := value[:len(value)-1]
	check := int(value[len(value)-1] - '0')

	sum := 0
	weight := 3
	for i := len(data) - 1; i >= 0; i-- {
		sum += int(data[i]-'0') * weight
		if weight == 3 {
			weight = 1
		} else {
			weight = 3
		}
	}

	return (10-sum%10)%10 == check
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// gs1CharacterSet82 es el conjunto de caracteres que GS1 admite para los
// identificadores de aplicacion alfanumericos, entre ellos el AI(21) del numero
// de serie.
const gs1CharacterSet82 = "!\"%&'()*+,-./0123456789:;<=>?" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ_" +
	"abcdefghijklmnopqrstuvwxyz"

func isGS1CharacterSet82(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune(gs1CharacterSet82, r) {
			return false
		}
	}
	return true
}

// isRegSlug admite mayusculas, digitos y guiones, la forma de los slugs que
// ADR-010 ejemplifica (`ANMAT`, `INSSJP-PAMI`).
func isRegSlug(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'A' && r <= 'Z':
		case r == '-':
		default:
			return false
		}
	}
	return true
}
