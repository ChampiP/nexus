# Arquitectura de Nexus

> Estado: propuesta v2 · 2026-10-03 · Versión original en inglés: [ARCHITECTURE.md](ARCHITECTURE.md).
> Si las dos versiones difieren, manda la inglesa.

Nexus es un **centro de control personal** que funciona en tu propia máquina, pensado para quien
trabaja todo el día frente a una computadora con Linux. Registra horas y, más adelante, muestra y
actúa sobre el trabajo que vive en otras herramientas (Notion, Google Calendar, Gmail, Obsidian,
Trello). Así, mirar la barra reemplaza abrir una ventana por cada herramienta.

Primero debe servirle a su autor, y después a cualquiera que lo instale desde un repositorio
público. Ese segundo objetivo define este documento: **seguro por defecto, local por defecto, y
nada expuesto a internet salvo que el usuario lo active explícitamente**.

---

## 1. Principios

1. **Local primero.** Todos los datos viven en un archivo SQLite en la máquina del usuario. No hay nube de Nexus ni cuentas.
2. **Funciona sin conexión y se degrada con elegancia.** Registrar horas nunca depende de la red, del daemon ni de una integración.
3. **Un núcleo, muchas caras.** La CLI, el TUI, el popup de la barra y los asistentes de IA son adaptadores sobre los mismos casos de uso.
4. **Mínimo privilegio.** Cada integración empieza en solo lectura y pide el permiso más pequeño que funcione.
5. **El humano aprueba los efectos.** Todo lo que cambie un sistema externo, o lo que pida una IA, necesita aprobación explícita dentro de Nexus.
6. **Tecnología aburrida.** Go, SQLite, servicios de usuario de systemd, notificaciones freedesktop y el llavero del sistema.
7. **Cambios reversibles.** Las migraciones del esquema solo agregan y se respaldan; los pasos destructivos van aparte y de forma explícita.
8. **Sin sobreingeniería.** Construir lo más simple que funcione hoy para un usuario; agregar una capa solo cuando aparezca una necesidad real. Rápido y bonito vale más que completo.

## 2. Investigación de viabilidad (2026-10-03)

Cada fila se verificó contra fuentes primarias, no solo contra resúmenes de búsqueda. Cuando las
fuentes no coincidían, la tabla se queda con la fuente primaria.

