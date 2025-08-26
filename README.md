# Retell AI Server - API REST y WebSocket

Este proyecto proporciona un servidor HTTP completo que expone la funcionalidad de Retell AI a través de endpoints REST y conexiones WebSocket para eventos en tiempo real.

## 🚀 **Características**

- ✅ **API REST completa** para manejo de llamadas Retell AI
- ✅ **WebSocket** para eventos en tiempo real
- ✅ **Gestión de sesiones** con múltiples clientes simultáneos
- ✅ **Control de audio** (mute/unmute, streams personalizados)
- ✅ **Monitoreo de estado** en tiempo real
- ✅ **Cliente de prueba** incluido para testing
- ✅ **Documentación completa** con ejemplos

## 📂 **Estructura del Proyecto**

```
AudioGO/
├── retellAI/                    # Librería Go de Retell AI
│   ├── audio.go                 # Procesamiento de audio
│   ├── client.go                # Cliente principal Retell
│   ├── events.go                # Sistema de eventos
│   ├── types.go                 # Tipos y estructuras
│   └── examples/                # Ejemplos de uso de la librería
├── server.go                    # Servidor HTTP/WebSocket principal
├── main_server.go              # Punto de entrada del servidor
├── test_client.go              # Cliente de prueba para testing
├── api_examples.md             # Documentación de la API
├── retell-server               # Ejecutable del servidor
├── test-client                 # Ejecutable del cliente de prueba
└── README.md                   # Esta documentación
```

## 🏃‍♂️ **Inicio Rápido**

### 1. Compilar el Proyecto

```bash
# Navegar al directorio del proyecto
cd AudioGO

# Compilar el servidor
go build -o retell-server main_server.go server.go

# Compilar el cliente de prueba
go build -o test-client test_client.go
```

### 2. Ejecutar el Servidor

```bash
# Ejecutar con puerto por defecto (8080)
./retell-server

# O especificar un puerto personalizado
./retell-server -port 9000
```

Salida esperada:
```
🚀 Retell AI Server starting on port 8080
📋 Available endpoints:
   GET  /health - Health check
   POST /start-call - Start a new call
   POST /stop-call - Stop an active call
   ...
```

### 3. Probar el Servidor

```bash
# Usando el cliente de prueba incluido
./test-client

# O manualmente con curl
curl -X GET http://localhost:8080/health
```

## 🔌 **Endpoints API**

### Endpoints REST

| Método | Endpoint | Descripción |
|--------|----------|-------------|
| `GET` | `/health` | Health check del servidor |
| `POST` | `/start-call` | Iniciar nueva llamada |
| `POST` | `/stop-call` | Detener llamada activa |
| `POST` | `/mute` | Silenciar micrófono |
| `POST` | `/unmute` | Activar micrófono |
| `POST` | `/send-custom-stream` | Enviar stream personalizado |
| `POST` | `/resume-microphone` | Reanudar micrófono |
| `GET` | `/call-status` | Obtener estado de llamada |
| `GET` | `/track-status` | Obtener estado de tracks |

### WebSocket

| Endpoint | Descripción |
|----------|-------------|
| `WS /ws?sessionId=X` | Conexión WebSocket para eventos en tiempo real |

## 📝 **Ejemplos de Uso**

### Iniciar una Llamada

```bash
curl -X POST http://localhost:8080/start-call \
  -H "Content-Type: application/json" \
  -d '{
    "sessionId": "mi-sesion-123",
    "accessToken": "tu-token-retell",
    "sampleRate": 16000,
    "emitRawAudioSamples": true
  }'
```

**Respuesta:**
```json
{
  "success": true,
  "message": "Call started successfully",
  "data": {
    "sessionId": "mi-sesion-123",
    "config": {...}
  }
}
```

### Conectar WebSocket (JavaScript)

```javascript
const ws = new WebSocket('ws://localhost:8080/ws?sessionId=mi-sesion-123');

ws.onmessage = function(event) {
    const message = JSON.parse(event.data);
    
    if (message.type === 'retell_event') {
        console.log('Evento Retell:', message.eventType, message.data);
    }
};
```

### Controlar Audio

```bash
# Silenciar
curl -X POST http://localhost:8080/mute \
  -H "Content-Type: application/json" \
  -d '{"sessionId": "mi-sesion-123"}'

# Activar
curl -X POST http://localhost:8080/unmute \
  -H "Content-Type: application/json" \
  -d '{"sessionId": "mi-sesion-123"}'
```

### Consultar Estado

```bash
# Estado de la llamada
curl "http://localhost:8080/call-status?sessionId=mi-sesion-123"

# Estado de tracks de audio
curl "http://localhost:8080/track-status?sessionId=mi-sesion-123"
```

## 🧪 **Testing**

El proyecto incluye un cliente de prueba completo que verifica todos los endpoints:

```bash
# Ejecutar todas las pruebas
./test-client

# Probar contra servidor en otro puerto
./test-client http://localhost:9000
```

Las pruebas incluyen:
- ✅ Health check
- ✅ Iniciar/detener llamadas
- ✅ Control de audio (mute/unmute)
- ✅ Streams personalizados
- ✅ Consulta de estado
- ✅ Manejo de errores

