# API Examples - Retell AI Server

Este documento muestra ejemplos de cómo usar los endpoints HTTP y WebSocket del servidor Retell AI.

## Iniciar el Servidor

```bash
# Compilar y ejecutar el servidor
go build -o retell-server main_server.go server.go
./retell-server -port 8080

# O ejecutar directamente
go run main_server.go server.go -port 8080
```

El servidor estará disponible en `http://localhost:8080`

## Endpoints HTTP

### 1. Health Check

**GET /health**

Verifica que el servidor esté funcionando.

```bash
curl -X GET http://localhost:8080/health
```

**Respuesta:**
```json
{
  "success": true,
  "message": "Retell AI Server is running",
  "data": {
    "activeSessions": 0,
    "version": "1.0.0"
  }
}
```

### 2. Iniciar Llamada

**POST /start-call**

Inicia una nueva llamada con Retell AI.

```bash
curl -X POST http://localhost:8080/start-call \
  -H "Content-Type: application/json" \
  -d '{
    "sessionId": "session-123",
    "accessToken": "your-retell-access-token",
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
    "sessionId": "session-123",
    "config": {
      "accessToken": "your-retell-access-token",
      "sampleRate": 16000,
      "emitRawAudioSamples": true
    }
  }
}
```

### 3. Detener Llamada

**POST /stop-call**

Termina una llamada activa.

```bash
curl -X POST http://localhost:8080/stop-call \
  -H "Content-Type: application/json" \
  -d '{
    "sessionId": "session-123"
  }'
```

**Respuesta:**
```json
{
  "success": true,
  "message": "Call stopped successfully",
  "data": {
    "sessionId": "session-123"
  }
}
```

### 4. Control de Audio

#### Silenciar Micrófono

**POST /mute**

```bash
curl -X POST http://localhost:8080/mute \
  -H "Content-Type: application/json" \
  -d '{
    "sessionId": "session-123"
  }'
```

#### Activar Micrófono

**POST /unmute**

```bash
curl -X POST http://localhost:8080/unmute \
  -H "Content-Type: application/json" \
  -d '{
    "sessionId": "session-123"
  }'
```

#### Enviar Stream Personalizado

**POST /send-custom-stream**

```bash
curl -X POST http://localhost:8080/send-custom-stream \
  -H "Content-Type: application/json" \
  -d '{
    "sessionId": "session-123",
    "streamId": "custom-audio-stream-456",
    "kind": "audio"
  }'
```

#### Reanudar Micrófono

**POST /resume-microphone**

```bash
curl -X POST http://localhost:8080/resume-microphone \
  -H "Content-Type: application/json" \
  -d '{
    "sessionId": "session-123"
  }'
```

### 5. Consultar Estado

#### Estado de la Llamada

**GET /call-status?sessionId=session-123**

```bash
curl -X GET "http://localhost:8080/call-status?sessionId=session-123"
```

**Respuesta:**
```json
{
  "success": true,
  "message": "Call status retrieved",
  "data": {
    "sessionId": "session-123",
    "isActive": true,
    "isConnected": true,
    "isAgentTalking": false,
    "hasWebSocket": true
  }
}
```

#### Estado de Tracks

**GET /track-status?sessionId=session-123**

```bash
curl -X GET "http://localhost:8080/track-status?sessionId=session-123"
```

**Respuesta:**
```json
{
  "success": true,
  "message": "Track status retrieved",
  "data": {
    "microphoneEnabled": true,
    "publishedTracks": [
      {
        "sid": "track-123",
        "kind": "audio",
        "name": "microphone",
        "source": "microphone"
      }
    ],
    "totalTracks": 1
  }
}
```

## WebSocket Connection

### Conectar a WebSocket

```javascript
const ws = new WebSocket('ws://localhost:8080/ws?sessionId=session-123');

ws.onopen = function(event) {
    console.log('✅ WebSocket conectado');
};

ws.onmessage = function(event) {
    const message = JSON.parse(event.data);
    console.log('📨 Mensaje recibido:', message);
    
    if (message.type === 'retell_event') {
        handleRetellEvent(message.eventType, message.data);
    }
};

ws.onclose = function(event) {
    console.log('🔌 WebSocket desconectado');
};

ws.onerror = function(error) {
    console.error('❌ Error WebSocket:', error);
};

// Enviar ping
ws.send(JSON.stringify({ type: 'ping' }));
```

### Eventos WebSocket

El servidor enviará los siguientes tipos de eventos:

#### 1. Evento de Conexión Establecida
```json
{
  "type": "connection_established",
  "sessionId": "session-123",
  "status": {
    "isConnected": true,
    "isAgentTalking": false
  }
}
```

#### 2. Eventos de Retell AI
```json
{
  "type": "retell_event",
  "eventType": "agent_start_talking",
  "data": null,
  "timestamp": "1647875400000"
}
```