| Pregunta | Hallazgo | Consecuencia para Nexus |
|---|---|---|
| ¿Se puede publicar como plugin público de Omarchy? | Sí. `omarchy plugin add <url-git>` clona **un repositorio git completo que tenga `manifest.json` en la raíz** dentro de `~/.config/omarchy/plugins/<id>/`. El instalador nunca compila ni ejecuta scripts. Los plugins corren **sin aislamiento** dentro de `omarchy-shell`. Existe un índice comunitario (plugins.omarchy.org) y más de 1.100 repositorios usan el tema de GitHub `omarchy-plugin`. | El plugin de la barra necesita **su propio repositorio** (el código Go no debe clonarse dentro de la carpeta de plugins del shell). El binario `nexus` se instala aparte. El QML debe ser pequeño y no guardar secretos. |
| ¿Puede ChatGPT web usar un servidor MCP de Nexus? | ChatGPT web solo llega a servidores MCP por **HTTPS público**; no puede llegar a `localhost`. La documentación de OpenAI describe el modo desarrollador con escritura como beta para Business/Enterprise/Edu, pero **el autor verificó el 2026-10-03 que una cuenta ChatGPT Plus se conecta a un servidor MCP propio expuesto con una URL de ngrok y usa sus herramientas**. OpenAI también ofrece **Secure MCP Tunnel** (`openai/tunnel-client`). | Es viable con el plan del autor: un servidor MCP HTTP local expuesto con un túnel y protegido por un token secreto en la URL (§11.3). El MCP local por stdio sigue disponible para agentes de escritorio. |
| ¿Qué versión de MCP? | La especificación vigente es **2026-07-28**: núcleo sin estado (sin `Mcp-Session-Id`), transportes stdio y Streamable HTTP, autorización basada en OAuth 2.1 + Protected Resource Metadata (RFC 9728) + resource indicators (RFC 8707). SDK oficial de Go: `github.com/modelcontextprotocol/go-sdk`. | Usar el SDK oficial de Go. Modelar el estado como identificadores explícitos en los argumentos de las herramientas, nunca como estado de la conexión. |
| ¿Gmail y Google Calendar en una app pública? | Los permisos de lectura/modificación de Gmail son **restringidos**. Una app pública con un cliente OAuth compartido necesitaría verificación de Google más una auditoría de seguridad **CASA** anual y de pago. Los permisos de Calendar son **sensibles** (verificación, sin CASA). La salida habitual para apps locales de código abierto es **"trae tu propio cliente"** (BYOC): cada usuario crea su propio proyecto de Google Cloud y un cliente OAuth de tipo "Desktop app". Si la pantalla de consentimiento queda en *Testing*, los tokens de actualización vencen a los 7 días; publicarla *In production* sin verificar lo evita (el usuario ve el aviso de "app no verificada"). | Los conectores de Google usan **BYOC**, nunca un client ID incluido en Nexus. Redirección a loopback + PKCE. Se entrega una guía de configuración. |
| ¿Notion? | Un **token de integración interna** por espacio de trabajo (el usuario le comparte las bases de datos). La versión **2025-09-03** de la API reemplazó las consultas de bases de datos por **data sources** (`data_source_id`). El límite es de unas 3 solicitudes por segundo. Hay webhooks, pero necesitan un endpoint público. | Token en el llavero; fijar `Notion-Version`; cliente con "token bucket"; consultas periódicas en lugar de webhooks (no hay endpoint público). |
| ¿Obsidian? | Una bóveda es una carpeta de archivos Markdown. | Adaptador de archivos con escrituras atómicas; sin API ni plugin. |
| ¿Trello? | Clave de API + token de usuario para herramientas personales. | Mismo puerto de conector; prioridad baja. |
| ¿Secretos en este escritorio? | `gnome-keyring` ofrece `org.freedesktop.secrets`. | Guardar los secretos con Secret Service (llavero). Nunca en SQLite, archivos de configuración ni el repositorio. |
| ¿Botones en las notificaciones? | El servidor de notificaciones de Omarchy anuncia `actions`. | "Volver al trabajo / +10 min / Terminar break" pueden ser botones en la propia notificación. |
| ¿El nombre `nexus` está libre? El nombre exacto `nexus` está libre en AUR y en los repositorios oficiales, pero `nexus-bin` y `nexus-cli` (sin relación, 0 y 1 votos) también instalan `/usr/bin/nexus`, y `nexus-bin` declara `provides=('nexus')`. | Paquete `nexus`, declarando `conflicts` con ambos (ver §13). |

## 3. Contexto del sistema

```
                    ┌───────────────────────────── máquina del usuario ────────────────────────────┐
                    │                                                                              │
 Terminal  ───────► │  nexus (CLI / TUI)  ──┐                                                      │
 Barra Omarchy ───► │  plugin barra (QML) ──┼──►  Núcleo Nexus (casos de uso)  ──►  SQLite (nexus.db)│
 Cliente IA (stdio)►│  nexus mcp ───────────┘            ▲                                         │
                    │                                    │                                         │
                    │  nexus daemon (systemd --user) ────┘  avisos · sincronización · outbox       │
                    │        │                                                                     │
                    └────────┼─────────────────────────────────────────────────────────────────────┘
                             ▼ HTTPS, credenciales propias del usuario (llavero)
              Notion · Google Calendar · Gmail · Trello          Bóveda Obsidian (archivos locales)
```

Opcional y apagado por defecto: endpoint MCP remoto para ChatGPT web (§11.3).

## 4. Estilo arquitectónico

