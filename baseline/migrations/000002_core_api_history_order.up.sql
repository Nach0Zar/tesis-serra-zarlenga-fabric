ALTER TABLE public.unit_events
    ADD COLUMN event_sequence BIGINT;

WITH ordered AS (
    SELECT gtin, numero_serie, tx_id,
           row_number() OVER (
               PARTITION BY gtin, numero_serie
               ORDER BY event_timestamp, tx_id
           ) AS sequence
      FROM public.unit_events
)
UPDATE public.unit_events AS event
   SET event_sequence = ordered.sequence
  FROM ordered
 WHERE event.gtin = ordered.gtin
   AND event.numero_serie = ordered.numero_serie
   AND event.tx_id = ordered.tx_id;

ALTER TABLE public.unit_events
    ALTER COLUMN event_sequence SET NOT NULL,
    ADD CONSTRAINT unit_events_sequence_unique
        UNIQUE (gtin, numero_serie, event_sequence);

DROP INDEX public.unit_events_history_idx;
CREATE INDEX unit_events_history_idx
    ON public.unit_events (gtin, numero_serie, event_sequence);

-- ReceiveTransfer admite una confirmacion documental opcional. La restriccion
-- recepcion_complete ya exige que, cuando se informe, el trio sea completo.
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
