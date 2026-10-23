CREATE INDEX medication_units_state_gtin_serial_idx
    ON public.medication_units (estado, gtin, numero_serie);
