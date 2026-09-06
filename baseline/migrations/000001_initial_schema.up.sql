CREATE TABLE public.organizations (
    msp_id TEXT PRIMARY KEY,
    id TEXT NOT NULL,
    id_type TEXT NOT NULL,
    agent_type TEXT NOT NULL,
    active BOOLEAN NOT NULL,
    CONSTRAINT organizations_msp_id_not_blank CHECK (btrim(msp_id) <> ''),
    CONSTRAINT organizations_id_not_blank CHECK (btrim(id) <> '' AND strpos(id, ':') = 0),
    CONSTRAINT organizations_id_type_valid CHECK (id_type IN ('GLN', 'CUFE', 'REG')),
    CONSTRAINT organizations_agent_type_valid CHECK (
        agent_type IN (
            'LABORATORY',
            'DISTRIBUTOR',
            'LOGISTICS_OPERATOR',
            'DRUGSTORE',
            'PHARMACY',
            'HEALTHCARE_FACILITY',
            'REGULATOR',
            'FINANCIER'
        )
    ),
    CONSTRAINT organizations_identity_kind_valid CHECK (
        (agent_type IN ('REGULATOR', 'FINANCIER') AND id_type = 'REG')
        OR
        (
            agent_type IN (
                'LABORATORY',
                'DISTRIBUTOR',
                'LOGISTICS_OPERATOR',
                'DRUGSTORE',
                'PHARMACY',
                'HEALTHCARE_FACILITY'
            )
            AND id_type IN ('GLN', 'CUFE')
        )
    ),
    CONSTRAINT organizations_canonical_identity_unique UNIQUE (id_type, id)
);

CREATE TABLE public.medication_units (
    gtin VARCHAR(14) NOT NULL,
    numero_serie VARCHAR(20) NOT NULL,
    lote TEXT NOT NULL,
    fecha_vencimiento DATE NOT NULL,
    custodio_actual TEXT NOT NULL,
    estado TEXT NOT NULL,
    ultima_actualizacion TIMESTAMPTZ NOT NULL,
    CONSTRAINT medication_units_pk PRIMARY KEY (gtin, numero_serie),
    CONSTRAINT medication_units_gtin_valid CHECK (gtin ~ '^[0-9]{14}$'),
    CONSTRAINT medication_units_numero_serie_valid CHECK (char_length(numero_serie) BETWEEN 1 AND 20),
    CONSTRAINT medication_units_lote_not_blank CHECK (btrim(lote) <> ''),
    CONSTRAINT medication_units_custodio_valid CHECK (custodio_actual ~ '^(GLN|CUFE):[^:]+$'),
    CONSTRAINT medication_units_estado_valid CHECK (
        estado IN (
            'EN_LABORATORIO',
            'EN_TRANSITO',
            'EN_CUSTODIA',
            'EN_CUARENTENA',
            'VENCIDO',
            'ROBADO',
            'EXTRAVIADO',
            'DETERIORADO',
            'RETIRADO_MERCADO',
            'PROHIBIDO',
            'DEVUELTO',
            'DISPENSADO',
            'DISPUESTO_FINAL'
        )
    )
);

CREATE INDEX medication_units_gtin_idx ON public.medication_units (gtin);

