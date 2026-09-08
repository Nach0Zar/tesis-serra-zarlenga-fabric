\set ON_ERROR_STOP on

DO $$
DECLARE
    actual_tables TEXT[];
BEGIN
    SELECT array_agg(table_name ORDER BY table_name)
      INTO actual_tables
      FROM information_schema.tables
     WHERE table_schema = 'public'
       AND table_type = 'BASE TABLE';

    IF actual_tables <> ARRAY[
        'lab_interventions',
        'medication_units',
        'organizations',
        'return_operations',
        'transfer_operations',
        'unit_events'
    ] THEN
        RAISE EXCEPTION 'unexpected public tables: %', actual_tables;
    END IF;
END;
$$;

DO $$
DECLARE
    actual_columns TEXT[];
BEGIN
    SELECT array_agg(column_name ORDER BY ordinal_position)
      INTO actual_columns
      FROM information_schema.columns
     WHERE table_schema = 'public' AND table_name = 'medication_units';
    IF actual_columns <> ARRAY[
        'gtin', 'numero_serie', 'lote', 'fecha_vencimiento',
        'custodio_actual', 'estado', 'ultima_actualizacion'
    ] THEN
        RAISE EXCEPTION 'medication_units does not mirror MedicationUnit: %', actual_columns;
    END IF;

    SELECT array_agg(column_name ORDER BY ordinal_position)
      INTO actual_columns
      FROM information_schema.columns
     WHERE table_schema = 'public' AND table_name = 'unit_events';
    IF actual_columns <> ARRAY[
        'gtin', 'numero_serie', 'tx_id', 'event_timestamp', 'operation',
        'invoker_msp_id', 'lote', 'fecha_vencimiento', 'custodio_actual',
        'estado', 'ultima_actualizacion', 'event_sequence'
    ] THEN
        RAISE EXCEPTION 'unit_events does not contain the complete snapshot: %', actual_columns;
    END IF;
END;
$$;

CREATE FUNCTION pg_temp.assert_table_signature(
    target_table REGCLASS,
    expected_signature TEXT[]
) RETURNS VOID
LANGUAGE plpgsql
AS $$
DECLARE
    actual_signature TEXT[];
BEGIN
    SELECT array_agg(
        format(
            '%s:%s:%s',
            attribute.attname,
            format_type(attribute.atttypid, attribute.atttypmod),
            CASE WHEN attribute.attnotnull THEN 'not null' ELSE 'nullable' END
        )
        ORDER BY attribute.attnum
    )
      INTO actual_signature
      FROM pg_attribute AS attribute
     WHERE attribute.attrelid = target_table
       AND attribute.attnum > 0
       AND NOT attribute.attisdropped;

    IF actual_signature IS DISTINCT FROM expected_signature THEN
        RAISE EXCEPTION 'unexpected signature for %: %', target_table, actual_signature;
    END IF;
END;
$$;

SELECT pg_temp.assert_table_signature('public.organizations', ARRAY[
    'msp_id:text:not null', 'id:text:not null', 'id_type:text:not null',
    'agent_type:text:not null', 'active:boolean:not null'
]);
SELECT pg_temp.assert_table_signature('public.medication_units', ARRAY[
    'gtin:character varying(14):not null',
    'numero_serie:character varying(20):not null', 'lote:text:not null',
    'fecha_vencimiento:date:not null', 'custodio_actual:text:not null',
    'estado:text:not null', 'ultima_actualizacion:timestamp with time zone:not null'
]);
SELECT pg_temp.assert_table_signature('public.lab_interventions', ARRAY[
    'gtin:character varying(14):not null',
    'numero_serie:character varying(20):not null', 'laboratorio:text:not null',
    'operacion:text:not null', 'motivo:text:not null',
    'expira_en:timestamp with time zone:not null', 'estado:text:not null',
    'emitida_por:text:not null', 'emitida_en:timestamp with time zone:not null',
    'consumida_en:timestamp with time zone:nullable',
    'revocada_en:timestamp with time zone:nullable',
    'motivo_revocacion:text:nullable'
]);
SELECT pg_temp.assert_table_signature('public.unit_events', ARRAY[
    'gtin:character varying(14):not null',
    'numero_serie:character varying(20):not null', 'tx_id:text:not null',
    'event_timestamp:timestamp with time zone:not null', 'operation:text:not null',
    'invoker_msp_id:text:not null', 'lote:text:not null',
    'fecha_vencimiento:date:not null', 'custodio_actual:text:not null',
    'estado:text:not null', 'ultima_actualizacion:timestamp with time zone:not null',
    'event_sequence:bigint:not null'
]);
SELECT pg_temp.assert_table_signature('public.transfer_operations', ARRAY[
    'gtin:character varying(14):not null',
    'numero_serie:character varying(20):not null', 'tx_id_despacho:text:not null',
    'emisor:text:not null', 'destinatario_pendiente:text:not null',
    'numero_remito:text:not null', 'numero_factura:text:not null',
    'cantidad:integer:not null', 'rule_id:text:not null',
    'schema_version:text:not null', 'despachada_en:timestamp with time zone:not null',
    'estado:text:not null', 'cerrada_en:timestamp with time zone:nullable',
    'motivo_cierre:text:nullable', 'recepcion_numero_remito:text:nullable',
    'recepcion_numero_factura:text:nullable', 'recepcion_cantidad:integer:nullable'
]);
SELECT pg_temp.assert_table_signature('public.return_operations', ARRAY[
    'gtin:character varying(14):not null',
    'numero_serie:character varying(20):not null',
    'tx_id_devolucion:text:not null', 'receptor_declarado:text:nullable',
    'motivo:text:not null', 'event_timestamp:timestamp with time zone:not null'
]);

