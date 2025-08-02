package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// MCPRequest represents a JSON-RPC request for MCP
type MCPRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// MCPResponse represents a JSON-RPC response from MCP
type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

// MCPError represents an error in MCP response
type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// SessionResponse represents the session creation response
type SessionResponse struct {
	SessionID string `json:"sessionId"`
	Status    string `json:"status"`
}

// MCPClient handles communication with the Docker MCP gateway
type MCPClient struct {
	BaseURL   string
	Client    *http.Client
	nextID    int
	sessionID string
}

// NewMCPClient creates a new MCP client instance
func NewMCPClient(baseURL string) *MCPClient {
	return &MCPClient{
		BaseURL: baseURL,
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
		nextID: 1,
	}
}

// CreateSession creates a new session with the MCP gateway
func (c *MCPClient) CreateSession() error {
	// Try different session creation endpoints
	sessionEndpoints := []string{
		"/session",
		"/api/session",
		"/sessions",
		"/create-session",
	}

	for _, endpoint := range sessionEndpoints {
		sessionURL := c.BaseURL + endpoint
		fmt.Printf("Trying to create session at: %s\n", sessionURL)

		// Try POST request for session creation
		req, err := http.NewRequest("POST", sessionURL, bytes.NewBuffer([]byte("{}")))
		if err != nil {
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := c.Client.Do(req)
		if err != nil {
			fmt.Printf("Failed to create session at %s: %v\n", endpoint, err)
			continue
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Session creation response (%d): %s\n", resp.StatusCode, string(body))

		if resp.StatusCode == 200 || resp.StatusCode == 201 {
			// Try to parse session response
			var sessionResp SessionResponse
			if err := json.Unmarshal(body, &sessionResp); err == nil && sessionResp.SessionID != "" {
				c.sessionID = sessionResp.SessionID
				fmt.Printf("Session created successfully: %s\n", c.sessionID)
				return nil
			}

			// Check for session ID in headers
			if sessionID := resp.Header.Get("X-Session-ID"); sessionID != "" {
				c.sessionID = sessionID
				fmt.Printf("Session ID from header: %s\n", c.sessionID)
				return nil
			}

			// Check cookies
			for _, cookie := range resp.Cookies() {
				if cookie.Name == "session_id" || cookie.Name == "sessionId" {
					c.sessionID = cookie.Value
					fmt.Printf("Session ID from cookie: %s\n", c.sessionID)
					return nil
				}
			}
		}
	}

	// If session creation failed, try to extract session from initialization
	return c.initializeWithSession()
}

// initializeWithSession tries to get session from initialization response
func (c *MCPClient) initializeWithSession() error {
	// Try initialization with session request
	initURL := c.BaseURL + "/initialize"
	
	initParams := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"clientInfo": map[string]interface{}{
			"name":    "golang-mcp-client",
			"version": "1.0.0",
		},
	}

	jsonData, _ := json.Marshal(initParams)
	req, err := http.NewRequest("POST", initURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create initialization request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send initialization request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Initialize response (%d): %s\n", resp.StatusCode, string(body))

	// Check for session ID in response
	if sessionID := resp.Header.Get("X-Session-ID"); sessionID != "" {
		c.sessionID = sessionID
		return nil
	}

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "session_id" || cookie.Name == "sessionId" {
			c.sessionID = cookie.Value
			return nil
		}
	}

	return fmt.Errorf("could not establish session")
}

// Initialize establishes connection with MCP server
func (c *MCPClient) Initialize() error {
	// First try to create a session
	if err := c.CreateSession(); err != nil {
		fmt.Printf("Session creation failed, trying direct initialization: %v\n", err)
	}

	initParams := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"clientInfo": map[string]interface{}{
			"name":    "golang-mcp-client",
			"version": "1.0.0",
		},
	}

	request := MCPRequest{
		JSONRPC: "2.0",
		ID:      c.getNextID(),
		Method:  "initialize",
		Params:  initParams,
	}

	response, err := c.sendRequest(request)
	if err != nil {
		return err
	}

	if response.Error != nil {
		return fmt.Errorf("initialization failed: %s", response.Error.Message)
	}

	fmt.Println("MCP connection initialized successfully")
	return nil
}

// ListTools gets available tools from the MCP server
func (c *MCPClient) ListTools() ([]interface{}, error) {
	request := MCPRequest{
		JSONRPC: "2.0",
		ID:      c.getNextID(),
		Method:  "tools/list",
		Params:  map[string]interface{}{},
	}

	response, err := c.sendRequest(request)
	if err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", response.Error.Code, response.Error.Message)
	}

	// Extract tools from result
	if result, ok := response.Result.(map[string]interface{}); ok {
		if tools, ok := result["tools"].([]interface{}); ok {
			return tools, nil
		}
	}

	return nil, fmt.Errorf("unexpected response format")
}

// GatherInformation uses alternative approaches to get information
func (c *MCPClient) GatherInformation(prompt, model string) (string, error) {
	// Try different approaches to get information

	// Approach 1: Direct API call (common for Docker gateways)
	if result, err := c.tryDirectAPI(prompt, model); err == nil {
		return result, nil
	}

	// Approach 2: Standard MCP flow
	if result, err := c.tryMCPFlow(prompt, model); err == nil {
		return result, nil
	}

	// Approach 3: Try different endpoints
	return c.tryAlternativeEndpoints(prompt, model)
}