CREATE TABLE public.unit_events (
    gtin VARCHAR(14) NOT NULL,
    numero_serie VARCHAR(20) NOT NULL,
    tx_id TEXT NOT NULL,
    event_timestamp TIMESTAMPTZ NOT NULL,
    operation TEXT NOT NULL,
    invoker_msp_id TEXT NOT NULL,
    lote TEXT NOT NULL,
    fecha_vencimiento DATE NOT NULL,
    custodio_actual TEXT NOT NULL,
    estado TEXT NOT NULL,
    ultima_actualizacion TIMESTAMPTZ NOT NULL,
    CONSTRAINT unit_events_pk PRIMARY KEY (gtin, numero_serie, tx_id),
    CONSTRAINT unit_events_unit_fk FOREIGN KEY (gtin, numero_serie)
        REFERENCES public.medication_units (gtin, numero_serie)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT unit_events_invoker_fk FOREIGN KEY (invoker_msp_id)
        REFERENCES public.organizations (msp_id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT unit_events_tx_id_not_blank CHECK (btrim(tx_id) <> ''),
    CONSTRAINT unit_events_operation_not_blank CHECK (btrim(operation) <> ''),
    CONSTRAINT unit_events_gtin_valid CHECK (gtin ~ '^[0-9]{14}$'),
    CONSTRAINT unit_events_numero_serie_valid CHECK (char_length(numero_serie) BETWEEN 1 AND 20),
    CONSTRAINT unit_events_lote_not_blank CHECK (btrim(lote) <> ''),
    CONSTRAINT unit_events_custodio_valid CHECK (custodio_actual ~ '^(GLN|CUFE):[^:]+$'),
    CONSTRAINT unit_events_estado_valid CHECK (
        estado IN (
            'EN_LABORATORIO',
            'EN_TRANSITO',
            'EN_CUSTODIA',
            'EN_CUARENTENA',
            'VENCIDO',
            'ROBADO',
            'EXTRAVIADO',
            'DETERIORADO',
            'RETIRADO_MERCADO',
            'PROHIBIDO',
            'DEVUELTO',
            'DISPENSADO',
            'DISPUESTO_FINAL'
        )
    )
);

CREATE INDEX unit_events_history_idx
    ON public.unit_events (gtin, numero_serie, event_timestamp, tx_id);

CREATE TABLE public.transfer_operations (
    gtin VARCHAR(14) NOT NULL,
    numero_serie VARCHAR(20) NOT NULL,
    tx_id_despacho TEXT NOT NULL,
    emisor TEXT NOT NULL,
    destinatario_pendiente TEXT NOT NULL,
    numero_remito TEXT NOT NULL,
    numero_factura TEXT NOT NULL,
    cantidad INTEGER NOT NULL,
    rule_id TEXT NOT NULL,
    schema_version TEXT NOT NULL,
    despachada_en TIMESTAMPTZ NOT NULL,
    estado TEXT NOT NULL,
    cerrada_en TIMESTAMPTZ,
    motivo_cierre TEXT,
    recepcion_numero_remito TEXT,
    recepcion_numero_factura TEXT,
    recepcion_cantidad INTEGER,
    CONSTRAINT transfer_operations_pk PRIMARY KEY (gtin, numero_serie, tx_id_despacho),
    CONSTRAINT transfer_operations_unit_fk FOREIGN KEY (gtin, numero_serie)
        REFERENCES public.medication_units (gtin, numero_serie)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT transfer_operations_tx_id_not_blank CHECK (btrim(tx_id_despacho) <> ''),
    CONSTRAINT transfer_operations_emisor_valid CHECK (emisor ~ '^(GLN|CUFE):[^:]+$'),
    CONSTRAINT transfer_operations_destinatario_valid CHECK (destinatario_pendiente ~ '^(GLN|CUFE):[^:]+$'),
    CONSTRAINT transfer_operations_documents_not_blank CHECK (
        btrim(numero_remito) <> '' AND btrim(numero_factura) <> ''
    ),
    CONSTRAINT transfer_operations_cantidad_valid CHECK (cantidad > 0),
    CONSTRAINT transfer_operations_rule_id_not_blank CHECK (btrim(rule_id) <> ''),
    CONSTRAINT transfer_operations_schema_version_valid CHECK (
        schema_version ~ '^[0-9]+\.[0-9]+\.[0-9]+$'
    ),
    CONSTRAINT transfer_operations_estado_valid CHECK (estado IN ('ACTIVA', 'CERRADA')),
    CONSTRAINT transfer_operations_motivo_cierre_valid CHECK (
        motivo_cierre IS NULL
        OR motivo_cierre IN ('RECEPCION', 'RECHAZO', 'EVENTO_EXTRAORDINARIO')
    ),
    CONSTRAINT transfer_operations_cierre_temporal_valid CHECK (
        cerrada_en IS NULL OR cerrada_en >= despachada_en
    ),
    CONSTRAINT transfer_operations_recepcion_complete CHECK (
        (recepcion_numero_remito IS NULL AND recepcion_numero_factura IS NULL AND recepcion_cantidad IS NULL)
        OR
        (
            btrim(recepcion_numero_remito) <> ''
            AND btrim(recepcion_numero_factura) <> ''
            AND recepcion_cantidad > 0
        )
    ),
    CONSTRAINT transfer_operations_lifecycle_valid CHECK (
        (
            estado = 'ACTIVA'
            AND cerrada_en IS NULL
            AND motivo_cierre IS NULL
            AND recepcion_numero_remito IS NULL
            AND recepcion_numero_factura IS NULL
            AND recepcion_cantidad IS NULL
        )
        OR
        (
            estado = 'CERRADA'
            AND cerrada_en IS NOT NULL
            AND motivo_cierre = 'RECEPCION'
            AND recepcion_numero_remito IS NOT NULL
            AND recepcion_numero_factura IS NOT NULL
            AND recepcion_cantidad IS NOT NULL
        )
        OR
        (
            estado = 'CERRADA'
            AND cerrada_en IS NOT NULL
            AND motivo_cierre IN ('RECHAZO', 'EVENTO_EXTRAORDINARIO')
            AND recepcion_numero_remito IS NULL
            AND recepcion_numero_factura IS NULL
            AND recepcion_cantidad IS NULL
        )
    )
);

CREATE UNIQUE INDEX transfer_operations_one_active_per_unit_idx
    ON public.transfer_operations (gtin, numero_serie)
    WHERE estado = 'ACTIVA';

CREATE TABLE public.return_operations (
    gtin VARCHAR(14) NOT NULL,
    numero_serie VARCHAR(20) NOT NULL,
    tx_id_devolucion TEXT NOT NULL,
    receptor_declarado TEXT,
    motivo TEXT NOT NULL,
    event_timestamp TIMESTAMPTZ NOT NULL,
    CONSTRAINT return_operations_pk PRIMARY KEY (gtin, numero_serie, tx_id_devolucion),
    CONSTRAINT return_operations_unit_fk FOREIGN KEY (gtin, numero_serie)
        REFERENCES public.medication_units (gtin, numero_serie)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT return_operations_tx_id_not_blank CHECK (btrim(tx_id_devolucion) <> ''),
    CONSTRAINT return_operations_receptor_valid CHECK (
        receptor_declarado IS NULL OR receptor_declarado ~ '^(GLN|CUFE):[^:]+$'
    ),
    CONSTRAINT return_operations_motivo_not_blank CHECK (btrim(motivo) <> '')
);

CREATE FUNCTION public.reject_immutable_table_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION '% is append-only; % is not allowed', TG_TABLE_NAME, TG_OP
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER unit_events_reject_row_mutation
    BEFORE UPDATE OR DELETE ON public.unit_events
    FOR EACH ROW EXECUTE FUNCTION public.reject_immutable_table_mutation();

CREATE TRIGGER unit_events_reject_truncate
    BEFORE TRUNCATE ON public.unit_events
    FOR EACH STATEMENT EXECUTE FUNCTION public.reject_immutable_table_mutation();

CREATE TRIGGER return_operations_reject_row_mutation
    BEFORE UPDATE OR DELETE ON public.return_operations
    FOR EACH ROW EXECUTE FUNCTION public.reject_immutable_table_mutation();

CREATE TRIGGER return_operations_reject_truncate
    BEFORE TRUNCATE ON public.return_operations
    FOR EACH STATEMENT EXECUTE FUNCTION public.reject_immutable_table_mutation();
