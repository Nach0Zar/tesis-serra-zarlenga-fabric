package core

import (
	"strings"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

const gs1CharacterSet82 = "!\"%&'()*+,-./0123456789:;<=>?" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ_" + "abcdefghijklmnopqrstuvwxyz"

func validateUnitRef(gtin, serial string) error {
	if err := validateGTIN(gtin); err != nil {
		return err
	}
	if serial == "" {
		return NewError(InvalidRequest, "el numero de serie es obligatorio")
	}
	if len(serial) > 20 {
		return NewError(InvalidRequest, "el numero de serie no puede superar los 20 caracteres").WithDetails(map[string]any{"numeroSerie": serial})
	}
	for _, character := range serial {
		if !strings.ContainsRune(gs1CharacterSet82, character) {
			return NewError(InvalidRequest, "el numero de serie contiene caracteres fuera del conjunto GS1 82").WithDetails(map[string]any{"numeroSerie": serial})
		}
	}
	if len(serial) == 20 && strings.HasPrefix(serial, "779") {
		return NewError(InvalidRequest, "un numero de serie de 20 caracteres no puede comenzar con \"779\"").WithDetails(map[string]any{"numeroSerie": serial})
	}
	return nil
}

func validateGTIN(gtin string) error {
	if len(gtin) != 14 || !allDigits(gtin) {
		return NewError(InvalidRequest, "el GTIN debe tener 14 digitos").WithDetails(map[string]any{"gtin": gtin})
	}
	if !validGS1CheckDigit(gtin) {
		return NewError(InvalidRequest, "el digito verificador GS1 del GTIN es invalido").WithDetails(map[string]any{"gtin": gtin})
	}
	return nil
}

// ValidateRegisterUnitRequest aplica las mismas validaciones al alta HTTP y al
// snapshot inicial de la baseline.
func ValidateRegisterUnitRequest(req RegisterUnitRequest) error {
	if err := validateUnitRef(req.GTIN, req.NumeroSerie); err != nil {
		return err
	}
	if req.Lote == "" {
		return NewError(InvalidRequest, "el lote es obligatorio")
	}
	if _, err := time.Parse(time.DateOnly, req.FechaVencimiento); err != nil {
		return NewError(InvalidRequest, "la fecha de vencimiento debe estar en formato ISO 8601 (YYYY-MM-DD)").WithDetails(map[string]any{"fechaVencimiento": req.FechaVencimiento})
	}
	return nil
}

func validateCommercial(data CommercialData) error {
	if data.NumeroRemito == "" || data.NumeroFactura == "" || data.Cantidad < 1 {
		return NewError(InvalidRequest, "los datos documentales deben incluir numeroRemito, numeroFactura y una cantidad mayor a cero")
	}
	return nil
}

func validateOrganization(req RegisterOrganizationRequest) error {
	if req.MSPID == "" {
		return NewError(InvalidRequest, "mspId es obligatorio")
	}
	custodial, err := domain.IsCustodialAgentType(req.AgentType)
	if err != nil {
		return internal(err, "no se pudo consultar el catalogo de agentType")
	}
	nonCustodial := req.AgentType == domain.AgentRegulator || req.AgentType == domain.AgentFinancier
	if !custodial && !nonCustodial {
		return NewError(InvalidRequest, "agentType %q fuera del catalogo", req.AgentType)
	}
	switch req.IDType {
	case IDTypeGLN, IDTypeCUFE:
		if !custodial {
			return NewError(InvalidRequest, "idType %s solo es valido con un agentType custodial", req.IDType)
		}
		if len(req.ID) != 13 || !allDigits(req.ID) || !validGS1CheckDigit(req.ID) {
			return NewError(InvalidRequest, "el identificador %s debe tener 13 digitos y verificador GS1 valido", req.IDType)
		}
	case IDTypeREG:
		if !nonCustodial {
			return NewError(InvalidRequest, "idType REG solo es valido con un agentType no custodial")
		}
		if req.ID == "" || !validREGSlug(req.ID) {
			return NewError(InvalidRequest, "un identificador REG debe ser un slug estable del organismo")
		}
	default:
		return NewError(InvalidRequest, "idType %q fuera del catalogo", req.IDType)
	}
	return nil
}

func parseCanonicalID(value string) (string, string, error) {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 || (parts[0] != IDTypeGLN && parts[0] != IDTypeCUFE) {
		return "", "", NewError(InvalidRequest, "el identificador canonico debe tener la forma GLN:<id> o CUFE:<id>")
	}
	if len(parts[1]) != 13 || !allDigits(parts[1]) || !validGS1CheckDigit(parts[1]) {
		return "", "", NewError(InvalidRequest, "el identificador canonico debe contener 13 digitos y verificador GS1 valido")
	}
	return parts[0], parts[1], nil
}

func validGS1CheckDigit(value string) bool {
	if len(value) < 2 || !allDigits(value) {
		return false
	}
	sum, weight := 0, 3
	for index := len(value) - 2; index >= 0; index-- {
		sum += int(value[index]-'0') * weight
		if weight == 3 {
			weight = 1
		} else {
			weight = 3
		}
	}
	return (10-sum%10)%10 == int(value[len(value)-1]-'0')
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func validREGSlug(value string) bool {
	for _, character := range value {
		if (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' {
			continue
		}
		return false
	}
	return value != ""
}
