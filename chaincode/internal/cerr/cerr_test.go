package cerr

import (
	"encoding/json"
	"errors"
	"testing"
)

// TestErrorSerializesContractFormat verifica el formato que fija
// docs/api-contract.md: el mensaje del error es un objeto JSON con `code`,
// `message` y `details` opcional.
func TestErrorSerializesContractFormat(t *testing.T) {
	err := New(TransferNotAuthorized, "el par %s -> %s no esta autorizado", "LABORATORY", "PHARMACY").
		WithDetails(map[string]any{"origen": "LABORATORY", "destino": "PHARMACY"})

	var decoded struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details"`
	}
	if unmarshalErr := json.Unmarshal([]byte(err.Error()), &decoded); unmarshalErr != nil {
		t.Fatalf("el mensaje del error no es un objeto JSON: %v", unmarshalErr)
	}
	if decoded.Code != string(TransferNotAuthorized) {
		t.Fatalf("code = %q", decoded.Code)
	}
	if decoded.Message != "el par LABORATORY -> PHARMACY no esta autorizado" {
		t.Fatalf("message = %q", decoded.Message)
	}
	if decoded.Details["origen"] != "LABORATORY" {
		t.Fatalf("details = %v", decoded.Details)
	}
}

// TestDetailsAreOptional deja fijado que la ausencia de `details` es valida.
func TestDetailsAreOptional(t *testing.T) {
	err := New(UnitNotFound, "la unidad no existe")

	var decoded map[string]any
	if unmarshalErr := json.Unmarshal([]byte(err.Error()), &decoded); unmarshalErr != nil {
		t.Fatalf("el mensaje del error no es un objeto JSON: %v", unmarshalErr)
	}
	if _, present := decoded["details"]; present {
		t.Fatal("details vacio no debe aparecer en la serializacion")
	}
}

// TestWithDetailsDoesNotMutateOriginal verifica que WithDetails devuelva una
// copia: los errores del catalogo se construyen y decoran en distintos puntos y
// no deben compartir estado.
func TestWithDetailsDoesNotMutateOriginal(t *testing.T) {
	base := New(InvalidRequest, "campo invalido")
	decorated := base.WithDetails(map[string]any{"campo": "gtin"})

	if base.Details != nil {
		t.Fatal("WithDetails muto el error original")
	}
	if decorated.Details["campo"] != "gtin" {
		t.Fatalf("la copia no lleva los detalles: %v", decorated.Details)
	}
	if base.Code != decorated.Code || base.Message != decorated.Message {
		t.Fatal("la copia debe conservar code y message")
	}
}

// TestInternalWrapsPlatformError verifica que una falla de la plataforma se
// tipifique como INTERNAL_ERROR conservando el contexto.
func TestInternalWrapsPlatformError(t *testing.T) {
	wrapped := Internal(errors.New("connection refused"), "no se pudo leer la unidad")

	if wrapped.Code != InternalError {
		t.Fatalf("code = %s", wrapped.Code)
	}
	if wrapped.Message != "no se pudo leer la unidad: connection refused" {
		t.Fatalf("message = %q", wrapped.Message)
	}
}

func TestParse(t *testing.T) {
	parsed, ok := Parse(New(OrgInactive, "organizacion no habilitada"))
	if !ok || parsed.Code != OrgInactive {
		t.Fatalf("Parse no recupero el error tipificado: %+v ok=%v", parsed, ok)
	}

	// Un error que no sigue el formato del contrato no debe interpretarse como
	// si lo hiciera: el cliente ramifica sobre `code` y un code vacio no es una
	// rama valida.
	cases := []error{
		nil,
		errors.New("un error suelto"),
		errors.New(`{"message":"sin code"}`),
	}
	for _, err := range cases {
		if _, ok := Parse(err); ok {
			t.Fatalf("Parse acepto un error fuera del formato del contrato: %v", err)
		}
	}
}
