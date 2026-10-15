# Demo de la defensa

Arranque en un comando de la demo visual del prototipo Fabric (DEMO-4, #146).
Esto pone en marcha la demo; qué se cuenta y en qué orden está en el guion,
[`docs/defense-demo.md`](../docs/defense-demo.md). El servidor y la pantalla se
documentan en [`client/README.md`](../client/README.md#servidor-de-la-demo-de-la-defensa).

## Requisitos

- Docker Desktop encendido, con los recursos de
  [`network/wslconfig.example`](../network/wslconfig.example) (6 CPU y 8 GB).
- Binarios de Fabric 2.5.16 y su `config/`, exportados como en
  [`network/README.md`](../network/README.md):

  ```bash
  export SNT_FABRIC_BIN_DIR=/ruta/a/fabric/bin
  export SNT_FABRIC_CFG_PATH=/ruta/a/fabric/config
  ```

- Go, `jq` y `curl`.
- El material criptográfico de `network/organizations/` ya generado.

## Comandos

Desde la raíz del repositorio:

| Comando | Qué hace |
|---|---|
| `demo/demo.sh check` | Checklist previo: Docker, recursos, binarios, herramientas, puerto libre y `network.sh verify` si la red está levantada |
| `demo/demo.sh up` | Levanta la red, crea el canal y despliega `snt` si hace falta, verifica, compila `snt-client`, inicia `demo-server` en segundo plano y espera a que el escenario esté listo |
| `demo/demo.sh up --open` | Lo mismo, y abre la pantalla en el navegador |
| `demo/demo.sh reset` | Empieza un ensayo nuevo: series nuevas y la unidad de robo preparada en la farmacia |
| `demo/demo.sh status` | Muestra si el servidor corre, si la red está en línea y en qué paso está el guion |
| `demo/demo.sh stop` | Detiene `demo-server`; la red sigue levantada |

La pantalla queda en `http://127.0.0.1:8080/`. Otro puerto loopback se elige con
`SNT_DEMO_ADDR=127.0.0.1:9090`; una dirección que no sea loopback se rechaza
antes de levantar nada.

`up` levanta y verifica la red **siempre**, también si `demo-server` ya está
corriendo, y sólo anuncia «Demo lista» cuando el escenario está preparado **y**
el servidor está conectado a la red. Por eso es también el comando para
recuperar la demo si la red se cae. Si la preparación había fallado mientras la
red estaba caída, la reintenta.

`up` delega en `network/network.sh`, que es idempotente y **conserva el ledger**:
no borra volúmenes. Cada ensayo es reproducible igual, porque cada reinicio usa
series nuevas y la pantalla muestra sólo los bloques del ensayo en curso. El
servidor escribe su log en `build/demo/demo-server.log`.

## Tiempos medidos

Medidos en la notebook de desarrollo (WSL2, 6 CPU, 8 GB), sobre un ledger con el
snapshot de 50.000 unidades:

| Situación | Tiempo | Criterio de #146 |
|---|---|---|
| `demo.sh up` con la red detenida (`network.sh down`) | 48 s | menos de 5 minutos |
| `demo.sh up` con todo ya levantado | 25 s (vuelve a verificar la red) | — |
| `network.sh down` con `demo-server` vivo → `demo.sh up` | 32 s, hasta que el servidor reconecta | — |
| `demo.sh reset` o el botón «Reiniciar» | 10–12 s | menos de 1 minuto |

## Sin internet

La demo no usa internet durante la defensa: la pantalla viaja dentro del binario
y la red corre en Docker local. Lo único que necesita conexión es la **primera**
compilación, para bajar los módulos de Go y las imágenes de Docker. Por eso hay
que correr `demo/demo.sh up` al menos una vez con conexión en la notebook de la
defensa, y repetir el ensayo completo con el Wi-Fi apagado.

## Checklist del día de la defensa

1. Encender Docker Desktop y esperar a que esté listo.
2. `demo/demo.sh check`: sin `[FALLA]`.
3. `demo/demo.sh up --open`.
4. En el navegador, pantalla completa (F11) y zoom en 100 %.
5. Hacer un paso de prueba y `demo/demo.sh reset`, para arrancar limpio.
6. Confirmar que la pantalla dice «red en línea» y «Listo para empezar».
7. Tener abierto el video de respaldo de DEMO-5 (#147).

## Ensayos y video de respaldo

`demo/rehearsal/` recorre el guion sobre la pantalla real, iniciando cada paso en
el segundo que le asigna el guion (20, 40, 65, 85, 110, 125 y 150 s), y graba el
video de los 3 minutos. Es el plan B que pide DOC-8 (#60): el video es mudo y se
narra en vivo con las mismas frases que la demo.

Con la demo levantada, la primera vez y con conexión:

```bash
cd demo/rehearsal
npm ci
npx playwright install chromium
```

Después:

```bash
npm run record   # graba el video y el registro de tiempos
npm run time     # sólo cronometra, sin video
```

Deja en `build/demo/rehearsal/` el video (`demo-<fecha>.webm`, entre 45 y 80 MB
a 1920 × 1080) y el registro (`ensayo-<fecha>.json`) con el inicio y la duración
de cada paso, y si entró en su ventana. Termina con error si un paso falla, no
entra en su ventana o la pantalla registra un error. El video no se versiona:
se guarda fuera del repositorio y se prueba en la notebook de la defensa, sin
conexión. En WSL, Chromium headless puede necesitar `libasound2`
(`sudo npx playwright install-deps chromium`).

### Registro de ensayos

DEMO-5 pide tres ensayos completos de 3 minutos o menos, con narración, uno de
ellos en la notebook y con el proyector de la defensa.

| Fecha | Máquina y pantalla | Maneja / narra | Duración | Observaciones |
|---|---|---|---|---|
| 2026-10-15 | Notebook de desarrollo, headless 1920 × 1080 | automático, sin narración | 3:00 | Ensayo técnico con `npm run time` y `npm run record`: cada paso empezó en su segundo y entró en su ventana; las transferencias tardan 4,2–5,7 s. No cuenta como ensayo de DEMO-5 |
| | | | | |
| | | | | |
| | | | | |

## Si algo falla en vivo

| Síntoma | Qué hacer |
|---|---|
| «sin conexión con la red» | `demo/demo.sh up` vuelve a levantar lo que falte; la pantalla se reconecta sola |
| «No se pudo preparar el escenario» | «Reiniciar» o `demo/demo.sh reset` |
| Un paso termina con error | «Reiniciar» y retomar el guion desde el paso 1; los pasos 1 a 7 tardan unos 20 s en total |
| La pantalla no carga | `demo/demo.sh status`, y si el servidor está detenido, `demo/demo.sh up` |
| Nada de lo anterior alcanza | Pasar al video de respaldo |