**Monolito modular con módulos hexagonales (puertos y adaptadores).**

- Un repositorio, un binario de Go (`nexus`) con subcomandos y una base de datos.
- El código se divide en **módulos (contextos acotados)**. Cada módulo es dueño de sus tipos de dominio, casos de uso, puertos y tablas.
- Las dependencias apuntan hacia adentro: adaptadores → casos de uso → dominio. Los adaptadores implementan los puertos que define el módulo.
- Los módulos se hablan solo a través de la interfaz de servicio pública del otro módulo o de eventos de dominio, **nunca leyendo sus tablas**.
- La estructura actual es por capas (`internal/domain`, `internal/app`, `internal/store`). Pasa a **paquete por módulo** cuando llegue el catálogo (el primer módulo nuevo), para que cada capacidad nueva agregue una carpeta en vez de tocar todas las capas.

Se descartan los microservicios: un usuario, una máquina, una base de datos. Aun así, un módulo se
puede extraer más adelante porque su frontera ya es un puerto.

## 5. Módulos

| Módulo | Responsabilidad | Depende de |
|---|---|---|
| `tracking` | Entradas de tiempo: iniciar, detener, editar, eliminar (con papelera), reportes. Tipo (`kind`): `work` o `break`. | `catalog` (búsqueda de proyecto) |
| `catalog` | Organizaciones → clientes → proyectos. Crear, renombrar, unir, mover, archivar. | — |
| `countdown` | Cuentas regresivas (breaks, bloques de foco) con hora de fin y estado de "pasado de tiempo". | `tracking` |
| `wellbeing` | Avisos de movimiento y postura según el tiempo de trabajo continuo y la inactividad. | `tracking` |
| `integrations` | Un conector por sistema externo detrás de un único puerto; motor de sincronización (inbox, outbox, enlaces). | `catalog`, `tracking` |
| `agenda` | Modelo de lectura para la vista "de un vistazo": eventos de hoy, tareas, correos sin leer. | `integrations` |
| `approvals` | Acciones pendientes que necesitan una decisión humana (pedidos de IA, escrituras externas). | — |
| `automation` *(tardío, opcional)* | Automatización de navegador en los sitios web propios del usuario. Proceso aparte. | `approvals` |

Código de plataforma compartido (no es un módulo y no tiene reglas de negocio): base de datos y
migraciones, reloj, configuración, llavero, notificaciones, logs, idiomas.

## 6. Estructura del código (objetivo)

```
cmd/nexus/                 raíz de composición: conecta módulos y adaptadores, nada más
internal/
  platform/
    db/                    apertura de SQLite, pragmas, migraciones por módulo, respaldo
    clock/ config/ keyring/ notify/ i18n/ log/
  tracking/
    domain.go              entidades, invariantes, errores de dominio
    service.go             casos de uso
    ports.go               interfaces que el módulo necesita
    sqlite.go              adaptador del repositorio
    *_test.go
  catalog/  countdown/  wellbeing/  approvals/  agenda/
  integrations/
    port.go                interfaz Connector, capacidades, tipo Change
    sync/                  inbox, outbox, planificador, tabla de enlaces
    notion/  gcal/  gmail/  obsidian/  trello/
  adapters/
    cli/                   comandos, presentadores, contrato JSON
    tui/                   interfaz Bubble Tea
    mcp/                   servidor MCP (stdio, luego Streamable HTTP)
    daemon/                bucle de larga duración, socket local
plugin/                    copia de desarrollo del plugin de la barra (se publica en su propio repositorio)
docs/
```

Un **test de arquitectura** (grafo de imports con `go list`) rompe el build si un módulo importa
el interior de otro, si `domain.go` importa algo distinto de la biblioteca estándar, o si un
adaptador importa otro adaptador.

## 7. Modelo de ejecución