#### 3. Respuesta Pong
```json
{
  "type": "pong",
  "timestamp": "1647875400000"
}
```

## Ejemplo de Flujo Completo

### 1. Usando cURL

```bash
# 1. Verificar servidor
curl -X GET http://localhost:8080/health

# 2. Iniciar llamada
curl -X POST http://localhost:8080/start-call \
  -H "Content-Type: application/json" \
  -d '{
    "sessionId": "demo-session",
    "accessToken": "your-token-here",
    "sampleRate": 16000,
    "emitRawAudioSamples": true
  }'

# 3. Verificar estado
curl -X GET "http://localhost:8080/call-status?sessionId=demo-session"

# 4. Silenciar micrófono
curl -X POST http://localhost:8080/mute \
  -H "Content-Type: application/json" \
  -d '{"sessionId": "demo-session"}'

# 5. Enviar stream personalizado
curl -X POST http://localhost:8080/send-custom-stream \
  -H "Content-Type: application/json" \
  -d '{
    "sessionId": "demo-session",
    "streamId": "custom-stream-001",
    "kind": "audio"
  }'

# 6. Reanudar micrófono
curl -X POST http://localhost:8080/resume-microphone \
  -H "Content-Type: application/json" \
  -d '{"sessionId": "demo-session"}'

# 7. Detener llamada
curl -X POST http://localhost:8080/stop-call \
  -H "Content-Type: application/json" \
  -d '{"sessionId": "demo-session"}'
```

### 2. Usando JavaScript (Cliente Web)

```html
<!DOCTYPE html>
<html>
<head>
    <title>Retell AI Client</title>
</head>
<body>
    <h1>Retell AI Demo</h1>
    <button onclick="startCall()">Iniciar Llamada</button>
    <button onclick="stopCall()">Detener Llamada</button>
    <button onclick="mute()">Silenciar</button>
    <button onclick="unmute()">Activar</button>
    <div id="status"></div>
    <div id="events"></div>

    <script>
        const API_BASE = 'http://localhost:8080';
        const sessionId = 'web-session-' + Date.now();
        let ws;

        // Conectar WebSocket
        function connectWebSocket() {
            ws = new WebSocket(`ws://localhost:8080/ws?sessionId=${sessionId}`);
            
            ws.onmessage = function(event) {
                const message = JSON.parse(event.data);
                document.getElementById('events').innerHTML += 
                    `<p>📨 ${message.type}: ${JSON.stringify(message)}</p>`;
            };
        }

        async function apiCall(endpoint, data = null) {
            const options = {
                method: data ? 'POST' : 'GET',
                headers: { 'Content-Type': 'application/json' }
            };
            
            if (data) {
                options.body = JSON.stringify(data);
            }
            
            const response = await fetch(`${API_BASE}${endpoint}`, options);
            return response.json();
        }

        async function startCall() {
            connectWebSocket();
            
            const result = await apiCall('/start-call', {
                sessionId: sessionId,
                accessToken: 'demo-token',
                sampleRate: 16000,
                emitRawAudioSamples: true
            });
            
            document.getElementById('status').innerHTML = 
                `<p>✅ ${result.message}</p>`;
        }

        async function stopCall() {
            const result = await apiCall('/stop-call', { sessionId });
            document.getElementById('status').innerHTML = 
                `<p>🛑 ${result.message}</p>`;
            
            if (ws) ws.close();
        }

        async function mute() {
            const result = await apiCall('/mute', { sessionId });
            document.getElementById('status').innerHTML += 
                `<p>🔇 ${result.message}</p>`;
        }

        async function unmute() {
            const result = await apiCall('/unmute', { sessionId });
            document.getElementById('status').innerHTML += 
                `<p>🔊 ${result.message}</p>`;
        }
    </script>
</body>
</html>
```

## Manejo de Errores

Todos los endpoints devuelven errores en el siguiente formato:

```json
{
  "success": false,
  "error": "Descripción del error"
}
```

### Códigos de Estado HTTP

- `200` - Operación exitosa
- `400` - Solicitud inválida (JSON malformado, parámetros faltantes)
- `404` - Sesión no encontrada
- `405` - Método no permitido
- `409` - Conflicto (ej: sesión ya activa)
- `500` - Error interno del servidor

## Consideraciones de Producción

1. **Autenticación**: Agregar autenticación JWT o API keys
2. **CORS**: Configurar CORS apropiadamente para producción
3. **Rate Limiting**: Implementar límites de tasa para prevenir abuso
4. **Logging**: Agregar logging estructurado
5. **Monitoring**: Implementar métricas y health checks
6. **SSL/TLS**: Usar HTTPS en producción
7. **Load Balancing**: Configurar balanceador de carga si es necesario
