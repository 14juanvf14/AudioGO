package retell

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	// API de Retell
	retellAPIURL  = "https://api.retellai.com/v2/create-web-call"
	retellAPIKey  = "key_0f67246ca1d5f2188e0c3eca14b7"
	retellAgentID = "agent_ac68d6c55a4740a3e2fc2ad4b5"
)

// RetellRequest representa la estructura de la petición a la API de Retell
type RetellRequest struct {
	AgentID string `json:"agent_id"`
}

// RetellResponse representa la estructura de la respuesta de la API de Retell
type RetellResponse struct {
	CallID      string `json:"call_id"`
	CallType    string `json:"call_type"`
	AgentID     string `json:"agent_id"`
	AccessToken string `json:"access_token"`
}

// GetAccessToken hace una petición a la API de Retell para obtener un access token
func GetAccessToken() (string, error) {
	// Crear el payload de la petición
	request := RetellRequest{
		AgentID: retellAgentID,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("error al serializar JSON: %v", err)
	}

	// Crear la petición HTTP
	req, err := http.NewRequest("POST", retellAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error al crear petición HTTP: %v", err)
	}

	// Configurar headers
	req.Header.Set("Authorization", "Bearer "+retellAPIKey)
	req.Header.Set("Content-Type", "application/json")

	// Hacer la petición
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error al hacer petición HTTP: %v", err)
	}
	defer resp.Body.Close()

	// Verificar el status code (200 OK o 201 Created)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("API devolvió status code %d", resp.StatusCode)
	}

	// Decodificar la respuesta
	var response RetellResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("error al decodificar respuesta JSON: %v", err)
	}

	if response.AccessToken == "" {
		return "", fmt.Errorf("access_token vacío en la respuesta")
	}

	return response.AccessToken, nil
}
