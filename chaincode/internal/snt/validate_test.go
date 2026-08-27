package snt

import (
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
)

// TestGS1CheckDigit cubre el calculo de digito verificador sobre los valores
// reales del proyecto: el GTIN de los ejemplos del contrato y los GLN del
// manifiesto fundacional.
func TestGS1CheckDigit(t *testing.T) {
	valid := []string{
		validGTIN,    // GTIN de los ejemplos de docs/api-contract.md
		labGLN,       // GLN del laboratorio del manifiesto
		drogueriaGLN, // GLN de la drogueria
		farmaciaGLN,  // GLN de la farmacia
		"7791234500031",
		"7791234500055",
	}
	for _, value := range valid {
		if !hasValidGS1CheckDigit(value) {
			t.Errorf("%s deberia tener digito verificador GS1 valido", value)
		}
	}

	invalid := []string{
		"07791234567890", // el valor que la version 2.0.0 del contrato corrigio
		"7791234500018",
		"779123450001A",
		"7",
		"",
	}
	for _, value := range invalid {
		if hasValidGS1CheckDigit(value) {
			t.Errorf("%s no deberia pasar la validacion de digito verificador", value)
		}
	}
}

func TestValidateGTIN(t *testing.T) {
	requireNoError(t, validateGTIN(validGTIN))

	cases := map[string]string{
		"longitud incorrecta":         "0779123456789",
		"digito verificador invalido": "07791234567890",
		"no numerico":                 "0779123456789X",
		"vacio":                       "",
	}
	for name, gtin := range cases {
		t.Run(name, func(t *testing.T) {
			requireCode(t, validateGTIN(gtin), cerr.InvalidRequest)
		})
	}
}

func TestValidateSerialNumber(t *testing.T) {
	valid := []string{
		"SN-0001-ABCD",         // ejemplo canonico del contrato
		"A",                    // minimo
		"12345678901234567890", // 20 caracteres que no empiezan con 779
		"ABC.123/XY",
	}
	for _, serial := range valid {
		if err := validateSerialNumber(serial); err != nil {
			t.Errorf("%q deberia ser un numero de serie valido: %v", serial, err)
		}
	}

	invalid := map[string]string{
		"vacio":                       "",
		"mas de 20 caracteres":        "123456789012345678901",
		"caracter fuera del set GS1":  "SN 0001",
		"20 caracteres que abren 779": "77912345678901234567",
	}
	for name, serial := range invalid {
		t.Run(name, func(t *testing.T) {
			requireCode(t, validateSerialNumber(serial), cerr.InvalidRequest)
		})
	}

	// Un serie que empieza con "779" pero NO ocupa los 20 caracteres es valido:
	// la restriccion aplica solo a la longitud maxima.
	requireNoError(t, validateSerialNumber("7791234567890123456"))
}

func TestValidateExpirationDate(t *testing.T) {
	requireNoError(t, validateExpirationDate("2027-12-31"))

	invalid := map[string]string{
		"vacia":                 "",
		"formato AAMMDD del 2D": "271231",
		"formato local":         "31/12/2027",
		"fecha inexistente":     "2027-02-30",
		"timestamp completo":    "2027-12-31T00:00:00Z",
	}
	for name, value := range invalid {
		t.Run(name, func(t *testing.T) {
			requireCode(t, validateExpirationDate(value), cerr.InvalidRequest)
		})
	}
}

func TestValidateOrganizationID(t *testing.T) {
	requireNoError(t, validateOrganizationID(IDTypeGLN, labGLN))
	requireNoError(t, validateOrganizationID(IDTypeCUFE, labGLN))
	requireNoError(t, validateOrganizationID(IDTypeREG, "ANMAT"))
	requireNoError(t, validateOrganizationID(IDTypeREG, "INSSJP-PAMI"))

	cases := []struct {
		name   string
		idType string
		id     string
	}{
		{"GLN corto", IDTypeGLN, "779123450001"},
		{"GLN con verificador invalido", IDTypeGLN, "7791234500018"},
		{"CUFE no numerico", IDTypeCUFE, "77912345000A7"},
		{"slug REG vacio", IDTypeREG, ""},
		{"slug REG con minusculas", IDTypeREG, "anmat"},
		{"idType desconocido", "DNI", "20123456789"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireCode(t, validateOrganizationID(tc.idType, tc.id), cerr.InvalidRequest)
		})
	}
}

// TestParseCanonicalID cubre la validacion 1 del receptor declarado de una
// devolucion (ADR-009, punto 2) y la del destino de un despacho: el
// identificador de un establecimiento es siempre GLN: o CUFE:, nunca REG:,
// porque los agentType no custodiales jamas son contraparte de una
// transferencia (ADR-010, punto 2).
func TestParseCanonicalID(t *testing.T) {
	idType, id, err := parseCanonicalID("GLN:" + labGLN)
	requireNoError(t, err)
	if idType != IDTypeGLN || id != labGLN {
		t.Fatalf("parseCanonicalID devolvio %s / %s", idType, id)
	}

	invalid := map[string]string{
		"sin separador":                   labGLN,
		"idType no custodial":             "REG:ANMAT",
		"idType desconocido":              "DNI:20123456789",
		"identificador con checksum malo": "GLN:7791234500018",
		"vacio":                           "",
	}
	for name, value := range invalid {
		t.Run(name, func(t *testing.T) {
			_, _, err := parseCanonicalID(value)
			requireCode(t, err, cerr.InvalidRequest)
		})
	}
}

// TestTxTimestampIsDeterministic fija la regla de modelo-datos.md §3.5: el
// timestamp sale de GetTxTimestamp() y es identico para todos los peers
// endosantes de la misma propuesta.
func TestTxTimestampIsDeterministic(t *testing.T) {
	stub := newMockStub()
	ctx := testContext(stub, labMSP, RoleOperator)

	first, err := txTimestamp(ctx)
	requireNoError(t, err)
	second, err := txTimestamp(ctx)
	requireNoError(t, err)

	if first != second || first != "2026-08-27T12:00:00Z" {
		t.Fatalf("txTimestamp no es determinístico: %q vs %q", first, second)
	}
}