| Proceso | Duración | Rol |
|---|---|---|
| `nexus <comando>` | una ejecución | CLI. Escribe SQLite directamente. |
| `nexus` (sin argumentos) | interactivo | TUI. Escribe SQLite directamente. |
| plugin de la barra | dentro de `omarchy-shell` | Llama a la CLI con arreglos de argumentos y lee su JSON versionado. Nunca toca SQLite. |
| `nexus daemon` | servicio `systemd --user` | Horas límite (fin del break, avisos de movimiento), notificaciones con botones, sincronización y envío del outbox, aprobaciones, MCP opcional por HTTP. |
| `nexus mcp` | lo inicia el cliente de IA | MCP por stdio. Mismos casos de uso, misma base de datos. |

Reglas:

- **El tracker funciona sin el daemon.** El estado "pasado de tiempo" se calcula a partir de las
  horas límite guardadas, así que la barra y el TUI siguen mostrando "break +12 min" aunque el
  daemon esté caído; solo se pierde la notificación.
- SQLite en modo WAL con tiempo de espera resuelve los pocos escritores simultáneos.
- Más adelante, el daemon expone un socket Unix local (permisos `0600`) para que las interfaces
  reciban cambios sin consultar cada pocos segundos. La consulta periódica queda como respaldo.

## 8. Arquitectura de datos

- **Un archivo:** `$XDG_DATA_HOME/nexus/nexus.db`, carpeta `0700`, archivo `0600`.
- **Migraciones por módulo:** tabla `schema_migrations(module, version, applied_at)`. Cada paso es
  transaccional e idempotente. **Antes de la primera migración pendiente** se escribe un respaldo
  con `VACUUM INTO nexus.db.bak-<fecha>`.
- **Expandir → migrar → verificar → contraer.** Los pasos de contracción (borrar columnas) van en
  una versión aparte y con una decisión explícita.
- **Identificadores:** claves enteras dentro de la base, más un `uid` ULID para toda entidad que
  sale del proceso (MCP, enlaces externos, exportaciones), para que los ids no se adivinen ni se confundan.
- **Tiempo:** se guarda como segundos Unix en UTC; la zona horaria se aplica solo al mostrar.
- **Eliminación:** los datos del usuario se **eliminan con papelera** (`deleted_at`) y se purgan
  tras un período de retención, así un borrado por error se puede deshacer.
- **Eventos y outbox:** los casos de uso agregan eventos de dominio (tabla `events`) en la misma
  transacción que el cambio. De ahí leen el outbox de los conectores y el registro de auditoría.
- **Identidad externa:** `external_links(local_type, local_uid, system, external_id, etag, synced_at)`.
  Nexus nunca empareja objetos externos por nombre.

### Modelo de dominio

```
Organización (p. ej. Holinsys, Emana)
 └─ Cliente (p. ej. Depilab)
     └─ Proyecto (p. ej. Depiloto, Lumirecon)
         └─ Tarea (task_uid)  { título, descripción }
             └─ Sesión (fila de entries) { kind: work|break, started_at, ended_at, deleted_at }

Countdown { kind: break|focus|custom, etiqueta, duración, started_at, ends_at, finished_at,
            entry_uid, resume_entry_uids[] }
Reminder  { regla, next_at, last_done_at }        PendingAction { requested_by, payload, status }
```

Una tarea agrupa sus sesiones con `entries.task_uid`: pausar y reanudar agrega una sesión a la misma
tarea, y las listas muestran una fila por tarea con su total. Un proyecto puede no tener cliente (proyectos personales) y un cliente puede no tener
organización. El campo de texto libre actual `entries.project` se migra a `projects` y se conserva
como columna de compatibilidad durante una versión (ver el plan del catálogo en
`odd/tasks/nexus-catalog.md`).

## 9. Breaks y bienestar

**Cuenta regresiva de break.** "Break de 1 h" inicia una entrada `break` y un `Countdown`. Si hay
temporizadores de trabajo corriendo, Nexus **pregunta cada vez** cuáles detener (todos vienen
marcados); los detenidos se recuerdan en `resume_entry_uids`. Cuando pasa la hora límite:

1. El daemon envía una notificación con botones: **Volver al trabajo** (termina el break y
   reinicia los temporizadores recordados), **+10 min**, **Terminar break**.
2. La barra muestra el tiempo excedido (`break +12m`) en color de alerta hasta que el usuario actúe.
3. Nexus nunca reinicia los temporizadores de trabajo por su cuenta; puede que el usuario aún no haya vuelto.

**Avisos de movimiento.** Una regla como "después de 50 minutos de trabajo continuo, sugiere
moverte 5 minutos". Solo cuenta tiempo mientras corre un temporizador de trabajo y el usuario no
está inactivo. Botones: **Hecho**, **En 10 min**, **Saltar**. Los avisos cumplidos se registran
para un resumen semanal. De dónde sale la señal de inactividad (el servicio de inactividad de
Omarchy o el protocolo de inactividad de Wayland) es una pregunta abierta.

## 10. Integraciones

### Puerto de conector

```go
type Connector interface {
    ID() string
    Capabilities() Capabilities               // ReadTasks, WriteTasks, ReadEvents, ...
    Pull(ctx context.Context, cursor Cursor) ([]Change, Cursor, error)
    Push(ctx context.Context, change Change) (ExternalRef, error)  // solo si tiene una capacidad de escritura
}
```

Un conector falso sirve para los tests de sincronización. Cada conector real vive en su propio
paquete y se registra en la raíz de composición.

### Reglas de sincronización

1. **Una fuente de verdad por campo.** Tiempos: Nexus. Tareas: Notion (o Trello). Eventos: Google
   Calendar. Correo: Gmail. Los conflictos se resuelven con esta regla, nunca con "gana la última escritura".
2. **Primero solo lectura.** Cada conector sale en solo lectura; escribir es un paso aparte y posterior.
3. **Outbox.** Las escrituras van al outbox en la misma transacción que el cambio local; el daemon
   las entrega con reintentos, espera progresiva y claves de idempotencia.
4. **Límites de uso** aplicados en el cliente ("token bucket" por conector), respetando `Retry-After`.
5. **Sin webhooks públicos** por defecto: los conectores consultan con cursores.

### Notas por sistema

| Sistema | Autenticación | Notas |
|---|---|---|
| Google Calendar | Cliente OAuth Desktop propio (BYOC), redirección a loopback, PKCE | Permiso sensible. Empezar con `calendar.readonly`. |
| Gmail | BYOC, mismo flujo | Permiso restringido. Empezar con metadatos o solo lectura y mostrar solo asunto y remitente. El texto de los correos es **entrada no confiable** (§12). |
| Notion | Token de integración interna | Fijar `Notion-Version: 2025-09-03` o posterior; usar `data_source_id`; ~3 solicitudes/s. |
| Obsidian | Ruta de una carpeta local | Leer tareas y front matter; escrituras atómicas; nunca borrar archivos del usuario. |
| Trello | Clave de API + token | Mismo puerto que Notion para datos de kanban. |

## 11. Acceso de IA (MCP)

### 11.1 Local, primero

`nexus mcp` habla MCP por **stdio** con el SDK oficial de Go. El cliente de IA lo inicia como
proceso hijo; nada escucha en la red. Hoy funciona con Codex CLI, Claude Code y otros clientes de escritorio.

Los agentes que tienen una terminal (Claude Code o Codex en otra ventana) ya pueden manejar Nexus
con la CLI y su contrato `--json`. MCP agrega herramientas tipadas y la política por niveles de
abajo, así que es la puerta preferida para los clientes de IA.

### 11.2 Diseño de herramientas