## 🌐 **Ejemplo Web Client**

Ejemplo de cliente web completo:

```html
<!DOCTYPE html>
<html>
<head>
    <title>Retell AI Client</title>
</head>
<body>
    <h1>Retell AI Demo</h1>
    <button onclick="startCall()">Iniciar Llamada</button>
    <button onclick="stopCall()">Detener</button>
    <button onclick="mute()">Silenciar</button>
    <button onclick="unmute()">Activar</button>
    
    <div id="status"></div>
    <div id="events"></div>

    <script>
        const sessionId = 'web-' + Date.now();
        let ws;

        async function startCall() {
            // Conectar WebSocket
            ws = new WebSocket(`ws://localhost:8080/ws?sessionId=${sessionId}`);
            ws.onmessage = (e) => {
                const msg = JSON.parse(e.data);
                document.getElementById('events').innerHTML += 
                    `<p>📨 ${msg.eventType || msg.type}: ${JSON.stringify(msg.data || {})}</p>`;
            };

            // Iniciar llamada
            const response = await fetch('http://localhost:8080/start-call', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    sessionId,
                    accessToken: 'demo-token',
                    sampleRate: 16000,
                    emitRawAudioSamples: true
                })
            });
            
            const result = await response.json();
            document.getElementById('status').innerHTML = 
                `<p>✅ ${result.message}</p>`;
        }

        async function stopCall() {
            const response = await fetch('http://localhost:8080/stop-call', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ sessionId })
            });
            
            if (ws) ws.close();
        }

        async function mute() {
            await fetch('http://localhost:8080/mute', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ sessionId })
            });
        }

        async function unmute() {
            await fetch('http://localhost:8080/unmute', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ sessionId })
            });
        }
    </script>
</body>
</html>
```

## 📊 **Eventos WebSocket**

El servidor emite los siguientes eventos a través de WebSocket:

### Eventos de Retell AI
- `call_started` - Llamada iniciada
- `call_ready` - Llamada lista para usar
- `call_ended` - Llamada terminada
- `agent_start_talking` - Agente empezó a hablar
- `agent_stop_talking` - Agente dejó de hablar
- `agent_media_stream_ready` - Stream del agente disponible
- `custom_media_stream_sent` - Stream personalizado enviado
- `microphone_resumed` - Micrófono reanudado
- `update` - Actualización del servidor
- `metadata` - Metadatos del servidor
- `error` - Error ocurrido

### Eventos del Sistema
- `connection_established` - Conexión WebSocket establecida
- `pong` - Respuesta a ping del cliente

## 🔧 **Configuración**

### Variables de Entorno

Puedes configurar el servidor con variables de entorno:

```bash
export RETELL_SERVER_PORT=8080
export RETELL_LOG_LEVEL=info
./retell-server
```

### Flags de Línea de Comandos

```bash
./retell-server -port 8080
```

## 🚨 **Manejo de Errores**

Todos los endpoints devuelven errores en formato estándar:

```json
{
  "success": false,
  "error": "Descripción del error"
}
```

### Códigos de Estado HTTP

- `200` - Operación exitosa
- `400` - Solicitud inválida
- `404` - Sesión no encontrada
- `405` - Método no permitido
- `409` - Conflicto (sesión ya activa)
- `500` - Error interno del servidor

## 🔐 **Consideraciones de Seguridad**

Para uso en producción, considera implementar:

1. **Autenticación**: JWT tokens o API keys
2. **CORS**: Configuración apropiada de CORS
3. **Rate Limiting**: Límites de tasa por IP/usuario
4. **HTTPS**: SSL/TLS en producción
5. **Validación**: Validación exhaustiva de entrada
6. **Logging**: Logs estructurados para auditoría

## 🔄 **Integración con la Librería**

El servidor utiliza la librería Go de Retell AI ubicada en `./retellAI/`. Esta librería es una transcripción completa del cliente JavaScript original y proporciona:

- Cliente WebRTC nativo con Pion
- Sistema de eventos equivalente a EventEmitter
- Manejo completo de audio y streams
- Todas las funcionalidades del cliente JavaScript original

## 📚 **Documentación Adicional**

- [`api_examples.md`](./api_examples.md) - Ejemplos detallados de la API
- [`retellAI/README.md`](./retellAI/README.md) - Documentación de la librería Go
- [`retellAI/examples/`](./retellAI/examples/) - Ejemplos de uso de la librería

## 🤝 **Contribución**

Para contribuir al proyecto:

1. Fork el repositorio
2. Crea una branch para tu feature
3. Implementa los cambios con tests
4. Ejecuta las pruebas: `./test-client`
5. Envía un Pull Request

## 📄 **Licencia**

Este proyecto es una transcripción educativa del cliente JavaScript de Retell AI a Go, manteniendo compatibilidad funcional completa.

---

**🎉 ¡Proyecto completado con éxito!**

Has transcrito exitosamente el cliente JavaScript de Retell AI a Go y lo has envuelto en un servidor HTTP/WebSocket completo, proporcionando una API REST robusta para manejar llamadas de IA por voz.