DO $$
DECLARE
    primary_keys TEXT[];
    foreign_keys TEXT[];
    check_counts BIGINT[];
BEGIN
    SELECT array_agg(constraint_record.conname ORDER BY constraint_record.conname)
      INTO primary_keys
      FROM pg_constraint AS constraint_record
     WHERE constraint_record.contype = 'p'
       AND constraint_record.connamespace = 'public'::regnamespace;
    IF primary_keys <> ARRAY[
        'lab_interventions_pk', 'medication_units_pk', 'organizations_pkey',
        'return_operations_pk', 'transfer_operations_pk', 'unit_events_pk'
    ] THEN
        RAISE EXCEPTION 'unexpected primary keys: %', primary_keys;
    END IF;

    SELECT array_agg(constraint_record.conname ORDER BY constraint_record.conname)
      INTO foreign_keys
      FROM pg_constraint AS constraint_record
     WHERE constraint_record.contype = 'f'
       AND constraint_record.connamespace = 'public'::regnamespace;
    IF foreign_keys <> ARRAY[
        'lab_interventions_issuer_fk', 'lab_interventions_unit_fk',
        'return_operations_unit_fk', 'transfer_operations_unit_fk',
        'unit_events_invoker_fk', 'unit_events_unit_fk'
    ] THEN
        RAISE EXCEPTION 'unexpected foreign keys: %', foreign_keys;
    END IF;

    SELECT array_agg(table_checks.check_count ORDER BY table_checks.table_name)
      INTO check_counts
      FROM (
          SELECT relation.relname AS table_name, count(*) AS check_count
            FROM pg_constraint AS constraint_record
            JOIN pg_class AS relation ON relation.oid = constraint_record.conrelid
           WHERE constraint_record.contype = 'c'
             AND constraint_record.connamespace = 'public'::regnamespace
           GROUP BY relation.relname
      ) AS table_checks;
    IF check_counts <> ARRAY[9, 5, 5, 3, 12, 7]::BIGINT[] THEN
        RAISE EXCEPTION 'unexpected check constraint counts: %', check_counts;
    END IF;
END;
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes
         WHERE schemaname = 'public'
           AND tablename = 'medication_units'
           AND indexname = 'medication_units_gtin_idx'
    ) THEN
        RAISE EXCEPTION 'missing medication_units GTIN index';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes
         WHERE schemaname = 'public'
           AND tablename = 'transfer_operations'
           AND indexname = 'transfer_operations_one_active_per_unit_idx'
           AND indexdef LIKE '%WHERE (estado = %ACTIVA%'
    ) THEN
        RAISE EXCEPTION 'missing partial unique active-transfer index';
    END IF;

    IF (SELECT count(*) FROM pg_constraint
         WHERE contype = 'f'
           AND conrelid IN (
               'public.lab_interventions'::regclass,
               'public.unit_events'::regclass,
               'public.transfer_operations'::regclass,
               'public.return_operations'::regclass
           )) <> 6 THEN
        RAISE EXCEPTION 'expected six foreign keys on event and operation tables';
    END IF;

    IF EXISTS (
        SELECT 1 FROM pg_trigger
         WHERE tgrelid = 'public.unit_events'::regclass
           AND NOT tgisinternal
    ) THEN
        RAISE EXCEPTION 'unit_events append-only convention must not be enforced by database triggers';
    END IF;

    IF (SELECT count(*) FROM pg_trigger
         WHERE tgrelid = 'public.return_operations'::regclass
           AND NOT tgisinternal) <> 2 THEN
        RAISE EXCEPTION 'return_operations must reject row mutations and truncation';
    END IF;