| Nivel | Ejemplos | Política |
|---|---|---|
| Lectura | `list_running`, `report`, `list_projects`, `agenda_today` | Permitido. |
| Escritura (local) | `start_timer`, `stop_timer`, `start_break` | Permitido en modo local; configurable. |
| Escritura (externa) | `move_notion_card`, `create_event` | Crea una **PendingAction**; el usuario la aprueba en Nexus (botón de notificación, popup o TUI). |
| Destructivo | borrar entradas, borrar proyectos, enviar correo | **No se expone** a clientes de IA. |

Los resultados que contienen texto de terceros (correos, descripciones de tareas) se devuelven
marcados claramente como datos, recortados y sin formato. Cada llamada MCP queda en el registro de auditoría.

### 11.3 Remoto (ChatGPT web), opcional

Camino verificado: ChatGPT se conecta a una URL MCP expuesta con un túnel. Nexus lo mantiene simple:

```
nexus mcp serve                                   # Streamable HTTP en 127.0.0.1, imprime la URL para pegar en ChatGPT
cloudflared tunnel --url http://127.0.0.1:<puerto> # túnel rápido de Cloudflare, sin cuenta
```

- **Apagado por defecto.** `nexus mcp serve` solo escucha en `127.0.0.1`. El túnel lo elige el
  usuario: túnel rápido de Cloudflare (sin iniciar sesión; URL aleatoria `*.trycloudflare.com` que
  cambia en cada reinicio; pensado para pruebas), ngrok, o un túnel con nombre de Cloudflare (URL
  fija, requiere cuenta).
- **Autenticación v1: token secreto en la URL.** Nexus genera un token aleatorio de 256 bits, lo
  guarda en el llavero y sirve MCP solo en `/mcp/<token>`; cualquier otra ruta devuelve 404.
  Funciona con la opción "sin autenticación" de los conectores de ChatGPT.
  - Debilidades conocidas: la URL *es* la contraseña. Puede quedar en los logs del túnel, en el
    historial y en la configuración de ChatGPT, y no vence.
  - Mitigaciones: `nexus mcp token rotate`, comparación en tiempo constante, límite de solicitudes,
    auditoría y un conjunto remoto reducido (nivel de lectura, acciones locales de temporizadores y
    PendingActions; sin herramientas destructivas ni escrituras externas directas).
- **Autenticación v2, solo si hace falta:** servidor de recursos OAuth 2.1 con tokens de corta
  duración y permisos acotados, cuando el acceso remoto pueda cambiar correos o el calendario, o
  cuando lo use más gente.
- **Interruptor de apagado:** detener `nexus mcp serve` o rotar el token corta el acceso al instante.

## 12. Modelo de seguridad

**Activos:** tokens OAuth y claves de API; contenido de correo, calendario y tareas; registros de tiempo.

**Amenazas y controles:**

| Amenaza | Control |
|---|---|
| Otro usuario local lee los datos | Carpeta de datos `0700`, base de datos `0600`; secretos solo en el llavero. |
| Un token se filtra por archivos, logs o el repositorio | Solo llavero; los logs ocultan secretos; nada de secretos en archivos de configuración. |
| Inyección de comandos desde texto del usuario | Todo subproceso usa arreglos de argumentos; el plugin QML nunca arma cadenas de shell. |
| Inyección de instrucciones (prompt injection) en correos o tareas | El texto de terceros se marca como datos y se recorta; las escrituras externas requieren aprobación humana en Nexus; las herramientas destructivas no se exponen. |
| Una IA o un token comprometido provoca escrituras dañinas | Aprobación de PendingActions, tokens de corta duración con permisos acotados, auditoría, interruptor de apagado. |
| Exposición a la red | Nada escucha en la red por defecto; HTTP solo en loopback; el acceso remoto es opcional. |
| Actualización maliciosa del plugin (el shell corre plugins sin aislamiento) | QML mínimo, sin acceso a red desde QML, tags de versión firmados; `omarchy plugin update` muestra el diff. |
| Cadena de suministro | Pocas dependencias, `govulncheck` en CI, checksums y firmas en las versiones. |

Antes de la primera versión pública hace falta un `SECURITY.md` con un canal privado para reportar vulnerabilidades.

