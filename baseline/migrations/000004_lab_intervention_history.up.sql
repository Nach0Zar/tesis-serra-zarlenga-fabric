CREATE TABLE public.lab_intervention_events (
    gtin VARCHAR(14) NOT NULL,
    numero_serie VARCHAR(20) NOT NULL,
    event_sequence BIGINT NOT NULL,
    tx_id TEXT NOT NULL,
    event_timestamp TIMESTAMPTZ NOT NULL,
    snapshot JSONB NOT NULL,
    CONSTRAINT lab_intervention_events_pk PRIMARY KEY (gtin, numero_serie, event_sequence),
    CONSTRAINT lab_intervention_events_tx_id_unique UNIQUE (tx_id),
    CONSTRAINT lab_intervention_events_unit_fk FOREIGN KEY (gtin, numero_serie)
        REFERENCES public.medication_units (gtin, numero_serie)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT lab_intervention_events_sequence_positive CHECK (event_sequence > 0),
    CONSTRAINT lab_intervention_events_tx_id_not_blank CHECK (btrim(tx_id) <> ''),
    CONSTRAINT lab_intervention_events_snapshot_object CHECK (
        jsonb_typeof(snapshot) = 'object'
        AND snapshot ?& ARRAY[
            'gtin', 'numeroSerie', 'laboratorio', 'operacion', 'motivo',
            'expiraEn', 'estado', 'emitidaPor', 'emitidaEn'
        ]
    )
);
