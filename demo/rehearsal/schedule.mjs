// Cronograma del guion de docs/defense-demo.md y criterio para dar un ensayo
// por correcto.

// Segundo en el que empieza cada paso y fin del guion.
export const SCHEDULE = [20, 40, 65, 85, 110, 125, 150];
export const END = 180;
// Margen del total sobre END, por el último tick del reloj.
export const TOTAL_TOLERANCE = 0.5;

// stepTiming mide un paso desde que se ejecuta hasta que su evidencia quedó
// verificada: un paso cuya alerta o bloque llega tarde no entra en su
// ventana aunque la operación haya terminado a tiempo.
export function stepTiming(index, start, evidenceVerifiedAt) {
  const windowEnd = SCHEDULE[index + 1] ?? END;
  return {
    duration: Number((evidenceVerifiedAt - start).toFixed(1)),
    windowEnd,
    fitsWindow: evidenceVerifiedAt <= windowEnd,
  };
}

// rehearsalPassed exige los siete pasos con evidencia y en su ventana, una
// pantalla sin errores, un total que no pase de END y, si se grabó, un video
// de la duración del guion. Vale igual para npm run record y npm run time.
export function rehearsalPassed({ steps, total, pageErrors, videoOk = true }) {
  return (
    steps.length === SCHEDULE.length &&
    steps.every((step) => step.ok && step.fitsWindow) &&
    pageErrors.length === 0 &&
    total <= END + TOTAL_TOLERANCE &&
    videoOk
  );
}
