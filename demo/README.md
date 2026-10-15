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
| `demo.sh up` con todo ya levantado | inmediato | — |
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

## Si algo falla en vivo

| Síntoma | Qué hacer |
|---|---|
| «sin conexión con la red» | `demo/demo.sh up` vuelve a levantar lo que falte; la pantalla se reconecta sola |
| «No se pudo preparar el escenario» | «Reiniciar» o `demo/demo.sh reset` |
| Un paso termina con error | «Reiniciar» y retomar el guion desde el paso 1; los pasos 1 a 7 tardan unos 20 s en total |
| La pantalla no carga | `demo/demo.sh status`, y si el servidor está detenido, `demo/demo.sh up` |
| Nada de lo anterior alcanza | Pasar al video de respaldo |