## 13. Distribución y publicación

**Dos artefactos que se publican juntos:**

| Artefacto | Contenido | Instalación |
|---|---|---|
| binario `nexus` | CLI, TUI, daemon, MCP | Paquete de AUR, release de GitHub (binario estático de Go, checksums, firma), `go install` |
| plugin de la barra | `manifest.json` en la raíz + QML + `Model.js` | `omarchy plugin add https://github.com/<dueño>/omarchy-nexus` |

- El plugin se desarrolla en `plugin/` dentro de este repositorio y **la CI lo copia** al
  repositorio del plugin, porque `omarchy plugin add` clona un repositorio completo desde su raíz.
- El plugin detecta si falta el binario o si es demasiado viejo, y muestra "Instalar Nexus" en lugar de fallar.
- **Versionado del contrato:** cada salida JSON de la CLI incluye `"api": N`. El plugin declara el
  rango que soporta. Romper el contrato implica una nueva versión mayor.
- **Nombres:** paquete `nexus` (AUR, compilado desde el código), binario `nexus`, id del plugin
  `<dueño>.nexus`. El paquete declara `conflicts=('nexus-bin' 'nexus-cli')` porque ambos instalan
  `/usr/bin/nexus`. Un paquete con binario precompilado necesita otro nombre, porque `nexus-bin` está ocupado.
- **Licencia:** Apache-2.0 (ya está en el repositorio).
- **systemd:** el paquete instala una unidad de usuario (`nexus.service`) **deshabilitada** hasta que el usuario la active.

## 14. Configuración, idioma y observabilidad

- **Configuración:** `$XDG_CONFIG_HOME/nexus/config.toml` (reglas de avisos, conectores activos, idioma).
- **Idioma:** los textos visibles pasan por catálogos de mensajes (`es`, `en`) elegidos desde la
  configuración o `LANG`. El código, los identificadores y los logs quedan en inglés.
- **Logs:** solo el daemon escribe logs, como líneas estructuradas `slog` en `$XDG_STATE_HOME/nexus/`,
  con rotación y ocultando secretos. `nexus doctor` revisa rutas, permisos, llavero, daemon,
  versión del plugin y el estado de los conectores.

## 15. Controles de calidad

| Capa | Test |
|---|---|
| Dominio y casos de uso | Tests unitarios con reloj falso y repositorios falsos |
| Adaptadores SQLite y migraciones | Tests con base temporal: conservación de datos, idempotencia, binario viejo + esquema nuevo |
| Contrato JSON de la CLI | Archivos "golden" por versión de `api` |
| Conectores | Respuestas grabadas y el conector falso; nunca cuentas reales en la CI |
| Plugin | Tests con `node` para `Model.js`; `omarchy plugin validate` en la CI |
| Arquitectura | Test del grafo de imports (§6) |
| Publicación | `go vet`, `gofmt`, `govulncheck`, build reproducible, checksums |

## 16. Hoja de ruta