END;
$$;

INSERT INTO public.organizations (msp_id, id, id_type, agent_type, active) VALUES
    ('LabMSP', '7791234500017', 'GLN', 'LABORATORY', TRUE),
    ('DistributorMSP', '7791234500024', 'GLN', 'DISTRIBUTOR', TRUE),
    ('LogisticsMSP', 'CUFE-LOG-01', 'CUFE', 'LOGISTICS_OPERATOR', TRUE),
    ('DrugstoreMSP', 'CUFE-DRUG-01', 'CUFE', 'DRUGSTORE', TRUE),
    ('FarmaciaMSP', '7791234500048', 'GLN', 'PHARMACY', TRUE),
    ('HealthcareMSP', 'CUFE-HOSP-01', 'CUFE', 'HEALTHCARE_FACILITY', TRUE),
    ('AnmatMSP', 'ANMAT', 'REG', 'REGULATOR', TRUE),
    ('FinanciadorMSP', 'INSSJP-PAMI', 'REG', 'FINANCIER', TRUE);

DO $$
BEGIN
    BEGIN
        INSERT INTO public.organizations (msp_id, id, id_type, agent_type, active)
        VALUES ('InvalidIdentityKindMSP', 'ANMAT-2', 'REG', 'PHARMACY', TRUE);
        RAISE EXCEPTION 'custodial organization accepted a REG identity';
    EXCEPTION WHEN check_violation THEN
        NULL;
    END;

    BEGIN
        INSERT INTO public.organizations (msp_id, id, id_type, agent_type, active)
        VALUES ('InvalidIdTypeMSP', '12345678', 'DNI', 'LABORATORY', TRUE);
        RAISE EXCEPTION 'organization accepted an unknown identity type';
    EXCEPTION WHEN check_violation THEN
        NULL;
    END;

    BEGIN
        INSERT INTO public.organizations (msp_id, id, id_type, agent_type, active)
        VALUES ('InvalidAgentMSP', '7791234500093', 'GLN', 'PATIENT', TRUE);
        RAISE EXCEPTION 'organization accepted an unknown agent type';
    EXCEPTION WHEN check_violation THEN
        NULL;
    END;
END;
$$;

INSERT INTO public.medication_units (
    gtin, numero_serie, lote, fecha_vencimiento,
    custodio_actual, estado, ultima_actualizacion
) VALUES (
    '07791234567898', 'SERIE-001', 'LOTE-001', DATE '2028-12-31',
    'GLN:7791234500017', 'EN_LABORATORIO', TIMESTAMPTZ '2026-09-06T12:00:00Z'
);

DO $$
BEGIN
    BEGIN
        INSERT INTO public.medication_units (
            gtin, numero_serie, lote, fecha_vencimiento,
            custodio_actual, estado, ultima_actualizacion
        ) VALUES (
            '07791234567898', 'SERIE-001', 'OTRO-LOTE', DATE '2028-12-31',
            'GLN:7791234500017', 'EN_LABORATORIO', clock_timestamp()
        );
        RAISE EXCEPTION 'duplicate GTIN and serial was accepted';
    EXCEPTION WHEN unique_violation THEN
        NULL;
    END;

    BEGIN
        INSERT INTO public.medication_units (
            gtin, numero_serie, lote, fecha_vencimiento,
            custodio_actual, estado, ultima_actualizacion
        ) VALUES (
            '07791234567898', 'SERIE-INVALIDA', 'LOTE-001', DATE '2028-12-31',
            'GLN:7791234500017', 'ESTADO_INEXISTENTE', clock_timestamp()
        );
        RAISE EXCEPTION 'invalid medication state was accepted';
    EXCEPTION WHEN check_violation THEN
        NULL;
    END;
END;
$$;

