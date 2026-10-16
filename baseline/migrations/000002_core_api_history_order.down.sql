DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM public.transfer_operations
         WHERE estado = 'CERRADA'
           AND motivo_cierre = 'RECEPCION'
           AND recepcion_numero_remito IS NULL
    ) THEN
        RAISE EXCEPTION 'cannot revert: receptions without commercial confirmation exist';
    END IF;
END;
$$;

ALTER TABLE public.transfer_operations
    DROP CONSTRAINT transfer_operations_lifecycle_valid,
    ADD CONSTRAINT transfer_operations_lifecycle_valid CHECK (
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
    );

DROP INDEX public.unit_events_history_idx;
ALTER TABLE public.unit_events
    DROP CONSTRAINT unit_events_sequence_unique,
    DROP COLUMN event_sequence;
CREATE INDEX unit_events_history_idx
    ON public.unit_events (gtin, numero_serie, event_timestamp, tx_id);
