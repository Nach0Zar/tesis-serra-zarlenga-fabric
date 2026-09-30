package snt

import (
	"strings"
	"testing"
)

// El esquema de claves compuestas es una decision de arquitectura, no un
// detalle de implementacion: de el dependen la consulta por clave compuesta
// parcial de `QueryUnitsByGTIN` (modelo-datos.md §2.2), el ciclo de vida del
// registro de operacion de ADR-006 punto 4 y la ausencia de contencion MVCC en
// los marcadores de participacion de ADR-007 punto 6.
//
// CC-1 fija ese esquema para las issues que lo consumen (CC-3, CC-5, EXT-4).
// Este test lo deja pinneado: un cambio de tipo de objeto o de orden de
// componentes rompe aca, y no en la primera consulta que no encuentra nada.
func TestCompositeKeySchema(t *testing.T) {
	stub := newMockStub()

	const (
		txIDDespacho   = "tx-despacho"
		txIDDevolucion = "tx-devolucion"
	)

	cases := []struct {
		name string
		// build construye la clave con el helper del paquete.
		build func() (string, error)
		// components es la secuencia que la clave debe tener, en orden:
		// tipo de objeto primero, atributos despues.
		components []string
	}{
		{
			name:       "MedicationUnit",
			build:      func() (string, error) { return medicationUnitKey(stub, validGTIN, validSerial) },
			components: []string{objectTypeMedicationUnit, validGTIN, validSerial},
		},
		{
			name:       "Organization",
			build:      func() (string, error) { return organizationKey(stub, labMSP) },
			components: []string{objectTypeOrganization, labMSP},
		},
		{
			name:       "LabIntervention",
			build:      func() (string, error) { return labInterventionKey(stub, validGTIN, validSerial) },
			components: []string{objectTypeLabIntervention, validGTIN, validSerial},
		},
		{
			// ADR-006, punto 4: a lo sumo una operacion ACTIVA por unidad, de
			// modo que su clave no lleva txId (ADR-004, regla 2).
			name:       "TransferOpActive",
			build:      func() (string, error) { return transferOpActiveKey(stub, validGTIN, validSerial) },
			components: []string{objectTypeTransferOpActive, validGTIN, validSerial},
		},
		{
			// El registro historico si lo lleva: una unidad acumula varias
			// operaciones cerradas.
			name:       "TransferOp",
			build:      func() (string, error) { return transferOpKey(stub, validGTIN, validSerial, txIDDespacho) },
			components: []string{objectTypeTransferOp, validGTIN, validSerial, txIDDespacho},
		},
		{
			// ADR-009, punto 2: la devolucion no nace de un despacho y tiene
			// clave propia, historica e inmutable.
			name:       "ReturnOp",
			build:      func() (string, error) { return returnOpKey(stub, validGTIN, validSerial, txIDDevolucion) },
			components: []string{objectTypeReturnOp, validGTIN, validSerial, txIDDevolucion},
		},
		{
			// ADR-007, punto 6: el txId va SIEMPRE ultimo, y los componentes
			// anteriores quedan disponibles para la consulta por clave
			// compuesta parcial.
			name:       "Participacion/Unidad",
			build:      func() (string, error) { return unitParticipationKey(stub, validGTIN, validSerial, "tx-1") },
			components: []string{objectTypeParticipation, participationTargetUnit, validGTIN, validSerial, "tx-1"},
		},
		{
			name:       "Participacion/Organizacion",
			build:      func() (string, error) { return organizationParticipationKey(stub, labMSP, "tx-1") },
			components: []string{objectTypeParticipation, participationTargetOrganization, labMSP, "tx-1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, err := tc.build()
			requireNoError(t, err)

			// Fabric arma la clave como \x00 + tipo + \x00 + atributo + \x00...
			got := strings.Split(strings.Trim(key, "\x00"), "\x00")
			if len(got) != len(tc.components) {
				t.Fatalf("la clave tiene %d componentes y el esquema declara %d: %q",
					len(got), len(tc.components), got)
			}
			for i, want := range tc.components {
				if got[i] != want {
					t.Errorf("componente %d = %q, se esperaba %q", i, got[i], want)
				}
			}
		})
	}
}

// TestParticipationKeyIsUniquePerTransaction materializa la propiedad de la que
// depende que las 50.000 altas del dataset de medicion no se serialicen: dos
// transacciones sobre la misma unidad escriben marcadores en claves distintas,
// y por lo tanto no compiten por la misma version del world state
// (ADR-007, punto 6.g).
func TestParticipationKeyIsUniquePerTransaction(t *testing.T) {
	stub := newMockStub()

	first, err := unitParticipationKey(stub, validGTIN, validSerial, "tx-1")
	requireNoError(t, err)
	second, err := unitParticipationKey(stub, validGTIN, validSerial, "tx-2")
	requireNoError(t, err)

	if first == second {
		t.Fatal("dos transacciones sobre la misma unidad produjeron la misma clave de marcador")
	}

	// El prefijo comun -- todo menos el txId -- es lo que deja los marcadores
	// de una unidad recuperables con GetPrivateDataByPartialCompositeKey.
	prefix := strings.TrimSuffix(first, "tx-1\x00")
	if !strings.HasPrefix(second, prefix) {
		t.Fatalf("los marcadores de una misma unidad no comparten prefijo consultable:\n  %q\n  %q", first, second)
	}
}