INSERT INTO public.lab_interventions (
    gtin, numero_serie, laboratorio, operacion, motivo, expira_en,
    estado, emitida_por, emitida_en
) VALUES (
    '07791234567898', 'SERIE-001', 'GLN:7791234500017',
    'WITHDRAW_FROM_MARKET', 'Retiro preventivo documentado',
    TIMESTAMPTZ '2026-10-01T12:00:00Z', 'ACTIVA', 'AnmatMSP',
    TIMESTAMPTZ '2026-09-06T12:01:00Z'
);

DO $$
BEGIN
    BEGIN
        UPDATE public.lab_interventions
           SET operacion = 'UNKNOWN_OPERATION'
         WHERE gtin = '07791234567898' AND numero_serie = 'SERIE-001';
        RAISE EXCEPTION 'invalid lab intervention operation was accepted';
    EXCEPTION WHEN check_violation THEN
        NULL;
    END;

    BEGIN
        UPDATE public.lab_interventions
           SET estado = 'UNKNOWN_STATE'
         WHERE gtin = '07791234567898' AND numero_serie = 'SERIE-001';
        RAISE EXCEPTION 'invalid lab intervention state was accepted';
    EXCEPTION WHEN check_violation THEN
        NULL;
    END;

    BEGIN
        UPDATE public.lab_interventions
           SET estado = 'CONSUMIDA'
         WHERE gtin = '07791234567898' AND numero_serie = 'SERIE-001';
        RAISE EXCEPTION 'consumed lab intervention without timestamp was accepted';
    EXCEPTION WHEN check_violation THEN
        NULL;
    END;
END;
$$;

UPDATE public.lab_interventions
   SET estado = 'CONSUMIDA',
       consumida_en = TIMESTAMPTZ '2026-09-06T12:03:00Z'
 WHERE gtin = '07791234567898'
   AND numero_serie = 'SERIE-001';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
          FROM public.lab_interventions
         WHERE gtin = '07791234567898'
           AND numero_serie = 'SERIE-001'
           AND laboratorio = 'GLN:7791234500017'
           AND operacion = 'WITHDRAW_FROM_MARKET'
           AND motivo = 'Retiro preventivo documentado'
           AND expira_en = TIMESTAMPTZ '2026-10-01T12:00:00Z'
           AND estado = 'CONSUMIDA'
           AND emitida_por = 'AnmatMSP'
           AND emitida_en = TIMESTAMPTZ '2026-09-06T12:01:00Z'
           AND consumida_en = TIMESTAMPTZ '2026-09-06T12:03:00Z'
           AND revocada_en IS NULL
           AND motivo_revocacion IS NULL
    ) THEN
        RAISE EXCEPTION 'lab intervention lifecycle was not preserved';
    END IF;
END;
$$;

INSERT INTO public.unit_events (
    gtin, numero_serie, tx_id, event_timestamp, operation, invoker_msp_id,
    lote, fecha_vencimiento, custodio_actual, estado, ultima_actualizacion,
    event_sequence
) VALUES (
    '07791234567898', 'SERIE-001', 'tx-register-001',
    TIMESTAMPTZ '2026-09-06T12:00:00Z', 'RegisterUnit', 'LabMSP',
    'LOTE-001', DATE '2028-12-31', 'GLN:7791234500017',
    'EN_LABORATORIO', TIMESTAMPTZ '2026-09-06T12:00:00Z', 1
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM public.unit_events
         WHERE tx_id = 'tx-register-001'
           AND event_timestamp = TIMESTAMPTZ '2026-09-06T12:00:00Z'
           AND operation = 'RegisterUnit'
           AND invoker_msp_id = 'LabMSP'
           AND gtin = '07791234567898'
           AND numero_serie = 'SERIE-001'
           AND lote = 'LOTE-001'
           AND fecha_vencimiento = DATE '2028-12-31'
           AND custodio_actual = 'GLN:7791234500017'
           AND estado = 'EN_LABORATORIO'
           AND ultima_actualizacion = TIMESTAMPTZ '2026-09-06T12:00:00Z'
    ) THEN
        RAISE EXCEPTION 'complete unit event snapshot was not preserved';
    END IF;
END;
$$;

DO $$
BEGIN
    IF (SELECT event_sequence FROM public.unit_events WHERE tx_id = 'tx-register-001') <> 1 THEN
        RAISE EXCEPTION 'migration did not backfill event_sequence';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes
         WHERE schemaname = 'public' AND indexname = 'unit_events_history_idx'
           AND indexdef LIKE '%(gtin, numero_serie, event_sequence)%'
    ) THEN
        RAISE EXCEPTION 'history index does not use event_sequence';
    END IF;