| Fase | Alcance | Terminada cuando |
|---|---|---|
| 0 | Tracker: CLI, TUI, popup de la barra | **Hecha** |
| 1 | Catálogo (organización → cliente → proyecto), editar y eliminar entradas con papelera, renombrar/unir proyectos, refactor a paquete por módulo, permisos de archivos | Los datos actuales migrados con respaldo; el TUI y la CLI pueden corregir cualquier error |
| 2 | Daemon, cuenta regresiva de break con botones en la notificación, avisos de movimiento, `nexus doctor` | Un break olvidado produce una notificación y un estado visible de tiempo excedido |
| 3 | Versión pública: repositorio del plugin, paquete de AUR, idiomas (`es`/`en`), tutorial de primer uso, `SECURITY.md`, CI | Un usuario nuevo instala ambos artefactos solo con el README |
| 4 | MCP: stdio local y `nexus mcp serve` detrás de un túnel con token secreto en la URL; niveles de lectura y escritura local; auditoría | ChatGPT (Plus) y un agente de escritorio pueden iniciar, detener y reportar temporizadores |
| 5 | Conectores de solo lectura: agenda de Calendar, tareas de Notion, correos sin leer de Gmail, Obsidian; agenda en el popup y el TUI | Los eventos y tareas de hoy se ven sin abrir otra ventana |
| 6 | Escritura de vuelta mediante outbox y PendingActions (mover una tarjeta de Notion, crear un evento) | Cada escritura externa se aprueba, se audita y se reintenta de forma segura |
| 7 | OAuth para el MCP remoto, solo si el acceso remoto debe cambiar correos o el calendario | Modelo de amenazas revisado; tokens de corta duración y acotados |
| 8 | Automatización de navegador en los sitios propios del usuario (proceso aparte) | Simulación y aprobación antes de cualquier cambio |
| Después | Dashboard de actividad: tiempo por aplicación (terminal, navegador, videos) a partir de los eventos de ventana activa de Hyprland, recogidos por el daemon y guardados localmente | Opcional; los datos nunca salen de la máquina |
| Después | Sincronización entre varias computadoras | Aún no planificada; el `uid` y el registro de eventos dejan la puerta abierta |

## 17. Decisiones y preguntas abiertas

**Decisiones**

- D1. Monolito modular, módulos hexagonales, paquete por módulo desde la fase 1.
- D2. SQLite como única base de datos; migraciones por módulo con respaldo automático.
- D3. Local primero; nada expuesto a la red por defecto.
- D4. Las integraciones de Google usan un cliente OAuth propio del usuario (BYOC).
- D5. Primero MCP local por stdio; el MCP remoto es opcional y llega después.
- D6. Las escrituras externas y las acciones pedidas por una IA requieren aprobación dentro de Nexus.
- D7. El plugin de la barra es un cliente delgado del contrato JSON de la CLI y vive en su propio repositorio.
- D8. Al iniciar un break se pregunta cada vez qué temporizadores en curso detener.
- D10. El paquete y el binario se llaman `nexus`.
- D9. El MCP remoto v1 usa un token secreto en la URL detrás de un túnel elegido por el usuario (se recomienda el túnel rápido de Cloudflare); OAuth solo cuando haga falta.

**Preguntas abiertas**

- P2. Fuente de la señal de inactividad para los avisos de movimiento.
- P3. Si OpenAI Secure MCP Tunnel puede servir a ChatGPT web con el plan del usuario.
- P6. Nombre del paquete con binario precompilado (`nexus-bin` está ocupado); elegir uno disponible antes de la primera versión pública.
- P5. Período de retención de las entradas en la papelera (propuesta: 30 días).

## 18. Fuentes

- Instalador de plugins y README del shell de Omarchy (local: `/usr/share/omarchy/shell/README.md`, `omarchy plugin add --help`); índice comunitario https://plugins.omarchy.org
- Especificación MCP 2026-07-28 y su changelog: https://modelcontextprotocol.io/specification/2026-07-28
- SDK de MCP para Go: https://github.com/modelcontextprotocol/go-sdk
- MCP en ChatGPT y Codex: https://learn.chatgpt.com/docs/extend/mcp.md
- MCP y Secure MCP Tunnel de OpenAI: https://developers.openai.com/api/docs/guides/tools-connectors-mcp
- Modo desarrollador y apps MCP en ChatGPT: https://help.openai.com/en/articles/12584461
- Verificación de permisos restringidos de Google: https://developers.google.com/identity/protocols/oauth2/production-readiness/restricted-scope-verification
- Auditoría de seguridad de Google (CASA): https://support.google.com/cloud/answer/13465431
- Guía de actualización a la API 2025-09-03 de Notion: https://developers.notion.com/guides/get-started/upgrade-guide-2025-09-03
- OWASP MCP Top 10 (inyección de instrucciones, envenenamiento de herramientas, manejo de tokens)
