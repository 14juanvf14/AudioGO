# Retell AI Go Client

Este paquete es una transcripción del cliente JavaScript de Retell AI a Go (Golang), proporcionando funcionalidades equivalentes para manejar llamadas de voz con inteligencia artificial usando WebRTC.

## Características Principales

- ✅ **Gestión de Llamadas**: Iniciar, detener y manejar el estado de las llamadas
- ✅ **Control de Audio**: Silenciar/activar micrófono, envío de streams personalizados  
- ✅ **Eventos en Tiempo Real**: Sistema de eventos similar a EventEmitter de JavaScript
- ✅ **Análisis de Audio**: Procesamiento y análisis de muestras de audio en tiempo real
- ✅ **WebRTC Nativo**: Utilizando la librería Pion WebRTC para Go
- ✅ **Manejo de Estado**: Control completo del estado de la conexión y tracks de audio

## Instalación

```bash
# Navegar a la carpeta del proyecto
cd retellAI

# Descargar dependencias
go mod download
```

## Dependencias

- `github.com/pion/webrtc/v3` - Implementación WebRTC para Go
- `github.com/gorilla/websocket` - Cliente WebSocket
- `github.com/google/uuid` - Generación de UUIDs

## Uso Básico

### 1. Crear Cliente y Configurar Eventos

```go
package main

import (
    "fmt"
    "retellAI"
)

func main() {
    // Crear cliente Retell
    client := retellAI.NewRetellWebClient()
    
    // Configurar listeners de eventos
    client.On(retellAI.EventCallStarted, func(data interface{}) {
        fmt.Println("¡Llamada iniciada!")
    })
    
    client.On(retellAI.EventAgentStartTalking, func(data interface{}) {
        fmt.Println("El agente comenzó a hablar")
    })
    
    client.On(retellAI.EventError, func(data interface{}) {
        fmt.Printf("Error: %v\n", data)
    })
}
```

### 2. Iniciar una Llamada

```go
config := retellAI.StartCallConfig{
    AccessToken:          "tu-token-de-acceso",
    SampleRate:          16000,
    EmitRawAudioSamples: true,  // Para análisis de audio
}

if err := client.StartCall(config); err != nil {
    log.Fatalf("Error al iniciar llamada: %v", err)
}
```

### 3. Control de Micrófono

```go
// Silenciar micrófono
if err := client.Mute(); err != nil {
    fmt.Printf("Error al silenciar: %v\n", err)
}

// Activar micrófono
if err := client.Unmute(); err != nil {
    fmt.Printf("Error al activar: %v\n", err)
}
```

### 4. Envío de Stream Personalizado

```go
// Crear stream personalizado
customStream := retellAI.NewMediaStreamTrack("audio", "mi-stream-personalizado")

// Enviar stream personalizado (desactiva micrófono)
if err := client.SendCustomMediaStream(customStream); err != nil {
    fmt.Printf("Error enviando stream: %v\n", err)
}

// Volver al micrófono
if err := client.ResumeMicrophone(); err != nil {
    fmt.Printf("Error reanudando micrófono: %v\n", err)
}
```

### 5. Monitoreo de Estado

```go
// Verificar estado de conexión
fmt.Printf("Conectado: %t\n", client.IsConnected())
fmt.Printf("Agente hablando: %t\n", client.IsAgentTalking())

// Obtener información de tracks
status := client.GetTrackStatus()
fmt.Printf("Micrófono habilitado: %t\n", status.MicrophoneEnabled)
fmt.Printf("Total de tracks: %d\n", status.TotalTracks)

// Obtener stream del agente
if agentStream := client.GetAgentMediaStream(); agentStream != nil {
    fmt.Printf("Stream del agente: %s\n", agentStream.ID)
}
```

## Eventos Disponibles

El cliente emite los siguientes eventos:

| Evento | Descripción |
|--------|-------------|
| `EventCallStarted` | La llamada se ha iniciado |
| `EventCallReady` | La llamada está lista (track del agente disponible) |
| `EventCallEnded` | La llamada ha terminado |
| `EventAgentStartTalking` | El agente comenzó a hablar |
| `EventAgentStopTalking` | El agente dejó de hablar |
| `EventAgentMediaStreamReady` | Stream de audio del agente disponible |
| `EventAgentMediaStreamActive` | Stream del agente activo |
| `EventAgentMediaStreamInactive` | Stream del agente inactivo |
| `EventCustomMediaStreamSent` | Stream personalizado enviado |
| `EventMicrophoneResumed` | Micrófono reanudado |
| `EventAudio` | Muestras de audio disponibles (si está habilitado) |
| `EventUpdate` | Actualización del servidor |
| `EventMetadata` | Metadatos del servidor |
| `EventNodeTransition` | Transición de nodo |
| `EventError` | Error ocurrido |

## Manejo de Audio

### Procesador de Audio

```go
// Crear procesador de audio
processor := retellAI.NewAudioProcessor(16000, 1, 16, 1024)

// Iniciar procesamiento
if err := processor.Start(); err != nil {
    log.Printf("Error iniciando procesador: %v", err)
}

// Procesar muestras
samples := []float32{0.1, 0.2, -0.1, -0.2}
processedSamples := processor.ProcessAudioSamples(samples)

// Calcular volumen
volume := processor.CalculateVolume(samples)
fmt.Printf("Volumen: %.2f dB\n", volume)
```

### Grabación de Audio

```go
// Crear grabador
recorder := retellAI.NewAudioRecorder()

// Iniciar grabación (requiere track remoto)
if err := recorder.StartRecording(remoteTrack); err != nil {
    log.Printf("Error iniciando grabación: %v", err)
}

// Detener y obtener muestras grabadas
recordedSamples := recorder.StopRecording()
fmt.Printf("Grabadas %d muestras\n", len(recordedSamples))
```

## Diferencias con la Versión JavaScript

### Equivalencias de Funcionalidades

| JavaScript | Go | Notas |
|------------|-----|--------|
| `EventEmitter` | `EventEmitter` struct | Implementación personalizada con goroutines |
| `livekit-client` | `pion/webrtc` | WebRTC nativo sin dependencias de C/C++ |
| `MediaStream` | `MediaStreamTrack` | Wrapper simplificado para tracks |
| `RemoteAudioTrack` | `*webrtc.TrackRemote` | Track remoto de Pion WebRTC |
| `createAudioAnalyser` | `AudioProcessor` | Procesador de audio personalizado |

### Adaptaciones Realizadas

1. **Gestión de Concurrencia**: Uso de goroutines y channels en lugar de callbacks
2. **Manejo de Memoria**: Gestión explícita de recursos con cleanup functions
3. **Tipado Fuerte**: Structs tipados en lugar de objetos JavaScript dinámicos
4. **Error Handling**: Manejo explícito de errores siguiendo convenciones de Go

## Ejemplo Completo

Ver `example.go` para un ejemplo completo que demuestra:
- Configuración de eventos
- Inicio y manejo de llamadas
- Control de micrófono
- Envío de streams personalizados
- Monitoreo de estado
- Cierre graceful

## Ejecutar el Ejemplo

```bash
# Desde la carpeta retellAI
cd examples
go run example.go

# O compilar y ejecutar
go build -o retell-example .
./retell-example
```

## Limitaciones Actuales

- **Simulación de WebSocket**: La conexión WebSocket está simulada (no conecta al servidor real)
- **Audio Mock**: Las muestras de audio son generadas para demostración
- **Codecs Limitados**: Soporte básico de codecs de audio

## Próximos Pasos

Para usar en producción, sería necesario:

1. Implementar conexión real a LiveKit/Retell AI servers
2. Integrar captura de audio del sistema
3. Añadir soporte completo de codecs
4. Implementar autenticación real
5. Optimizar para rendimiento en producción

## Licencia

Este código es una transcripción educativa del cliente JavaScript de Retell AI a Go.