END;
$$;

INSERT INTO public.transfer_operations (
    gtin, numero_serie, tx_id_despacho, emisor, destinatario_pendiente,
    numero_remito, numero_factura, cantidad, rule_id, schema_version,
    despachada_en, estado
) VALUES (
    '07791234567898', 'SERIE-001', 'tx-dispatch-001',
    'GLN:7791234500017', 'GLN:7791234500048',
    'R-001', 'F-001', 1, 'R-LAB-PHARMACY', '1.0.0',
    TIMESTAMPTZ '2026-09-06T12:05:00Z', 'ACTIVA'
);

DO $$
BEGIN
    BEGIN
        INSERT INTO public.transfer_operations (
            gtin, numero_serie, tx_id_despacho, emisor, destinatario_pendiente,
            numero_remito, numero_factura, cantidad, rule_id, schema_version,
            despachada_en, estado
        ) VALUES (
            '07791234567898', 'SERIE-001', 'tx-dispatch-duplicate-active',
            'GLN:7791234500017', 'GLN:7791234500048',
            'R-002', 'F-002', 1, 'R-LAB-PHARMACY', '1.0.0',
            TIMESTAMPTZ '2026-09-06T12:06:00Z', 'ACTIVA'
        );
        RAISE EXCEPTION 'second active transfer was accepted';
    EXCEPTION WHEN unique_violation THEN
        NULL;
    END;
END;
$$;

UPDATE public.transfer_operations
   SET estado = 'CERRADA',
       cerrada_en = TIMESTAMPTZ '2026-09-06T12:10:00Z',
       motivo_cierre = 'RECEPCION',
       recepcion_numero_remito = 'R-001',
       recepcion_numero_factura = 'F-001',
       recepcion_cantidad = 1
 WHERE gtin = '07791234567898'
   AND numero_serie = 'SERIE-001'
   AND tx_id_despacho = 'tx-dispatch-001';

UPDATE public.transfer_operations
   SET recepcion_numero_remito = NULL,
       recepcion_numero_factura = NULL,
       recepcion_cantidad = NULL
 WHERE gtin = '07791234567898'
   AND numero_serie = 'SERIE-001'
   AND tx_id_despacho = 'tx-dispatch-001';

-- Dejar la fila compatible con la migracion down para que el test de
-- reversibilidad pueda restaurar la restriccion anterior.
UPDATE public.transfer_operations
   SET recepcion_numero_remito = 'R-001',
       recepcion_numero_factura = 'F-001',
       recepcion_cantidad = 1
 WHERE gtin = '07791234567898'
   AND numero_serie = 'SERIE-001'
   AND tx_id_despacho = 'tx-dispatch-001';

INSERT INTO public.transfer_operations (
    gtin, numero_serie, tx_id_despacho, emisor, destinatario_pendiente,
    numero_remito, numero_factura, cantidad, rule_id, schema_version,
    despachada_en, estado
) VALUES (
    '07791234567898', 'SERIE-001', 'tx-dispatch-002',
    'GLN:7791234500048', 'GLN:7791234500017',
    'R-003', 'F-003', 1, 'R-PHARMACY-LAB', '1.0.0',
    TIMESTAMPTZ '2026-09-06T12:15:00Z', 'ACTIVA'
);

INSERT INTO public.return_operations (
    gtin, numero_serie, tx_id_devolucion, receptor_declarado, motivo, event_timestamp
) VALUES (
    '07791234567898', 'SERIE-001', 'tx-return-001', NULL,
    'Devolución documentada sin receptor privado',
    TIMESTAMPTZ '2026-09-06T12:20:00Z'
);

DO $$
BEGIN
    BEGIN
        UPDATE public.return_operations SET motivo = 'tampered';
        RAISE EXCEPTION 'return_operations UPDATE was accepted';
    EXCEPTION WHEN SQLSTATE '55000' THEN
        NULL;
    END;

    BEGIN
        DELETE FROM public.return_operations;
        RAISE EXCEPTION 'return_operations DELETE was accepted';
    EXCEPTION WHEN SQLSTATE '55000' THEN
        NULL;
    END;

    BEGIN
        TRUNCATE public.return_operations;
        RAISE EXCEPTION 'return_operations TRUNCATE was accepted';
    EXCEPTION WHEN SQLSTATE '55000' THEN
        NULL;
    END;
END;
$$;