// tryDirectAPI tries direct API calls without MCP protocol
func (c *MCPClient) tryDirectAPI(prompt, model string) (string, error) {
	endpoints := []string{
		"/api/chat",
		"/chat",
		"/api/completion",
		"/completion",
		"/api/generate",
		"/generate",
	}

	requestBody := map[string]interface{}{
		"prompt": prompt,
		"model":  model,
		"query":  prompt,
		"input":  prompt,
	}

	for _, endpoint := range endpoints {
		url := c.BaseURL + endpoint
		fmt.Printf("Trying direct API at: %s\n", url)

		jsonData, _ := json.Marshal(requestBody)
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		if c.sessionID != "" {
			req.Header.Set("X-Session-ID", c.sessionID)
			req.Header.Set("Session-ID", c.sessionID)
		}

		resp, err := c.Client.Do(req)
		if err != nil {
			fmt.Printf("Failed to call %s: %v\n", endpoint, err)
			continue
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Direct API response (%d): %s\n", resp.StatusCode, string(body))

		if resp.StatusCode == 200 {
			// Try to parse response
			var result map[string]interface{}
			if err := json.Unmarshal(body, &result); err == nil {
				if text := c.extractTextFromResult(result); text != "" {
					return text, nil
				}
			}
			return string(body), nil
		}
	}

	return "", fmt.Errorf("direct API calls failed")
}

// tryMCPFlow tries the standard MCP protocol flow
func (c *MCPClient) tryMCPFlow(prompt, model string) (string, error) {
	if err := c.Initialize(); err != nil {
		return "", fmt.Errorf("failed to initialize MCP connection: %w", err)
	}

	tools, err := c.ListTools()
	if err != nil {
		return "", fmt.Errorf("failed to list tools: %w", err)
	}

	if len(tools) == 0 {
		return "", fmt.Errorf("no tools available")
	}

	// Use first available tool
	toolMap := tools[0].(map[string]interface{})
	toolName := toolMap["name"].(string)

	arguments := map[string]interface{}{
		"prompt": prompt,
		"model":  model,
	}

	result, err := c.CallTool(toolName, arguments)
	if err != nil {
		return "", err
	}

	return c.extractTextFromResult(result), nil
}

// tryAlternativeEndpoints tries various endpoint patterns
func (c *MCPClient) tryAlternativeEndpoints(prompt, model string) (string, error) {
	// Try with query parameters
	params := url.Values{}
	params.Add("prompt", prompt)
	params.Add("model", model)
	params.Add("q", prompt)

	endpoints := []string{
		"/?" + params.Encode(),
		"/api?" + params.Encode(),
		"/query?" + params.Encode(),
	}

	for _, endpoint := range endpoints {
		url := c.BaseURL + endpoint
		fmt.Printf("Trying GET request to: %s\n", url)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			continue
		}

		if c.sessionID != "" {
			req.Header.Set("X-Session-ID", c.sessionID)
		}

		resp, err := c.Client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("GET response (%d): %s\n", resp.StatusCode, string(body))

		if resp.StatusCode == 200 {
			return string(body), nil
		}
	}

	return "", fmt.Errorf("all approaches failed")
}

// CallTool calls a specific tool with arguments
func (c *MCPClient) CallTool(toolName string, arguments map[string]interface{}) (interface{}, error) {
	params := map[string]interface{}{
		"name":      toolName,
		"arguments": arguments,
	}

	request := MCPRequest{
		JSONRPC: "2.0",
		ID:      c.getNextID(),
		Method:  "tools/call",
		Params:  params,
	}

	response, err := c.sendRequest(request)
	if err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", response.Error.Code, response.Error.Message)
	}

	return response.Result, nil
}

// extractTextFromResult tries to extract text from various result formats
func (c *MCPClient) extractTextFromResult(result interface{}) string {
	if resultMap, ok := result.(map[string]interface{}); ok {
		textFields := []string{"text", "response", "answer", "result", "output", "message", "content"}
		for _, field := range textFields {
			if text, ok := resultMap[field].(string); ok && text != "" {
				return text
			}
		}

		// Try nested content
		if content, ok := resultMap["content"].([]interface{}); ok && len(content) > 0 {
			if contentItem, ok := content[0].(map[string]interface{}); ok {
				if text, ok := contentItem["text"].(string); ok {
					return text
				}
			}
		}
	}

	// Return raw result as JSON
	resultBytes, _ := json.MarshalIndent(result, "", "  ")
	return string(resultBytes)
}

// sendRequest sends a JSON-RPC request to the MCP server
func (c *MCPClient) sendRequest(request MCPRequest) (*MCPResponse, error) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	fmt.Printf("Sending MCP request: %s\n", string(jsonData))

	req, err := http.NewRequest("POST", c.BaseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if c.sessionID != "" {
		req.Header.Set("X-Session-ID", c.sessionID)
		req.Header.Set("Session-ID", c.sessionID)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	fmt.Printf("MCP response (%d): %s\n", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	var mcpResponse MCPResponse
	if err := json.Unmarshal(body, &mcpResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &mcpResponse, nil
}

// getNextID returns the next request ID
func (c *MCPClient) getNextID() int {
	id := c.nextID
	c.nextID++
	return id
}

// AIFunction is the main function that takes prompt and model parameters
func AIFunction(prompt, model string) (string, error) {
	if prompt == "" {
		return "", fmt.Errorf("prompt cannot be empty")
	}
	if model == "" {
		return "", fmt.Errorf("model cannot be empty")
	}

	client := NewMCPClient("http://localhost:8080")
	return client.GatherInformation(prompt, model)
}

// Main function for testing
func main() {
	prompt := "When did ozzy die according to wikipedia?"
	model := "gpt-3.5-turbo"

	fmt.Printf("Calling AIFunction with:\n")
	fmt.Printf("Prompt: %s\n", prompt)
	fmt.Printf("Model: %s\n", model)
	fmt.Println("---")

	result, err := AIFunction(prompt, model)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Result: %s\n", result)
}